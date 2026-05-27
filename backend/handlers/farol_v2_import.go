package handlers

// farol_v2_import.go — Importação CSV para vendas_importadas (novo Farol 2026).
//
// POST /api/v2/vendas/import?tipo_base=ATUAL&ano=2026&mes=5
//   — Aceita multipart/form-data com campo "file" (CSV ; separado)
//   — Responde com Server-Sent Events (SSE) de progresso
//
// GET  /api/v2/vendas/periodos — lista períodos disponíveis por tipo_base
// DELETE /api/v2/vendas/clear?tipo_base=ATUAL — remove todas as vendas do tipo

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
	"time"
	"unicode/utf8"
)

// ─── VendasImportHandler — POST /api/v2/vendas/import ───────────────────────

const vendasBatchSize = 500

func VendasImportHandler(db *sql.DB) http.HandlerFunc {
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

		// ── Parâmetros ────────────────────────────────────────────────────────
		tipoBase := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("tipo_base")))
		if tipoBase != "ATUAL" && tipoBase != "COMPARATIVA" {
			http.Error(w, `{"error":"tipo_base deve ser ATUAL ou COMPARATIVA"}`, http.StatusBadRequest)
			return
		}
		anoStr := strings.TrimSpace(r.URL.Query().Get("ano"))
		mesStr := strings.TrimSpace(r.URL.Query().Get("mes"))
		ano, errA := strconv.Atoi(anoStr)
		mes, errM := strconv.Atoi(mesStr)
		if errA != nil || ano < 2000 || ano > 2100 || errM != nil || mes < 1 || mes > 12 {
			http.Error(w, `{"error":"ano (2000-2100) e mes (1-12) obrigatórios"}`, http.StatusBadRequest)
			return
		}

		// ── Ler arquivo ───────────────────────────────────────────────────────
		_ = r.ParseMultipartForm(256 << 20)
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

		// Strip UTF-8 BOM
		if len(rawBytes) >= 3 && rawBytes[0] == 0xEF && rawBytes[1] == 0xBB && rawBytes[2] == 0xBF {
			rawBytes = rawBytes[3:]
		}
		// Converte Latin-1/Windows-1252 → UTF-8
		if !utf8.Valid(rawBytes) {
			out := make([]byte, 0, len(rawBytes)*2)
			for _, b := range rawBytes {
				if b < 0x80 {
					out = append(out, b)
				} else {
					var buf [4]byte
					n := utf8.EncodeRune(buf[:], rune(b))
					out = append(out, buf[:n]...)
				}
			}
			rawBytes = out
		}

		lineCount := bytes.Count(rawBytes, []byte{'\n'})
		if len(rawBytes) > 0 && rawBytes[len(rawBytes)-1] != '\n' {
			lineCount++
		}
		estimatedRows := lineCount - 1
		if estimatedRows < 0 {
			estimatedRows = 0
		}
		log.Printf("[VendasImport] empresa=%s tipo=%s ano=%d mes=%d arquivo=%dKB ~%d linhas",
			spCtx.EmpresaID, tipoBase, ano, mes, len(rawBytes)/1024, estimatedRows)

		// ── SSE ───────────────────────────────────────────────────────────────
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

		// ── Parse CSV ─────────────────────────────────────────────────────────
		csvReader := csv.NewReader(bytes.NewReader(rawBytes))
		csvReader.Comma = ';'
		csvReader.LazyQuotes = true
		csvReader.TrimLeadingSpace = true
		csvReader.FieldsPerRecord = -1

		headerRow, err := csvReader.Read()
		if err != nil {
			sendEvent(map[string]any{"error": "falha ao ler cabeçalho CSV"})
			return
		}

		norm := func(s string) string {
			s = strings.ToLower(strings.TrimSpace(s))
			s = strings.ReplaceAll(s, " ", "")
			s = strings.ReplaceAll(s, "_", "")
			return s
		}
		colMap := make(map[string]int, len(headerRow))
		for i, h := range headerRow {
			colMap[norm(h)] = i
		}
		col := func(def int, candidates ...string) int {
			for _, c := range candidates {
				if idx, ok := colMap[norm(c)]; ok {
					return idx
				}
			}
			return def
		}

		iCodGerente    := col(-1, "codgerente", "cod_gerente")
		iNomeGerente   := col(-1, "gerente", "nome_gerente")
		iCodSup        := col(-1, "codsupervisor", "cod_supervisor")
		iNomeSup       := col(-1, "supervisor", "nome_supervisor")
		iCodRca        := col(-1, "codusur", "cod_rca", "codrca")
		iNomeRca       := col(-1, "rca", "nome_rca")
		iQtcliRca      := col(-1, "qtclirca", "qtcli_rca")
		iCodFornec     := col(-1, "codfornec", "cod_fornec")
		iNomeFornec    := col(-1, "fornecedor", "nome_fornec")
		iCodCli        := col(-1, "codcli", "cod_cli")
		iNomeCli       := col(-1, "cliente", "nome_cli")
		iUf            := col(-1, "uf")
		iEmpresa       := col(-1, "empresa")
		iCodProd       := col(-1, "codprod", "cod_prod")
		iNomeProd      := col(-1, "produto", "nome_prod")
		iQt            := col(-1, "qt", "quantidade")
		iPvenda        := col(-1, "pvenda", "valor", "vl_venda")
		iPeriodo       := col(-1, "periodo")
		iEstado        := col(-1, "estado")

		getField := func(row []string, idx int) string {
			if idx < 0 || idx >= len(row) {
				return ""
			}
			return strings.TrimSpace(row[idx])
		}
		parseNum := func(s string) float64 {
			s = strings.TrimSpace(s)
			s = strings.ReplaceAll(s, ".", "")
			s = strings.ReplaceAll(s, ",", ".")
			v, _ := strconv.ParseFloat(s, 64)
			return v
		}
		parseInt3 := func(s string) int {
			s = strings.TrimSpace(s)
			// Remove non-digit characters
			cleaned := strings.Map(func(r rune) rune {
				if r >= '0' && r <= '9' {
					return r
				}
				return -1
			}, s)
			if cleaned == "" {
				return 0
			}
			v, _ := strconv.Atoi(cleaned)
			return v
		}
		detectEstado := func(periodo, estadoField string) string {
			if strings.Contains(strings.ToUpper(periodo), "TRANS") ||
				strings.Contains(strings.ToUpper(estadoField), "TRANS") {
				return "TRANSMITIDO"
			}
			return "FATURADO"
		}

		// ── Apaga dados anteriores do mesmo tipo_base + periodo ────────────────
		_, err = db.Exec(
			`DELETE FROM vendas_importadas WHERE empresa_id=$1 AND tipo_base=$2 AND ano=$3 AND mes=$4`,
			spCtx.EmpresaID, tipoBase, ano, mes,
		)
		if err != nil {
			sendEvent(map[string]any{"error": "erro ao limpar dados anteriores: " + err.Error()})
			return
		}

		// ── Batch insert ──────────────────────────────────────────────────────
		type vendaRow struct {
			codGerente, nomeGerente       string
			codSup, nomeSup               string
			codRca, nomeRca               string
			qtcliRca                      int
			codFornec, nomeFornec         string
			codCli, nomeCli               string
			uf, empresa                   string
			codProd, nomeProd             string
			qt, pvenda                    float64
			estado                        string
		}

		var batch []vendaRow
		processed, importados := 0, 0

		flush := func() error {
			if len(batch) == 0 {
				return nil
			}
			// Build bulk insert
			var sb strings.Builder
			sb.WriteString(`INSERT INTO vendas_importadas
				(empresa_id, tipo_base, ano, mes, estado,
				 cod_gerente, nome_gerente, cod_supervisor, nome_supervisor,
				 cod_rca, nome_rca, qtcli_rca,
				 cod_fornec, nome_fornec, cod_cli, nome_cli, uf, empresa,
				 cod_prod, nome_prod, qt, pvenda)
				VALUES `)
			args := make([]any, 0, len(batch)*22)
			for i, row := range batch {
				if i > 0 {
					sb.WriteString(",")
				}
				base := i * 22
				fmt.Fprintf(&sb, "($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d)",
					base+1, base+2, base+3, base+4, base+5,
					base+6, base+7, base+8, base+9,
					base+10, base+11, base+12,
					base+13, base+14, base+15, base+16, base+17, base+18,
					base+19, base+20, base+21, base+22,
				)
				args = append(args,
					spCtx.EmpresaID, tipoBase, ano, mes, row.estado,
					row.codGerente, row.nomeGerente, row.codSup, row.nomeSup,
					row.codRca, row.nomeRca, row.qtcliRca,
					row.codFornec, row.nomeFornec, row.codCli, row.nomeCli, row.uf, row.empresa,
					row.codProd, row.nomeProd, row.qt, row.pvenda,
				)
			}
			_, err := db.Exec(sb.String(), args...)
			if err != nil {
				return err
			}
			importados += len(batch)
			batch = batch[:0]
			return nil
		}

		for {
			csvRow, err := csvReader.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				continue
			}

			codFornec := getField(csvRow, iCodFornec)
			codRca    := getField(csvRow, iCodRca)
			codCli    := getField(csvRow, iCodCli)
			// Skip rows without any identifier
			if codFornec == "" && codRca == "" && codCli == "" {
				continue
			}

			periodo := getField(csvRow, iPeriodo)
			estadoField := getField(csvRow, iEstado)

			row := vendaRow{
				codGerente:  getField(csvRow, iCodGerente),
				nomeGerente: getField(csvRow, iNomeGerente),
				codSup:      getField(csvRow, iCodSup),
				nomeSup:     getField(csvRow, iNomeSup),
				codRca:      codRca,
				nomeRca:     getField(csvRow, iNomeRca),
				qtcliRca:    parseInt3(getField(csvRow, iQtcliRca)),
				codFornec:   codFornec,
				nomeFornec:  getField(csvRow, iNomeFornec),
				codCli:      codCli,
				nomeCli:     getField(csvRow, iNomeCli),
				uf:          getField(csvRow, iUf),
				empresa:     getField(csvRow, iEmpresa),
				codProd:     getField(csvRow, iCodProd),
				nomeProd:    getField(csvRow, iNomeProd),
				qt:          parseNum(getField(csvRow, iQt)),
				pvenda:      parseNum(getField(csvRow, iPvenda)),
				estado:      detectEstado(periodo, estadoField),
			}
			batch = append(batch, row)
			processed++

			if len(batch) >= vendasBatchSize {
				if err := flush(); err != nil {
					sendEvent(map[string]any{"error": "erro ao inserir batch: " + err.Error()})
					return
				}
				sendEvent(map[string]any{"processed": processed, "importados": importados})
			}
		}
		if err := flush(); err != nil {
			sendEvent(map[string]any{"error": "erro ao inserir batch final: " + err.Error()})
			return
		}

		log.Printf("[VendasImport] concluído: %d linhas → %d importadas", processed, importados)
		sendEvent(map[string]any{
			"done":       true,
			"processed":  processed,
			"importados": importados,
			"tipo_base":  tipoBase,
			"ano":        ano,
			"mes":        mes,
		})
	}
}

