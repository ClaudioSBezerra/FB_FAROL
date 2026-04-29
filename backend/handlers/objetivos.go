package handlers

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// periodoSeqMax retorna o valor máximo permitido de periodo_seq por tipo.
var periodoSeqMax = map[string]int{
	"MENSAL":      12,
	"TRIMESTRAL":  4,
	"SEMESTRAL":   2,
	"ANUAL":       1,
}

// ObjetivosImportHandler processa o upload de CSV de objetivos de vendas.
// POST /api/objetivos/upload-csv?tipo_periodo=MENSAL&ano=2025&periodo_seq=1
func ObjetivosImportHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")

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
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			// fallback para body direto
		}

		var reader io.Reader
		file, _, ferr := r.FormFile("file")
		if ferr == nil {
			defer file.Close()
			// Strip UTF-8 BOM se presente (arquivos Excel)
			bom := make([]byte, 3)
			n, _ := file.Read(bom)
			if n == 3 && bom[0] == 0xEF && bom[1] == 0xBB && bom[2] == 0xBF {
				reader = file // BOM consumido
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

		// skip header
		if _, err := csvReader.Read(); err != nil {
			http.Error(w, `{"error":"falha ao ler cabeçalho CSV"}`, http.StatusBadRequest)
			return
		}

		tx, err := db.Begin()
		if err != nil {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()

		var imported, updated, skipped int

		for {
			record, rerr := csvReader.Read()
			if rerr == io.EOF {
				break
			}
			if rerr != nil || len(record) < 12 {
				skipped++
				continue
			}

			// Mapear por posição (imune a renomeação de colunas)
			codSupStr  := strings.TrimSpace(record[0])
			codRCAStr  := strings.TrimSpace(record[1])
			codDepto   := strings.TrimSpace(record[2])
			departamento := strings.TrimSpace(record[3])
			codSec     := strings.TrimSpace(record[4])
			secao      := strings.TrimSpace(record[5])
			codFornec  := strings.TrimSpace(record[6])
			fornecedor := strings.TrimSpace(record[7])
			codProd    := strings.TrimSpace(record[8])
			qtdCliStr  := strings.TrimSpace(record[9])
			vlAntStr   := strings.TrimSpace(record[10])
			vlCorStr   := strings.TrimSpace(record[11])

			codRCA, errRCA := strconv.Atoi(codRCAStr)
			if errRCA != nil || codFornec == "" || codProd == "" {
				skipped++
				continue
			}

			var codSupPtr *int
			if cs, e := strconv.Atoi(codSupStr); e == nil {
				codSupPtr = &cs
			}

			qtdCli, _ := strconv.Atoi(qtdCliStr)

			vlAnt, errA := strconv.ParseFloat(strings.ReplaceAll(vlAntStr, ",", "."), 64)
			vlCor, errC := strconv.ParseFloat(strings.ReplaceAll(vlCorStr, ",", "."), 64)
			if errA != nil || errC != nil {
				skipped++
				continue
			}

			// Savepoint por linha
			if _, err = tx.Exec("SAVEPOINT sp_obj"); err != nil {
				skipped++
				continue
			}

			var wasNew bool
			err = tx.QueryRow(`
				INSERT INTO objetivos_importados
				  (empresa_id, tipo_periodo, ano, periodo_seq,
				   cod_supervisor, cod_rca,
				   cod_depto, departamento, cod_sec, secao,
				   cod_fornec, fornecedor, cod_prod,
				   qtd_clientes, vl_anterior, vl_corrente)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
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
				codSupPtr, codRCA,
				nullableString(codDepto), nullableString(departamento),
				nullableString(codSec), nullableString(secao),
				codFornec, nullableString(fornecedor), codProd,
				qtdCli, vlAnt, vlCor,
			).Scan(&wasNew)

			if err != nil {
				tx.Exec("ROLLBACK TO SAVEPOINT sp_obj") //nolint
				skipped++
				continue
			}
			tx.Exec("RELEASE SAVEPOINT sp_obj") //nolint

			if wasNew {
				imported++
			} else {
				updated++
			}
		}

		if err := tx.Commit(); err != nil {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(map[string]int{
			"importados":  imported,
			"atualizados": updated,
			"ignorados":   skipped,
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
