package handlers

// sp_propostas.go — Dashboard de Urgência e Aprovação de Propostas
//
// Story 5.1 — API do Dashboard de Urgência
//
// GET  /api/sp/propostas              → lista propostas (filtros: cd_id, job_id, tipo, status)
// GET  /api/sp/propostas/resumo       → contadores por tipo e status
// GET  /api/sp/propostas/motivos-rejeicao → lista tipos de rejeição (sp_tipo_rejeicao)
// PUT  /api/sp/propostas/{id}         → edição inline (sugestao_editada)
// POST /api/sp/propostas/{id}/aprovar → aprovação individual
// POST /api/sp/propostas/{id}/rejeitar→ rejeição individual (body: {motivo_rejeicao_id})
// POST /api/sp/propostas/aprovar-lote       → aprovação em lote por job_id ou cd_id
// POST /api/sp/propostas/aprovar-selecionados → aprovação de IDs específicos (filtrados)
//
// Semântica de urgência (delta = sugestao_calibragem - capacidade_atual):
//   tipo=falta  → delta > 0  (sugestão aumenta capacidade → slot pequeno demais → falta de espaço no picking)
//   tipo=espaco → delta < 0  (sugestão reduz capacidade → slot grande demais → excesso de espaço)

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"fb_farol/services"
)

// ─── DTOs ─────────────────────────────────────────────────────────────────────

type PropostaResponse struct {
	ID                 int64   `json:"id"`
	JobID              string  `json:"job_id"`
	EnderecoID         int64   `json:"endereco_id"`
	CdID               int     `json:"cd_id"`
	CodFilial          int     `json:"cod_filial"`
	CodProd            int     `json:"codprod"`
	Produto            string  `json:"produto"`
	Departamento       *string `json:"departamento,omitempty"`
	Secao              *string `json:"secao,omitempty"`
	Rua                *int    `json:"rua"`
	Predio             *int    `json:"predio"`
	Apto               *int    `json:"apto"`
	ClasseVenda        *string `json:"classe_venda"`
	CapacidadeAtual    *int    `json:"capacidade_atual"`
	SugestaoCalibragem int     `json:"sugestao_calibragem"`
	Delta              int     `json:"delta"`
	Justificativa      *string `json:"justificativa"`
	Status             string  `json:"status"`
	AprovadoPor        *string `json:"aprovado_por,omitempty"`
	AprovadoEm         *string `json:"aprovado_em,omitempty"`
	SugestaoEditada    *int      `json:"sugestao_editada,omitempty"`
	EditadoPor         *string   `json:"editado_por,omitempty"`
	EditadoEm          *string   `json:"editado_em,omitempty"`
	CreatedAt          string    `json:"created_at"`
	GiroDiaCx          *float64  `json:"giro_dia_cx,omitempty"`
	MedVendaCx         *float64  `json:"med_venda_cx,omitempty"`
	PontoReposicao     *int      `json:"ponto_reposicao,omitempty"`
	Participacao       *float64  `json:"participacao,omitempty"` // % participação na curva ABC
	Prioridade         int       `json:"prioridade"`              // score 0..100 calculado em runtime
}

type PropostasResumo struct {
	TotalPendente    int `json:"total_pendente"`
	TotalAprovada    int `json:"total_aprovada"`
	TotalRejeitada   int `json:"total_rejeitada"`
	FaltaPendente    int `json:"falta_pendente"`
	EspacoPendente   int `json:"espaco_pendente"`
	CalibradoTotal   int `json:"calibrado_total"`
	IgnoradoTotal    int `json:"ignorado_total"`
	CurvaAMantida    int `json:"curva_a_mantida"`
}

// ─── Lista de Propostas ───────────────────────────────────────────────────────

