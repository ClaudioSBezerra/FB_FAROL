package handlers

// farol_auth.go — FarolAuthMiddleware
//
// Implementa o RBAC Farol em cima do AuthMiddleware herdado (auth.go — nunca modificar).
// Cadeia de execução:
//   1. AuthMiddleware  → valida JWT, verifica blacklist, injeta claims no contexto
//   2. FarolAuthMiddleware → lê user_id do contexto, carrega sp_role + filiais do banco
//
// Perfis (sp_role_type em 101_sp_rbac.sql):
//   admin_fbtax     → acesso total; ignora restrições de empresa/filial
//   gestor_geral    → acesso a todas as filiais do tenant (empresa)
//   gestor_filial   → acesso apenas às filiais vinculadas em sp_user_filiais
//   somente_leitura → apenas leitura; sem aprovação ou edição inline

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"
)

// ─── Contexto Farol ───────────────────────────────────────────────────────

type spCtxKey string

const SpContextKey spCtxKey = "sp_context"

// FarolContext é injetado no request context por FarolAuthMiddleware.
type FarolContext struct {
	UserID     string
	SpRole     string  // admin_fbtax | gestor_geral | gestor_filial | somente_leitura
	EmpresaID  string
	FilialIDs  []int   // IDs de filiais acessíveis; vazio quando AllFiliais = true
	AllFiliais bool    // true para admin_fbtax e gestor_geral
}

// GetSpContext extrai o FarolContext do request. Retorna nil se não encontrado.
func GetSpContext(r *http.Request) *FarolContext {
	ctx, _ := r.Context().Value(SpContextKey).(*FarolContext)
	return ctx
}

// ─── Hierarquia de perfis ─────────────────────────────────────────────────────

// spRoleLevel mapeia cada perfil a um nível numérico. Quanto maior, mais privilégios.
var spRoleLevel = map[string]int{
	"somente_leitura": 1,
	"gestor_filial":   2,
	"gestor_geral":    3,
	"admin_fbtax":     4,
}

// hasSpRole retorna true se o perfil do usuário satisfaz o nível mínimo exigido.
func hasSpRole(userRole, required string) bool {
	return spRoleLevel[userRole] >= spRoleLevel[required]
}

// CanWrite retorna true se o perfil permite escrita (gestor_filial ou acima).
func (s *FarolContext) CanWrite() bool {
	return hasSpRole(s.SpRole, "gestor_filial")
}

// CanApprove retorna true se o perfil permite aprovar/rejeitar propostas (gestor_filial ou acima).
func (s *FarolContext) CanApprove() bool {
	return hasSpRole(s.SpRole, "gestor_filial")
}

// IsAdminFbtax retorna true para admin_fbtax (acesso cross-tenant).
func (s *FarolContext) IsAdminFbtax() bool {
	return s.SpRole == "admin_fbtax"
}

// IsMasterTenant retorna true quando a empresa ativa pertence ao grupo MASTER
// (criado pela migration 024). Usado para gatear operações sensíveis que
// o frontend expõe apenas para usuários MASTER.
func (s *FarolContext) IsMasterTenant(db *sql.DB) bool {
	var groupName sql.NullString
	err := db.QueryRow(`
		SELECT COALESCE(eg.name, '')
		FROM companies c
		LEFT JOIN enterprise_groups eg ON eg.id = c.group_id
		WHERE c.id = $1::uuid
	`, s.EmpresaID).Scan(&groupName)
	if err != nil {
		return false
	}
	return groupName.String == "MASTER"
}

// TargetUserInSameTenant verifica se o usuário alvo pertence ao mesmo grupo
// empresarial do usuário autenticado (ou se o caller é MASTER, que vê todos).
// Previne cross-tenant updates/deletes.
func (s *FarolContext) TargetUserInSameTenant(db *sql.DB, targetUserID string) bool {
	if s.IsMasterTenant(db) {
		return true
	}
	var count int
	err := db.QueryRow(`
		SELECT COUNT(1) FROM (
			SELECT 1 FROM user_environments ue
			JOIN companies c ON c.id = ue.preferred_company_id
			WHERE ue.user_id = $1::uuid AND c.group_id = (
				SELECT group_id FROM companies WHERE id = $2::uuid
			)
			UNION
			SELECT 1 FROM farol.sp_user_filiais uf
			JOIN companies c ON c.id = uf.empresa_id
			WHERE uf.user_id = $1::uuid AND c.group_id = (
				SELECT group_id FROM companies WHERE id = $2::uuid
			)
		) x
	`, targetUserID, s.EmpresaID).Scan(&count)
	if err != nil {
		return false
	}
	return count > 0
}

// HasFilialAccess verifica se o usuário tem acesso à filial informada.
func (s *FarolContext) HasFilialAccess(filialID int) bool {
	if s.AllFiliais {
		return true
	}
	for _, id := range s.FilialIDs {
		if id == filialID {
			return true
		}
	}
	return false
}

// ─── Middleware ───────────────────────────────────────────────────────────────

