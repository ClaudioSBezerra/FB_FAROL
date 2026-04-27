package handlers

// sp_audit.go — Audit log: escrita e consulta
//
// Helper writeAuditLog() grava no sp_audit_log (imutável).
// GET /api/sp/admin/audit-log retorna os registros (apenas MASTER/admin_fbtax).

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
)

// writeAuditLog insere uma entrada no audit log.
// Fallback quando não há transação disponível — falha apenas é logada.
// Prefira writeAuditLogTx() para operações sensíveis (limpar dados, CRUD de usuário).
func writeAuditLog(db *sql.DB, empresaID, userID, entidade, entidadeID, acao string, payload any) {
	var payloadJSON []byte
	if payload != nil {
		payloadJSON, _ = json.Marshal(payload)
	}
	_, err := db.Exec(`
		INSERT INTO farol.sp_audit_log (empresa_id, user_id, entidade, entidade_id, acao, payload)
		VALUES ($1, $2::uuid, $3, $4, $5, $6)
	`, empresaID, userID, entidade, entidadeID, acao, payloadJSON)
	if err != nil {
		log.Printf("writeAuditLog: erro CRÍTICO ao gravar auditoria (%s/%s/%s): %v",
			entidade, entidadeID, acao, err)
	}
}

// writeAuditLogTx insere uma entrada no audit log usando a transação do caller.
// Retorna erro para que o caller possa rollback se o audit falhar — garantindo
// que operações sensíveis nunca sejam aplicadas sem registro.
func writeAuditLogTx(tx *sql.Tx, empresaID, userID, entidade, entidadeID, acao string, payload any) error {
	var payloadJSON []byte
	if payload != nil {
		var err error
		payloadJSON, err = json.Marshal(payload)
		if err != nil {
			return err
		}
	}
	_, err := tx.Exec(`
		INSERT INTO farol.sp_audit_log (empresa_id, user_id, entidade, entidade_id, acao, payload)
		VALUES ($1, $2::uuid, $3, $4, $5, $6)
	`, empresaID, userID, entidade, entidadeID, acao, payloadJSON)
	return err
}

// ─── Consulta do Audit Log ───────────────────────────────────────────────────

type AuditLogEntry struct {
	ID         int64           `json:"id"`
	UserID     *string         `json:"user_id,omitempty"`
	UserEmail  *string         `json:"user_email,omitempty"`
	UserName   *string         `json:"user_name,omitempty"`
	Entidade   string          `json:"entidade"`
	EntidadeID string          `json:"entidade_id"`
	Acao       string          `json:"acao"`
	Payload    json.RawMessage `json:"payload,omitempty"`
	CreatedAt  string          `json:"created_at"`
}

// SpAuditLogHandler lista o audit log da empresa.
// GET /api/sp/admin/audit-log?limit=100
func SpAuditLogHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		spCtx := GetSpContext(r)
		if spCtx == nil || !spCtx.IsAdminFbtax() || !spCtx.IsMasterTenant(db) {
			http.Error(w, "Forbidden: apenas usuários MASTER podem visualizar o audit log", http.StatusForbidden)
			return
		}

		limit := 200
		if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v <= 1000 {
			limit = v
		}

		rows, err := db.Query(`
			SELECT a.id, a.user_id::text, u.email, u.full_name,
			       a.entidade, a.entidade_id, a.acao, a.payload,
			       TO_CHAR(a.created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
			FROM farol.sp_audit_log a
			LEFT JOIN users u ON u.id = a.user_id
			WHERE a.empresa_id = $1
			ORDER BY a.created_at DESC
			LIMIT $2
		`, spCtx.EmpresaID, limit)
		if err != nil {
			http.Error(w, "Database error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var entries []AuditLogEntry
		for rows.Next() {
			var e AuditLogEntry
			if err := rows.Scan(&e.ID, &e.UserID, &e.UserEmail, &e.UserName,
				&e.Entidade, &e.EntidadeID, &e.Acao, &e.Payload, &e.CreatedAt); err != nil {
				continue
			}
			entries = append(entries, e)
		}
		if entries == nil {
			entries = []AuditLogEntry{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(entries)
	}
}
