package handlers

// sp_usuarios.go — CRUD de usuários Farol
//
// Endpoints protegidos por FarolAuthMiddleware (perfil mínimo: admin_fbtax).
// Operações:
//   GET    /api/sp/usuarios         → lista usuários da empresa ativa
//   POST   /api/sp/usuarios         → cria usuário (admin_fbtax)
//   PUT    /api/sp/usuarios/{id}    → atualiza sp_role (admin_fbtax)
//   DELETE /api/sp/usuarios/{id}    → remove usuário (admin_fbtax)
//   PUT    /api/sp/usuarios/{id}/filiais → define filiais acessíveis

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"
)

// ─── DTOs ─────────────────────────────────────────────────────────────────────

type SpUsuarioResponse struct {
	ID              string    `json:"id"`
	Email           string    `json:"email"`
	FullName        string    `json:"full_name"`
	SpRole          string    `json:"sp_role"`
	IsVerified      bool      `json:"is_verified"`
	TrialEndsAt     time.Time `json:"trial_ends_at"`
	CreatedAt       string    `json:"created_at"`
	// Hierarquia (user_environments)
	EnvironmentID   string `json:"environment_id"`
	EnvironmentName string `json:"environment_name"`
	GroupID         string `json:"group_id"`
	GroupName       string `json:"group_name"`
	CompanyID       string `json:"company_id"`
	CompanyName     string `json:"company_name"`
	// Filiais vinculadas (preenchidas pela query de listagem)
	AllFiliais bool  `json:"all_filiais"`
	FilialIDs  []int `json:"filial_ids"`
}

type SpUpdateRoleRequest struct {
	SpRole        string `json:"sp_role"`        // admin_fbtax | gestor_geral | gestor_filial | somente_leitura
	FullName      string `json:"full_name"`      // opcional — atualiza nome se informado
	EnvironmentID string `json:"environment_id"` // opcional — reatribui hierarquia
	GroupID       string `json:"group_id"`       // ignorado (derivado da empresa)
	CompanyID     string `json:"company_id"`     // define preferred_company_id
	TrialEndsAt   string `json:"trial_ends_at"`  // opcional — "YYYY-MM-DD"; renova licença
}

type SpVincularFiliaisRequest struct {
	AllFiliais bool  `json:"all_filiais"`
	FilialIDs  []int `json:"filial_ids"` // ignorado quando all_filiais = true
}

// ─── Handlers ────────────────────────────────────────────────────────────────

