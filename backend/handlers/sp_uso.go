package handlers

// sp_uso.go — Rastreamento de uso por módulo (E1)
//
// POST /api/sp/uso          → ingestão de evento de uso (qualquer sp_role autenticado)
// GET  /api/sp/admin/uso    → relatório agregado por usuário × módulo (MASTER-only)

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

// ─── POST /api/sp/uso ────────────────────────────────────────────────────────

func SpUsageIngestHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		var body struct {
			Modulo     string `json:"modulo"`
			Caminho    string `json:"caminho"`
			DuracaoSeg int    `json:"duracao_seg"`
			SessaoID   string `json:"sessao_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Modulo == "" || body.Caminho == "" {
			http.Error(w, "payload inválido", http.StatusBadRequest)
			return
		}
		// Visitas muito curtas (< 2s) ou absurdamente longas (> 2h) são descartadas
		if body.DuracaoSeg < 2 {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if body.DuracaoSeg > 7200 {
			body.DuracaoSeg = 7200
		}

		// Fire-and-forget: não bloqueamos o response
		go func() {
			db.Exec(`
				INSERT INTO farol.sp_usage_log
					(empresa_id, user_id, modulo, caminho, duracao_seg, sessao_id)
				VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6)
			`, spCtx.EmpresaID, spCtx.UserID, body.Modulo, body.Caminho, body.DuracaoSeg, body.SessaoID)
		}()

		w.WriteHeader(http.StatusNoContent)
	}
}

// ─── GET /api/sp/admin/uso ───────────────────────────────────────────────────

type UsageRow struct {
	UserID       string    `json:"user_id"`
	UserEmail    string    `json:"user_email"`
	UserName     string    `json:"user_name"`
	EmpresaNome  string    `json:"empresa_nome"`
	Modulo       string    `json:"modulo"`
	TotalVisitas int64     `json:"total_visitas"`
	TempoTotal   int64     `json:"tempo_total_seg"`
	TempoMedio   int       `json:"tempo_medio_seg"`
	UltimaVisita time.Time `json:"ultima_visita"`
}

func SpUsageReportHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		if !spCtx.IsMasterTenant(db) {
			http.Error(w, "Acesso restrito ao administrador MASTER", http.StatusForbidden)
			return
		}

		diasStr := r.URL.Query().Get("dias")
		dias, err := strconv.Atoi(diasStr)
		if err != nil || dias <= 0 {
			dias = 30
		}
		if dias > 365 {
			dias = 365
		}

		rows, err := db.Query(`
			SELECT
				ul.user_id::text,
				COALESCE(u.email, ''),
				COALESCE(u.full_name, u.email, ''),
				COALESCE(c.name, ''),
				ul.modulo,
				COUNT(*)                        AS total_visitas,
				COALESCE(SUM(ul.duracao_seg), 0) AS tempo_total_seg,
				COALESCE(AVG(ul.duracao_seg)::int, 0) AS tempo_medio_seg,
				MAX(ul.created_at)              AS ultima_visita
			FROM farol.sp_usage_log ul
			LEFT JOIN users u ON u.id = ul.user_id
			LEFT JOIN companies c ON c.id = ul.empresa_id
			WHERE ul.created_at >= NOW() - (INTERVAL '1 day' * $1)
			GROUP BY ul.user_id, u.email, u.full_name, c.name, ul.modulo
			ORDER BY ultima_visita DESC
			LIMIT 2000
		`, dias)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		result := []UsageRow{}
		for rows.Next() {
			var row UsageRow
			if err := rows.Scan(
				&row.UserID, &row.UserEmail, &row.UserName, &row.EmpresaNome,
				&row.Modulo, &row.TotalVisitas, &row.TempoTotal, &row.TempoMedio, &row.UltimaVisita,
			); err != nil {
				continue
			}
			result = append(result, row)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}
}
