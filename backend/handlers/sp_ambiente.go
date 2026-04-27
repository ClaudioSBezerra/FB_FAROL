package handlers

// sp_ambiente.go — CRUD de Filiais, CDs, Parâmetros do Motor e Planos
//
// Story 3.2 — CRUD de Filiais e CDs
// Story 3.4 — CRUD de Parâmetros do Motor e Duplicação de CD
// Story 3.5 — Gestão de Planos e Limites
//
// Rotas:
//   GET/POST         /api/sp/filiais
//   GET/PUT/DELETE   /api/sp/filiais/{id}
//   GET/POST         /api/sp/filiais/{id}/cds
//   GET/PUT/DELETE   /api/sp/cds/{id}
//   POST             /api/sp/cds/{id}/duplicar
//   GET/PUT          /api/sp/cds/{id}/params
//   GET/PUT          /api/sp/plano

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
)

// ─── DTOs ─────────────────────────────────────────────────────────────────────

type SpFilialRequest struct {
	CodFilial int    `json:"cod_filial"`
	Nome      string `json:"nome"`
	Ativo     *bool  `json:"ativo"`
}

type SpFilialResponse struct {
	ID        int    `json:"id"`
	CodFilial int    `json:"cod_filial"`
	Nome      string `json:"nome"`
	Ativo     bool   `json:"ativo"`
	NumCDs    int    `json:"num_cds"`
	CreatedAt string `json:"created_at"`
}

type SpCDRequest struct {
	Nome      string `json:"nome"`
	Descricao string `json:"descricao"`
	Ativo     *bool  `json:"ativo"`
}

type SpCDResponse struct {
	ID         int     `json:"id"`
	FilialID   int     `json:"filial_id"`
	Nome       string  `json:"nome"`
	Descricao  string  `json:"descricao"`
	Ativo      bool    `json:"ativo"`
	FonteCDID  *int    `json:"fonte_cd_id,omitempty"`
	CreatedAt  string  `json:"created_at"`
}

type SpMotorParamsRequest struct {
	DiasAnalise        int     `json:"dias_analise"`
	CurvaAMaxEst       int     `json:"curva_a_max_est"`
	CurvaBMaxEst       int     `json:"curva_b_max_est"`
	CurvaCMaxEst       int     `json:"curva_c_max_est"`
	FatorSeguranca     float64 `json:"fator_seguranca"`
	CurvaANuncaReduz   bool    `json:"curva_a_nunca_reduz"`
	MinCapacidade      int     `json:"min_capacidade"`
	RetencaoCsvMeses   int     `json:"retencao_csv_meses"`
}

type SpMotorParamsResponse struct {
	ID                 int     `json:"id"`
	CDID               int     `json:"cd_id"`
	DiasAnalise        int     `json:"dias_analise"`
	CurvaAMaxEst       int     `json:"curva_a_max_est"`
	CurvaBMaxEst       int     `json:"curva_b_max_est"`
	CurvaCMaxEst       int     `json:"curva_c_max_est"`
	FatorSeguranca     float64 `json:"fator_seguranca"`
	CurvaANuncaReduz   bool    `json:"curva_a_nunca_reduz"`
	MinCapacidade      int     `json:"min_capacidade"`
	RetencaoCsvMeses   int     `json:"retencao_csv_meses"`
	UpdatedAt          string  `json:"updated_at"`
}

type SpPlanoResponse struct {
	Plano        string  `json:"plano"`
	MaxFiliais   int     `json:"max_filiais"`
	MaxCDs       int     `json:"max_cds"`
	MaxUsuarios  int     `json:"max_usuarios"`
	Ativo        bool    `json:"ativo"`
	ValidoAte    *string `json:"valido_ate"`
	// Uso atual
	UsadoFiliais int `json:"usado_filiais"`
	UsadoCDs     int `json:"usado_cds"`
	UsadoUsuarios int `json:"usado_usuarios"`
}

