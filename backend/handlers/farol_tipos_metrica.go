package handlers

// farol_tipos_metrica.go — Catálogo de Tipos de Métrica (Épico 1 do módulo
// Painel de Gestão de Metas por Indústria)
//
// Um Tipo de Métrica é uma forma de cálculo reutilizável (ex.: "Cobertura
// por Rede") que 2+ indústrias/fornecedores podem instanciar com parâmetros
// próprios (vínculo, Épico 2 — ainda não implementado). A genericidade fica
// inteira em ParametrosSchema (JSONB) — nunca em coluna nova da tabela; ver
// migrations/214_tipos_metrica.sql e o teste de generalidade do FR1
// (prd-FB_FAROL-2026-09-02/prd.md linha 70).
//
// Rotas (todas exigem sp_role >= gestor_geral — catálogo é transversal, não
// por filial, e alimenta programas com impacto financeiro em contrato de
// fornecedor):
//   GET/POST     /api/farol/tipos-metrica         (autenticado)
//   PUT/DELETE   /api/farol/tipos-metrica/{id}     (autenticado)

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
)

// ─── DTOs ─────────────────────────────────────────────────────────────────────

// níveisAgregacaoValidos espelha o CHECK constraint de
// migrations/214_tipos_metrica.sql — os níveis vêm da hierarquia
// organizacional real do Farol, não são parâmetro livre do Tipo de Métrica.
var niveisAgregacaoValidos = map[string]bool{
	"ggv": true, "crv": true, "rca": true, "rede": true, "cliente": true,
}

type ParametroSchemaDTO struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Type  string `json:"type"`
}

type TipoMetricaRequest struct {
	Nome             string               `json:"nome"`
	Descricao        string               `json:"descricao"`
	NivelAgregacao   string               `json:"nivel_agregacao"`
	ParametrosSchema []ParametroSchemaDTO `json:"parametros_schema"`
	Ativo            *bool                `json:"ativo"`
}

type TipoMetricaResponse struct {
	ID               int                  `json:"id"`
	Nome             string               `json:"nome"`
	Descricao        string               `json:"descricao,omitempty"`
	NivelAgregacao   string               `json:"nivel_agregacao"`
	ParametrosSchema []ParametroSchemaDTO `json:"parametros_schema"`
	Ativo            bool                 `json:"ativo"`
	CreatedAt        string               `json:"created_at"`
}

// ─── Validação ────────────────────────────────────────────────────────────────

