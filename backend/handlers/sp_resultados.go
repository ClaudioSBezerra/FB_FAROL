package handlers

// sp_resultados.go — Painel de Resultados Contratuais (Epic 9)
//
// Story 9.1 — KPIs contratuais Grupo JC: calibração, ofensores A/B,
//             caixas ociosas, reposições emergenciais e acessos picking (90d).
//
// GET /api/sp/resultados?cd_id=X  (cd_id opcional)

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
)

// ─── DTOs ─────────────────────────────────────────────────────────────────────

// CicloKPI — métricas de um job/ciclo
type CicloKPI struct {
	JobID             string  `json:"job_id"`
	CicloNum          int     `json:"ciclo_num"`          // 1=mais recente, 4=mais antigo
	CriadoEm          string  `json:"criado_em"`
	TotalEnderecos    int     `json:"total_enderecos"`
	CalibradosOk      int     `json:"calibrados_ok"`
	PctCalibrados     float64 `json:"pct_calibrados"`     // calculado em Go
	OfensoresFaltaAB  int     `json:"ofensores_falta_ab"`
	CaixasOciosas     int     `json:"caixas_ociosas"`
	CaixasAprovadas   int     `json:"caixas_aprovadas"`
	PctRealocado      float64 `json:"pct_realocado"`      // calculado em Go
	AcessosEmergencia int     `json:"acessos_emergencia"`
	AcessosTotal      int     `json:"acessos_total"`
}

// SpResultadosCD — um CD com seus últimos N ciclos
type SpResultadosCD struct {
	CdID       int        `json:"cd_id"`
	CdNome     string     `json:"cd_nome"`
	FilialNome string     `json:"filial_nome"`
	Ciclos     []CicloKPI `json:"ciclos"` // 1..4 itens, índice 0 = mais recente
}

// SpResultadosResponse — resposta completa
type SpResultadosResponse struct {
	Empresa *CicloKPI        `json:"empresa"` // soma ponderada dos ciclos mais recentes de cada CD; nil se sem dados
	CDs     []SpResultadosCD `json:"cds"`
}

// ─── Handler ─────────────────────────────────────────────────────────────────