// SpPropostasHandler lista propostas com filtros opcionais.
// GET /api/sp/propostas?cd_id=X&job_id=Y&tipo=falta|espaco&status=pendente&limit=100
func SpPropostasHandler(db *sql.DB) http.HandlerFunc {
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

		q := r.URL.Query()
		cdIDStr := q.Get("cd_id")
		jobIDStr := q.Get("job_id")
		tipo     := q.Get("tipo")   // falta | espaco | "" (todos)
		status   := q.Get("status") // pendente | aprovada | rejeitada | "" (todos)
		limitStr := q.Get("limit")

		limit := 200
		if limitStr != "" {
			if v, err := strconv.Atoi(limitStr); err == nil && v > 0 {
				limit = v
			}
		}

		query := `
			SELECT p.id, p.job_id, p.endereco_id, p.cd_id, p.cod_filial, p.codprod,
			       COALESCE(p.produto,''), e.departamento, e.secao,
			       p.rua, p.predio, p.apto, p.classe_venda,
			       p.capacidade_atual, p.sugestao_calibragem, p.delta, p.justificativa,
			       p.status, p.aprovado_por::text, TO_CHAR(p.aprovado_em,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
			       p.sugestao_editada, p.editado_por::text,
			       TO_CHAR(p.editado_em,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
			       TO_CHAR(p.created_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
			       CASE WHEN e.unidade_master > 0 AND e.qt_giro_dia IS NOT NULL
			            THEN ROUND(e.qt_giro_dia / e.unidade_master, 2)
			            ELSE NULL END,
			       e.med_venda_cx,
			       e.ponto_reposicao,
			       e.participacao
			FROM farol.sp_propostas p
			LEFT JOIN farol.sp_enderecos e ON e.id = p.endereco_id
			WHERE p.empresa_id = $1
		`
		args := []interface{}{spCtx.EmpresaID}
		idx := 2

		if cdIDStr != "" {
			query += fmt.Sprintf(" AND p.cd_id = $%d", idx)
			args = append(args, cdIDStr)
			idx++
		}
		if jobIDStr != "" {
			query += fmt.Sprintf(" AND p.job_id = $%d", idx)
			args = append(args, jobIDStr)
			idx++
		}
		if status != "" {
			query += fmt.Sprintf(" AND p.status = $%d", idx)
			args = append(args, status)
			idx++
		}
		switch tipo {
		case "falta":
			query += " AND p.delta > 0"
		case "espaco":
			query += " AND p.delta < 0"
		case "calibrado":
			query += " AND p.status = 'calibrado'"
		case "ignorado":
			query += " AND p.status = 'ignorado'"
		case "curva_a_mantida":
			query += " AND p.classe_venda = 'A' AND p.delta = 0 AND p.justificativa LIKE '%mantida%'"
		}

		query += fmt.Sprintf(" ORDER BY ABS(p.delta) DESC LIMIT $%d", idx)
		args = append(args, limit)

		rows, err := db.Query(query, args...)
		if err != nil {
			http.Error(w, "Database error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var propostas []PropostaResponse
		for rows.Next() {
			var p PropostaResponse
			if err := rows.Scan(
				&p.ID, &p.JobID, &p.EnderecoID, &p.CdID, &p.CodFilial, &p.CodProd,
				&p.Produto, &p.Departamento, &p.Secao,
				&p.Rua, &p.Predio, &p.Apto, &p.ClasseVenda,
				&p.CapacidadeAtual, &p.SugestaoCalibragem, &p.Delta, &p.Justificativa,
				&p.Status, &p.AprovadoPor, &p.AprovadoEm,
				&p.SugestaoEditada, &p.EditadoPor, &p.EditadoEm,
				&p.CreatedAt, &p.GiroDiaCx,
				&p.MedVendaCx, &p.PontoReposicao, &p.Participacao,
			); err != nil {
				continue
			}
			p.Prioridade = services.CalcularPrioridade(
				p.ClasseVenda, p.Delta, p.CapacidadeAtual,
				p.GiroDiaCx, p.MedVendaCx, p.PontoReposicao,
			)
			propostas = append(propostas, p)
		}
		if propostas == nil {
			propostas = []PropostaResponse{}
		}

		w.Header().Set("Content-Type", "application/json")
		// Cache curto no browser para refetches rápidos (mudou filtro,
		// voltou para a página); private = não cacheia em proxy compartilhado.
		w.Header().Set("Cache-Control", "private, max-age=10")
		json.NewEncoder(w).Encode(propostas)
	}
}

// ─── Resumo / Contadores ──────────────────────────────────────────────────────

// SpPropostasResumoHandler retorna contadores agregados por tipo e status.
// GET /api/sp/propostas/resumo?cd_id=X&job_id=Y
func SpPropostasResumoHandler(db *sql.DB) http.HandlerFunc {
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

		q := r.URL.Query()
		cdIDStr := q.Get("cd_id")
		jobIDStr := q.Get("job_id")

		filter := "WHERE empresa_id = $1"
		args := []interface{}{spCtx.EmpresaID}
		idx := 2

		if cdIDStr != "" {
			filter += fmt.Sprintf(" AND cd_id = $%d", idx)
			args = append(args, cdIDStr)
			idx++
		}
		if jobIDStr != "" {
			filter += fmt.Sprintf(" AND job_id = $%d", idx)
			args = append(args, jobIDStr)
		}

		query := `
			SELECT
				COUNT(*) FILTER (WHERE status = 'pendente')             AS total_pendente,
				COUNT(*) FILTER (WHERE status = 'aprovada')             AS total_aprovada,
				COUNT(*) FILTER (WHERE status = 'rejeitada')            AS total_rejeitada,
				COUNT(*) FILTER (WHERE status = 'pendente' AND delta > 0) AS falta_pendente,
				COUNT(*) FILTER (WHERE status = 'pendente' AND delta < 0) AS espaco_pendente,
				COUNT(*) FILTER (WHERE status = 'calibrado')            AS calibrado_total,
				COUNT(*) FILTER (WHERE status = 'ignorado')             AS ignorado_total,
				COUNT(*) FILTER (WHERE classe_venda = 'A' AND delta = 0 AND justificativa LIKE '%mantida%') AS curva_a_mantida
			FROM farol.sp_propostas
			` + filter

		var resumo PropostasResumo
		err := db.QueryRow(query, args...).Scan(
			&resumo.TotalPendente, &resumo.TotalAprovada, &resumo.TotalRejeitada,
			&resumo.FaltaPendente, &resumo.EspacoPendente, &resumo.CalibradoTotal,
			&resumo.IgnoradoTotal, &resumo.CurvaAMantida,
		)
		if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resumo)
	}
}

