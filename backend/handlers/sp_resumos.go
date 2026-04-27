package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"fb_farol/services"
)

// ── Destinatários do resumo ──────────────────────────────────────────────────

type destinatarioResp struct {
	ID            int    `json:"id"`
	CdID          int    `json:"cd_id"`
	CdNome        string `json:"cd_nome,omitempty"`
	FilialNome    string `json:"filial_nome,omitempty"`
	NomeCompleto  string `json:"nome_completo"`
	Cargo         string `json:"cargo"`
	Email         string `json:"email"`
	Ativo         bool   `json:"ativo"`
	CriadoEm      string `json:"criado_em"`
	AtualizadoEm  string `json:"atualizado_em"`
}

// SpDestinatariosHandler — CRUD de destinatários do resumo executivo (admin_fbtax)
// GET    /api/sp/admin/destinatarios?cd_id=X  → lista
// POST   /api/sp/admin/destinatarios          → cria  {cd_id, nome_completo, cargo, email}
// PUT    /api/sp/admin/destinatarios/{id}     → atualiza {nome_completo, cargo, email, ativo}
// DELETE /api/sp/admin/destinatarios/{id}     → remove
func SpDestinatariosHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Path /api/sp/admin/destinatarios[/{id}]
		path := strings.TrimPrefix(r.URL.Path, "/api/sp/admin/destinatarios")
		path = strings.Trim(path, "/")

		switch r.Method {
		case http.MethodGet:
			// Filtros opcionais:
			//   cd_id      → apenas destinatários do CD específico
			//   empresa_id → apenas destinatários daquela empresa (admin)
			// Sem nenhum: admin_fbtax vê TUDO; demais só da própria empresa.
			cdIDStr := r.URL.Query().Get("cd_id")
			empresaIDQuery := r.URL.Query().Get("empresa_id")
			spCtx := GetSpContext(r)

			query := `
				SELECT d.id, d.cd_id, c.nome, COALESCE(f.nome, ''),
				       d.nome_completo, COALESCE(d.cargo,''), d.email, d.ativo,
				       to_char(d.criado_em, 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
				       to_char(d.atualizado_em, 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
				  FROM farol.sp_destinatarios_resumo d
				  JOIN farol.sp_centros_dist c ON c.id = d.cd_id
			     LEFT JOIN farol.sp_filiais f ON f.id = c.filial_id
			`
			args := []interface{}{}
			where := []string{}
			if cdIDStr != "" {
				cdID, _ := strconv.Atoi(cdIDStr)
				args = append(args, cdID)
				where = append(where, fmt.Sprintf("d.cd_id = $%d", len(args)))
			}
			// Filtragem por empresa:
			//   - admin_fbtax (MASTER): se passou empresa_id na query, filtra por esse;
			//     senão vê todas as empresas (necessário para ver destinatários de
			//     outras empresas onde ele atua como admin global).
			//   - demais perfis: sempre filtra pela empresa do contexto.
			isAdmin := spCtx != nil && spCtx.IsAdminFbtax()
			if isAdmin {
				if empresaIDQuery != "" {
					args = append(args, empresaIDQuery)
					where = append(where, fmt.Sprintf("c.empresa_id = $%d::uuid", len(args)))
				}
			} else if spCtx != nil && spCtx.EmpresaID != "" {
				args = append(args, spCtx.EmpresaID)
				where = append(where, fmt.Sprintf("c.empresa_id = $%d::uuid", len(args)))
			}
			if len(where) > 0 {
				query += " WHERE " + strings.Join(where, " AND ")
			}
			query += " ORDER BY f.nome, c.nome, d.ativo DESC, d.nome_completo"

			rows, err := db.Query(query, args...)
			if err != nil {
				http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
				return
			}
			defer rows.Close()
			out := []destinatarioResp{}
			for rows.Next() {
				var d destinatarioResp
				if err := rows.Scan(&d.ID, &d.CdID, &d.CdNome, &d.FilialNome, &d.NomeCompleto, &d.Cargo, &d.Email, &d.Ativo, &d.CriadoEm, &d.AtualizadoEm); err == nil {
					out = append(out, d)
				}
			}
			json.NewEncoder(w).Encode(out)

		case http.MethodPost:
			var body struct {
				CdID         int    `json:"cd_id"`
				NomeCompleto string `json:"nome_completo"`
				Cargo        string `json:"cargo"`
				Email        string `json:"email"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.CdID == 0 || body.NomeCompleto == "" || body.Email == "" {
				http.Error(w, `{"error":"campos obrigatórios: cd_id, nome_completo, email"}`, http.StatusBadRequest)
				return
			}
			var id int
			err := db.QueryRow(`
				INSERT INTO farol.sp_destinatarios_resumo (cd_id, nome_completo, cargo, email)
				VALUES ($1, $2, NULLIF($3, ''), $4)
				RETURNING id
			`, body.CdID, body.NomeCompleto, body.Cargo, body.Email).Scan(&id)
			if err != nil {
				http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
				return
			}
			json.NewEncoder(w).Encode(map[string]int{"id": id})

		case http.MethodPut:
			id, _ := strconv.Atoi(path)
			if id == 0 {
				http.Error(w, `{"error":"id obrigatório"}`, http.StatusBadRequest)
				return
			}
			var body struct {
				CdID         *int   `json:"cd_id"`
				NomeCompleto string `json:"nome_completo"`
				Cargo        string `json:"cargo"`
				Email        string `json:"email"`
				Ativo        *bool  `json:"ativo"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			ativo := true
			if body.Ativo != nil {
				ativo = *body.Ativo
			}
			cdID := 0
			if body.CdID != nil {
				cdID = *body.CdID
			}
			_, err := db.Exec(`
				UPDATE farol.sp_destinatarios_resumo
				   SET cd_id         = COALESCE(NULLIF($6, 0), cd_id),
				       nome_completo = COALESCE(NULLIF($2, ''), nome_completo),
				       cargo         = NULLIF($3, ''),
				       email         = COALESCE(NULLIF($4, ''), email),
				       ativo         = $5,
				       atualizado_em = NOW()
				 WHERE id = $1
			`, id, body.NomeCompleto, body.Cargo, body.Email, ativo, cdID)
			if err != nil {
				http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
				return
			}
			w.Write([]byte(`{"ok":true}`))

		case http.MethodDelete:
			id, _ := strconv.Atoi(path)
			if id == 0 {
				http.Error(w, `{"error":"id obrigatório"}`, http.StatusBadRequest)
				return
			}
			_, err := db.Exec(`DELETE FROM farol.sp_destinatarios_resumo WHERE id = $1`, id)
			if err != nil {
				http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
				return
			}
			w.Write([]byte(`{"ok":true}`))

		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	}
}