// SpResultadosHandler retorna os KPIs contratuais dos últimos 4 ciclos por CD.
// GET /api/sp/resultados?cd_id=X
//
// Filtro cd_id é opcional. Quando omitido, retorna todos os CDs da empresa.
// Quando fornecido, valida pertencimento à empresa antes de executar a query.
//
// Nota: delta é GENERATED ALWAYS STORED em sp_propostas — não inserir manualmente.
// A query usa CTEs separados (end_agg, emerg_agg, prop_agg) para evitar row
// multiplication no SUM(qt_acesso_90) causado por LEFT JOIN 1:N.
func SpResultadosHandler(db *sql.DB) http.HandlerFunc {
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

		// ── Validação de cd_id (F5) ──────────────────────────────────────────
		cdIDStr := r.URL.Query().Get("cd_id")
		var cdIDFilter *int
		if cdIDStr != "" {
			parsed, err := strconv.Atoi(cdIDStr)
			if err != nil {
				http.Error(w, "cd_id inválido", http.StatusBadRequest)
				return
			}
			// Verifica que o CD pertence à empresa do token
			var exists bool
			err = db.QueryRow(
				`SELECT EXISTS(SELECT 1 FROM farol.sp_centros_dist cd
				              JOIN farol.sp_filiais f ON f.id = cd.filial_id
				              WHERE cd.id = $1 AND f.empresa_id = $2)`,
				parsed, spCtx.EmpresaID,
			).Scan(&exists)
			if err != nil || !exists {
				http.Error(w, "CD não encontrado", http.StatusNotFound)
				return
			}
			cdIDFilter = &parsed
		}

		// ── Query CTE ────────────────────────────────────────────────────────
		// CTEs separados para sp_enderecos e sp_propostas evitam row multiplication:
		// end_agg  → total_enderecos, acessos_total   (sem JOIN com propostas)
		// emerg_agg→ acessos_emergencia               (enderecos com proposta delta>0)
		// prop_agg → calibrados_ok, ofensores, caixas (somente de sp_propostas)
		query := `
			WITH ultimos_jobs AS (
				SELECT j.id AS job_id, j.cd_id, j.created_at,
				       cd.nome AS cd_nome, f.nome AS filial_nome,
				       ROW_NUMBER() OVER (PARTITION BY j.cd_id ORDER BY j.created_at DESC) AS rn
				FROM farol.sp_csv_jobs j
				JOIN farol.sp_centros_dist cd ON cd.id = j.cd_id
				JOIN farol.sp_filiais f ON f.id = cd.filial_id
				WHERE j.empresa_id = $1 AND j.status = 'done'`

		args := []any{spCtx.EmpresaID}
		if cdIDFilter != nil {
			query += " AND j.cd_id = $2"
			args = append(args, *cdIDFilter)
		}

		query += `
			),
			jobs_top4 AS (SELECT * FROM ultimos_jobs WHERE rn <= 4),
			end_agg AS (
				SELECT e.job_id,
				       COUNT(*)                         AS total_enderecos,
				       COALESCE(SUM(e.qt_acesso_90), 0) AS acessos_total
				FROM farol.sp_enderecos e
				WHERE e.job_id IN (SELECT job_id FROM jobs_top4)
				GROUP BY e.job_id
			),
			emerg_agg AS (
				SELECT e.job_id,
				       COALESCE(SUM(e.qt_acesso_90), 0) AS acessos_emergencia
				FROM farol.sp_enderecos e
				WHERE e.job_id IN (SELECT job_id FROM jobs_top4)
				  AND EXISTS (
				      SELECT 1 FROM farol.sp_propostas p
				      WHERE p.job_id = e.job_id AND p.endereco_id = e.id AND p.delta > 0
				  )
				GROUP BY e.job_id
			),
			prop_agg AS (
				SELECT p.job_id,
				       COUNT(*) FILTER (WHERE p.delta = 0)                                              AS calibrados_ok,
				       COUNT(*) FILTER (WHERE p.delta > 0 AND p.classe_venda IN ('A','B'))               AS ofensores_falta_ab,
				       COALESCE(SUM(ABS(p.delta)) FILTER (WHERE p.delta < 0), 0)                         AS caixas_ociosas,
				       COALESCE(SUM(ABS(p.delta)) FILTER (WHERE p.delta < 0 AND p.status = 'aprovada'), 0) AS caixas_aprovadas
				FROM farol.sp_propostas p
				WHERE p.job_id IN (SELECT job_id FROM jobs_top4)
				GROUP BY p.job_id
			)
			SELECT
				jt.cd_id, jt.cd_nome, jt.filial_nome,
				jt.job_id::text,
				TO_CHAR(jt.created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"') AS criado_em,
				jt.rn AS ciclo_num,
				COALESCE(ea.total_enderecos,    0) AS total_enderecos,
				COALESCE(pa.calibrados_ok,      0) AS calibrados_ok,
				COALESCE(pa.ofensores_falta_ab, 0) AS ofensores_falta_ab,
				COALESCE(pa.caixas_ociosas,     0) AS caixas_ociosas,
				COALESCE(pa.caixas_aprovadas,   0) AS caixas_aprovadas,
				COALESCE(em.acessos_emergencia, 0) AS acessos_emergencia,
				COALESCE(ea.acessos_total,      0) AS acessos_total
			FROM jobs_top4 jt
			LEFT JOIN end_agg   ea ON ea.job_id = jt.job_id
			LEFT JOIN emerg_agg em ON em.job_id = jt.job_id
			LEFT JOIN prop_agg  pa ON pa.job_id = jt.job_id
			ORDER BY cd_id, ciclo_num`

		rows, err := db.Query(query, args...)
		if err != nil {
			http.Error(w, "Database error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		// ── Agrupar por CD ───────────────────────────────────────────────────
		cdMap := make(map[int]*SpResultadosCD)
		cdOrder := []int{}

		for rows.Next() {
			var (
				cdID, cicloNum                                          int
				cdNome, filialNome, jobID, criadoEm                    string
				totalEnderecos, calibradosOk, ofensoresFaltaAB         int
				caixasOciosas, caixasAprovadas                         int
				acessosEmergencia, acessosTotal                        int
			)
			if err := rows.Scan(
				&cdID, &cdNome, &filialNome, &jobID, &criadoEm, &cicloNum,
				&totalEnderecos, &calibradosOk, &ofensoresFaltaAB,
				&caixasOciosas, &caixasAprovadas,
				&acessosEmergencia, &acessosTotal,
			); err != nil {
				continue
			}

			ciclo := CicloKPI{
				JobID:             jobID,
				CicloNum:          cicloNum,
				CriadoEm:          criadoEm,
				TotalEnderecos:    totalEnderecos,
				CalibradosOk:      calibradosOk,
				PctCalibrados:     safeDiv(float64(calibradosOk), float64(totalEnderecos)) * 100,
				OfensoresFaltaAB:  ofensoresFaltaAB,
				CaixasOciosas:     caixasOciosas,
				CaixasAprovadas:   caixasAprovadas,
				PctRealocado:      safeDiv(float64(caixasAprovadas), float64(caixasOciosas)) * 100,
				AcessosEmergencia: acessosEmergencia,
				AcessosTotal:      acessosTotal,
			}

			if _, ok := cdMap[cdID]; !ok {
				cdMap[cdID] = &SpResultadosCD{
					CdID:       cdID,
					CdNome:     cdNome,
					FilialNome: filialNome,
					Ciclos:     []CicloKPI{},
				}
				cdOrder = append(cdOrder, cdID)
			}
			cdMap[cdID].Ciclos = append(cdMap[cdID].Ciclos, ciclo)
		}

		// ── Montar slice de CDs na ordem de inserção ─────────────────────────
		cds := make([]SpResultadosCD, 0, len(cdOrder))
		for _, id := range cdOrder {
			cds = append(cds, *cdMap[id])
		}

		// ── Calcular empresa consolidada ─────────────────────────────────────
		// Pega o ciclo mais recente (Ciclos[0]) de cada CD e soma campos absolutos.
		// Percentuais: média ponderada por total_enderecos.
		// Empresa = nil se nenhum CD tem ciclos (F4).
		var empresa *CicloKPI
		if len(cds) > 0 {
			emp := CicloKPI{}
			for _, cd := range cds {
				if len(cd.Ciclos) == 0 {
					continue
				}
				c := cd.Ciclos[0] // ciclo mais recente
				emp.TotalEnderecos    += c.TotalEnderecos
				emp.CalibradosOk      += c.CalibradosOk
				emp.OfensoresFaltaAB  += c.OfensoresFaltaAB
				emp.CaixasOciosas     += c.CaixasOciosas
				emp.CaixasAprovadas   += c.CaixasAprovadas
				emp.AcessosEmergencia += c.AcessosEmergencia
				emp.AcessosTotal      += c.AcessosTotal
			}
			if emp.TotalEnderecos > 0 {
				// Percentuais: derivados dos absolutos já somados (equivale à média ponderada por total_enderecos)
				emp.PctCalibrados = safeDiv(float64(emp.CalibradosOk), float64(emp.TotalEnderecos)) * 100
				emp.PctRealocado  = safeDiv(float64(emp.CaixasAprovadas), float64(emp.CaixasOciosas)) * 100
				empresa = &emp
			}
		}

		// ── Resposta ─────────────────────────────────────────────────────────
		resp := SpResultadosResponse{
			Empresa: empresa,
			CDs:     cds,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

// safeDiv retorna 0 quando o denominador é zero.
func safeDiv(num, den float64) float64 {
	if den == 0 {
		return 0
	}
	return num / den
}

// ─── Histórico (gráfico de evolução) ─────────────────────────────────────────

// HistoricoKPI — snapshot de um job para o gráfico de evolução
type HistoricoKPI struct {
	JobID           string  `json:"job_id"`
	CriadoEm        string  `json:"criado_em"`
	TotalEnderecos  int     `json:"total_enderecos"`
	CalibradosOk    int     `json:"calibrados_ok"`
	PctCalibrados   float64 `json:"pct_calibrados"`
	OfensoresFalta  int     `json:"ofensores_falta"`   // Curva A/B delta > 0
	OfensoresEspaco int     `json:"ofensores_espaco"`  // COUNT delta < 0
}

// SpHistoricoResponse — série histórica de um CD
type SpHistoricoResponse struct {
	CdID       int            `json:"cd_id"`
	CdNome     string         `json:"cd_nome"`
	FilialNome string         `json:"filial_nome"`
	Pontos     []HistoricoKPI `json:"pontos"` // do mais antigo ao mais recente
}

// SpResultadosHistoricoHandler retorna a série histórica de KPIs de um CD (todos os jobs).
// GET /api/sp/resultados/historico?cd_id=X  (cd_id obrigatório)
func SpResultadosHistoricoHandler(db *sql.DB) http.HandlerFunc {
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

		cdIDStr := r.URL.Query().Get("cd_id")
		if cdIDStr == "" {
			http.Error(w, "cd_id obrigatório", http.StatusBadRequest)
			return
		}
		cdID, err := strconv.Atoi(cdIDStr)
		if err != nil {
			http.Error(w, "cd_id inválido", http.StatusBadRequest)
			return
		}

		// Valida pertencimento à empresa
		var exists bool
		err = db.QueryRow(
			`SELECT EXISTS(SELECT 1 FROM farol.sp_centros_dist cd
			              JOIN farol.sp_filiais f ON f.id = cd.filial_id
			              WHERE cd.id = $1 AND f.empresa_id = $2)`,
			cdID, spCtx.EmpresaID,
		).Scan(&exists)
		if err != nil || !exists {
			http.Error(w, "CD não encontrado", http.StatusNotFound)
			return
		}

		rows, err := db.Query(`
			WITH todos_jobs AS (
				SELECT j.id AS job_id, j.created_at,
				       cd.nome AS cd_nome, f.nome AS filial_nome
				FROM farol.sp_csv_jobs j
				JOIN farol.sp_centros_dist cd ON cd.id = j.cd_id
				JOIN farol.sp_filiais f ON f.id = cd.filial_id
				WHERE j.empresa_id = $1 AND j.cd_id = $2 AND j.status = 'done'
			),
			end_agg AS (
				SELECT e.job_id,
				       COUNT(*) AS total_enderecos
				FROM farol.sp_enderecos e
				WHERE e.job_id IN (SELECT job_id FROM todos_jobs)
				GROUP BY e.job_id
			),
			prop_agg AS (
				SELECT p.job_id,
				       COUNT(*) FILTER (WHERE p.delta = 0)                               AS calibrados_ok,
				       COUNT(*) FILTER (WHERE p.delta > 0 AND p.classe_venda IN ('A','B')) AS ofensores_falta,
				       COUNT(*) FILTER (WHERE p.delta < 0)                               AS ofensores_espaco
				FROM farol.sp_propostas p
				WHERE p.job_id IN (SELECT job_id FROM todos_jobs)
				GROUP BY p.job_id
			)
			SELECT
				j.cd_nome, j.filial_nome,
				j.job_id::text,
				TO_CHAR(j.created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"') AS criado_em,
				COALESCE(ea.total_enderecos,   0) AS total_enderecos,
				COALESCE(pa.calibrados_ok,     0) AS calibrados_ok,
				COALESCE(pa.ofensores_falta,   0) AS ofensores_falta,
				COALESCE(pa.ofensores_espaco,  0) AS ofensores_espaco
			FROM todos_jobs j
			LEFT JOIN end_agg   ea ON ea.job_id = j.job_id
			LEFT JOIN prop_agg  pa ON pa.job_id = j.job_id
			ORDER BY j.created_at ASC
		`, spCtx.EmpresaID, cdID)
		if err != nil {
			http.Error(w, "Database error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		resp := SpHistoricoResponse{CdID: cdID, Pontos: []HistoricoKPI{}}
		for rows.Next() {
			var (
				cdNome, filialNome, jobID, criadoEm string
				totalEnderecos, calibradosOk         int
				ofensoresFalta, ofensoresEspaco      int
			)
			if err := rows.Scan(
				&cdNome, &filialNome, &jobID, &criadoEm,
				&totalEnderecos, &calibradosOk,
				&ofensoresFalta, &ofensoresEspaco,
			); err != nil {
				continue
			}
			if resp.CdNome == "" {
				resp.CdNome = cdNome
				resp.FilialNome = filialNome
			}
			resp.Pontos = append(resp.Pontos, HistoricoKPI{
				JobID:           jobID,
				CriadoEm:        criadoEm,
				TotalEnderecos:  totalEnderecos,
				CalibradosOk:    calibradosOk,
				PctCalibrados:   safeDiv(float64(calibradosOk), float64(totalEnderecos)) * 100,
				OfensoresFalta:  ofensoresFalta,
				OfensoresEspaco: ofensoresEspaco,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}