// ─── VendasPeriodosHandler — GET /api/v2/vendas/periodos ────────────────────

type v2PeriodoItem struct {
	TipoBase string `json:"tipo_base"`
	Ano      int    `json:"ano"`
	Mes      int    `json:"mes"`
	Label    string `json:"label"`
	Total    int    `json:"total"`
}

func VendasPeriodosHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		rows, err := db.Query(`
			SELECT tipo_base, ano, mes, COUNT(*) as total
			FROM vendas_importadas
			WHERE empresa_id = $1
			GROUP BY tipo_base, ano, mes
			ORDER BY tipo_base, ano DESC, mes DESC
		`, spCtx.EmpresaID)
		if err != nil {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		items := []v2PeriodoItem{}
		for rows.Next() {
			var it v2PeriodoItem
			if rows.Scan(&it.TipoBase, &it.Ano, &it.Mes, &it.Total) == nil {
				it.Label = fmtMesAno(it.Mes, it.Ano)
				items = append(items, it)
			}
		}
		json.NewEncoder(w).Encode(items)
	}
}

// ─── VendasClearHandler — DELETE /api/v2/vendas/clear ───────────────────────

func VendasClearHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
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

		tipoBase := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("tipo_base")))
		var res sql.Result
		var err error
		if tipoBase == "ATUAL" || tipoBase == "COMPARATIVA" {
			res, err = db.Exec(
				`DELETE FROM vendas_importadas WHERE empresa_id=$1 AND tipo_base=$2`,
				spCtx.EmpresaID, tipoBase,
			)
		} else {
			res, err = db.Exec(
				`DELETE FROM vendas_importadas WHERE empresa_id=$1`,
				spCtx.EmpresaID,
			)
		}
		if err != nil {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		n, _ := res.RowsAffected()
		json.NewEncoder(w).Encode(map[string]any{"deleted": n})
	}
}

