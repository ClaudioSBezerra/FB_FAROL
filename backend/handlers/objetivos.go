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
			qtdCli       int64
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

		// Registra todos os códigos vistos para auto-criar gestores/rcas ausentes
		uniqueSups := make(map[int64]struct{})
		uniqueRCAs := make(map[int64]struct{})

		flushBatch := func() {
			if len(buf) == 0 {
				return
			}
			n := len(buf)
			codSups      := make([]int64,   n)
			codRCAs      := make([]int64,   n)
			codDeptos    := make([]string,  n)
			deptos       := make([]string,  n)
			codSecs      := make([]string,  n)
			secoes       := make([]string,  n)
			codFornecs   := make([]string,  n)
			fornecedores := make([]string,  n)
			codProds     := make([]string,  n)
			qtdClis      := make([]int64,   n)
			vlAnts       := make([]float64, n)
			vlCors       := make([]float64, n)
			for i, row := range buf {
				codSups[i]      = row.codSup
				codRCAs[i]      = row.codRCA
				codDeptos[i]    = row.codDepto
				deptos[i]       = row.departamento
				codSecs[i]      = row.codSec
				secoes[i]       = row.secao
				codFornecs[i]   = row.codFornec
				fornecedores[i] = row.fornecedor
				codProds[i]     = row.codProd
				qtdClis[i]      = row.qtdCli
				vlAnts[i]       = row.vlAnt
				vlCors[i]       = row.vlCor
			}
			qrows, qerr := tx.Query(`
				INSERT INTO objetivos_importados (
				    empresa_id, tipo_periodo, ano, periodo_seq,
				    cod_supervisor, cod_rca,
				    cod_depto, departamento, cod_sec, secao,
				    cod_fornec, fornecedor, cod_prod,
				    qtd_clientes, vl_anterior, vl_corrente
				)
				SELECT
				    $1, $2, $3, $4,
				    NULLIF(unnest($5::bigint[]),  -1)::int,
				    unnest($6::bigint[])::int,
				    NULLIF(unnest($7::text[]),  ''),
				    NULLIF(unnest($8::text[]),  ''),
				    NULLIF(unnest($9::text[]),  ''),
				    NULLIF(unnest($10::text[]), ''),
				    unnest($11::text[]),
				    NULLIF(unnest($12::text[]), ''),
				    unnest($13::text[]),
				    unnest($14::bigint[])::int,
				    unnest($15::float8[]),
				    unnest($16::float8[])
				ON CONFLICT (empresa_id, tipo_periodo, ano, periodo_seq,
				             cod_supervisor, cod_rca, cod_depto, cod_sec, cod_fornec, cod_prod)
				DO UPDATE SET
				    fornecedor   = EXCLUDED.fornecedor,
				    departamento = EXCLUDED.departamento,
				    secao        = EXCLUDED.secao,
				    qtd_clientes = EXCLUDED.qtd_clientes,
				    vl_anterior  = EXCLUDED.vl_anterior,
				    vl_corrente  = EXCLUDED.vl_corrente,
				    importado_em = NOW()
				RETURNING (xmax = 0)`,
				spCtx.EmpresaID, tipoPeriodo, ano, seq,
				pq.Array(codSups), pq.Array(codRCAs),
				pq.Array(codDeptos), pq.Array(deptos),
				pq.Array(codSecs), pq.Array(secoes),
				pq.Array(codFornecs), pq.Array(fornecedores), pq.Array(codProds),
				pq.Array(qtdClis), pq.Array(vlAnts), pq.Array(vlCors),
			)
			if qerr != nil {
				log.Printf("[ObjetivosImport] batch erro: %v", qerr)
				skipped += n
			} else {
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
				uniqueSups[codSup] = struct{}{}
			}
			uniqueRCAs[codRCA] = struct{}{}

			qtdCli, _ := strconv.ParseInt(strings.TrimSpace(record[9]), 10, 64)
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
				qtdCli:       qtdCli,
				vlAnt:        vlAnt,
				vlCor:        vlCor,
			})

			if len(buf) == objetivosBatchSize {
				flushBatch()
			}
		}

		// flush do batch final
		flushBatch()

		// ── Auto-criar gestores ausentes ──────────────────────────────────────
		var gestoresCriados, rcasCriados int64

		if len(uniqueSups) > 0 {
			supCodes := make([]int64, 0, len(uniqueSups))
			supNomes := make([]string, 0, len(uniqueSups))
			for code := range uniqueSups {
				supCodes = append(supCodes, code)
				supNomes = append(supNomes, fmt.Sprintf("Supervisor %d", code))
			}
			res, ierr := tx.Exec(`
				INSERT INTO gestores (empresa_id, cod_supervisor, nome)
				SELECT $1, unnest($2::bigint[])::int, unnest($3::text[])
				ON CONFLICT (empresa_id, cod_supervisor) DO NOTHING`,
				spCtx.EmpresaID, pq.Array(supCodes), pq.Array(supNomes),
			)
			if ierr != nil {
				log.Printf("[ObjetivosImport] auto-insert gestores: %v", ierr)
			} else {
				gestoresCriados, _ = res.RowsAffected()
			}
		}

		// ── Auto-criar RCAs ausentes ──────────────────────────────────────────
		if len(uniqueRCAs) > 0 {
			rcaCodes := make([]int64, 0, len(uniqueRCAs))
			rcaNomes := make([]string, 0, len(uniqueRCAs))
			for code := range uniqueRCAs {
				rcaCodes = append(rcaCodes, code)
				rcaNomes = append(rcaNomes, fmt.Sprintf("RCA %d", code))
			}
			res, ierr := tx.Exec(`
				INSERT INTO rcas (empresa_id, cod_rca, nome)
				SELECT $1, unnest($2::bigint[])::int, unnest($3::text[])
				ON CONFLICT (empresa_id, cod_rca) DO NOTHING`,
				spCtx.EmpresaID, pq.Array(rcaCodes), pq.Array(rcaNomes),
			)
			if ierr != nil {
				log.Printf("[ObjetivosImport] auto-insert rcas: %v", ierr)
			} else {
				rcasCriados, _ = res.RowsAffected()
			}
		}

		if err := tx.Commit(); err != nil {
			log.Printf("[ObjetivosImport] commit erro: %v", err)
			sendEvent(map[string]any{"error": err.Error()})
			return
		}

		log.Printf("[ObjetivosImport] concluído: importados=%d atualizados=%d ignorados=%d gestores_criados=%d rcas_criados=%d",
			imported, updated, skipped, gestoresCriados, rcasCriados)

		sendEvent(map[string]any{
			"done":              true,
			"importados":        imported,
			"atualizados":       updated,
			"ignorados":         skipped,
			"gestores_criados":  gestoresCriados,
			"rcas_criados":      rcasCriados,
		})
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
