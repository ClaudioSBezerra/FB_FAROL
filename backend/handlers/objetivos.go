package handlers

import (
	"bytes"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
	"strconv"
	"strings"

	"github.com/lib/pq"
)

// periodoSeqMax retorna o valor máximo permitido de periodo_seq por tipo.
var periodoSeqMax = map[string]int{
	"MENSAL":     12,
	"TRIMESTRAL": 4,
	"SEMESTRAL":  2,
	"ANUAL":      1,
}

const objetivosBatchSize = 500

// ObjetivosImportHandler processa o upload de CSV de objetivos de vendas.
// POST /api/objetivos/upload-csv?tipo_periodo=MENSAL&ano=2025&periodo_seq=1
//
// Resposta SSE:
//
//	{"total":N}
//	{"processed":i,"importados":x,"atualizados":y,"ignorados":z}  × batches
//	{"done":true,"importados":x,"atualizados":y,"ignorados":z,"gestores_criados":a,"rcas_criados":b}
func ObjetivosImportHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		if !RequireWrite(spCtx, w) {
			return
		}

		// ── Validar parâmetros ────────────────────────────────────────────────
		tipoPeriodo := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("tipo_periodo")))
		anoStr := strings.TrimSpace(r.URL.Query().Get("ano"))
		seqStr := strings.TrimSpace(r.URL.Query().Get("periodo_seq"))

		maxSeq, tipoOK := periodoSeqMax[tipoPeriodo]
		if !tipoOK {
			http.Error(w, `{"error":"tipo_periodo inválido — use MENSAL, TRIMESTRAL, SEMESTRAL ou ANUAL"}`, http.StatusBadRequest)
			return
		}
		ano, err := strconv.Atoi(anoStr)
		if err != nil || ano < 2000 || ano > 2100 {
			http.Error(w, `{"error":"ano inválido"}`, http.StatusBadRequest)
			return
		}
		seq, err := strconv.Atoi(seqStr)
		if err != nil || seq < 1 || seq > maxSeq {
			http.Error(w, `{"error":"periodo_seq inválido para `+tipoPeriodo+` (1–`+strconv.Itoa(maxSeq)+`)"}`, http.StatusBadRequest)
			return
		}

		// ── Ler arquivo em bytes (uma única leitura) ──────────────────────────
		if err := r.ParseMultipartForm(128 << 20); err != nil {
			// fallback para body direto
		}

		var rawReader io.Reader
		file, _, ferr := r.FormFile("file")
		if ferr == nil {
			defer file.Close()
			rawReader = file
		} else {
			rawReader = r.Body
		}

		rawBytes, err := io.ReadAll(rawReader)
		if err != nil || len(rawBytes) == 0 {
			http.Error(w, `{"error":"falha ao ler arquivo"}`, http.StatusBadRequest)
			return
		}

		// Strip UTF-8 BOM (arquivos Excel)
		if len(rawBytes) >= 3 && rawBytes[0] == 0xEF && rawBytes[1] == 0xBB && rawBytes[2] == 0xBF {
			rawBytes = rawBytes[3:]
		}

		// Estimativa rápida de total via contagem de '\n'
		lineCount := bytes.Count(rawBytes, []byte{'\n'})
		if len(rawBytes) > 0 && rawBytes[len(rawBytes)-1] != '\n' {
			lineCount++
		}
		estimatedRows := lineCount - 1
		if estimatedRows < 0 {
			estimatedRows = 0
		}

		log.Printf("[ObjetivosImport] arquivo=%dKB estimado=%d linhas tipo=%s ano=%d seq=%d empresa=%s",
			len(rawBytes)/1024, estimatedRows, tipoPeriodo, ano, seq, spCtx.EmpresaID)

		// ── Switch para SSE ───────────────────────────────────────────────────
		flusher, canFlush := w.(http.Flusher)
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Accel-Buffering", "no")

		sendEvent := func(v any) {
			b, _ := json.Marshal(v)
			fmt.Fprintf(w, "data: %s\n\n", b)
			if canFlush {
				flusher.Flush()
			}
		}

		// Heartbeat a cada 15s para evitar timeout do proxy reverso (nginx/Coolify)
		// durante imports longos. Comentários SSE (": ping") são ignorados pelo cliente.
		stopHeartbeat := make(chan struct{})
		go func() {
			t := time.NewTicker(15 * time.Second)
			defer t.Stop()
			for {
				select {
				case <-t.C:
					fmt.Fprint(w, ": ping\n\n")
					if canFlush {
						flusher.Flush()
					}
				case <-stopHeartbeat:
					return
				}
			}
		}()
		defer close(stopHeartbeat)

		sendEvent(map[string]any{"total": estimatedRows})

		// ── CSV streaming + batch upsert ──────────────────────────────────────
		csvReader := csv.NewReader(bytes.NewReader(rawBytes))
		csvReader.Comma = ';'
		csvReader.LazyQuotes = true
		csvReader.TrimLeadingSpace = true
		csvReader.FieldsPerRecord = -1

		if _, err := csvReader.Read(); err != nil {
			sendEvent(map[string]any{"error": "falha ao ler cabeçalho CSV"})
			return
		}

		type batchRow struct {
			codSup       int64 // -1 = NULL
			codRCA       int64
			codDepto     string
			departamento string
			codSec       string
			secao        string
			codFornec    string
			fornecedor   string
			codProd      string
			codCli       int64  // CODCLI do CSV — código do cliente (numérico)
			vlAnt        float64
			vlCor        float64
		}

		tx, err := db.Begin()
		if err != nil {
			sendEvent(map[string]any{"error": err.Error()})
			return
		}
		defer tx.Rollback()

		var imported, updated, skipped int
		processed := 0
		buf := make([]batchRow, 0, objetivosBatchSize)

		flushBatch := func() {
			if len(buf) == 0 {
				return
			}

			// De-duplica o batch: o CSV pode ter a mesma chave várias vezes
			// (mesmo sup × rca × depto × sec × fornec × prod). PostgreSQL rejeita
			// INSERT ... ON CONFLICT DO UPDATE quando o mesmo alvo seria atualizado
			// duas vezes no mesmo statement (erro 21000). Mantém a última ocorrência.
			seen := make(map[string]int, len(buf))
			deduped := make([]batchRow, 0, len(buf))
			for _, row := range buf {
				key := fmt.Sprintf("%d|%d|%s|%s|%s|%s|%s",
					row.codSup, row.codRCA, row.codDepto, row.codSec, row.codFornec, row.codProd, row.codCli)
				if idx, ok := seen[key]; ok {
					deduped[idx] = row // última ocorrência sobrescreve
				} else {
					seen[key] = len(deduped)
					deduped = append(deduped, row)
				}
			}

			n := len(deduped)
			codSups      := make([]int64,   n)
			codRCAs      := make([]int64,   n)
			codDeptos    := make([]string,  n)
			deptos       := make([]string,  n)
			codSecs      := make([]string,  n)
			secoes       := make([]string,  n)
			codFornecs   := make([]string,  n)
			fornecedores := make([]string,  n)
			codProds     := make([]string,  n)
			codClis      := make([]int64,   n)
			vlAnts       := make([]float64, n)
			vlCors       := make([]float64, n)
			for i, row := range deduped {
				codSups[i]      = row.codSup
				codRCAs[i]      = row.codRCA
				codDeptos[i]    = row.codDepto
				deptos[i]       = row.departamento
				codSecs[i]      = row.codSec
				secoes[i]       = row.secao
				codFornecs[i]   = row.codFornec
				fornecedores[i] = row.fornecedor
				codProds[i]     = row.codProd
				codClis[i]      = row.codCli
				vlAnts[i]       = row.vlAnt
				vlCors[i]       = row.vlCor
			}

			// SAVEPOINT por batch: evita que um erro aborte a transação inteira
			if _, serr := tx.Exec("SAVEPOINT sp_batch"); serr != nil {
				log.Printf("[ObjetivosImport] savepoint erro: %v", serr)
				skipped += n
				processed += n
				buf = buf[:0]
				sendEvent(map[string]any{"processed": processed, "importados": imported, "atualizados": updated, "ignorados": skipped})
				return
			}

			// Usa FROM unnest(a1, a2, ...) AS t(...) — sintaxe correta para arrays
			// de tipos diferentes; évita o comportamento indefinido de múltiplos
			// unnest() no SELECT em versões anteriores ao PostgreSQL 10.
			qrows, qerr := tx.Query(`
				INSERT INTO objetivos_importados (
				    empresa_id, tipo_periodo, ano, periodo_seq,
				    cod_supervisor, cod_rca,
				    cod_depto, departamento, cod_sec, secao,
				    cod_fornec, fornecedor, cod_prod, cod_cli,
				    vl_anterior, vl_corrente
				)
				SELECT
				    $1, $2, $3, $4,
				    NULLIF(t.csup, -1)::int,
				    t.crca::int,
				    NULLIF(t.cdep, ''),
				    NULLIF(t.dep,  ''),
				    NULLIF(t.csec, ''),
				    NULLIF(t.sec,  ''),
				    t.cforn,
				    NULLIF(t.forn, ''),
				    t.cprod,
				    t.ccli,
				    t.vant,
				    t.vcor
				FROM unnest(
				    $5::bigint[], $6::bigint[],
				    $7::text[], $8::text[], $9::text[], $10::text[],
				    $11::text[], $12::text[], $13::text[], $14::bigint[],
				    $15::float8[], $16::float8[]
				) AS t(csup, crca, cdep, dep, csec, sec, cforn, forn, cprod, ccli, vant, vcor)
				ON CONFLICT (empresa_id, tipo_periodo, ano, periodo_seq,
				             cod_supervisor, cod_rca, cod_depto, cod_sec, cod_fornec, cod_prod, cod_cli)
				DO UPDATE SET
				    fornecedor   = EXCLUDED.fornecedor,
				    departamento = EXCLUDED.departamento,
				    secao        = EXCLUDED.secao,
				    vl_anterior  = EXCLUDED.vl_anterior,
				    vl_corrente  = EXCLUDED.vl_corrente,
				    importado_em = NOW()
				RETURNING (xmax = 0)`,
				spCtx.EmpresaID, tipoPeriodo, ano, seq,
				pq.Array(codSups), pq.Array(codRCAs),
				pq.Array(codDeptos), pq.Array(deptos),
				pq.Array(codSecs), pq.Array(secoes),
				pq.Array(codFornecs), pq.Array(fornecedores), pq.Array(codProds), pq.Array(codClis),
				pq.Array(vlAnts), pq.Array(vlCors),
			)
			if qerr != nil {
				log.Printf("[ObjetivosImport] batch erro (n=%d): %v", n, qerr)
				tx.Exec("ROLLBACK TO SAVEPOINT sp_batch") //nolint
				skipped += n
			} else {
				// Itera rows ANTES de liberar o savepoint.
				// Em lib/pq os DataRows são lidos lazily pela conexão;
				// chamar Exec() antes de Close() invalida o cursor silenciosamente.
				for qrows.Next() {
					var isNew bool
					qrows.Scan(&isNew) //nolint
					if isNew {
						imported++
					} else {
						updated++
					}
				}
				qrows.Close()
				tx.Exec("RELEASE SAVEPOINT sp_batch") //nolint
			}
			processed += n
			buf = buf[:0]
			sendEvent(map[string]any{"processed": processed, "importados": imported, "atualizados": updated, "ignorados": skipped})
		}

		// ── Scan CSV linha a linha ────────────────────────────────────────────
		for {
			record, rerr := csvReader.Read()
			if rerr == io.EOF {
				break
			}
			if rerr != nil || len(record) < 12 {
				skipped++
				processed++
				continue
			}

			codRCA, errRCA := strconv.ParseInt(strings.TrimSpace(record[1]), 10, 64)
			codFornec := strings.TrimSpace(record[6])
			codProd := strings.TrimSpace(record[8])
			if errRCA != nil || codFornec == "" || codProd == "" {
				skipped++
				processed++
				continue
			}

			var codSup int64 = -1
			if cs, e := strconv.ParseInt(strings.TrimSpace(record[0]), 10, 64); e == nil {
				codSup = cs
			}

			codCli, _ := strconv.ParseInt(strings.TrimSpace(record[9]), 10, 64)
			vlAnt, errA := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(record[10]), ",", "."), 64)
			vlCor, errC := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(record[11]), ",", "."), 64)
			if errA != nil || errC != nil {
				skipped++
				processed++
				continue
			}

			buf = append(buf, batchRow{
				codSup:       codSup,
				codRCA:       codRCA,
				codDepto:     strings.TrimSpace(record[2]),
				departamento: strings.TrimSpace(record[3]),
				codSec:       strings.TrimSpace(record[4]),
				secao:        strings.TrimSpace(record[5]),
				codFornec:    codFornec,
				fornecedor:   strings.TrimSpace(record[7]),
				codProd:      codProd,
				codCli:       codCli,
				vlAnt:        vlAnt,
				vlCor:        vlCor,
			})

			if len(buf) == objetivosBatchSize {
				flushBatch()
			}
		}

		// flush do batch final
		flushBatch()

		if err := tx.Commit(); err != nil {
			log.Printf("[ObjetivosImport] commit erro: %v", err)
			sendEvent(map[string]any{"error": err.Error()})
			return
		}

		log.Printf("[ObjetivosImport] concluído: importados=%d atualizados=%d ignorados=%d",
			imported, updated, skipped)

		sendEvent(map[string]any{
			"done":        true,
			"importados":  imported,
			"atualizados": updated,
			"ignorados":   skipped,
		})
	}
}