// SpListUsuariosHandler lista os usuários com acesso Farol.
//
// Escopo de visibilidade:
//   - Tenant master (companies.group_id IS NULL): lista TODOS os usuários do sistema.
//   - Tenant normal (group_id preenchido): lista apenas usuários do mesmo enterprise_group,
//     mesmo que o usuário logado seja admin_fbtax.
func SpListUsuariosHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// ── Determina escopo: master ou tenant normal ────────────────────────
		// "master" = empresa ativa pertence ao grupo 'MASTER' (migration 024).
		// Usuários do grupo MASTER enxergam todos os usuários do sistema.
		// Qualquer outro grupo (ex: JC) enxerga apenas o próprio grupo.
		var groupName sql.NullString
		var rawGroupID sql.NullString
		if err := db.QueryRow(`
			SELECT COALESCE(eg.name, '')::text, COALESCE(eg.id::text, '')
			FROM companies c
			LEFT JOIN enterprise_groups eg ON eg.id = c.group_id
			WHERE c.id = $1::uuid
		`, spCtx.EmpresaID).Scan(&groupName, &rawGroupID); err != nil {
			log.Printf("SpListUsuarios group lookup: %v", err)
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		isMaster := groupName.String == "MASTER"

		// ── SELECT base (idêntico nos dois casos) ─────────────────────────────
		const selectBase = `
			SELECT u.id::text, u.email, u.full_name, u.sp_role::text, u.is_verified, u.trial_ends_at, u.created_at::text,
			    COALESCE(ue.environment_id::text, ''),
			    COALESCE(e.name, ''),
			    COALESCE(ue.preferred_company_id::text, ''),
			    COALESCE(pc.name, ''),
			    COALESCE(eg.id::text, ''),
			    COALESCE(eg.name, '')
			FROM users u
			LEFT JOIN LATERAL (
			    SELECT environment_id, preferred_company_id
			    FROM user_environments
			    WHERE user_id = u.id
			    LIMIT 1
			) ue ON true
			LEFT JOIN environments e ON e.id = ue.environment_id
			LEFT JOIN companies pc ON pc.id = ue.preferred_company_id
			LEFT JOIN enterprise_groups eg ON eg.id = pc.group_id`

		var (
			rows *sql.Rows
			err  error
		)
		if isMaster {
			// Tenant master: todos os usuários do sistema
			rows, err = db.Query(selectBase + ` ORDER BY u.full_name`)
		} else {
			// Tenant normal: apenas usuários do mesmo grupo empresarial
			// $1 = group_id da empresa ativa  $2 = user_id do solicitante
			rows, err = db.Query(selectBase+`
				WHERE u.id = $2
				   OR EXISTS (
				       SELECT 1 FROM user_environments ue2
				       JOIN companies c ON c.id = ue2.preferred_company_id
				       WHERE ue2.user_id = u.id AND c.group_id = $1::uuid
				   )
				   OR EXISTS (
				       SELECT 1 FROM farol.sp_user_filiais uf
				       JOIN companies c ON c.id = uf.empresa_id
				       WHERE uf.user_id = u.id AND c.group_id = $1::uuid
				   )
				ORDER BY u.full_name`,
				rawGroupID.String, spCtx.UserID)
		}
		if err != nil {
			log.Printf("SpListUsuarios: %v", err)
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var usuarios []SpUsuarioResponse
		for rows.Next() {
			var u SpUsuarioResponse
			if err := rows.Scan(&u.ID, &u.Email, &u.FullName, &u.SpRole,
				&u.IsVerified, &u.TrialEndsAt, &u.CreatedAt,
				&u.EnvironmentID, &u.EnvironmentName,
				&u.CompanyID, &u.CompanyName,
				&u.GroupID, &u.GroupName); err != nil {
				continue
			}
			// Carrega filiais vinculadas para este usuário nessa empresa
			fRows, err := db.Query(`
				SELECT filial_id, all_filiais
				FROM farol.sp_user_filiais
				WHERE user_id = $1 AND empresa_id = $2
			`, u.ID, spCtx.EmpresaID)
			if err == nil {
				defer fRows.Close()
				for fRows.Next() {
					var fid *int
					var all bool
					if err := fRows.Scan(&fid, &all); err == nil {
						if all {
							u.AllFiliais = true
						} else if fid != nil {
							u.FilialIDs = append(u.FilialIDs, *fid)
						}
					}
				}
			}
			if u.FilialIDs == nil {
				u.FilialIDs = []int{}
			}
			usuarios = append(usuarios, u)
		}
		if usuarios == nil {
			usuarios = []SpUsuarioResponse{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(usuarios)
	}
}

// SpUpdateRoleHandler atualiza o sp_role de um usuário (admin_fbtax only).
func SpUpdateRoleHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		spCtx := GetSpContext(r)
		if spCtx == nil || !spCtx.IsAdminFbtax() {
			http.Error(w, "Forbidden: apenas admin_fbtax pode alterar perfis", http.StatusForbidden)
			return
		}

		targetID := strings.TrimPrefix(r.URL.Path, "/api/sp/usuarios/")
		targetID = strings.TrimSuffix(targetID, "/role")
		if targetID == "" {
			http.Error(w, "User ID required", http.StatusBadRequest)
			return
		}

		var req SpUpdateRoleRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		validRoles := map[string]bool{
			"admin_fbtax": true, "gestor_geral": true,
			"gestor_filial": true, "somente_leitura": true,
		}
		if !validRoles[req.SpRole] {
			http.Error(w, "sp_role inválido", http.StatusBadRequest)
			return
		}

		// Parse opcional de trial_ends_at (renovação de licença)
		var trialEnds *time.Time
		if req.TrialEndsAt != "" {
			t, parseErr := time.Parse("2006-01-02", req.TrialEndsAt)
			if parseErr != nil {
				http.Error(w, "trial_ends_at inválido (use YYYY-MM-DD)", http.StatusBadRequest)
				return
			}
			tUTC := t.UTC()
			trialEnds = &tUTC
		}

		res, err := db.Exec(
			`UPDATE users SET
			    sp_role = $1,
			    full_name = CASE WHEN $2 != '' THEN $2 ELSE full_name END,
			    trial_ends_at = CASE WHEN $4::timestamptz IS NOT NULL THEN $4::timestamptz ELSE trial_ends_at END
			 WHERE id = $3`,
			req.SpRole, req.FullName, targetID, trialEnds,
		)
		if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		if n, _ := res.RowsAffected(); n == 0 {
			http.Error(w, "User not found", http.StatusNotFound)
			return
		}

		// Audit log da renovação de licença (se aplicável)
		if trialEnds != nil {
			writeAuditLog(db, spCtx.EmpresaID, spCtx.UserID, "usuario", targetID, "renovar_licenca", map[string]any{
				"trial_ends_at": req.TrialEndsAt,
			})
		}

		// Reatribuição de hierarquia (ambiente + empresa preferida)
		if req.EnvironmentID != "" {
			tx, txErr := db.Begin()
			if txErr == nil {
				_, _ = tx.Exec("DELETE FROM user_environments WHERE user_id = $1", targetID)
				if req.CompanyID != "" {
					_, txErr = tx.Exec(`
						INSERT INTO user_environments (user_id, environment_id, role, preferred_company_id)
						VALUES ($1, $2, 'user', $3)
					`, targetID, req.EnvironmentID, req.CompanyID)
				} else {
					_, txErr = tx.Exec(
						"INSERT INTO user_environments (user_id, environment_id, role) VALUES ($1, $2, 'user')",
						targetID, req.EnvironmentID,
					)
				}
				if txErr == nil {
					tx.Commit()
					log.Printf("SpUpdateRole: user %s reatribuído env=%s empresa=%s (by %s)",
						targetID, req.EnvironmentID, req.CompanyID, spCtx.UserID)
				} else {
					tx.Rollback()
					log.Printf("SpUpdateRole: falha ao reatribuir hierarquia: %v", txErr)
				}
			}
		}

		log.Printf("SpUpdateRole: user %s → sp_role=%s (by %s)", targetID, req.SpRole, spCtx.UserID)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"message": "Perfil atualizado com sucesso"})
	}
}

