package handlers

// farol_v2_import.go — Importação CSV para vendas_importadas (novo Farol 2026).
//
// Padrão job-based async (mesmo do FB_APU01):
//   POST /api/v2/vendas/import  → lê arquivo, cria job, inicia worker goroutine, retorna {job_id}
//   GET  /api/v2/vendas/job/{id}          → status do job (polling a cada 2s)
//   POST /api/v2/vendas/job/{id}/cancel   → cancela job em andamento
//   GET  /api/v2/vendas/periodos          → lista períodos importados
//   DELETE /api/v2/vendas/clear           → remove período

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
)

// importJobs guarda a função de cancelamento de cada job ativo.
// Keyed por job UUID; removido ao término (done/error/cancelled).
var importJobs sync.Map // string → context.CancelFunc

// ─── VendasImportHandler — POST /api/v2/vendas/import ───────────────────────

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
		ano, errA := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("ano")))
		mes, errM := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("mes")))
		if errA != nil || ano < 2000 || ano > 2100 || errM != nil || mes < 1 || mes > 12 {
			http.Error(w, `{"error":"ano (2000-2100) e mes (1-12) obrigatórios"}`, http.StatusBadRequest)
			return
		}

		// ── Ler arquivo — suporta até 1 GB (350 MB típico) ───────────────────
		_ = r.ParseMultipartForm(512 << 20)
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
		// Latin-1 / Windows-1252 → UTF-8
		if !utf8.Valid(rawBytes) {
			out := make([]byte, 0, len(rawBytes)+len(rawBytes)/8)
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
		log.Printf("[VendasImport] empresa=%s tipo=%s %d/%d arquivo=%dMB ~%d linhas",
			spCtx.EmpresaID, tipoBase, ano, mes, len(rawBytes)/1024/1024, estimatedRows)

		// ── Cria job no banco ─────────────────────────────────────────────────
		var jobID string
		err = db.QueryRow(`
			INSERT INTO vendas_import_jobs
				(empresa_id, tipo_base, ano, mes, status, total_lines)
			VALUES ($1, $2, $3, $4, 'pending', $5)
			RETURNING id`,
			spCtx.EmpresaID, tipoBase, ano, mes, estimatedRows,
		).Scan(&jobID)
		if err != nil {
			http.Error(w, `{"error":"erro ao criar job: `+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}

		// ── Inicia worker goroutine ───────────────────────────────────────────
		ctx, cancel := context.WithCancel(context.Background())
		importJobs.Store(jobID, cancel)

		// rawBytes é passado por referência ao goroutine (GC mantém vivo até ele terminar)
		go processImportJob(ctx, db, jobID, rawBytes, spCtx, tipoBase, ano, mes)

		// Retorna job_id imediatamente
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"job_id": jobID})
	}
}

// ─── processImportJob — worker goroutine ─────────────────────────────────────

const (
	copyChunkRows  = 100_000 // linhas por chunk de COPY (permite cancelar entre chunks)
	progressUpdate = 2 * time.Second
)