// ObjetivosRCAHandler retorna dados de vw_obj_rca_fornecedor filtrados por período.
// GET /api/objetivos/rca-fornecedor?tipo_periodo=MENSAL&ano=2025&periodo_seq=1&q=texto
func ObjetivosRCAHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		tipo := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("tipo_periodo")))
		anoStr := strings.TrimSpace(r.URL.Query().Get("ano"))
		seqStr := strings.TrimSpace(r.URL.Query().Get("periodo_seq"))
		busca := strings.TrimSpace(r.URL.Query().Get("q"))

		if _, ok := periodoSeqMax[tipo]; !ok {
			http.Error(w, `{"error":"tipo_periodo inválido"}`, http.StatusBadRequest)
			return
		}
		ano, err := strconv.Atoi(anoStr)
		if err != nil || ano < 2000 || ano > 2100 {
			http.Error(w, `{"error":"ano inválido"}`, http.StatusBadRequest)
			return
		}
		seq, err := strconv.Atoi(seqStr)
		if err != nil || seq < 1 {
			http.Error(w, `{"error":"periodo_seq inválido"}`, http.StatusBadRequest)
			return
		}

		likeParam := "%" + busca + "%"
		rows, err := db.Query(`
			SELECT cod_supervisor, nome_supervisor, cod_rca, nome_rca,
			       cod_fornec, fornecedor, qtd_produtos, qtd_clientes,
			       vl_anterior, vl_corrente
			FROM vw_obj_rca_fornecedor
			WHERE empresa_id = $1
			  AND tipo_periodo = $2
			  AND ano = $3
			  AND periodo_seq = $4
			  AND ($5 = '%%' OR nome_rca ILIKE $5 OR nome_supervisor ILIKE $5 OR fornecedor ILIKE $5)
			ORDER BY nome_supervisor, nome_rca, fornecedor`,
			spCtx.EmpresaID, tipo, ano, seq, likeParam)
		if err != nil {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		type row struct {
			CodSupervisor  *int    `json:"cod_supervisor"`
			NomeSupervisor string  `json:"nome_supervisor"`
			CodRCA         int     `json:"cod_rca"`
			NomeRCA        string  `json:"nome_rca"`
			CodFornec      string  `json:"cod_fornec"`
			Fornecedor     string  `json:"fornecedor"`
			QtdProdutos    int64   `json:"qtd_produtos"`
			QtdClientes    int64   `json:"qtd_clientes"`
			VlAnterior     float64 `json:"vl_anterior"`
			VlCorrente     float64 `json:"vl_corrente"`
		}
		result := make([]row, 0)
		for rows.Next() {
			var rw row
			var supNull          sql.NullInt64
			var nomeSup, nomeRCA, fornec sql.NullString
			if err := rows.Scan(&supNull, &nomeSup, &rw.CodRCA, &nomeRCA,
				&rw.CodFornec, &fornec, &rw.QtdProdutos, &rw.QtdClientes,
				&rw.VlAnterior, &rw.VlCorrente); err == nil {
				if supNull.Valid {
					v := int(supNull.Int64)
					rw.CodSupervisor = &v
				}
				rw.NomeSupervisor = nomeSup.String
				rw.NomeRCA = nomeRCA.String
				rw.Fornecedor = fornec.String
				result = append(result, rw)
			}
		}
		json.NewEncoder(w).Encode(result)
	}
}