// ── Listagem e geração de resumos ─────────────────────────────────────────────

type resumoListItem struct {
	ID             int    `json:"id"`
	CdID           int    `json:"cd_id"`
	PeriodoInicio  string `json:"periodo_inicio"`
	PeriodoFim     string `json:"periodo_fim"`
	CriadoEm       string `json:"criado_em"`
	EnviadoEm      string `json:"enviado_em,omitempty"`
	EnviadoPara    int    `json:"enviado_para_count"`
}

// SpResumosHandler — GET /api/sp/relatorios?cd_id=X → lista resumos
func SpResumosHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		cdIDStr := r.URL.Query().Get("cd_id")
		if cdIDStr == "" {
			http.Error(w, `{"error":"cd_id obrigatório"}`, http.StatusBadRequest)
			return
		}
		cdID, _ := strconv.Atoi(cdIDStr)

		rows, err := db.Query(`
			SELECT id, cd_id,
			       to_char(periodo_inicio, 'YYYY-MM-DD'),
			       to_char(periodo_fim, 'YYYY-MM-DD'),
			       to_char(criado_em, 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
			       COALESCE(to_char(enviado_em, 'YYYY-MM-DD"T"HH24:MI:SS"Z"'), ''),
			       COALESCE(array_length(enviado_para, 1), 0)
			  FROM farol.sp_relatorios_semanais
			 WHERE cd_id = $1
			 ORDER BY periodo_fim DESC, id DESC
			 LIMIT 50
		`, cdID)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		out := []resumoListItem{}
		for rows.Next() {
			var it resumoListItem
			if err := rows.Scan(&it.ID, &it.CdID, &it.PeriodoInicio, &it.PeriodoFim, &it.CriadoEm, &it.EnviadoEm, &it.EnviadoPara); err == nil {
				out = append(out, it)
			}
		}
		json.NewEncoder(w).Encode(out)
	}
}