func processImportJob(ctx context.Context, db *sql.DB, jobID string,
	rawBytes []byte, spCtx *FarolContext, tipoBase string, ano, mes int) {

	defer func() {
		importJobs.Delete(jobID)
		if rec := recover(); rec != nil {
			log.Printf("[ImportJob:%s] panic: %v", jobID, rec)
			db.Exec(`UPDATE vendas_import_jobs SET status='error', message=$1, atualizado_em=NOW() WHERE id=$2`,
				fmt.Sprintf("erro interno: %v", rec), jobID)
		}
	}()

	markStatus := func(status, msg string) {
		db.Exec(`UPDATE vendas_import_jobs SET status=$1, message=$2, atualizado_em=NOW() WHERE id=$3`,
			status, msg, jobID)
	}

	markStatus("processing", "")

	// ── Contador atômico de linhas processadas ────────────────────────────────
	var processed atomic.Int64

	// Goroutine de progresso: atualiza DB a cada 2s usando conexão separada do pool
	progCtx, stopProg := context.WithCancel(context.Background())
	defer stopProg()
	go func() {
		ticker := time.NewTicker(progressUpdate)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				n := processed.Load()
				// Lê total_lines do job para calcular %
				var total int64
				db.QueryRow(`SELECT total_lines FROM vendas_import_jobs WHERE id=$1`, jobID).Scan(&total)
				pct := 0
				if total > 0 && n > 0 {
					pct = int(n * 99 / total) // máx 99% até confirmar o commit
				}
				db.Exec(`UPDATE vendas_import_jobs SET importados=$1, progress=$2, atualizado_em=NOW() WHERE id=$3`,
					n, pct, jobID)
			case <-progCtx.Done():
				return
			}
		}
	}()

	// ── Parse CSV ─────────────────────────────────────────────────────────────
	csvReader := csv.NewReader(bytes.NewReader(rawBytes))
	csvReader.Comma = ';'
	csvReader.LazyQuotes = true
	csvReader.TrimLeadingSpace = true
	csvReader.FieldsPerRecord = -1

	headerRow, err := csvReader.Read()
	if err != nil {
		markStatus("error", "falha ao ler cabeçalho CSV")
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

	iCodGerente  := col(-1, "codgerente", "cod_gerente")
	iNomeGerente := col(-1, "gerente", "nome_gerente")
	iCodSup      := col(-1, "codsupervisor", "cod_supervisor")
	iNomeSup     := col(-1, "supervisor", "nome_supervisor")
	iCodRca      := col(-1, "codusur", "cod_rca", "codrca")
	iNomeRca     := col(-1, "rca", "nome_rca")
	iQtcliRca    := col(-1, "qtclirca", "qtcli_rca")
	iCodFornec   := col(-1, "codfornec", "cod_fornec")
	iNomeFornec  := col(-1, "fornecedor", "nome_fornec")
	iCodCli      := col(-1, "codcli", "cod_cli")
	iNomeCli     := col(-1, "cliente", "nome_cli")
	iUf          := col(-1, "uf")
	iEmpresa     := col(-1, "empresa")
	iCodProd     := col(-1, "codprod", "cod_prod")
	iNomeProd    := col(-1, "produto", "nome_prod")
	iQt          := col(-1, "qt", "quantidade")
	iPvenda      := col(-1, "pvenda", "valor", "vl_venda")
	iPeriodo     := col(-1, "periodo")
	iEstado      := col(-1, "estado")

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
		cleaned := strings.Map(func(r rune) rune {
			if r >= '0' && r <= '9' {
				return r
			}
			return -1
		}, strings.TrimSpace(s))
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

	// ── Lê todas as linhas do CSV em memória para poder fazer chunks ──────────
	type vendaRaw struct {
		vals [22]any
	}
	var allRows []vendaRaw

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
		if codFornec == "" && codRca == "" && codCli == "" {
			continue
		}
		periodo    := getField(csvRow, iPeriodo)
		estadoF   := getField(csvRow, iEstado)

		var r vendaRaw
		r.vals[0]  = spCtx.EmpresaID
		r.vals[1]  = tipoBase
		r.vals[2]  = ano
		r.vals[3]  = mes
		r.vals[4]  = detectEstado(periodo, estadoF)
		r.vals[5]  = getField(csvRow, iCodGerente)
		r.vals[6]  = getField(csvRow, iNomeGerente)
		r.vals[7]  = getField(csvRow, iCodSup)
		r.vals[8]  = getField(csvRow, iNomeSup)
		r.vals[9]  = codRca
		r.vals[10] = getField(csvRow, iNomeRca)
		r.vals[11] = parseInt3(getField(csvRow, iQtcliRca))
		r.vals[12] = codFornec
		r.vals[13] = getField(csvRow, iNomeFornec)
		r.vals[14] = codCli
		r.vals[15] = getField(csvRow, iNomeCli)
		r.vals[16] = getField(csvRow, iUf)
		r.vals[17] = getField(csvRow, iEmpresa)
		r.vals[18] = getField(csvRow, iCodProd)
		r.vals[19] = getField(csvRow, iNomeProd)
		r.vals[20] = parseNum(getField(csvRow, iQt))
		r.vals[21] = parseNum(getField(csvRow, iPvenda))
		allRows = append(allRows, r)
	}

	// Libera rawBytes da memória agora que o CSV foi parseado
	rawBytes = nil

	if len(allRows) == 0 {
		markStatus("error", "nenhuma linha válida encontrada no arquivo")
		return
	}

	// ── Transação única: DELETE + COPY em chunks ──────────────────────────────
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		if ctx.Err() != nil {
			markStatus("cancelled", "cancelado pelo usuário")
		} else {
			markStatus("error", "erro ao iniciar transação: "+err.Error())
		}
		return
	}

	_, err = tx.ExecContext(ctx,
		`DELETE FROM vendas_importadas WHERE empresa_id=$1 AND tipo_base=$2 AND ano=$3 AND mes=$4`,
		spCtx.EmpresaID, tipoBase, ano, mes,
	)
	if err != nil {
		tx.Rollback()
		if ctx.Err() != nil {
			markStatus("cancelled", "cancelado pelo usuário")
		} else {
			markStatus("error", "erro ao limpar dados anteriores: "+err.Error())
		}
		return
	}

	// COPY em chunks de copyChunkRows — entre chunks checamos o contexto (cancelamento)
	for start := 0; start < len(allRows); start += copyChunkRows {
		// Verifica cancelamento entre chunks
		if ctx.Err() != nil {
			tx.Rollback()
			markStatus("cancelled", "cancelado pelo usuário")
			log.Printf("[ImportJob:%s] cancelado após %d linhas", jobID, processed.Load())
			return
		}

		end := start + copyChunkRows
		if end > len(allRows) {
			end = len(allRows)
		}
		chunk := allRows[start:end]

		stmt, err := tx.PrepareContext(ctx, pq.CopyIn("vendas_importadas",
			"empresa_id", "tipo_base", "ano", "mes", "estado",
			"cod_gerente", "nome_gerente", "cod_supervisor", "nome_supervisor",
			"cod_rca", "nome_rca", "qtcli_rca",
			"cod_fornec", "nome_fornec", "cod_cli", "nome_cli", "uf", "empresa",
			"cod_prod", "nome_prod", "qt", "pvenda",
		))
		if err != nil {
			tx.Rollback()
			if ctx.Err() != nil {
				markStatus("cancelled", "cancelado pelo usuário")
			} else {
				markStatus("error", "erro ao preparar COPY: "+err.Error())
			}
			return
		}

		for i := range chunk {
			if _, err = stmt.Exec(chunk[i].vals[:]...); err != nil {
				stmt.Close()
				tx.Rollback()
				markStatus("error", "erro ao enfileirar linha: "+err.Error())
				return
			}
			processed.Add(1)
		}

		if _, err = stmt.Exec(); err != nil { // flush buffer COPY
			stmt.Close()
			tx.Rollback()
			markStatus("error", "erro ao finalizar COPY: "+err.Error())
			return
		}
		stmt.Close()
	}

	if err = tx.Commit(); err != nil {
		if ctx.Err() != nil {
			markStatus("cancelled", "cancelado pelo usuário")
		} else {
			markStatus("error", "erro ao confirmar importação: "+err.Error())
		}
		return
	}

	importados := int(processed.Load())
	log.Printf("[ImportJob:%s] concluído: %d linhas via COPY", jobID, importados)

	// Sinaliza "Consolidando..." e faz REFRESH da view materializada.
	// CONCURRENTLY não bloqueia leituras concorrentes (requer unique index, criado na migration 141).
	db.Exec(`UPDATE vendas_import_jobs
		SET progress=91, message='Consolidando dados...', atualizado_em=NOW()
		WHERE id=$1`, jobID)

	if _, err := db.Exec(`REFRESH MATERIALIZED VIEW CONCURRENTLY farol.mv_farol_resumo`); err != nil {
		log.Printf("[ImportJob:%s] WARN: refresh view: %v", jobID, err)
	}

	db.Exec(`UPDATE vendas_import_jobs
		SET status='done', progress=100, importados=$1, message='', atualizado_em=NOW()
		WHERE id=$2`, importados, jobID)

	// Criação de usuários em background — não bloqueia
	go func() {
		criados, err := syncUsuariosFromImport(db, spCtx, tipoBase, ano, mes)
		if err != nil {
			log.Printf("[SyncUsuarios] erro: %v", err)
		} else {
			log.Printf("[SyncUsuarios] %d criados", criados)
		}
	}()
}

