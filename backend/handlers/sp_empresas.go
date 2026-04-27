package handlers

// sp_empresas.go — Gestão de empresas (bloqueio/desbloqueio) MASTER-only
//
// Endpoints:
//   GET  /api/sp/admin/empresas                → lista empresas com status de bloqueio
//   POST /api/sp/admin/empresas/{id}/bloquear  → bloqueia empresa (body: {motivo})
//   POST /api/sp/admin/empresas/{id}/desbloquear → desbloqueia empresa
//
// Todos protegidos por MASTER (IsMasterTenant) e auditados em sp_audit_log.

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"
)

type EmpresaBloqueio struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	TradeName      string  `json:"trade_name"`
	GroupID        string  `json:"group_id"`
	GroupName      string  `json:"group_name"`
	BlockedAt      *string `json:"blocked_at,omitempty"`
	BlockedReason  *string `json:"blocked_reason,omitempty"`
	BlockedByEmail *string `json:"blocked_by_email,omitempty"`
}

// SpListEmpresasHandler lista empresas (MASTER vê todas, demais não autorizados).
// GET /api/sp/admin/empresas
func SpListEmpresasHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		spCtx := GetSpContext(r)
		if spCtx == nil || !spCtx.IsAdminFbtax() || !spCtx.IsMasterTenant(db) {
			http.Error(w, "Forbidden: apenas MASTER", http.StatusForbidden)
			return
		}

		rows, err := db.Query(`
			SELECT c.id::text, c.name, COALESCE(c.trade_name,''), c.group_id::text,
			       COALESCE(eg.name,''),
			       TO_CHAR(c.blocked_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
			       c.blocked_reason,
			       (SELECT email FROM users WHERE id = c.blocked_by)
			FROM companies c
			LEFT JOIN enterprise_groups eg ON eg.id = c.group_id
			ORDER BY eg.name, c.name
		`)
		if err != nil {
			http.Error(w, "Database error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var empresas []EmpresaBloqueio
		for rows.Next() {
			var e EmpresaBloqueio
			var blockedAt sql.NullString
			if err := rows.Scan(&e.ID, &e.Name, &e.TradeName, &e.GroupID, &e.GroupName,
				&blockedAt, &e.BlockedReason, &e.BlockedByEmail); err != nil {
				continue
			}
			if blockedAt.Valid && blockedAt.String != "" {
				s := blockedAt.String
				e.BlockedAt = &s
			}
			empresas = append(empresas, e)
		}
		if empresas == nil {
			empresas = []EmpresaBloqueio{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(empresas)
	}
}

// SpEmpresaBloqueioHandler despacha bloquear/desbloquear por sufixo do path.
// POST /api/sp/admin/empresas/{id}/bloquear    body: {"motivo": "..."}
// POST /api/sp/admin/empresas/{id}/desbloquear
func SpEmpresaBloqueioHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		spCtx := GetSpContext(r)
		if spCtx == nil || !spCtx.IsAdminFbtax() || !spCtx.IsMasterTenant(db) {
			http.Error(w, "Forbidden: apenas MASTER", http.StatusForbidden)
			return
		}

		// /api/sp/admin/empresas/{id}/bloquear ou /desbloquear
		path := strings.TrimPrefix(r.URL.Path, "/api/sp/admin/empresas/")
		parts := strings.Split(path, "/")
		if len(parts) != 2 {
			http.Error(w, "URL inválida", http.StatusBadRequest)
			return
		}
		empresaID := parts[0]
		action := parts[1]
		if empresaID == "" || (action != "bloquear" && action != "desbloquear") {
			http.Error(w, "Ação inválida", http.StatusBadRequest)
			return
		}
		if empresaID == spCtx.EmpresaID {
			http.Error(w, "Não é possível bloquear sua própria empresa ativa", http.StatusBadRequest)
			return
		}

		tx, err := db.Begin()
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()

		var res sql.Result
		var payload map[string]any

		if action == "bloquear" {
			var body struct {
				Motivo string `json:"motivo"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if strings.TrimSpace(body.Motivo) == "" {
				http.Error(w, "motivo obrigatório para bloquear", http.StatusBadRequest)
				return
			}
			res, err = tx.Exec(`
				UPDATE companies
				SET blocked_at = $1, blocked_reason = $2, blocked_by = $3::uuid
				WHERE id = $4::uuid AND blocked_at IS NULL
			`, time.Now().UTC(), body.Motivo, spCtx.UserID, empresaID)
			payload = map[string]any{"motivo": body.Motivo}
		} else {
			res, err = tx.Exec(`
				UPDATE companies
				SET blocked_at = NULL, blocked_reason = NULL, blocked_by = NULL
				WHERE id = $1::uuid AND blocked_at IS NOT NULL
			`, empresaID)
			payload = map[string]any{}
		}

		if err != nil {
			http.Error(w, "Database error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			http.Error(w, "Empresa não encontrada ou já está no estado solicitado", http.StatusNotFound)
			return
		}

		// Audit log na mesma tx
		acao := "bloquear_empresa"
		if action == "desbloquear" {
			acao = "desbloquear_empresa"
		}
		if err := writeAuditLogTx(tx, spCtx.EmpresaID, spCtx.UserID, "empresa", empresaID, acao, payload); err != nil {
			http.Error(w, "Erro ao gravar auditoria: "+err.Error(), http.StatusInternalServerError)
			return
		}

		if err := tx.Commit(); err != nil {
			http.Error(w, "Commit error", http.StatusInternalServerError)
			return
		}

		log.Printf("SpEmpresaBloqueio: %s empresa=%s por %s", acao, empresaID, spCtx.UserID)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"message": "OK"})
	}
}