// SpCriarUsuarioRequest — payload para POST /api/sp/usuarios
type SpCriarUsuarioRequest struct {
	FullName      string `json:"full_name"`
	Email         string `json:"email"`
	Password      string `json:"password"`
	SpRole        string `json:"sp_role"`        // gestor_geral | gestor_filial | somente_leitura
	TrialDias     int    `json:"trial_dias"`     // fallback: 0 = 365 dias
	TrialEndsAt   string `json:"trial_ends_at"`  // "2006-01-02" — tem prioridade sobre trial_dias
	AllFiliais    bool   `json:"all_filiais"`
	FilialIDs     []int  `json:"filial_ids"`
	EnvironmentID string `json:"environment_id"` // opcional — override do auto-detect
	GroupID       string `json:"group_id"`       // opcional
	CompanyID     string `json:"company_id"`     // opcional — override da empresa ativa para filiais
}

// SpCriarUsuarioHandler cria um novo usuário vinculado à empresa ativa (admin_fbtax only).
// POST /api/sp/usuarios
func SpCriarUsuarioHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		spCtx := GetSpContext(r)
		if spCtx == nil || !spCtx.IsAdminFbtax() {
			http.Error(w, "Forbidden: apenas admin_fbtax pode criar usuários", http.StatusForbidden)
			return
		}
		// H2 fix: criação de usuário só é permitida para MASTER (casar com frontend)
		if !spCtx.IsMasterTenant(db) {
			http.Error(w, "Forbidden: apenas usuários MASTER podem criar contas", http.StatusForbidden)
			return
		}

		var req SpCriarUsuarioRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		if req.FullName == "" || req.Email == "" || req.Password == "" {
			http.Error(w, "full_name, email e password são obrigatórios", http.StatusBadRequest)
			return
		}

		validRoles := map[string]bool{
			"admin_fbtax": true, "gestor_geral": true, "gestor_filial": true, "somente_leitura": true,
		}
		if req.SpRole == "" {
			req.SpRole = "somente_leitura"
		}
		if !validRoles[req.SpRole] {
			http.Error(w, "sp_role inválido", http.StatusBadRequest)
			return
		}

		var trialEndsAt time.Time
		if req.TrialEndsAt != "" {
			if t, err2 := time.Parse("2006-01-02", req.TrialEndsAt); err2 == nil {
				trialEndsAt = t.UTC()
			}
		}
		if trialEndsAt.IsZero() {
			dias := req.TrialDias
			if dias <= 0 {
				dias = 365
			}
			trialEndsAt = time.Now().Add(time.Duration(dias) * 24 * time.Hour)
		}

		hash, err := HashPassword(req.Password)
		if err != nil {
			http.Error(w, "Erro ao processar senha", http.StatusInternalServerError)
			return
		}

		// Cria usuário na tabela pública
		var userID string
		err = db.QueryRow(`
			INSERT INTO users (email, password_hash, full_name, trial_ends_at, is_verified, role, sp_role)
			VALUES ($1, $2, $3, $4, TRUE, 'user', $5)
			RETURNING id
		`, req.Email, hash, req.FullName, trialEndsAt, req.SpRole).Scan(&userID)
		if err != nil {
			if strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "duplicate") {
				http.Error(w, "E-mail já cadastrado", http.StatusConflict)
				return
			}
			log.Printf("SpCriarUsuario: insert user error: %v", err)
			http.Error(w, "Erro ao criar usuário", http.StatusInternalServerError)
			return
		}

		// Vincula ao environment (usa o fornecido ou auto-detecta pela empresa ativa)
		envIDToUse := req.EnvironmentID
		if envIDToUse == "" {
			var derivedEnv string
			_ = db.QueryRow(`
				SELECT eg.environment_id
				FROM companies c
				JOIN enterprise_groups eg ON eg.id = c.group_id
				WHERE c.id = $1
				LIMIT 1
			`, spCtx.EmpresaID).Scan(&derivedEnv)
			envIDToUse = derivedEnv
		}
		// Empresa para vínculo de filiais (usa company_id fornecido ou empresa ativa)
		empresaIDToUse := spCtx.EmpresaID
		if req.CompanyID != "" {
			empresaIDToUse = req.CompanyID
		}

		if envIDToUse != "" {
			if empresaIDToUse != "" {
				// preferred_company_id garante que GetEffectiveCompanyID retorna a empresa certa
				_, _ = db.Exec(`
					INSERT INTO user_environments (user_id, environment_id, role, preferred_company_id)
					VALUES ($1, $2, 'user', $3) ON CONFLICT DO NOTHING
				`, userID, envIDToUse, empresaIDToUse)
			} else {
				_, _ = db.Exec(
					"INSERT INTO user_environments (user_id, environment_id, role) VALUES ($1, $2, 'user') ON CONFLICT DO NOTHING",
					userID, envIDToUse,
				)
			}
		}

		// Vincula filiais no Farol
		if req.AllFiliais {
			_, _ = db.Exec(`
				INSERT INTO farol.sp_user_filiais (user_id, empresa_id, filial_id, all_filiais)
				VALUES ($1, $2, NULL, TRUE) ON CONFLICT DO NOTHING
			`, userID, empresaIDToUse)
		} else {
			for _, fid := range req.FilialIDs {
				_, _ = db.Exec(`
					INSERT INTO farol.sp_user_filiais (user_id, empresa_id, filial_id, all_filiais)
					VALUES ($1, $2, $3, FALSE) ON CONFLICT DO NOTHING
				`, userID, empresaIDToUse, fid)
			}
		}

		log.Printf("SpCriarUsuario: criado user %s (%s) sp_role=%s empresa=%s por %s",
			userID, req.Email, req.SpRole, spCtx.EmpresaID, spCtx.UserID)

		writeAuditLog(db, spCtx.EmpresaID, spCtx.UserID, "usuario", userID, "criar_usuario", map[string]any{
			"email": req.Email, "full_name": req.FullName, "sp_role": req.SpRole,
		})

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"id": userID, "message": "Usuário criado com sucesso"})
	}
}

