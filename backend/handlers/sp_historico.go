package handlers

// sp_historico.go — Histórico de Calibragem e Compliance
//
// Story 7.2 — Registro automático + fechamento de ciclo
// Story 7.3 — API de Histórico e Compliance
//
// GET  /api/sp/historico              → lista ciclos de calibragem
// POST /api/sp/historico/{id}/fechar  → fecha ciclo manualmente (calcula contagens finais)
// GET  /api/sp/historico/compliance   → indicadores por CD: última calibragem, dias decorridos

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// ─── DTOs ─────────────────────────────────────────────────────────────────────

type HistoricoResponse struct {
	ID             int64   `json:"id"`
	JobID          *string `json:"job_id,omitempty"`
	CdID           int     `json:"cd_id"`
	CdNome         string  `json:"cd_nome"`
	FilialNome     string  `json:"filial_nome"`
	TotalPropostas int     `json:"total_propostas"`
	Aprovadas      int     `json:"aprovadas"`
	Rejeitadas     int     `json:"rejeitadas"`
	Pendentes      int     `json:"pendentes"`
	CurvaA         int     `json:"curva_a"`
	CurvaB         int     `json:"curva_b"`
	CurvaC         int     `json:"curva_c"`
	ExecutadoPor   *string `json:"executado_por,omitempty"`
	ExecutadoEm    string  `json:"executado_em"`
	ConcluidoEm    *string `json:"concluido_em,omitempty"`
	Status         string  `json:"status"`
	Observacao     *string `json:"observacao,omitempty"`
}

type ComplianceCD struct {
	CdID              int     `json:"cd_id"`
	CdNome            string  `json:"cd_nome"`
	FilialNome        string  `json:"filial_nome"`
	CodFilial         int     `json:"cod_filial"`
	// Calibragem
	UltimaCalibragem  *string `json:"ultima_calibragem"`
	DiasDesdeUltima   *int    `json:"dias_desde_ultima"`
	UltimoStatus      *string `json:"ultimo_status"`
	TotalCiclos       int     `json:"total_ciclos"`
	// Importações
	UltimoImportEm    *string `json:"ultimo_import_em"`
	DiasDesdeImport   *int    `json:"dias_desde_import"`
	TotalImports      int     `json:"total_imports"`
	// Propostas pendentes
	PropostasPendentes  int  `json:"propostas_pendentes"`
	DiasOldestPendente  *int `json:"dias_oldest_pendente"`
	// Gestor responsável
	UltimoGestorNome  *string `json:"ultimo_gestor_nome"`
	// Status calculado
	StatusCompliance  string  `json:"status_compliance"` // ok|atencao|critico|aguardando_motor|nunca_iniciado
	Alerta            bool    `json:"alerta"`
}

// ─── Criar histórico (chamado pelo motor) ─────────────────────────────────────

// CriarHistorico insere um ciclo 'em_andamento' quando o motor é disparado.
// Retorna o ID criado (ou 0 em caso de erro silencioso).
func CriarHistorico(db *sql.DB, jobID, empresaID, userID string, cdID int) int64 {
	var id int64
	err := db.QueryRow(`
		INSERT INTO farol.sp_historico (job_id, cd_id, empresa_id, executado_por, executado_em)
		VALUES ($1, $2, $3, $4::uuid, now())
		RETURNING id
	`, jobID, cdID, empresaID, userID).Scan(&id)
	if err != nil {
		return 0
	}
	return id
}

// FecharHistoricoAuto fecha automaticamente o ciclo calculando contagens atuais das propostas.
// Chamado em background pelo motor após geração de propostas.
func FecharHistoricoAuto(db *sql.DB, historicoID int64, jobID string) {
	if historicoID == 0 {
		return
	}
	_, _ = db.Exec(`
		UPDATE farol.sp_historico h
		SET
			total_propostas = sub.total,
			aprovadas       = sub.aprovadas,
			rejeitadas      = sub.rejeitadas,
			pendentes       = sub.pendentes,
			curva_a         = sub.curva_a,
			curva_b         = sub.curva_b,
			curva_c         = sub.curva_c
		FROM (
			SELECT
				COUNT(*)                                              AS total,
				COUNT(*) FILTER (WHERE status = 'aprovada')          AS aprovadas,
				COUNT(*) FILTER (WHERE status = 'rejeitada')         AS rejeitadas,
				COUNT(*) FILTER (WHERE status = 'pendente')          AS pendentes,
				COUNT(*) FILTER (WHERE classe_venda = 'A')           AS curva_a,
				COUNT(*) FILTER (WHERE classe_venda = 'B')           AS curva_b,
				COUNT(*) FILTER (WHERE classe_venda NOT IN ('A','B') OR classe_venda IS NULL) AS curva_c
			FROM farol.sp_propostas
			WHERE job_id = $1
		) sub
		WHERE h.id = $2
	`, jobID, historicoID)
}