type SpAtualizarPlanoRequest struct {
	Plano       string  `json:"plano"`
	MaxFiliais  int     `json:"max_filiais"`
	MaxCDs      int     `json:"max_cds"`
	MaxUsuarios int     `json:"max_usuarios"`
	ValidoAte   *string `json:"valido_ate"`
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func pathSegment(path, prefix string) string {
	s := strings.TrimPrefix(path, prefix)
	parts := strings.Split(strings.Trim(s, "/"), "/")
	if len(parts) > 0 {
		return parts[0]
	}
	return ""
}

func checkLimiteFiliais(db *sql.DB, empresaID string) error {
	var max, usado int
	db.QueryRow(`SELECT max_filiais FROM farol.sp_subscription_limits WHERE empresa_id = $1`, empresaID).Scan(&max)
	db.QueryRow(`SELECT COUNT(*) FROM farol.sp_filiais WHERE empresa_id = $1 AND ativo = TRUE`, empresaID).Scan(&usado)
	if max != -1 && usado >= max {
		return &limitExceededError{"filiais", max}
	}
	return nil
}

func checkLimiteCDs(db *sql.DB, empresaID string) error {
	var max, usado int
	db.QueryRow(`SELECT max_cds FROM farol.sp_subscription_limits WHERE empresa_id = $1`, empresaID).Scan(&max)
	db.QueryRow(`
		SELECT COUNT(*) FROM farol.sp_centros_dist cd
		JOIN farol.sp_filiais f ON f.id = cd.filial_id
		WHERE f.empresa_id = $1 AND cd.ativo = TRUE
	`, empresaID).Scan(&usado)
	if max != -1 && usado >= max {
		return &limitExceededError{"CDs", max}
	}
	return nil
}

type limitExceededError struct {
	resource string
	max      int
}

func (e *limitExceededError) Error() string {
	return "limite do plano atingido: " + e.resource + " (" + strconv.Itoa(e.max) + " máximo)"
}

// ─── Filiais ──────────────────────────────────────────────────────────────────

// SpFiliaisHandler — GET/POST /api/sp/filiais
func SpFiliaisHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		switch r.Method {
		case http.MethodGet:
			rows, err := db.Query(`
				SELECT f.id, f.cod_filial, f.nome, f.ativo, f.created_at,
				       COUNT(cd.id) AS num_cds
				FROM farol.sp_filiais f
				LEFT JOIN farol.sp_centros_dist cd ON cd.filial_id = f.id AND cd.ativo = TRUE
				WHERE f.empresa_id = $1
				GROUP BY f.id
				ORDER BY f.nome ASC
			`, spCtx.EmpresaID)
			if err != nil {
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
			defer rows.Close()

			var filiais []SpFilialResponse
			for rows.Next() {
				var f SpFilialResponse
				if err := rows.Scan(&f.ID, &f.CodFilial, &f.Nome, &f.Ativo, &f.CreatedAt, &f.NumCDs); err != nil {
					continue
				}
				filiais = append(filiais, f)
			}
			if filiais == nil {
				filiais = []SpFilialResponse{}
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(filiais)

		case http.MethodPost:
			if !RequireWrite(spCtx, w) {
				return
			}
			if err := checkLimiteFiliais(db, spCtx.EmpresaID); err != nil {
				http.Error(w, err.Error(), http.StatusUnprocessableEntity)
				return
			}
			var req SpFilialRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Nome == "" || req.CodFilial == 0 {
				http.Error(w, "cod_filial e nome são obrigatórios", http.StatusBadRequest)
				return
			}
			var id int
			err := db.QueryRow(`
				INSERT INTO farol.sp_filiais (empresa_id, cod_filial, nome)
				VALUES ($1, $2, $3)
				RETURNING id
			`, spCtx.EmpresaID, req.CodFilial, req.Nome).Scan(&id)
			if err != nil {
				if strings.Contains(err.Error(), "unique") {
					http.Error(w, "cod_filial já cadastrado para esta empresa", http.StatusConflict)
					return
				}
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
			log.Printf("SpFiliais: criada filial %d (cod=%d) empresa %s por %s", id, req.CodFilial, spCtx.EmpresaID, spCtx.UserID)
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]int{"id": id})

		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

// SpFilialItemHandler — GET/PUT/DELETE /api/sp/filiais/{id}
func SpFilialItemHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		idStr := pathSegment(r.URL.Path, "/api/sp/filiais/")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			http.Error(w, "ID inválido", http.StatusBadRequest)
			return
		}

		switch r.Method {
		case http.MethodPut:
			if !RequireWrite(spCtx, w) {
				return
			}
			var req SpFilialRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "Invalid body", http.StatusBadRequest)
				return
			}
			ativo := true
			if req.Ativo != nil {
				ativo = *req.Ativo
			}
			res, err := db.Exec(`
				UPDATE farol.sp_filiais
				SET nome = COALESCE(NULLIF($1,''), nome), ativo = $2, updated_at = now()
				WHERE id = $3 AND empresa_id = $4
			`, req.Nome, ativo, id, spCtx.EmpresaID)
			if err != nil {
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
			if n, _ := res.RowsAffected(); n == 0 {
				http.Error(w, "Filial não encontrada", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"message": "Filial atualizada"})

		case http.MethodDelete:
			if !spCtx.CanApprove() {
				http.Error(w, "Forbidden: gestor_geral+ necessário para remover filiais", http.StatusForbidden)
				return
			}
			// Soft delete: marca ativo = false
			res, err := db.Exec(`
				UPDATE farol.sp_filiais SET ativo = FALSE, updated_at = now()
				WHERE id = $1 AND empresa_id = $2
			`, id, spCtx.EmpresaID)
			if err != nil {
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
			if n, _ := res.RowsAffected(); n == 0 {
				http.Error(w, "Filial não encontrada", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"message": "Filial desativada"})

		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

// ─── CDs ──────────────────────────────────────────────────────────────────────

// SpCDsHandler — GET/POST /api/sp/filiais/{id}/cds
func SpCDsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Path: /api/sp/filiais/{filialID}/cds
		parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/sp/filiais/"), "/"), "/")
		filialIDStr := parts[0]
		filialID, err := strconv.Atoi(filialIDStr)
		if err != nil {
			http.Error(w, "filial ID inválido", http.StatusBadRequest)
			return
		}

		// Verifica que a filial pertence à empresa
		var existe bool
		db.QueryRow(`SELECT EXISTS(SELECT 1 FROM farol.sp_filiais WHERE id = $1 AND empresa_id = $2)`,
			filialID, spCtx.EmpresaID).Scan(&existe)
		if !existe {
			http.Error(w, "Filial não encontrada", http.StatusNotFound)
			return
		}

		switch r.Method {
		case http.MethodGet:
			rows, err := db.Query(`
				SELECT id, filial_id, nome, COALESCE(descricao,''), ativo, fonte_cd_id, created_at
				FROM farol.sp_centros_dist
				WHERE filial_id = $1
				ORDER BY nome ASC
			`, filialID)
			if err != nil {
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
			defer rows.Close()

			var cds []SpCDResponse
			for rows.Next() {
				var cd SpCDResponse
				if err := rows.Scan(&cd.ID, &cd.FilialID, &cd.Nome, &cd.Descricao, &cd.Ativo, &cd.FonteCDID, &cd.CreatedAt); err != nil {
					continue
				}
				cds = append(cds, cd)
			}
			if cds == nil {
				cds = []SpCDResponse{}
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(cds)

		case http.MethodPost:
			if !RequireWrite(spCtx, w) {
				return
			}
			if err := checkLimiteCDs(db, spCtx.EmpresaID); err != nil {
				http.Error(w, err.Error(), http.StatusUnprocessableEntity)
				return
			}
			var req SpCDRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Nome == "" {
				http.Error(w, "nome é obrigatório", http.StatusBadRequest)
				return
			}
			var cdID int
			err := db.QueryRow(`
				INSERT INTO farol.sp_centros_dist (filial_id, empresa_id, nome, descricao, criado_por)
				VALUES ($1, $2, $3, $4, $5)
				RETURNING id
			`, filialID, spCtx.EmpresaID, req.Nome, req.Descricao, spCtx.UserID).Scan(&cdID)
			if err != nil {
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
			// Cria parâmetros padrão para o novo CD
			db.Exec(`
				INSERT INTO farol.sp_motor_params (cd_id, empresa_id)
				VALUES ($1, $2) ON CONFLICT (cd_id) DO NOTHING
			`, cdID, spCtx.EmpresaID)

			log.Printf("SpCDs: criado CD %d filial %d empresa %s por %s", cdID, filialID, spCtx.EmpresaID, spCtx.UserID)
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]int{"id": cdID})

		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

// SpCDItemHandler — GET/PUT/DELETE /api/sp/cds/{id}
func SpCDItemHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		idStr := pathSegment(r.URL.Path, "/api/sp/cds/")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			http.Error(w, "ID inválido", http.StatusBadRequest)
			return
		}

		switch r.Method {
		case http.MethodGet:
			var cd SpCDResponse
			err := db.QueryRow(`
				SELECT cd.id, cd.filial_id, cd.nome, COALESCE(cd.descricao,''), cd.ativo, cd.fonte_cd_id, cd.created_at
				FROM farol.sp_centros_dist cd
				JOIN farol.sp_filiais f ON f.id = cd.filial_id
				WHERE cd.id = $1 AND f.empresa_id = $2
			`, id, spCtx.EmpresaID).Scan(&cd.ID, &cd.FilialID, &cd.Nome, &cd.Descricao, &cd.Ativo, &cd.FonteCDID, &cd.CreatedAt)
			if err == sql.ErrNoRows {
				http.Error(w, "CD não encontrado", http.StatusNotFound)
				return
			} else if err != nil {
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(cd)

		case http.MethodPut:
			if !RequireWrite(spCtx, w) {
				return
			}
			var req SpCDRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "Invalid body", http.StatusBadRequest)
				return
			}
			ativo := true
			if req.Ativo != nil {
				ativo = *req.Ativo
			}
			res, err := db.Exec(`
				UPDATE farol.sp_centros_dist cd
				SET nome = COALESCE(NULLIF($1,''), nome),
				    descricao = $2,
				    ativo = $3,
				    updated_at = now()
				FROM farol.sp_filiais f
				WHERE cd.id = $4 AND cd.filial_id = f.id AND f.empresa_id = $5
			`, req.Nome, req.Descricao, ativo, id, spCtx.EmpresaID)
			if err != nil {
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
			if n, _ := res.RowsAffected(); n == 0 {
				http.Error(w, "CD não encontrado", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"message": "CD atualizado"})

		case http.MethodDelete:
			if !spCtx.CanApprove() {
				http.Error(w, "Forbidden: gestor_geral+ para remover CDs", http.StatusForbidden)
				return
			}
			res, err := db.Exec(`
				UPDATE farol.sp_centros_dist cd
				SET ativo = FALSE, updated_at = now()
				FROM farol.sp_filiais f
				WHERE cd.id = $1 AND cd.filial_id = f.id AND f.empresa_id = $2
			`, id, spCtx.EmpresaID)
			if err != nil {
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
			if n, _ := res.RowsAffected(); n == 0 {
				http.Error(w, "CD não encontrado", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"message": "CD desativado"})

		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

// SpDuplicarCDHandler — POST /api/sp/cds/{id}/duplicar
// Copia o CD (nome + parâmetros) criando um novo CD na mesma filial.
func SpDuplicarCDHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		spCtx := GetSpContext(r)
		if spCtx == nil || !RequireWrite(spCtx, w) {
			return
		}
		if err := checkLimiteCDs(db, spCtx.EmpresaID); err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}

		idStr := pathSegment(r.URL.Path, "/api/sp/cds/")
		fonteID, err := strconv.Atoi(idStr)
		if err != nil {
			http.Error(w, "ID inválido", http.StatusBadRequest)
			return
		}

		var req struct {
			Nome string `json:"nome"`
		}
		json.NewDecoder(r.Body).Decode(&req)

		tx, err := db.Begin()
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()

		// Busca CD fonte
		var filialID int
		var nomeBase string
		err = tx.QueryRow(`
			SELECT cd.filial_id, cd.nome
			FROM farol.sp_centros_dist cd
			JOIN farol.sp_filiais f ON f.id = cd.filial_id
			WHERE cd.id = $1 AND f.empresa_id = $2
		`, fonteID, spCtx.EmpresaID).Scan(&filialID, &nomeBase)
		if err == sql.ErrNoRows {
			http.Error(w, "CD fonte não encontrado", http.StatusNotFound)
			return
		} else if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}

		novoNome := req.Nome
		if novoNome == "" {
			novoNome = nomeBase + " (cópia)"
		}

		var novoCDID int
		err = tx.QueryRow(`
			INSERT INTO farol.sp_centros_dist (filial_id, empresa_id, nome, fonte_cd_id, criado_por)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING id
		`, filialID, spCtx.EmpresaID, novoNome, fonteID, spCtx.UserID).Scan(&novoCDID)
		if err != nil {
			http.Error(w, "Database error ao duplicar CD", http.StatusInternalServerError)
			return
		}

		// Copia parâmetros do motor do CD fonte
		_, err = tx.Exec(`
			INSERT INTO farol.sp_motor_params
			  (cd_id, empresa_id, dias_analise, curva_a_max_est, curva_b_max_est,
			   curva_c_max_est, fator_seguranca, curva_a_nunca_reduz, min_capacidade)
			SELECT $1, $2, dias_analise, curva_a_max_est, curva_b_max_est,
			       curva_c_max_est, fator_seguranca, curva_a_nunca_reduz, min_capacidade
			FROM farol.sp_motor_params
			WHERE cd_id = $3
			ON CONFLICT (cd_id) DO NOTHING
		`, novoCDID, spCtx.EmpresaID, fonteID)
		if err != nil {
			http.Error(w, "Database error ao copiar parâmetros", http.StatusInternalServerError)
			return
		}
		// Se o fonte não tinha parâmetros, cria padrão
		tx.Exec(`
			INSERT INTO farol.sp_motor_params (cd_id, empresa_id)
			VALUES ($1, $2) ON CONFLICT (cd_id) DO NOTHING
		`, novoCDID, spCtx.EmpresaID)

		if err := tx.Commit(); err != nil {
			http.Error(w, "Commit error", http.StatusInternalServerError)
			return
		}

		log.Printf("SpDuplicarCD: CD %d duplicado como %d por %s", fonteID, novoCDID, spCtx.UserID)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]int{"id": novoCDID})
	}
}

