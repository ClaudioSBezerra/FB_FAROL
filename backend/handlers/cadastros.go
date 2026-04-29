package handlers

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ─── UF validation ────────────────────────────────────────────────────────────

var validUFs = map[string]bool{
	"AC": true, "AL": true, "AP": true, "AM": true, "BA": true,
	"CE": true, "DF": true, "ES": true, "GO": true, "MA": true,
	"MT": true, "MS": true, "MG": true, "PA": true, "PB": true,
	"PR": true, "PE": true, "PI": true, "RJ": true, "RN": true,
	"RS": true, "RO": true, "RR": true, "SC": true, "SP": true,
	"SE": true, "TO": true,
}

// ─── Extraction helpers ───────────────────────────────────────────────────────

func findFirstSep(s string) (int, int) {
	if i := strings.Index(s, " - "); i != -1 {
		return i, 3
	}
	if i := strings.Index(s, "- "); i != -1 {
		return i, 2
	}
	return -1, 0
}

func findLastSep(s string) (int, int) {
	if i := strings.LastIndex(s, " - "); i != -1 {
		return i, 3
	}
	if i := strings.LastIndex(s, "- "); i != -1 {
		return i, 2
	}
	return -1, 0
}

// extractGestorInfo extrai uf (2 chars) e regiao do campo SUPERVISOR do CSV.
// Exemplos:
//
//	"GO - CALDAS NOVAS - LILIAM"    → uf=GO, regiao=CALDAS NOVAS
//	"TO - EDILSON GIL DA ROCHA"     → uf=TO, regiao=(vazio)
//	"MARANHÃO- DIVAIR PIRES"        → uf=MA, regiao=(vazio)
//	"V7 - GO NORTE/METROP - WALISTON" → uf=GO, regiao=GO NORTE/METROP
func extractGestorInfo(supervisor string) (uf, regiao string) {
	s := strings.TrimSpace(strings.ToUpper(supervisor))

	if strings.HasPrefix(s, "MARANHÃO") || strings.HasPrefix(s, "MARANHAO") {
		return "MA", ""
	}

	isV7 := strings.HasPrefix(s, "V7 - ")
	work := s
	if isV7 {
		work = s[5:]
	}

	sepIdx, sepLen := findFirstSep(work)
	if sepIdx == -1 {
		return "", ""
	}

	token := strings.TrimSpace(work[:sepIdx])
	afterFirst := strings.TrimSpace(work[sepIdx+sepLen:])

	if isV7 {
		if len(token) >= 2 && validUFs[token[:2]] {
			return token[:2], token
		}
		return "", ""
	}

	if len(token) != 2 || !validUFs[token] {
		return "", ""
	}
	uf = token

	lastSepIdx, _ := findLastSep(afterFirst)
	if lastSepIdx <= 0 {
		return uf, ""
	}
	regiao = strings.TrimSpace(afterFirst[:lastSepIdx])
	return uf, regiao
}

func extractRCATipo(nome string) string {
	upper := strings.ToUpper(nome)
	switch {
	case strings.Contains(upper, "(CRV)"):
		return "CRV"
	case strings.Contains(upper, "(GGV)"):
		return "GGV"
	case strings.Contains(upper, "(JUR)"):
		return "JUR"
	case strings.HasPrefix(upper, "TELEV-"), strings.HasPrefix(upper, "TELEV "):
		return "TELEVENDAS"
	default:
		return "RCA"
	}
}

func extractRCAAtivo(nome string) bool {
	return !strings.Contains(strings.ToUpper(nome), "-SAIU")
}