// SpResumoItemHandler — GET /api/sp/relatorios/{id} → detalhe completo (json + markdown)
func SpResumoItemHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		path := strings.TrimPrefix(r.URL.Path, "/api/sp/relatorios/")
		id, _ := strconv.Atoi(strings.Trim(path, "/"))
		if id == 0 {
			http.Error(w, `{"error":"id obrigatório"}`, http.StatusBadRequest)
			return
		}

		var (
			cdID                       int
			periodoIni, periodoFim     string
			dadosJSON, narrativa       string
			criadoEm                   string
			enviadoEm, erroEnvio       sql.NullString
		)
		err := db.QueryRow(`
			SELECT cd_id,
			       to_char(periodo_inicio, 'YYYY-MM-DD'),
			       to_char(periodo_fim, 'YYYY-MM-DD'),
			       dados_json::text, narrativa_md,
			       to_char(criado_em, 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
			       to_char(enviado_em, 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
			       COALESCE(erro_envio, '')
			  FROM farol.sp_relatorios_semanais
			 WHERE id = $1
		`, id).Scan(&cdID, &periodoIni, &periodoFim, &dadosJSON, &narrativa, &criadoEm, &enviadoEm, &erroEnvio)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusNotFound)
			return
		}

		// dados_json vem como string; embute como objeto cru
		out := map[string]interface{}{
			"id":             id,
			"cd_id":          cdID,
			"periodo_inicio": periodoIni,
			"periodo_fim":    periodoFim,
			"dados":          json.RawMessage(dadosJSON),
			"narrativa_md":   narrativa,
			"criado_em":      criadoEm,
			"enviado_em":     enviadoEm.String,
			"erro_envio":     erroEnvio.String,
		}
		json.NewEncoder(w).Encode(out)
	}
}

// SpResumoGerarHandler — POST /api/sp/relatorios/gerar?cd_id=X (master)
//   gera o resumo executivo da última semana e retorna o id criado
func SpResumoGerarHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[resumo-handler] %s %s", r.Method, r.URL.String())
		if r.Method != http.MethodPost {
			log.Printf("[resumo-handler] método inválido: %s", r.Method)
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		cdIDStr := r.URL.Query().Get("cd_id")
		if cdIDStr == "" {
			log.Printf("[resumo-handler] cd_id ausente na URL")
			http.Error(w, `{"error":"cd_id obrigatório"}`, http.StatusBadRequest)
			return
		}
		cdID, _ := strconv.Atoi(cdIDStr)

		spCtx := GetSpContext(r)
		criadoPor := ""
		if spCtx != nil {
			criadoPor = spCtx.UserID
			log.Printf("[resumo-handler] CD=%d user=%s role=%s", cdID, criadoPor, spCtx.SpRole)
		} else {
			log.Printf("[resumo-handler] CD=%d sem spContext", cdID)
		}

		id, _, _, err := services.GerarResumoExecutivo(db, cdID, criadoPor)
		if err != nil {
			log.Printf("[resumo-handler] CD=%d erro: %v", cdID, err)
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
			return
		}

		log.Printf("[resumo-handler] CD=%d sucesso, relatório id=%d", cdID, id)
		json.NewEncoder(w).Encode(map[string]int{"id": id})
	}
}

// SpResumoEnviarHandler — POST /api/sp/relatorios/{id}/enviar (master)
//   envia o resumo por email aos destinatários ativos do CD
func SpResumoEnviarHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		// Path: /api/sp/relatorios/{id}/enviar
		path := strings.TrimPrefix(r.URL.Path, "/api/sp/relatorios/")
		path = strings.TrimSuffix(path, "/enviar")
		id, _ := strconv.Atoi(strings.Trim(path, "/"))
		if id == 0 {
			http.Error(w, `{"error":"id obrigatório"}`, http.StatusBadRequest)
			return
		}

		enviados, err := services.EnviarResumoPorEmail(db, id)
		erroMsg := ""
		if err != nil {
			erroMsg = err.Error()
		}
		if len(enviados) > 0 {
			_ = services.MarcarEnviado(db, id, enviados, erroMsg)
		}
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"enviados": enviados,
			"total":    len(enviados),
		})
	}
}