// ─── IndustriasConfigHandler — GET/POST /api/v2/industrias ──────────────────

type industriaConfig struct {
	CodFornec           string  `json:"cod_fornec"`
	NomeFornec          string  `json:"nome_fornec"`
	ProgramaDistribuicao string `json:"programa_distribuicao"`
	TravaMinima         float64 `json:"trava_minima_qt"`
}

func IndustriasConfigHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		switch r.Method {
		case http.MethodGet:
			rows, err := db.Query(`
				SELECT cod_fornec, nome_fornec, programa_distribuicao, trava_minima_qt
				FROM industrias_config
				WHERE empresa_id=$1
				ORDER BY nome_fornec
			`, spCtx.EmpresaID)
			if err != nil {
				http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
				return
			}
			defer rows.Close()
			items := []industriaConfig{}
			for rows.Next() {
				var it industriaConfig
				if rows.Scan(&it.CodFornec, &it.NomeFornec, &it.ProgramaDistribuicao, &it.TravaMinima) == nil {
					items = append(items, it)
				}
			}
			json.NewEncoder(w).Encode(items)

		case http.MethodPut, http.MethodPost:
			if !RequireWrite(spCtx, w) {
				return
			}
			var it industriaConfig
			if err := json.NewDecoder(r.Body).Decode(&it); err != nil || it.CodFornec == "" {
				http.Error(w, `{"error":"payload inválido"}`, http.StatusBadRequest)
				return
			}
			if it.TravaMinima < 0 {
				it.TravaMinima = 1
			}
			_, err := db.Exec(`
				INSERT INTO industrias_config
					(empresa_id, cod_fornec, nome_fornec, programa_distribuicao, trava_minima_qt)
				VALUES ($1,$2,$3,$4,$5)
				ON CONFLICT (empresa_id, cod_fornec) DO UPDATE SET
					nome_fornec=EXCLUDED.nome_fornec,
					programa_distribuicao=EXCLUDED.programa_distribuicao,
					trava_minima_qt=EXCLUDED.trava_minima_qt,
					atualizado_em=NOW()
			`, spCtx.EmpresaID, it.CodFornec, it.NomeFornec, it.ProgramaDistribuicao, it.TravaMinima)
			if err != nil {
				http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"ok": true})

		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	}
}

// ─── Util ─────────────────────────────────────────────────────────────────────

var mesNomes = []string{"", "Jan", "Fev", "Mar", "Abr", "Mai", "Jun", "Jul", "Ago", "Set", "Out", "Nov", "Dez"}

func fmtMesAno(mes, ano int) string {
	if mes < 1 || mes > 12 {
		return fmt.Sprintf("%d", ano)
	}
	return fmt.Sprintf("%s/%d", mesNomes[mes], ano)
}