// SpDeletarUsuarioHandler remove um usuário do sistema (admin_fbtax only).
// DELETE /api/sp/usuarios/{id}
func SpDeletarUsuarioHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		spCtx := GetSpContext(r)
		if spCtx == nil || !spCtx.IsAdminFbtax() {
			http.Error(w, "Forbidden: apenas admin_fbtax pode excluir usuários", http.StatusForbidden)
			return
		}
		// H2 fix: exclusão só permitida para MASTER (casar com frontend)
		if !spCtx.IsMasterTenant(db) {
			http.Error(w, "Forbidden: apenas usuários MASTER podem excluir contas", http.StatusForbidden)
			return
		}

		// Extrai {id} do path /api/sp/usuarios/{id} (tolera barras extras/sufixos inválidos)
		targetID := strings.TrimPrefix(r.URL.Path, "/api/sp/usuarios/")
		targetID = strings.Trim(targetID, "/")
		if targetID == "" || strings.Contains(targetID, "/") {
			http.Error(w, "User ID inválido", http.StatusBadRequest)
			return
		}
		if strings.EqualFold(targetID, spCtx.UserID) {
			http.Error(w, "Não é possível excluir o próprio usuário", http.StatusBadRequest)
			return
		}

		// H1 fix: bloqueia cross-tenant delete. Só permite se caller é MASTER
		// ou se o alvo pertence ao mesmo grupo empresarial.
		if !spCtx.TargetUserInSameTenant(db, targetID) {
			http.Error(w, "Forbidden: usuário fora do seu escopo", http.StatusForbidden)
			return
		}

		// Busca dados para o audit log antes de deletar (dentro da tx para consistência)
		var email, fullName string
		_ = db.QueryRow("SELECT email, full_name FROM users WHERE id = $1", targetID).Scan(&email, &fullName)

		tx, err := db.Begin()
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()

		_, _ = tx.Exec("DELETE FROM farol.sp_user_filiais WHERE user_id = $1", targetID)
		_, _ = tx.Exec("DELETE FROM user_environments WHERE user_id = $1", targetID)
		res, err := tx.Exec("DELETE FROM users WHERE id = $1", targetID)
		if err != nil {
			http.Error(w, "Erro ao excluir usuário: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if n, _ := res.RowsAffected(); n == 0 {
			http.Error(w, "Usuário não encontrado", http.StatusNotFound)
			return
		}

		// H3 fix: audit log dentro da tx — se falhar, rollback da operação.
		if err := writeAuditLogTx(tx, spCtx.EmpresaID, spCtx.UserID, "usuario", targetID, "excluir_usuario", map[string]any{
			"email": email, "full_name": fullName,
		}); err != nil {
			http.Error(w, "Erro ao gravar auditoria: "+err.Error(), http.StatusInternalServerError)
			return
		}

		if err := tx.Commit(); err != nil {
			http.Error(w, "Commit error", http.StatusInternalServerError)
			return
		}

		log.Printf("SpDeletarUsuario: excluído user %s (%s) por %s", targetID, email, spCtx.UserID)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"message": "Usuário excluído com sucesso"})
	}
}