// ─── Handler: lista histórico ─────────────────────────────────────────────────

// SpHistoricoHandler lista os ciclos de calibragem da empresa.
// GET /api/sp/historico?cd_id=X&limit=50
func SpHistoricoHandler(db *sql.DB) http.HandlerFunc {
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

		cdIDFilter := r.URL.Query().Get("cd_id")
		limitStr   := r.URL.Query().Get("limit")
		limit := 50
		if limitStr != "" {
			fmt.Sscan(limitStr, &limit)
			if limit <= 0 || limit > 200 {
				limit = 50
			}
		}

		query := `
			SELECT h.id, h.job_id::text, h.cd_id,
			       cd.nome, f.nome,
			       h.total_propostas, h.aprovadas, h.rejeitadas, h.pendentes,
			       h.curva_a, h.curva_b, h.curva_c,
			       h.executado_por::text,
			       TO_CHAR(h.executado_em,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
			       TO_CHAR(h.concluido_em,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
			       h.status, h.observacao
			FROM farol.sp_historico h
			JOIN farol.sp_centros_dist cd ON cd.id = h.cd_id
			JOIN farol.sp_filiais f ON f.id = cd.filial_id
			WHERE h.empresa_id = $1
		`
		args := []any{spCtx.EmpresaID}
		if cdIDFilter != "" {
			query += " AND h.cd_id = $2"
			args = append(args, cdIDFilter)
		}
		query += fmt.Sprintf(" ORDER BY h.executado_em DESC LIMIT %d", limit)

		rows, err := db.Query(query, args...)
		if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var historico []HistoricoResponse
		for rows.Next() {
			var h HistoricoResponse
			if err := rows.Scan(
				&h.ID, &h.JobID, &h.CdID, &h.CdNome, &h.FilialNome,
				&h.TotalPropostas, &h.Aprovadas, &h.Rejeitadas, &h.Pendentes,
				&h.CurvaA, &h.CurvaB, &h.CurvaC,
				&h.ExecutadoPor, &h.ExecutadoEm, &h.ConcluidoEm,
				&h.Status, &h.Observacao,
			); err != nil {
				continue
			}
			historico = append(historico, h)
		}
		if historico == nil {
			historico = []HistoricoResponse{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(historico)
	}
}

// ─── Handler: fechar ciclo manualmente ───────────────────────────────────────

// SpHistoricoFecharHandler recalcula contagens e marca ciclo como 'concluido'.
// POST /api/sp/historico/{id}/fechar
func SpHistoricoFecharHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		spCtx := GetSpContext(r)
		if spCtx == nil || !spCtx.CanApprove() {
			http.Error(w, "Forbidden: gestor_geral+ necessário", http.StatusForbidden)
			return
		}

		idStr := strings.TrimPrefix(r.URL.Path, "/api/sp/historico/")
		idStr  = strings.TrimSuffix(idStr, "/fechar")
		var historicoID int64
		fmt.Sscan(idStr, &historicoID)
		if historicoID == 0 {
			http.Error(w, "ID inválido", http.StatusBadRequest)
			return
		}

		// Verifica que pertence à empresa
		var jobID string
		err := db.QueryRow(`
			SELECT COALESCE(job_id::text,'') FROM farol.sp_historico
			WHERE id = $1 AND empresa_id = $2
		`, historicoID, spCtx.EmpresaID).Scan(&jobID)
		if err == sql.ErrNoRows {
			http.Error(w, "Histórico não encontrado", http.StatusNotFound)
			return
		}

		// Recalcula e fecha
		_, err = db.Exec(`
			UPDATE farol.sp_historico h
			SET status = 'concluido', concluido_em = $1,
				total_propostas = sub.total,
				aprovadas       = sub.aprovadas,
				rejeitadas      = sub.rejeitadas,
				pendentes       = sub.pendentes,
				curva_a         = sub.curva_a,
				curva_b         = sub.curva_b,
				curva_c         = sub.curva_c
			FROM (
				SELECT
					COUNT(*)                                              AS total,
					COUNT(*) FILTER (WHERE status = 'aprovada')          AS aprovadas,
					COUNT(*) FILTER (WHERE status = 'rejeitada')         AS rejeitadas,
					COUNT(*) FILTER (WHERE status = 'pendente')          AS pendentes,
					COUNT(*) FILTER (WHERE classe_venda = 'A')           AS curva_a,
					COUNT(*) FILTER (WHERE classe_venda = 'B')           AS curva_b,
					COUNT(*) FILTER (WHERE classe_venda NOT IN ('A','B') OR classe_venda IS NULL) AS curva_c
				FROM farol.sp_propostas
				WHERE job_id = $2
			) sub
			WHERE h.id = $3
		`, time.Now().UTC(), jobID, historicoID)
		if err != nil {
			http.Error(w, "Erro ao fechar ciclo: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"message": "Ciclo fechado com sucesso"})
	}
}

// ─── Cálculo de status de compliance ─────────────────────────────────────────

func calcularStatusCompliance(c ComplianceCD) string {
	// Nunca houve qualquer import
	if c.TotalImports == 0 {
		return "nunca_iniciado"
	}
	// Tem import mas motor nunca foi rodado (0 ciclos)
	if c.TotalCiclos == 0 {
		if c.DiasDesdeImport != nil && *c.DiasDesdeImport > 7 {
			return "critico" // Importou há mais de 7 dias e não rodou o motor
		}
		return "aguardando_motor"
	}
	// Tem ciclos — verifica recência
	if c.DiasDesdeUltima != nil {
		if *c.DiasDesdeUltima > 60 {
			return "critico"
		}
		if *c.DiasDesdeUltima > 30 {
			return "atencao"
		}
	}
	// Propostas paradas
	if c.DiasOldestPendente != nil && *c.DiasOldestPendente > 14 {
		return "critico"
	}
	if c.DiasOldestPendente != nil && *c.DiasOldestPendente > 7 {
		return "atencao"
	}
	return "ok"
}

// ─── Handler: compliance ──────────────────────────────────────────────────────

// SpComplianceHandler retorna indicadores de compliance por CD.
// GET /api/sp/historico/compliance?filial_id=X
// Alerta se última calibragem > 30 dias ou CD nunca calibrado.
func SpComplianceHandler(db *sql.DB) http.HandlerFunc {
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

		filialFilter := r.URL.Query().Get("filial_id")

		query := `
			SELECT
				cd.id, cd.nome, f.nome, f.cod_filial,
				MAX(h.executado_em) AS ultima_calibragem,
				EXTRACT(DAY FROM now() - MAX(h.executado_em))::int AS dias_desde_ultima,
				(SELECT h2.status FROM farol.sp_historico h2
				 WHERE h2.cd_id = cd.id AND h2.empresa_id = $1
				 ORDER BY h2.executado_em DESC LIMIT 1) AS ultimo_status,
				COUNT(DISTINCT h.id) AS total_ciclos,
				MAX(j.created_at)   AS ultimo_import,
				EXTRACT(DAY FROM now() - MAX(j.created_at))::int AS dias_desde_import,
				COUNT(DISTINCT j.id) AS total_imports,
				COUNT(p.id) FILTER (WHERE p.status = 'pendente') AS propostas_pendentes,
				EXTRACT(DAY FROM now() - MIN(p.created_at) FILTER (WHERE p.status = 'pendente'))::int AS dias_oldest_pendente,
				(SELECT u.full_name FROM farol.sp_historico h3
				 JOIN public.users u ON u.id = h3.executado_por
				 WHERE h3.cd_id = cd.id AND h3.empresa_id = $1
				 ORDER BY h3.executado_em DESC LIMIT 1) AS ultimo_gestor
			FROM farol.sp_centros_dist cd
			JOIN farol.sp_filiais f ON f.id = cd.filial_id
			LEFT JOIN farol.sp_historico h ON h.cd_id = cd.id AND h.empresa_id = $1
			LEFT JOIN farol.sp_csv_jobs j ON j.cd_id = cd.id AND j.empresa_id = $1
			LEFT JOIN farol.sp_propostas p ON p.cd_id = cd.id AND p.empresa_id = $1
			WHERE cd.empresa_id = $1 AND cd.ativo = true
		`
		args := []any{spCtx.EmpresaID}
		if filialFilter != "" {
			query += " AND f.id = $2"
			args = append(args, filialFilter)
		}
		query += " GROUP BY cd.id, cd.nome, f.id, f.nome, f.cod_filial ORDER BY f.nome, cd.nome"

		rows, err := db.Query(query, args...)
		if err != nil {
			http.Error(w, "Database error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var cds []ComplianceCD
		for rows.Next() {
			var c ComplianceCD
			if err := rows.Scan(
				&c.CdID, &c.CdNome, &c.FilialNome, &c.CodFilial,
				&c.UltimaCalibragem, &c.DiasDesdeUltima,
				&c.UltimoStatus, &c.TotalCiclos,
				&c.UltimoImportEm, &c.DiasDesdeImport, &c.TotalImports,
				&c.PropostasPendentes, &c.DiasOldestPendente,
				&c.UltimoGestorNome,
			); err != nil {
				continue
			}
			c.StatusCompliance = calcularStatusCompliance(c)
			c.Alerta = c.StatusCompliance == "critico" || c.StatusCompliance == "atencao" ||
				c.StatusCompliance == "nunca_iniciado" || c.StatusCompliance == "aguardando_motor"
			cds = append(cds, c)
		}
		if cds == nil {
			cds = []ComplianceCD{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cds)
	}
}