// ─── Parâmetros do Motor ──────────────────────────────────────────────────────

// SpMotorParamsHandler — GET/PUT /api/sp/cds/{id}/params
func SpMotorParamsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		idStr := pathSegment(r.URL.Path, "/api/sp/cds/")
		cdID, err := strconv.Atoi(idStr)
		if err != nil {
			http.Error(w, "CD ID inválido", http.StatusBadRequest)
			return
		}

		// Verifica que o CD pertence à empresa
		var existe bool
		db.QueryRow(`
			SELECT EXISTS(
			  SELECT 1 FROM farol.sp_centros_dist cd
			  JOIN farol.sp_filiais f ON f.id = cd.filial_id
			  WHERE cd.id = $1 AND f.empresa_id = $2
			)
		`, cdID, spCtx.EmpresaID).Scan(&existe)
		if !existe {
			http.Error(w, "CD não encontrado", http.StatusNotFound)
			return
		}

		switch r.Method {
		case http.MethodGet:
			var p SpMotorParamsResponse
			// Tenta query com retencao_csv_meses (pós-migration 108).
			// Se a coluna ainda não existir no banco, faz fallback com default 6.
			scanParams := func(q string) error {
				return db.QueryRow(q, cdID).Scan(
					&p.ID, &p.CDID, &p.DiasAnalise, &p.CurvaAMaxEst, &p.CurvaBMaxEst,
					&p.CurvaCMaxEst, &p.FatorSeguranca, &p.CurvaANuncaReduz, &p.MinCapacidade,
					&p.RetencaoCsvMeses, &p.UpdatedAt,
				)
			}
			const qFull = `
				SELECT id, cd_id, dias_analise, curva_a_max_est, curva_b_max_est,
				       curva_c_max_est, fator_seguranca, curva_a_nunca_reduz, min_capacidade,
				       COALESCE(retencao_csv_meses, 6), updated_at
				FROM farol.sp_motor_params WHERE cd_id = $1
			`
			const qLegacy = `
				SELECT id, cd_id, dias_analise, curva_a_max_est, curva_b_max_est,
				       curva_c_max_est, fator_seguranca, curva_a_nunca_reduz, min_capacidade,
				       6, updated_at
				FROM farol.sp_motor_params WHERE cd_id = $1
			`
			err := scanParams(qFull)
			if err == sql.ErrNoRows {
				// Cria padrão on-demand
				db.Exec(`INSERT INTO farol.sp_motor_params (cd_id, empresa_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
					cdID, spCtx.EmpresaID)
				err = scanParams(qFull)
			}
			if err != nil {
				// Fallback: coluna retencao_csv_meses ainda não existe (migration pendente)
				p.RetencaoCsvMeses = 6
				if ferr := scanParams(qLegacy); ferr != nil {
					http.Error(w, "Database error", http.StatusInternalServerError)
					return
				}
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(p)

		case http.MethodPut:
			if !RequireWrite(spCtx, w) {
				return
			}
			var req SpMotorParamsRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "Invalid body", http.StatusBadRequest)
				return
			}
			if req.DiasAnalise == 0 {
				req.DiasAnalise = 90
			}
			if req.CurvaAMaxEst == 0 {
				req.CurvaAMaxEst = 7
			}
			if req.CurvaBMaxEst == 0 {
				req.CurvaBMaxEst = 15
			}
			if req.CurvaCMaxEst == 0 {
				req.CurvaCMaxEst = 30
			}
			if req.FatorSeguranca == 0 {
				req.FatorSeguranca = 1.10
			}
			if req.MinCapacidade == 0 {
				req.MinCapacidade = 1
			}
			if req.RetencaoCsvMeses == 0 {
				req.RetencaoCsvMeses = 6
			}
			if req.RetencaoCsvMeses > 60 {
				req.RetencaoCsvMeses = 60
			}
			// Tenta salvar com retencao_csv_meses; se a coluna não existir (migration pendente), salva sem ela
			_, err := db.Exec(`
				INSERT INTO farol.sp_motor_params
				  (cd_id, empresa_id, dias_analise, curva_a_max_est, curva_b_max_est,
				   curva_c_max_est, fator_seguranca, curva_a_nunca_reduz, min_capacidade,
				   retencao_csv_meses, updated_by)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
				ON CONFLICT (cd_id) DO UPDATE SET
				  dias_analise        = EXCLUDED.dias_analise,
				  curva_a_max_est     = EXCLUDED.curva_a_max_est,
				  curva_b_max_est     = EXCLUDED.curva_b_max_est,
				  curva_c_max_est     = EXCLUDED.curva_c_max_est,
				  fator_seguranca     = EXCLUDED.fator_seguranca,
				  curva_a_nunca_reduz = EXCLUDED.curva_a_nunca_reduz,
				  min_capacidade      = EXCLUDED.min_capacidade,
				  retencao_csv_meses  = EXCLUDED.retencao_csv_meses,
				  updated_by          = EXCLUDED.updated_by,
				  updated_at          = now()
			`, cdID, spCtx.EmpresaID, req.DiasAnalise, req.CurvaAMaxEst, req.CurvaBMaxEst,
				req.CurvaCMaxEst, req.FatorSeguranca, req.CurvaANuncaReduz, req.MinCapacidade,
				req.RetencaoCsvMeses, spCtx.UserID)
			if err != nil {
				// Fallback: coluna retencao_csv_meses ainda não existe
				_, err = db.Exec(`
					INSERT INTO farol.sp_motor_params
					  (cd_id, empresa_id, dias_analise, curva_a_max_est, curva_b_max_est,
					   curva_c_max_est, fator_seguranca, curva_a_nunca_reduz, min_capacidade, updated_by)
					VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
					ON CONFLICT (cd_id) DO UPDATE SET
					  dias_analise        = EXCLUDED.dias_analise,
					  curva_a_max_est     = EXCLUDED.curva_a_max_est,
					  curva_b_max_est     = EXCLUDED.curva_b_max_est,
					  curva_c_max_est     = EXCLUDED.curva_c_max_est,
					  fator_seguranca     = EXCLUDED.fator_seguranca,
					  curva_a_nunca_reduz = EXCLUDED.curva_a_nunca_reduz,
					  min_capacidade      = EXCLUDED.min_capacidade,
					  updated_by          = EXCLUDED.updated_by,
					  updated_at          = now()
				`, cdID, spCtx.EmpresaID, req.DiasAnalise, req.CurvaAMaxEst, req.CurvaBMaxEst,
					req.CurvaCMaxEst, req.FatorSeguranca, req.CurvaANuncaReduz, req.MinCapacidade, spCtx.UserID)
			}
			if err != nil {
				http.Error(w, "Database error: "+err.Error(), http.StatusInternalServerError)
				return
			}
			log.Printf("SpMotorParams: CD %d atualizado por %s", cdID, spCtx.UserID)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"message": "Parâmetros atualizados"})

		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

// ─── Planos ───────────────────────────────────────────────────────────────────

// SpPlanoHandler — GET/PUT /api/sp/plano
func SpPlanoHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		switch r.Method {
		case http.MethodGet:
			var p SpPlanoResponse
			var validoAte *string
			err := db.QueryRow(`
				SELECT plano, max_filiais, max_cds, max_usuarios, ativo,
				       TO_CHAR(valido_ate, 'YYYY-MM-DD')
				FROM farol.sp_subscription_limits
				WHERE empresa_id = $1
			`, spCtx.EmpresaID).Scan(&p.Plano, &p.MaxFiliais, &p.MaxCDs, &p.MaxUsuarios, &p.Ativo, &validoAte)
			if err == sql.ErrNoRows {
				// Insere plano básico e retorna
				db.Exec(`
					INSERT INTO farol.sp_subscription_limits (empresa_id, plano, max_filiais, max_cds, max_usuarios)
					VALUES ($1, 'basic', 1, 3, 5) ON CONFLICT DO NOTHING
				`, spCtx.EmpresaID)
				p = SpPlanoResponse{Plano: "basic", MaxFiliais: 1, MaxCDs: 3, MaxUsuarios: 5, Ativo: true}
			} else if err != nil {
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
			p.ValidoAte = validoAte

			// Uso atual
			db.QueryRow(`SELECT COUNT(*) FROM farol.sp_filiais WHERE empresa_id = $1 AND ativo = TRUE`, spCtx.EmpresaID).Scan(&p.UsadoFiliais)
			db.QueryRow(`
				SELECT COUNT(*) FROM farol.sp_centros_dist cd
				JOIN farol.sp_filiais f ON f.id = cd.filial_id
				WHERE f.empresa_id = $1 AND cd.ativo = TRUE
			`, spCtx.EmpresaID).Scan(&p.UsadoCDs)
			db.QueryRow(`
				SELECT COUNT(DISTINCT user_id) FROM farol.sp_user_filiais WHERE empresa_id = $1
			`, spCtx.EmpresaID).Scan(&p.UsadoUsuarios)

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(p)

		case http.MethodPut:
			if !spCtx.IsAdminFbtax() {
				http.Error(w, "Forbidden: apenas admin_fbtax pode alterar planos", http.StatusForbidden)
				return
			}
			var req SpAtualizarPlanoRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "Invalid body", http.StatusBadRequest)
				return
			}
			_, err := db.Exec(`
				INSERT INTO farol.sp_subscription_limits
				  (empresa_id, plano, max_filiais, max_cds, max_usuarios, valido_ate)
				VALUES ($1, $2, $3, $4, $5, $6::TIMESTAMPTZ)
				ON CONFLICT (empresa_id) DO UPDATE SET
				  plano        = EXCLUDED.plano,
				  max_filiais  = EXCLUDED.max_filiais,
				  max_cds      = EXCLUDED.max_cds,
				  max_usuarios = EXCLUDED.max_usuarios,
				  valido_ate   = EXCLUDED.valido_ate,
				  updated_at   = now()
			`, spCtx.EmpresaID, req.Plano, req.MaxFiliais, req.MaxCDs, req.MaxUsuarios, req.ValidoAte)
			if err != nil {
				http.Error(w, "Database error: "+err.Error(), http.StatusInternalServerError)
				return
			}
			log.Printf("SpPlano: empresa %s → plano %s por %s", spCtx.EmpresaID, req.Plano, spCtx.UserID)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"message": "Plano atualizado"})

		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}