// FarolAuthMiddleware valida JWT (via AuthMiddleware) e carrega o perfil Farol.
//
// requiredSpRole: perfil mínimo exigido (ex: "gestor_filial"). Passar "" para apenas validar
// autenticação sem checar perfil específico.
func FarolAuthMiddleware(db *sql.DB, next http.HandlerFunc, requiredSpRole string) http.HandlerFunc {
	// Encadeia sobre o AuthMiddleware herdado (auth.go). "" = sem exigência de role APU02.
	return AuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
		userID := GetUserIDFromContext(r)
		if userID == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Determina a empresa ativa (X-Company-ID header ou preferred_company_id)
		empresaID, err := GetEffectiveCompanyID(db, userID, r.Header.Get("X-Company-ID"))
		if err != nil || empresaID == "" {
			http.Error(w, "Could not determine active company", http.StatusUnauthorized)
			return
		}

		// Carrega sp_role, role (auth geral) e trial_ends_at do banco
		var spRole, userRole string
		var trialEndsAt sql.NullTime
		if err := db.QueryRow(
			"SELECT sp_role, role, trial_ends_at FROM users WHERE id = $1", userID,
		).Scan(&spRole, &userRole, &trialEndsAt); err != nil {
			if err == sql.ErrNoRows {
				http.Error(w, "User not found", http.StatusUnauthorized)
			} else {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
			return
		}

		// Verifica perfil mínimo exigido pela rota
		if requiredSpRole != "" && !hasSpRole(spRole, requiredSpRole) {
			http.Error(w, "Forbidden: Farol role insuficiente", http.StatusForbidden)
			return
		}

		// Enforcement contínuo da licença: bloqueia user se trial_ends_at passou.
		// role=admin (geral) bypassa para permitir MASTER renovar mesmo após expirar.
		if userRole != "admin" && trialEndsAt.Valid && trialEndsAt.Time.Before(time.Now()) {
			http.Error(w, "Licença expirada. Contate o administrador MASTER para renovar.", http.StatusForbidden)
			return
		}

		// Enforcement contínuo do bloqueio de empresa. role=admin bypassa.
		if userRole != "admin" {
			var blockedAt sql.NullTime
			_ = db.QueryRow(
				"SELECT blocked_at FROM companies WHERE id = $1::uuid", empresaID,
			).Scan(&blockedAt)
			if blockedAt.Valid {
				http.Error(w, "Empresa bloqueada. Contate o administrador MASTER.", http.StatusForbidden)
				return
			}
		}

		// Monta FarolContext com escopo de filiais
		spCtx := &FarolContext{
			UserID:    userID,
			SpRole:    spRole,
			EmpresaID: empresaID,
		}

		// admin_fbtax e gestor_geral têm acesso irrestrito às filiais do tenant
		if spRole == "admin_fbtax" || spRole == "gestor_geral" {
			spCtx.AllFiliais = true
		} else {
			// gestor_filial e somente_leitura: carrega filiais via sp_user_filiais
			rows, err := db.Query(`
				SELECT filial_id, all_filiais
				FROM farol.sp_user_filiais
				WHERE user_id = $1 AND empresa_id = $2
			`, userID, empresaID)
			if err == nil {
				defer rows.Close()
				for rows.Next() {
					var filialID *int
					var allFiliais bool
					if scanErr := rows.Scan(&filialID, &allFiliais); scanErr != nil {
						continue
					}
					if allFiliais {
						spCtx.AllFiliais = true
						spCtx.FilialIDs = nil
						break
					}
					if filialID != nil {
						spCtx.FilialIDs = append(spCtx.FilialIDs, *filialID)
					}
				}
			}
		}

		ctx := context.WithValue(r.Context(), SpContextKey, spCtx)
		next(w, r.WithContext(ctx))
	}, "")
}

// ─── SpMeHandler ─────────────────────────────────────────────────────────────

// SpMeHandler retorna o sp_role do usuário autenticado para a empresa ativa.
// GET /api/sp/me  — sem perfil mínimo (qualquer usuário Farol autenticado)
func SpMeHandler(db *sql.DB) http.HandlerFunc {
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
		w.Header().Set("Content-Type", "application/json")
		// Retorna apenas o sp_role; outros campos já estão no /api/auth/me
		type spMeResponse struct {
			SpRole string `json:"sp_role"`
		}
		json.NewEncoder(w).Encode(spMeResponse{SpRole: spCtx.SpRole})
	}
}

// ─── Helpers de resposta ──────────────────────────────────────────────────────

// RequireWrite aborta com 403 se o perfil não permite escrita.
func RequireWrite(spCtx *FarolContext, w http.ResponseWriter) bool {
	if !spCtx.CanWrite() {
		http.Error(w, "Forbidden: perfil somente_leitura não permite alterações", http.StatusForbidden)
		return false
	}
	return true
}

// RequireApprove aborta com 403 se o perfil não permite aprovação de propostas.
func RequireApprove(spCtx *FarolContext, w http.ResponseWriter) bool {
	if !spCtx.CanApprove() {
		http.Error(w, "Forbidden: apenas gestor_geral ou admin_fbtax podem aprovar propostas", http.StatusForbidden)
		return false
	}
	return true
}