// ─── Item: edição, aprovação, rejeição ───────────────────────────────────────

// SpPropostaItemHandler despacha por método e sufixo do path.
// PUT  /api/sp/propostas/{id}          → edição inline
// POST /api/sp/propostas/{id}/aprovar  → aprovar
// POST /api/sp/propostas/{id}/rejeitar → rejeitar
func SpPropostaItemHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		if !spCtx.CanApprove() {
			http.Error(w, "Forbidden: gestor_geral+ necessário", http.StatusForbidden)
			return
		}

		path := r.URL.Path // /api/sp/propostas/{id} ou /api/sp/propostas/{id}/aprovar
		parts := strings.Split(strings.TrimPrefix(path, "/api/sp/propostas/"), "/")
		if len(parts) == 0 || parts[0] == "" {
			http.Error(w, "ID da proposta obrigatório", http.StatusBadRequest)
			return
		}
		propostaID, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			http.Error(w, "ID inválido", http.StatusBadRequest)
			return
		}

		action := ""
		if len(parts) > 1 {
			action = parts[1] // "aprovar" | "rejeitar"
		}

		switch {
		case r.Method == http.MethodPut && action == "":
			editarProposta(db, spCtx, propostaID, w, r)
		case r.Method == http.MethodPost && action == "aprovar":
			mudarStatusProposta(db, spCtx, propostaID, "aprovada", nil, w)
		case r.Method == http.MethodPost && action == "rejeitar":
			mudarStatusProposta(db, spCtx, propostaID, "rejeitada", r, w)
		case r.Method == http.MethodPost && action == "ignorar":
			ignorarProposta(db, spCtx, propostaID, w, r)
		default:
			http.Error(w, "Not found", http.StatusNotFound)
		}
	}
}