// validarTipoMetricaRequest aplica as regras que não dá pra deixar só pro
// CHECK constraint do banco (mensagem amigável em vez de erro SQL cru).
func validarTipoMetricaRequest(req TipoMetricaRequest) string {
	if strings.TrimSpace(req.Nome) == "" {
		return "nome é obrigatório"
	}
	if !niveisAgregacaoValidos[req.NivelAgregacao] {
		return "nivel_agregacao inválido — use ggv, crv, rca, rede ou cliente"
	}
	if len(req.ParametrosSchema) == 0 {
		return "todo Tipo de Métrica exige pelo menos 1 parâmetro em parametros_schema"
	}
	vistos := map[string]bool{}
	for _, p := range req.ParametrosSchema {
		key := strings.TrimSpace(p.Key)
		if key == "" || strings.TrimSpace(p.Label) == "" || strings.TrimSpace(p.Type) == "" {
			return "cada parâmetro precisa de key, label e type preenchidos"
		}
		if vistos[key] {
			return "parâmetro com key duplicada: " + key
		}
		vistos[key] = true
	}
	return ""
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func scanTipoMetrica(row interface{ Scan(...any) error }) (*TipoMetricaResponse, error) {
	var (
		t             TipoMetricaResponse
		descricao     sql.NullString
		parametrosRaw []byte
	)
	if err := row.Scan(&t.ID, &t.Nome, &descricao, &t.NivelAgregacao, &parametrosRaw, &t.Ativo, &t.CreatedAt); err != nil {
		return nil, err
	}
	t.Descricao = descricao.String
	t.ParametrosSchema = []ParametroSchemaDTO{}
	if len(parametrosRaw) > 0 {
		_ = json.Unmarshal(parametrosRaw, &t.ParametrosSchema)
	}
	return &t, nil
}

// ─── TiposMetricaHandler — GET/POST /api/farol/tipos-metrica ──────────────

func TiposMetricaHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		switch r.Method {
		case http.MethodGet:
			rows, err := db.Query(`
				SELECT id, nome, descricao, nivel_agregacao, parametros_schema, ativo, created_at
				FROM farol.tipos_metrica
				WHERE empresa_id = $1
				ORDER BY nome ASC
			`, spCtx.EmpresaID)
			if err != nil {
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
			defer rows.Close()

			tipos := []TipoMetricaResponse{}
			for rows.Next() {
				t, err := scanTipoMetrica(rows)
				if err != nil {
					http.Error(w, "Database error", http.StatusInternalServerError)
					return
				}
				tipos = append(tipos, *t)
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(tipos)

		case http.MethodPost:
			if !hasSpRole(spCtx.SpRole, "gestor_geral") {
				http.Error(w, "Forbidden: gestor_geral necessário", http.StatusForbidden)
				return
			}
			var req TipoMetricaRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "corpo da requisição inválido", http.StatusBadRequest)
				return
			}
			if msg := validarTipoMetricaRequest(req); msg != "" {
				http.Error(w, msg, http.StatusBadRequest)
				return
			}
			ativo := true
			if req.Ativo != nil {
				ativo = *req.Ativo
			}
			parametrosJSON, err := json.Marshal(req.ParametrosSchema)
			if err != nil {
				http.Error(w, "parametros_schema inválido", http.StatusBadRequest)
				return
			}

			tx, err := db.Begin()
			if err != nil {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
			defer tx.Rollback()

			var id int
			var createdAt string
			err = tx.QueryRow(`
				INSERT INTO farol.tipos_metrica (empresa_id, nome, descricao, nivel_agregacao, parametros_schema, ativo)
				VALUES ($1, $2, NULLIF($3,''), $4, $5, $6)
				RETURNING id, created_at
			`, spCtx.EmpresaID, req.Nome, req.Descricao, req.NivelAgregacao, parametrosJSON, ativo).Scan(&id, &createdAt)
			if err != nil {
				if strings.Contains(err.Error(), "uq_farol_tipos_metrica_empresa_nome") {
					http.Error(w, "Já existe um Tipo de Métrica com esse nome", http.StatusConflict)
					return
				}
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}

			auditPayload := map[string]any{
				"nome": req.Nome, "descricao": req.Descricao, "nivel_agregacao": req.NivelAgregacao,
				"parametros_schema": req.ParametrosSchema, "ativo": ativo,
			}
			if err := writeAuditLogTx(tx, spCtx.EmpresaID, spCtx.UserID, "tipos_metrica", strconv.Itoa(id), "criar", auditPayload); err != nil {
				http.Error(w, "Erro ao gravar auditoria", http.StatusInternalServerError)
				return
			}

			if err := tx.Commit(); err != nil {
				http.Error(w, "Commit error", http.StatusInternalServerError)
				return
			}
			log.Printf("TiposMetrica: criado tipo %d (%s) empresa %s por %s", id, req.Nome, spCtx.EmpresaID, spCtx.UserID)
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]int{"id": id})

		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

// ─── TipoMetricaItemHandler — PUT/DELETE /api/farol/tipos-metrica/{id} ────

func TipoMetricaItemHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		idStr := pathSegment(r.URL.Path, "/api/farol/tipos-metrica/")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			http.Error(w, "ID inválido", http.StatusBadRequest)
			return
		}

		switch r.Method {
		case http.MethodPut:
			if !hasSpRole(spCtx.SpRole, "gestor_geral") {
				http.Error(w, "Forbidden: gestor_geral necessário", http.StatusForbidden)
				return
			}
			var req TipoMetricaRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "corpo da requisição inválido", http.StatusBadRequest)
				return
			}
			if msg := validarTipoMetricaRequest(req); msg != "" {
				http.Error(w, msg, http.StatusBadRequest)
				return
			}
			ativo := true
			if req.Ativo != nil {
				ativo = *req.Ativo
			}
			parametrosJSON, err := json.Marshal(req.ParametrosSchema)
			if err != nil {
				http.Error(w, "parametros_schema inválido", http.StatusBadRequest)
				return
			}

			tx, err := db.Begin()
			if err != nil {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
			defer tx.Rollback()

			// Busca o estado atual ANTES do update — é o "valor anterior" do
			// audit log (NFR1) e também serve pra detectar 404 antes de gastar
			// um UPDATE.
			antes, err := scanTipoMetrica(tx.QueryRow(`
				SELECT id, nome, descricao, nivel_agregacao, parametros_schema, ativo, created_at
				FROM farol.tipos_metrica WHERE id = $1 AND empresa_id = $2
			`, id, spCtx.EmpresaID))
			if err != nil {
				if err == sql.ErrNoRows {
					http.Error(w, "Tipo de Métrica não encontrado", http.StatusNotFound)
					return
				}
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}

			_, err = tx.Exec(`
				UPDATE farol.tipos_metrica
				SET nome = $1, descricao = NULLIF($2,''), nivel_agregacao = $3,
				    parametros_schema = $4, ativo = $5, updated_at = now()
				WHERE id = $6 AND empresa_id = $7
			`, req.Nome, req.Descricao, req.NivelAgregacao, parametrosJSON, ativo, id, spCtx.EmpresaID)
			if err != nil {
				if strings.Contains(err.Error(), "uq_farol_tipos_metrica_empresa_nome") {
					http.Error(w, "Já existe um Tipo de Métrica com esse nome", http.StatusConflict)
					return
				}
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}

			auditPayload := map[string]any{
				"antes": antes,
				"depois": map[string]any{
					"nome": req.Nome, "descricao": req.Descricao, "nivel_agregacao": req.NivelAgregacao,
					"parametros_schema": req.ParametrosSchema, "ativo": ativo,
				},
			}
			if err := writeAuditLogTx(tx, spCtx.EmpresaID, spCtx.UserID, "tipos_metrica", strconv.Itoa(id), "editar", auditPayload); err != nil {
				http.Error(w, "Erro ao gravar auditoria", http.StatusInternalServerError)
				return
			}

			if err := tx.Commit(); err != nil {
				http.Error(w, "Commit error", http.StatusInternalServerError)
				return
			}
			log.Printf("TiposMetrica: atualizado tipo %d empresa %s por %s", id, spCtx.EmpresaID, spCtx.UserID)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"message": "Tipo de Métrica atualizado"})

		case http.MethodDelete:
			if !hasSpRole(spCtx.SpRole, "gestor_geral") {
				http.Error(w, "Forbidden: gestor_geral necessário", http.StatusForbidden)
				return
			}
			res, err := db.Exec(`DELETE FROM farol.tipos_metrica WHERE id = $1 AND empresa_id = $2`, id, spCtx.EmpresaID)
			if err != nil {
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
			if n, _ := res.RowsAffected(); n == 0 {
				http.Error(w, "Tipo de Métrica não encontrado", http.StatusNotFound)
				return
			}
			log.Printf("TiposMetrica: removido tipo %d empresa %s por %s", id, spCtx.EmpresaID, spCtx.UserID)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"message": "Tipo de Métrica removido"})

		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}