func nullableString(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

// ─── Structs ─────────────────────────────────────────────────────────────────

type GestorRow struct {
	CodSupervisor int        `json:"cod_supervisor"`
	Nome          string     `json:"nome"`
	UF            *string    `json:"uf"`
	Regiao        *string    `json:"regiao"`
	Atuacao       *string    `json:"atuacao"`
	Ativo         bool       `json:"ativo"`
	QtdRCAs       int        `json:"qtd_rcas"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type RCARow struct {
	CodRCA        int       `json:"cod_rca"`
	Nome          string    `json:"nome"`
	CodFilial     *string   `json:"cod_filial"`
	Tipo          string    `json:"tipo"`
	Ativo         bool      `json:"ativo"`
	CodSupervisor *int      `json:"cod_supervisor"`
	GestorNome    *string   `json:"gestor_nome"`
	UF            *string   `json:"uf"`
	Regiao        *string   `json:"regiao"`
	Atuacao       *string   `json:"atuacao"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// ─── Gestores ─────────────────────────────────────────────────────────────────

// CadastrosGestoresHandler handles GET/POST /api/cadastros/gestores and PUT/DELETE /api/cadastros/gestores/{id}
func CadastrosGestoresHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		idStr := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/cadastros/gestores"), "/")

		if idStr == "" {
			switch r.Method {
			case http.MethodGet:
				listGestores(w, r, db)
			case http.MethodPost:
				createGestor(w, r, db)
			default:
				http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			}
			return
		}

		id, err := strconv.Atoi(idStr)
		if err != nil {
			http.Error(w, `{"error":"invalid id"}`, http.StatusBadRequest)
			return
		}
		switch r.Method {
		case http.MethodPut:
			updateGestor(w, r, db, id)
		case http.MethodDelete:
			deleteGestor(w, r, db, id)
		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	}
}

func listGestores(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	uf := strings.TrimSpace(r.URL.Query().Get("uf"))
	ativo := r.URL.Query().Get("ativo")

	query := `
		SELECT g.cod_supervisor, g.nome, g.uf, g.regiao, g.atuacao, g.ativo,
		       COUNT(gr.cod_rca) AS qtd_rcas, g.created_at, g.updated_at
		FROM gestores g
		LEFT JOIN gestor_rca gr ON gr.cod_supervisor = g.cod_supervisor
		WHERE 1=1`
	args := []interface{}{}
	idx := 1

	if q != "" {
		query += ` AND (g.nome ILIKE $` + strconv.Itoa(idx) + ` OR CAST(g.cod_supervisor AS TEXT) LIKE $` + strconv.Itoa(idx) + `)`
		args = append(args, "%"+q+"%")
		idx++
	}
	if uf != "" {
		query += ` AND g.uf = $` + strconv.Itoa(idx)
		args = append(args, strings.ToUpper(uf))
		idx++
	}
	if ativo == "true" {
		query += ` AND g.ativo = TRUE`
	} else if ativo == "false" {
		query += ` AND g.ativo = FALSE`
	}

	query += ` GROUP BY g.cod_supervisor ORDER BY g.nome ASC`

	rows, err := db.Query(query, args...)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	result := make([]GestorRow, 0)
	for rows.Next() {
		var g GestorRow
		if err := rows.Scan(&g.CodSupervisor, &g.Nome, &g.UF, &g.Regiao, &g.Atuacao,
			&g.Ativo, &g.QtdRCAs, &g.CreatedAt, &g.UpdatedAt); err != nil {
			continue
		}
		result = append(result, g)
	}
	json.NewEncoder(w).Encode(result)
}

func createGestor(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	var req struct {
		CodSupervisor int     `json:"cod_supervisor"`
		Nome          string  `json:"nome"`
		UF            *string `json:"uf"`
		Regiao        *string `json:"regiao"`
		Ativo         bool    `json:"ativo"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
		return
	}

	var g GestorRow
	err := db.QueryRow(`
		INSERT INTO gestores (cod_supervisor, nome, uf, regiao, ativo)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING cod_supervisor, nome, uf, regiao, atuacao, ativo, created_at, updated_at`,
		req.CodSupervisor, req.Nome, req.UF, req.Regiao, req.Ativo,
	).Scan(&g.CodSupervisor, &g.Nome, &g.UF, &g.Regiao, &g.Atuacao, &g.Ativo, &g.CreatedAt, &g.UpdatedAt)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(g)
}

func updateGestor(w http.ResponseWriter, r *http.Request, db *sql.DB, id int) {
	var req struct {
		Nome   string  `json:"nome"`
		UF     *string `json:"uf"`
		Regiao *string `json:"regiao"`
		Ativo  bool    `json:"ativo"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
		return
	}

	var g GestorRow
	err := db.QueryRow(`
		UPDATE gestores SET nome=$1, uf=$2, regiao=$3, ativo=$4, updated_at=NOW()
		WHERE cod_supervisor=$5
		RETURNING cod_supervisor, nome, uf, regiao, atuacao, ativo, created_at, updated_at`,
		req.Nome, req.UF, req.Regiao, req.Ativo, id,
	).Scan(&g.CodSupervisor, &g.Nome, &g.UF, &g.Regiao, &g.Atuacao, &g.Ativo, &g.CreatedAt, &g.UpdatedAt)
	if err == sql.ErrNoRows {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(g)
}

func deleteGestor(w http.ResponseWriter, r *http.Request, db *sql.DB, id int) {
	var count int
	db.QueryRow(`SELECT COUNT(*) FROM gestor_rca WHERE cod_supervisor=$1`, id).Scan(&count)
	if count > 0 {
		http.Error(w, `{"error":"gestor possui RCAs vinculados — remova os vínculos antes de excluir"}`, http.StatusConflict)
		return
	}
	res, err := db.Exec(`DELETE FROM gestores WHERE cod_supervisor=$1`, id)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ─── RCAs ─────────────────────────────────────────────────────────────────────

// CadastrosRCAsHandler handles GET/POST /api/cadastros/rcas and PUT/DELETE /api/cadastros/rcas/{id}
func CadastrosRCAsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		idStr := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/cadastros/rcas"), "/")

		if idStr == "" {
			switch r.Method {
			case http.MethodGet:
				listRCAs(w, r, db)
			case http.MethodPost:
				createRCA(w, r, db)
			default:
				http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			}
			return
		}

		id, err := strconv.Atoi(idStr)
		if err != nil {
			http.Error(w, `{"error":"invalid id"}`, http.StatusBadRequest)
			return
		}
		switch r.Method {
		case http.MethodPut:
			updateRCA(w, r, db, id)
		case http.MethodDelete:
			deleteRCA(w, r, db, id)
		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	}
}

func listRCAs(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	uf := strings.TrimSpace(r.URL.Query().Get("uf"))
	supStr := strings.TrimSpace(r.URL.Query().Get("cod_supervisor"))
	ativo := r.URL.Query().Get("ativo")

	query := `
		SELECT r.cod_rca, r.nome, r.cod_filial, r.tipo, r.ativo,
		       g.cod_supervisor, g.nome, g.uf, g.regiao, g.atuacao,
		       r.created_at, r.updated_at
		FROM rcas r
		LEFT JOIN gestor_rca gr ON gr.cod_rca = r.cod_rca
		LEFT JOIN gestores g ON g.cod_supervisor = gr.cod_supervisor
		WHERE 1=1`
	args := []interface{}{}
	idx := 1

	if q != "" {
		query += ` AND (r.nome ILIKE $` + strconv.Itoa(idx) + ` OR CAST(r.cod_rca AS TEXT) LIKE $` + strconv.Itoa(idx) + `)`
		args = append(args, "%"+q+"%")
		idx++
	}
	if uf != "" {
		query += ` AND g.uf = $` + strconv.Itoa(idx)
		args = append(args, strings.ToUpper(uf))
		idx++
	}
	if supStr != "" {
		if supID, err := strconv.Atoi(supStr); err == nil {
			query += ` AND g.cod_supervisor = $` + strconv.Itoa(idx)
			args = append(args, supID)
			idx++
		}
	}
	if ativo == "true" {
		query += ` AND r.ativo = TRUE`
	} else if ativo == "false" {
		query += ` AND r.ativo = FALSE`
	}

	query += ` ORDER BY r.nome ASC`

	rows, err := db.Query(query, args...)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	result := make([]RCARow, 0)
	for rows.Next() {
		var rc RCARow
		if err := rows.Scan(
			&rc.CodRCA, &rc.Nome, &rc.CodFilial, &rc.Tipo, &rc.Ativo,
			&rc.CodSupervisor, &rc.GestorNome, &rc.UF, &rc.Regiao, &rc.Atuacao,
			&rc.CreatedAt, &rc.UpdatedAt,
		); err != nil {
			continue
		}
		result = append(result, rc)
	}
	json.NewEncoder(w).Encode(result)
}

func createRCA(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	var req struct {
		CodRCA        int     `json:"cod_rca"`
		Nome          string  `json:"nome"`
		CodFilial     *string `json:"cod_filial"`
		Tipo          string  `json:"tipo"`
		Ativo         bool    `json:"ativo"`
		CodSupervisor *int    `json:"cod_supervisor"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
		return
	}
	if req.Tipo == "" {
		req.Tipo = "RCA"
	}

	tx, err := db.Begin()
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	_, err = tx.Exec(`INSERT INTO rcas (cod_rca, nome, cod_filial, tipo, ativo) VALUES ($1,$2,$3,$4,$5)`,
		req.CodRCA, req.Nome, req.CodFilial, req.Tipo, req.Ativo)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	if req.CodSupervisor != nil {
		tx.Exec(`INSERT INTO gestor_rca (cod_supervisor, cod_rca) VALUES ($1,$2) ON CONFLICT DO NOTHING`,
			*req.CodSupervisor, req.CodRCA)
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{"cod_rca": req.CodRCA})
}

func updateRCA(w http.ResponseWriter, r *http.Request, db *sql.DB, id int) {
	var req struct {
		Nome          string  `json:"nome"`
		CodFilial     *string `json:"cod_filial"`
		Tipo          string  `json:"tipo"`
		Ativo         bool    `json:"ativo"`
		CodSupervisor *int    `json:"cod_supervisor"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
		return
	}

	tx, _ := db.Begin()
	defer tx.Rollback()

	res, err := tx.Exec(`UPDATE rcas SET nome=$1, cod_filial=$2, tipo=$3, ativo=$4, updated_at=NOW() WHERE cod_rca=$5`,
		req.Nome, req.CodFilial, req.Tipo, req.Ativo, id)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	tx.Exec(`DELETE FROM gestor_rca WHERE cod_rca=$1`, id)
	if req.CodSupervisor != nil {
		tx.Exec(`INSERT INTO gestor_rca (cod_supervisor, cod_rca) VALUES ($1,$2) ON CONFLICT DO NOTHING`,
			*req.CodSupervisor, id)
	}
	tx.Commit()

	w.WriteHeader(http.StatusNoContent)
}

func deleteRCA(w http.ResponseWriter, r *http.Request, db *sql.DB, id int) {
	res, err := db.Exec(`DELETE FROM rcas WHERE cod_rca=$1`, id)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ─── Upload CSV ───────────────────────────────────────────────────────────────

// UploadCadastrosCSVHandler faz parse do CSV RCAS_ATIVOS (separador ;) e upsert
// em gestores, rcas e gestor_rca. POST /api/cadastros/rcas/upload-csv
func UploadCadastrosCSVHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		if err := r.ParseMultipartForm(10 << 20); err != nil {
			// fallback: try plain body
		}

		var reader io.Reader
		file, _, err := r.FormFile("file")
		if err == nil {
			defer file.Close()
			reader = file
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

		// Verifica se as tabelas existem antes de iniciar o upload
		var tablesOK bool
		db.QueryRow(`SELECT EXISTS(
			SELECT 1 FROM information_schema.tables
			WHERE table_schema='public' AND table_name='gestores'
		)`).Scan(&tablesOK)
		if !tablesOK {
			http.Error(w, `{"error":"tabelas de cadastro não encontradas — aguarde a migração do banco ou contate o suporte"}`, http.StatusInternalServerError)
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
			record, err := csvReader.Read()
			if err == io.EOF {
				break
			}
			if err != nil || len(record) < 4 {
				skipped++
				continue
			}

			codSupStr := strings.TrimSpace(record[0])
			nomeSupv := strings.TrimSpace(record[1])
			codRCAStr := strings.TrimSpace(record[2])
			nomeRCA := strings.TrimSpace(record[3])
			codFilial := ""
			if len(record) >= 5 {
				codFilial = strings.TrimSpace(record[4])
			}

			codSup, errSup := strconv.Atoi(codSupStr)
			codRCA, errRCA := strconv.Atoi(codRCAStr)
			if errSup != nil || errRCA != nil {
				skipped++
				continue
			}

			uf, regiao := extractGestorInfo(nomeSupv)
			tipo := extractRCATipo(nomeRCA)
			ativo := extractRCAAtivo(nomeRCA)

			var ufPtr, regiaoPtr, filialPtr *string
			if uf != "" {
				ufPtr = &uf
			}
			if regiao != "" {
				regiaoPtr = &regiao
			}
			if codFilial != "" {
				filialPtr = &codFilial
			}

			// Savepoint por linha — impede que erros individuais abortem toda a transação
			if _, err = tx.Exec("SAVEPOINT sp_row"); err != nil {
				skipped++
				continue
			}

			rowOk := true

			// Upsert gestor
			var wasNew bool
			err = tx.QueryRow(`
				INSERT INTO gestores (cod_supervisor, nome, uf, regiao, ativo)
				VALUES ($1, $2, $3, $4, TRUE)
				ON CONFLICT (cod_supervisor) DO UPDATE SET
				  nome = EXCLUDED.nome, uf = EXCLUDED.uf, regiao = EXCLUDED.regiao,
				  updated_at = NOW()
				RETURNING (xmax = 0)`, codSup, nomeSupv, ufPtr, regiaoPtr,
			).Scan(&wasNew)
			if err != nil {
				tx.Exec("ROLLBACK TO SAVEPOINT sp_row") //nolint
				skipped++
				rowOk = false
			}

			if rowOk {
				// Upsert RCA
				err = tx.QueryRow(`
					INSERT INTO rcas (cod_rca, nome, cod_filial, tipo, ativo)
					VALUES ($1, $2, $3, $4, $5)
					ON CONFLICT (cod_rca) DO UPDATE SET
					  nome = EXCLUDED.nome, cod_filial = EXCLUDED.cod_filial,
					  tipo = EXCLUDED.tipo, ativo = EXCLUDED.ativo, updated_at = NOW()
					RETURNING (xmax = 0)`, codRCA, nomeRCA, filialPtr, tipo, ativo,
				).Scan(&wasNew)
				if err != nil {
					tx.Exec("ROLLBACK TO SAVEPOINT sp_row") //nolint
					skipped++
					rowOk = false
				}
			}

			if rowOk {
				// Upsert vínculo
				tx.Exec(`INSERT INTO gestor_rca (cod_supervisor, cod_rca) VALUES ($1, $2) ON CONFLICT DO NOTHING`, codSup, codRCA) //nolint
				tx.Exec("RELEASE SAVEPOINT sp_row")                                                                                //nolint
				if wasNew {
					imported++
				} else {
					updated++
				}
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