// ObjetivosSupervisorHandler retorna dados de vw_obj_supervisor filtrados por período.
// GET /api/objetivos/supervisor?tipo_periodo=MENSAL&ano=2025&periodo_seq=1&q=texto
func ObjetivosSupervisorHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		tipo := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("tipo_periodo")))
		anoStr := strings.TrimSpace(r.URL.Query().Get("ano"))
		seqStr := strings.TrimSpace(r.URL.Query().Get("periodo_seq"))
		busca := strings.TrimSpace(r.URL.Query().Get("q"))

		if _, ok := periodoSeqMax[tipo]; !ok {
			http.Error(w, `{"error":"tipo_periodo inválido"}`, http.StatusBadRequest)
			return
		}
		ano, err := strconv.Atoi(anoStr)
		if err != nil || ano < 2000 || ano > 2100 {
			http.Error(w, `{"error":"ano inválido"}`, http.StatusBadRequest)
			return
		}
		seq, err := strconv.Atoi(seqStr)
		if err != nil || seq < 1 {
			http.Error(w, `{"error":"periodo_seq inválido"}`, http.StatusBadRequest)
			return
		}

		likeParam := "%" + busca + "%"
		rows, err := db.Query(`
			SELECT cod_supervisor, nome_supervisor,
			       cod_fornec, fornecedor,
			       qtd_rcas, qtd_produtos, qtd_clientes,
			       vl_anterior, vl_corrente
			FROM vw_obj_supervisor
			WHERE empresa_id = $1
			  AND tipo_periodo = $2
			  AND ano = $3
			  AND periodo_seq = $4
			  AND ($5 = '%%' OR nome_supervisor ILIKE $5 OR fornecedor ILIKE $5)
			ORDER BY nome_supervisor, fornecedor`,
			spCtx.EmpresaID, tipo, ano, seq, likeParam)
		if err != nil {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		type row struct {
			CodSupervisor  *int    `json:"cod_supervisor"`
			NomeSupervisor string  `json:"nome_supervisor"`
			CodFornec      string  `json:"cod_fornec"`
			Fornecedor     string  `json:"fornecedor"`
			QtdRCAs        int64   `json:"qtd_rcas"`
			QtdProdutos    int64   `json:"qtd_produtos"`
			QtdClientes    int64   `json:"qtd_clientes"`
			VlAnterior     float64 `json:"vl_anterior"`
			VlCorrente     float64 `json:"vl_corrente"`
		}
		result := make([]row, 0)
		for rows.Next() {
			var rw row
			var supNull     sql.NullInt64
			var nomeSup, fornec sql.NullString
			if err := rows.Scan(&supNull, &nomeSup, &rw.CodFornec, &fornec,
				&rw.QtdRCAs, &rw.QtdProdutos, &rw.QtdClientes,
				&rw.VlAnterior, &rw.VlCorrente); err == nil {
				if supNull.Valid {
					v := int(supNull.Int64)
					rw.CodSupervisor = &v
				}
				rw.NomeSupervisor = nomeSup.String
				rw.Fornecedor = fornec.String
				result = append(result, rw)
			}
		}
		json.NewEncoder(w).Encode(result)
	}
}

// ObjetivosPeriosHandler lista os períodos disponíveis para a empresa ativa.
// GET /api/objetivos/periodos
func ObjetivosPeriosHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		rows, err := db.Query(`
			SELECT DISTINCT tipo_periodo, ano, periodo_seq
			FROM objetivos_importados
			WHERE empresa_id = $1
			ORDER BY ano DESC, periodo_seq DESC`, spCtx.EmpresaID)
		if err != nil {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		type periodo struct {
			TipoPeriodo string `json:"tipo_periodo"`
			Ano         int    `json:"ano"`
			PeriodoSeq  int    `json:"periodo_seq"`
		}
		result := make([]periodo, 0)
		for rows.Next() {
			var p periodo
			if rows.Scan(&p.TipoPeriodo, &p.Ano, &p.PeriodoSeq) == nil {
				result = append(result, p)
			}
		}
		json.NewEncoder(w).Encode(result)
	}
}