// ─── VendasJobHandler — GET/POST /api/v2/vendas/job/{id}[/cancel] ────────────

func VendasJobHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		// Extrai jobID e ação do path: /api/v2/vendas/job/{id}[/cancel]
		path := strings.TrimPrefix(r.URL.Path, "/api/v2/vendas/job/")
		parts := strings.SplitN(path, "/", 2)
		jobID := parts[0]
		action := ""
		if len(parts) > 1 {
			action = parts[1]
		}

		if jobID == "" {
			http.Error(w, `{"error":"job_id obrigatório"}`, http.StatusBadRequest)
			return
		}

		// ── Cancel ───────────────────────────────────────────────────────────
		if action == "cancel" && r.Method == http.MethodPost {
			// Verifica que o job pertence a esta empresa
			var empID string
			err := db.QueryRow(`SELECT empresa_id FROM vendas_import_jobs WHERE id=$1`, jobID).Scan(&empID)
			if err != nil || empID != spCtx.EmpresaID {
				http.Error(w, `{"error":"job não encontrado"}`, http.StatusNotFound)
				return
			}
			if fn, ok := importJobs.Load(jobID); ok {
				fn.(context.CancelFunc)()
			}
			// Garante status = cancelled mesmo se a goroutine já terminou
			db.Exec(`UPDATE vendas_import_jobs
				SET status='cancelled', message='Cancelado pelo usuário', atualizado_em=NOW()
				WHERE id=$1 AND status IN ('pending','processing')`, jobID)
			json.NewEncoder(w).Encode(map[string]bool{"cancelled": true})
			return
		}

		// ── Status ───────────────────────────────────────────────────────────
		if r.Method == http.MethodGet {
			var job struct {
				ID         string `json:"id"`
				TipoBase   string `json:"tipo_base"`
				Ano        int    `json:"ano"`
				Mes        int    `json:"mes"`
				Status     string `json:"status"`
				Progress   int    `json:"progress"`
				TotalLines int    `json:"total_lines"`
				Importados int    `json:"importados"`
				Message    string `json:"message"`
			}
			err := db.QueryRow(`
				SELECT id, tipo_base, ano, mes, status, progress, total_lines, importados, message
				FROM vendas_import_jobs
				WHERE id=$1 AND empresa_id=$2`,
				jobID, spCtx.EmpresaID,
			).Scan(&job.ID, &job.TipoBase, &job.Ano, &job.Mes,
				&job.Status, &job.Progress, &job.TotalLines, &job.Importados, &job.Message)
			if err != nil {
				http.Error(w, `{"error":"job não encontrado"}`, http.StatusNotFound)
				return
			}
			json.NewEncoder(w).Encode(job)
			return
		}

		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// ─── syncUsuariosFromImport ───────────────────────────────────────────────────