// SpVincularFiliaisHandler define (replace) as filiais acessíveis para um usuário.
func SpVincularFiliaisHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		spCtx := GetSpContext(r)
		if spCtx == nil || !spCtx.IsAdminFbtax() {
			http.Error(w, "Forbidden: apenas admin_fbtax pode vincular filiais", http.StatusForbidden)
			return
		}

		// Path: /api/sp/usuarios/{id}/filiais
		path := strings.TrimPrefix(r.URL.Path, "/api/sp/usuarios/")
		targetID := strings.TrimSuffix(path, "/filiais")
		if targetID == "" || targetID == path {
			http.Error(w, "User ID required", http.StatusBadRequest)
			return
		}

		var req SpVincularFiliaisRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		tx, err := db.Begin()
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()

		// Remove vínculos anteriores deste usuário nesta empresa
		if _, err = tx.Exec(
			"DELETE FROM farol.sp_user_filiais WHERE user_id = $1 AND empresa_id = $2",
			targetID, spCtx.EmpresaID,
		); err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}

		if req.AllFiliais {
			// Insere vínculo "todas as filiais"
			_, err = tx.Exec(`
				INSERT INTO farol.sp_user_filiais (user_id, empresa_id, filial_id, all_filiais)
				VALUES ($1, $2, NULL, TRUE)
			`, targetID, spCtx.EmpresaID)
		} else {
			// Insere uma linha por filial específica
			for _, fid := range req.FilialIDs {
				_, err = tx.Exec(`
					INSERT INTO farol.sp_user_filiais (user_id, empresa_id, filial_id, all_filiais)
					VALUES ($1, $2, $3, FALSE)
				`, targetID, spCtx.EmpresaID, fid)
				if err != nil {
					break
				}
			}
		}
		if err != nil {
			http.Error(w, "Database error ao vincular filiais", http.StatusInternalServerError)
			return
		}

		if err := tx.Commit(); err != nil {
			http.Error(w, "Commit error", http.StatusInternalServerError)
			return
		}

		log.Printf("SpVincularFiliais: user %s empresa %s all=%v filiais=%v (by %s)",
			targetID, spCtx.EmpresaID, req.AllFiliais, req.FilialIDs, spCtx.UserID)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"message": "Filiais vinculadas com sucesso"})
	}
}

// ─── Vínculos multi-empresa ───────────────────────────────────────────────────

