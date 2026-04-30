package handlers

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
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

// batchSize define o número de linhas por INSERT batch (unnest).
const batchSize = 500

// ObjetivosImportHandler processa o upload de CSV de objetivos de vendas.
// POST /api/objetivos/upload-csv?tipo_periodo=MENSAL&ano=2025&periodo_seq=1
//
// Resposta: text/event-stream (SSE)
//
//	{"total":N}
//	{"processed":i,"importados":x,"atualizados":y,"ignorados":z}  × N/batchSize
//	{"done":true,"importados":x,"atualizados":y,"ignorados":z}
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

		// ── Ler arquivo ───────────────────────────────────────────────────────
		if err := r.ParseMultipartForm(64 << 20); err != nil {
			// fallback para body direto
		}

		var reader io.Reader
		file, _, ferr := r.FormFile("file")
		if ferr == nil {
			defer file.Close()
			bom := make([]byte, 3)
			n, _ := file.Read(bom)
			if n == 3 && bom[0] == 0xEF && bom[1] == 0xBB && bom[2] == 0xBF {
				reader = file
			} else {
				reader = io.MultiReader(strings.NewReader(string(bom[:n])), file)
			}
		} else {
			reader = r.Body
		}

		csvReader := csv.NewReader(reader)
		csvReader.Comma = ';'
		csvReader.LazyQuotes = true
		csvReader.TrimLeadingSpace = true
		csvReader.FieldsPerRecord = -1 // permite linhas com número variável de colunas

		// Lê tudo em memória (precisa do total para o SSE e para os batch arrays)
		allRecords, err := csvReader.ReadAll()
		if err != nil || len(allRecords) < 1 {
			http.Error(w, `{"error":"falha ao ler CSV"}`, http.StatusBadRequest)
			return
		}
		dataRows := allRecords[1:] // descarta cabeçalho

		// ── Pré-validação em Go (zero round-trips) ───────────────────────────
		type validRow struct {
			codSup      int64 // -1 = NULL (NULLIF na query)
			codRCA      int64
			codDepto    string
			departamento string
			codSec      string
			secao       string
			codFornec   string
			fornecedor  string
			codProd     string
			qtdCli      int64
			vlAnt       float64
			vlCor       float64
		}

		valid := make([]validRow, 0, len(dataRows))
		preSkipped := 0

		for _, rec := range dataRows {
			if len(rec) < 12 {
				preSkipped++
				continue
			}

			codRCA, errRCA := strconv.ParseInt(strings.TrimSpace(rec[1]), 10, 64)
			if errRCA != nil {
				preSkipped++
				continue
			}
			codFornec := strings.TrimSpace(rec[6])
			codProd := strings.TrimSpace(rec[8])
			if codFornec == "" || codProd == "" {
				preSkipped++
				continue
			}

			var codSup int64 = -1
			if cs, e := strconv.ParseInt(strings.TrimSpace(rec[0]), 10, 64); e == nil {
				codSup = cs
			}

			qtdCli, _ := strconv.ParseInt(strings.TrimSpace(rec[9]), 10, 64)

			vlAnt, errA := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(rec[10]), ",", "."), 64)
			vlCor, errC := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(rec[11]), ",", "."), 64)
			if errA != nil || errC != nil {
				preSkipped++
				continue
			}

			valid = append(valid, validRow{
				codSup:      codSup,
				codRCA:      codRCA,
				codDepto:    strings.TrimSpace(rec[2]),
				departamento: strings.TrimSpace(rec[3]),
				codSec:      strings.TrimSpace(rec[4]),
				secao:       strings.TrimSpace(rec[5]),
				codFornec:   codFornec,
				fornecedor:  strings.TrimSpace(rec[7]),
				codProd:     codProd,
				qtdCli:      qtdCli,
				vlAnt:       vlAnt,
				vlCor:       vlCor,
			})
		}

		// ── Switch para SSE ──────────────────────────────────────────────────
		flusher, canFlush := w.(http.Flusher)
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Accel-Buffering", "no")
		w.Header().Set("Connection", "keep-alive")

		sendEvent := func(v any) {
			b, _ := json.Marshal(v)
			fmt.Fprintf(w, "data: %s\n\n", b)
			if canFlush {
				flusher.Flush()
			}
		}

		// total = linhas válidas (as únicas que precisam de DB)
		sendEvent(map[string]any{"total": len(valid)})

		if len(valid) == 0 {
			sendEvent(map[string]any{"done": true, "importados": 0, "atualizados": 0, "ignorados": preSkipped})
			return
		}

		tx, err := db.Begin()
		if err != nil {
			sendEvent(map[string]any{"error": err.Error()})
			return
		}
		defer tx.Rollback()

		var imported, updated, dbSkipped int
		processed := 0

		for batchStart := 0; batchStart < len(valid); batchStart += batchSize {
			end := batchStart + batchSize
			if end > len(valid) {
				end = len(valid)
			}
			batch := valid[batchStart:end]
			n := len(batch)

			// Monta arrays paralelos para unnest
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

			for i, row := range batch {
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
				dbSkipped += n
				processed += n
				sendEvent(map[string]any{"processed": processed, "importados": imported, "atualizados": updated, "ignorados": preSkipped + dbSkipped})
				continue
			}

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

			processed += n
			sendEvent(map[string]any{"processed": processed, "importados": imported, "atualizados": updated, "ignorados": preSkipped + dbSkipped})
		}

		if err := tx.Commit(); err != nil {
			sendEvent(map[string]any{"error": err.Error()})
			return
		}

		sendEvent(map[string]any{
			"done":        true,
			"importados":  imported,
			"atualizados": updated,
			"ignorados":   preSkipped + dbSkipped,
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