func editarProposta(db *sql.DB, spCtx *FarolContext, id int64, w http.ResponseWriter, r *http.Request) {
	var body struct {
		SugestaoEditada int `json:"sugestao_editada"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.SugestaoEditada <= 0 {
		http.Error(w, "sugestao_editada inválida (deve ser > 0)", http.StatusBadRequest)
		return
	}

	res, err := db.Exec(`
		UPDATE farol.sp_propostas
		SET sugestao_editada = $1, editado_por = $2::uuid, editado_em = $3
		WHERE id = $4 AND empresa_id = $5 AND status = 'pendente'
	`, body.SugestaoEditada, spCtx.UserID, time.Now().UTC(), id, spCtx.EmpresaID)
	if err != nil {
		http.Error(w, "Erro ao editar proposta: "+err.Error(), http.StatusInternalServerError)
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		http.Error(w, "Proposta não encontrada ou não está pendente", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Proposta editada"})
}

func mudarStatusProposta(db *sql.DB, spCtx *FarolContext, id int64, novoStatus string, r *http.Request, w http.ResponseWriter) {
	// Para rejeição: lê motivo_rejeicao_id do body (obrigatório)
	var motivoID *int
	if novoStatus == "rejeitada" && r != nil {
		var body struct {
			MotivoRejeicaoID *int `json:"motivo_rejeicao_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.MotivoRejeicaoID == nil {
			http.Error(w, "motivo_rejeicao_id obrigatório para rejeição", http.StatusBadRequest)
			return
		}
		motivoID = body.MotivoRejeicaoID
	}

	var res sql.Result
	var err error
	if motivoID != nil {
		res, err = db.Exec(`
			UPDATE farol.sp_propostas
			SET status = $1, aprovado_por = $2::uuid, aprovado_em = $3, motivo_rejeicao_id = $6
			WHERE id = $4 AND empresa_id = $5 AND status = 'pendente'
		`, novoStatus, spCtx.UserID, time.Now().UTC(), id, spCtx.EmpresaID, *motivoID)
	} else {
		res, err = db.Exec(`
			UPDATE farol.sp_propostas
			SET status = $1, aprovado_por = $2::uuid, aprovado_em = $3
			WHERE id = $4 AND empresa_id = $5 AND status = 'pendente'
		`, novoStatus, spCtx.UserID, time.Now().UTC(), id, spCtx.EmpresaID)
	}
	if err != nil {
		http.Error(w, "Erro ao atualizar proposta: "+err.Error(), http.StatusInternalServerError)
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		http.Error(w, "Proposta não encontrada ou não está pendente", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Status atualizado para " + novoStatus})
}

// ignorarProposta adiciona o produto na lista de ignorados e marca a proposta como 'ignorado'.
// POST /api/sp/propostas/{id}/ignorar
// Body (opcional): { "motivo": "texto livre" }
func ignorarProposta(db *sql.DB, spCtx *FarolContext, id int64, w http.ResponseWriter, r *http.Request) {
	var body struct {
		TipoIgnoradoID *int `json:"tipo_ignorado_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.TipoIgnoradoID == nil {
		http.Error(w, "tipo_ignorado_id obrigatório", http.StatusBadRequest)
		return
	}

	// Busca dados da proposta para popular sp_ignorados
	var cdID, codprod, codFilial int
	var produto string
	err := db.QueryRow(`
		SELECT cd_id, codprod, cod_filial, COALESCE(produto,'')
		FROM farol.sp_propostas
		WHERE id = $1 AND empresa_id = $2 AND status IN ('pendente','calibrado')
	`, id, spCtx.EmpresaID).Scan(&cdID, &codprod, &codFilial, &produto)
	if err == sql.ErrNoRows {
		http.Error(w, "Proposta não encontrada ou não está pendente/calibrada", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Erro ao buscar proposta: "+err.Error(), http.StatusInternalServerError)
		return
	}

	tx, err := db.Begin()
	if err != nil {
		http.Error(w, "Erro ao iniciar transação", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	// Insere em sp_ignorados (ON CONFLICT — atualiza tipo e responsável)
	_, err = tx.Exec(`
		INSERT INTO farol.sp_ignorados
		  (empresa_id, cd_id, codprod, cod_filial, produto, tipo_ignorado_id, ignorado_por)
		VALUES ($1,$2,$3,$4,$5,$6,$7::uuid)
		ON CONFLICT (empresa_id, cd_id, codprod, cod_filial)
		DO UPDATE SET tipo_ignorado_id = EXCLUDED.tipo_ignorado_id,
		              ignorado_por = EXCLUDED.ignorado_por,
		              created_at = now()
	`, spCtx.EmpresaID, cdID, codprod, codFilial, produto, *body.TipoIgnoradoID, spCtx.UserID)
	if err != nil {
		http.Error(w, "Erro ao registrar ignorado: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Marca proposta como 'ignorado'
	_, err = tx.Exec(`
		UPDATE farol.sp_propostas
		SET status = 'ignorado', aprovado_por = $1::uuid, aprovado_em = $2
		WHERE id = $3 AND empresa_id = $4
	`, spCtx.UserID, time.Now().UTC(), id, spCtx.EmpresaID)
	if err != nil {
		http.Error(w, "Erro ao atualizar proposta: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if err := tx.Commit(); err != nil {
		http.Error(w, "Erro ao confirmar operação", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Produto ignorado com sucesso"})
}

// nilIfEmpty é uma versão local para string (evita dependência cruzada com csv_worker).
func nilIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// ─── Motivos de Rejeição ─────────────────────────────────────────────────────

// SpMotivoRejeicaoHandler lista os tipos de rejeição ativos.
// GET /api/sp/propostas/motivos-rejeicao
func SpMotivoRejeicaoHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		rows, err := db.Query(`
			SELECT id, codigo, descricao FROM farol.sp_tipo_rejeicao
			WHERE ativo = true ORDER BY codigo
		`)
		if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		type motivo struct {
			ID       int    `json:"id"`
			Codigo   int    `json:"codigo"`
			Descricao string `json:"descricao"`
		}
		var lista []motivo
		for rows.Next() {
			var m motivo
			if rows.Scan(&m.ID, &m.Codigo, &m.Descricao) == nil {
				lista = append(lista, m)
			}
		}
		if lista == nil {
			lista = []motivo{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(lista)
	}
}

// ─── Aprovação em Lote ────────────────────────────────────────────────────────

// SpPropostasAprovarLoteHandler aprova todas as propostas pendentes de um job ou CD.
// POST /api/sp/propostas/aprovar-lote
// Body: { "job_id": "uuid" } ou { "cd_id": 123 } ou { "tipo": "falta|espaco", "cd_id": 123 }
func SpPropostasAprovarLoteHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		if !spCtx.CanApprove() {
			http.Error(w, "Forbidden: gestor_geral+ necessário", http.StatusForbidden)
			return
		}

		var body struct {
			JobID string `json:"job_id"`
			CdID  *int   `json:"cd_id"`
			Tipo  string `json:"tipo"` // falta | espaco | "" (todos)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "JSON inválido", http.StatusBadRequest)
			return
		}
		if body.JobID == "" && body.CdID == nil {
			http.Error(w, "job_id ou cd_id obrigatório", http.StatusBadRequest)
			return
		}

		filter := "WHERE empresa_id = $2 AND status = 'pendente'"
		args := []interface{}{time.Now().UTC(), spCtx.EmpresaID}
		idx := 3

		if body.JobID != "" {
			filter += fmt.Sprintf(" AND job_id = $%d", idx)
			args = append(args, body.JobID)
			idx++
		}
		if body.CdID != nil {
			filter += fmt.Sprintf(" AND cd_id = $%d", idx)
			args = append(args, *body.CdID)
			idx++
		}
		switch body.Tipo {
		case "falta":
			filter += " AND delta > 0"
		case "espaco":
			filter += " AND delta < 0"
		}

		// Passa user como $idx
		filter += fmt.Sprintf(" RETURNING id")
		query := fmt.Sprintf(`
			UPDATE farol.sp_propostas
			SET status = 'aprovada', aprovado_por = $%d::uuid, aprovado_em = $1
			%s
		`, idx, filter)
		args = append(args, spCtx.UserID)

		rows, err := db.Query(query, args...)
		if err != nil {
			http.Error(w, "Erro ao aprovar em lote: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		count := 0
		for rows.Next() {
			var id int64
			rows.Scan(&id)
			count++
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message":  fmt.Sprintf("%d propostas aprovadas", count),
			"aprovadas": count,
		})
	}
}

// SpPropostasAprovarSelecionadosHandler aprova propostas por IDs específicos.
// POST /api/sp/propostas/aprovar-selecionados
// Body: { "ids": [1, 2, 3, ...] }
// Aprova apenas as que pertencem à empresa ativa e estão com status 'pendente'.
func SpPropostasAprovarSelecionadosHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		if !spCtx.CanApprove() {
			http.Error(w, "Forbidden: gestor_filial+ necessário", http.StatusForbidden)
			return
		}

		var body struct {
			IDs []int64 `json:"ids"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.IDs) == 0 {
			http.Error(w, "ids obrigatório e não pode ser vazio", http.StatusBadRequest)
			return
		}

		args := []interface{}{time.Now().UTC(), spCtx.EmpresaID, spCtx.UserID}
		placeholders := make([]string, len(body.IDs))
		for i, id := range body.IDs {
			args = append(args, id)
			placeholders[i] = fmt.Sprintf("$%d", i+4)
		}

		query := fmt.Sprintf(`
			UPDATE farol.sp_propostas
			SET status = 'aprovada', aprovado_por = $3::uuid, aprovado_em = $1
			WHERE empresa_id = $2 AND status = 'pendente'
			  AND id IN (%s)
			RETURNING id
		`, strings.Join(placeholders, ","))

		rows, err := db.Query(query, args...)
		if err != nil {
			http.Error(w, "Erro ao aprovar: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		count := 0
		for rows.Next() {
			var id int64
			rows.Scan(&id)
			count++
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message":   fmt.Sprintf("%d propostas aprovadas", count),
			"aprovadas": count,
		})
	}
}

// ─── GET /api/sp/propostas/ruas ───────────────────────────────────────────────
// Retorna lista de ruas distintas com propostas aprovadas para um CD ou job.
// Usado pelo frontend para popular o seletor de ruas na emissão do PDF.

func SpPropostasRuasHandler(db *sql.DB) http.HandlerFunc {
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

		q := r.URL.Query()
		jobIDStr := q.Get("job_id")
		cdIDStr  := q.Get("cd_id")
		if jobIDStr == "" && cdIDStr == "" {
			http.Error(w, "job_id ou cd_id obrigatório", http.StatusBadRequest)
			return
		}

		filter := "WHERE empresa_id = $1 AND rua IS NOT NULL AND status = 'aprovada'"
		args   := []any{spCtx.EmpresaID}
		idx    := 2

		if jobIDStr != "" {
			filter += fmt.Sprintf(" AND job_id = $%d", idx)
			args = append(args, jobIDStr)
		} else {
			filter += fmt.Sprintf(" AND cd_id = $%d", idx)
			v, err := strconv.Atoi(cdIDStr)
			if err != nil {
				http.Error(w, "cd_id inválido", http.StatusBadRequest)
				return
			}
			args = append(args, v)
		}

		rows, err := db.Query(
			fmt.Sprintf("SELECT DISTINCT rua FROM farol.sp_propostas %s ORDER BY rua", filter),
			args...,
		)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		ruas := []int{}
		for rows.Next() {
			var rua int
			if err := rows.Scan(&rua); err == nil {
				ruas = append(ruas, rua)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ruas)
	}
}