type SpVinculoResponse struct {
	EmpresaID   string `json:"empresa_id"`
	EmpresaNome string `json:"empresa_nome"`
	AllFiliais  bool   `json:"all_filiais"`
	FilialIDs   []int  `json:"filial_ids"`
}

type SpSaveVinculoItem struct {
	EmpresaID  string `json:"empresa_id"`
	AllFiliais bool   `json:"all_filiais"`
	FilialIDs  []int  `json:"filial_ids"`
}

// SpGetVinculosHandler retorna todas as associações empresa→filiais de um usuário.
// GET /api/sp/usuarios/{id}/vinculos
func SpGetVinculosHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		spCtx := GetSpContext(r)
		if spCtx == nil || !spCtx.IsAdminFbtax() {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		path := strings.TrimPrefix(r.URL.Path, "/api/sp/usuarios/")
		targetID := strings.TrimSuffix(path, "/vinculos")
		if targetID == "" || targetID == path {
			http.Error(w, "User ID required", http.StatusBadRequest)
			return
		}

		rows, err := db.Query(`
			SELECT uf.empresa_id, COALESCE(c.name, ''), uf.all_filiais, uf.filial_id
			FROM farol.sp_user_filiais uf
			LEFT JOIN companies c ON c.id = uf.empresa_id
			WHERE uf.user_id = $1
			ORDER BY c.name, uf.filial_id
		`, targetID)
		if err != nil {
			log.Printf("SpGetVinculos: %v", err)
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		vinculoMap := map[string]*SpVinculoResponse{}
		var order []string
		for rows.Next() {
			var empID, empNome string
			var allFiliais bool
			var filialID *int
			if err := rows.Scan(&empID, &empNome, &allFiliais, &filialID); err != nil {
				continue
			}
			if _, ok := vinculoMap[empID]; !ok {
				vinculoMap[empID] = &SpVinculoResponse{
					EmpresaID: empID, EmpresaNome: empNome, FilialIDs: []int{},
				}
				order = append(order, empID)
			}
			v := vinculoMap[empID]
			if allFiliais {
				v.AllFiliais = true
			} else if filialID != nil {
				v.FilialIDs = append(v.FilialIDs, *filialID)
			}
		}

		result := make([]SpVinculoResponse, 0, len(order))
		for _, id := range order {
			result = append(result, *vinculoMap[id])
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}
}

// SpSaveVinculosHandler substitui associações empresa→filiais de um usuário (multi-empresa).
// PUT /api/sp/usuarios/{id}/vinculos
// Body: [{empresa_id, all_filiais, filial_ids}] — empresas sem entradas ficam sem acesso.
func SpSaveVinculosHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		spCtx := GetSpContext(r)
		if spCtx == nil || !spCtx.IsAdminFbtax() {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		path := strings.TrimPrefix(r.URL.Path, "/api/sp/usuarios/")
		targetID := strings.TrimSuffix(path, "/vinculos")
		if targetID == "" || targetID == path {
			http.Error(w, "User ID required", http.StatusBadRequest)
			return
		}

		var vinculos []SpSaveVinculoItem
		if err := json.NewDecoder(r.Body).Decode(&vinculos); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		tx, err := db.Begin()
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()

		for _, v := range vinculos {
			if _, err = tx.Exec(
				"DELETE FROM farol.sp_user_filiais WHERE user_id = $1 AND empresa_id = $2",
				targetID, v.EmpresaID,
			); err != nil {
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
			if v.AllFiliais {
				_, err = tx.Exec(`
					INSERT INTO farol.sp_user_filiais (user_id, empresa_id, filial_id, all_filiais)
					VALUES ($1, $2, NULL, TRUE)
				`, targetID, v.EmpresaID)
			} else {
				for _, fid := range v.FilialIDs {
					_, err = tx.Exec(`
						INSERT INTO farol.sp_user_filiais (user_id, empresa_id, filial_id, all_filiais)
						VALUES ($1, $2, $3, FALSE)
					`, targetID, v.EmpresaID, fid)
					if err != nil {
						break
					}
				}
			}
			if err != nil {
				http.Error(w, "Database error ao vincular filiais", http.StatusInternalServerError)
				return
			}
		}

		if err = tx.Commit(); err != nil {
			http.Error(w, "Commit error", http.StatusInternalServerError)
			return
		}

		log.Printf("SpSaveVinculos: user %s atualizado com %d empresas por %s", targetID, len(vinculos), spCtx.UserID)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"message": "Vínculos atualizados com sucesso"})
	}
}