func syncUsuariosFromImport(db *sql.DB, spCtx *FarolContext, tipoBase string, ano, mes int) (int, error) {
	type entrada struct {
		tipo string
		cod  string
		nome string
	}

	empresaShort := spCtx.EmpresaID
	if len(empresaShort) > 8 {
		empresaShort = empresaShort[:8]
	}

	var pessoas []entrada
	queries := []struct {
		tipo    string
		colCod  string
		colNome string
	}{
		{"ggv", "cod_gerente", "nome_gerente"},
		{"supervisor", "cod_supervisor", "nome_supervisor"},
		{"rca", "cod_rca", "nome_rca"},
	}
	for _, q := range queries {
		qSQL := fmt.Sprintf(`
			SELECT DISTINCT %s, %s FROM vendas_importadas
			WHERE empresa_id=$1 AND tipo_base=$2 AND ano=$3 AND mes=$4
			  AND %s != '' AND %s != ''`,
			q.colCod, q.colNome, q.colCod, q.colNome)
		rows, err := db.Query(qSQL, spCtx.EmpresaID, tipoBase, ano, mes)
		if err != nil {
			continue
		}
		for rows.Next() {
			var cod, nome string
			if scanErr := rows.Scan(&cod, &nome); scanErr == nil && cod != "" {
				pessoas = append(pessoas, entrada{q.tipo, cod, nome})
			}
		}
		rows.Close()
	}
	if len(pessoas) == 0 {
		return 0, nil
	}

	var envID sql.NullString
	_ = db.QueryRow(`SELECT environment_id FROM user_environments WHERE user_id=$1 LIMIT 1`,
		spCtx.UserID).Scan(&envID)

	trialEndsAt := time.Date(2099, 12, 31, 0, 0, 0, 0, time.UTC)
	criados := 0

	for _, p := range pessoas {
		codSafe := strings.ToLower(strings.Map(func(r rune) rune {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
				return r
			}
			return '-'
		}, p.cod))
		email := fmt.Sprintf("%s.%s@%s.farol.local", p.tipo, codSafe, empresaShort)

		// bcrypt cost 10 (vs auth.go cost 14) — auto-generated accounts only.
		// Runs sequentially for 200+ RCAs; cost 14 ≈ 2s each → 400s total.
		hashBytes, herr := bcrypt.GenerateFromPassword([]byte("Farol@"+p.cod), 10)
		if herr != nil {
			continue
		}
		hash := string(hashBytes)

		var userID string
		err := db.QueryRow(`
			INSERT INTO users (email, password_hash, full_name, trial_ends_at, is_verified, role, sp_role, tipo_persona, cod_referencia)
			VALUES ($1, $2, $3, $4, TRUE, 'user', 'somente_leitura', $5, $6)
			ON CONFLICT (email) DO NOTHING
			RETURNING id`,
			email, hash, p.nome, trialEndsAt, p.tipo, p.cod,
		).Scan(&userID)
		if err == sql.ErrNoRows {
			continue // email já existia — OK, ignora
		}
		if err != nil {
			log.Printf("[SyncUsuarios] falha ao criar %s (%s): %v", email, p.nome, err)
			continue
		}

		if envID.Valid && envID.String != "" {
			_, _ = db.Exec(`
				INSERT INTO user_environments (user_id, environment_id, role, preferred_company_id)
				VALUES ($1, $2, 'user', $3) ON CONFLICT DO NOTHING`,
				userID, envID.String, spCtx.EmpresaID)
		}
		_, _ = db.Exec(`
			INSERT INTO farol.sp_user_filiais (user_id, empresa_id, filial_id, all_filiais)
			VALUES ($1, $2, NULL, TRUE) ON CONFLICT DO NOTHING`,
			userID, spCtx.EmpresaID)

		criados++
		log.Printf("[SyncUsuarios] criado: %s %s (%s)", p.tipo, p.nome, email)
	}

	return criados, nil
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
	CodFornec            string  `json:"cod_fornec"`
	NomeFornec           string  `json:"nome_fornec"`
	ProgramaDistribuicao string  `json:"programa_distribuicao"`
	TravaMinima          float64 `json:"trava_minima_qt"`
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
				FROM industrias_config WHERE empresa_id=$1 ORDER BY nome_fornec
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
