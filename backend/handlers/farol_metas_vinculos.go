package handlers

// farol_metas_vinculos.go — Vínculo Indústria × Tipo de Métrica (Épico 2,
// Story 2.1, módulo Painel de Gestão de Metas por Indústria)
//
// Um Vínculo associa uma Indústria (farol.industrias) a um Tipo de Métrica
// (farol.tipos_metrica), guardando os valores concretos dos parâmetros que
// o Tipo de Métrica exige. Ver migrations/216_metas_vinculos.sql.
//
// Rotas (sp_role >= gestor_geral, mesmo padrão de tipos-metrica):
//   GET/POST     /api/farol/metas-vinculos         (autenticado)
//   PUT/DELETE   /api/farol/metas-vinculos/{id}     (autenticado)

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/lib/pq"
)

// ─── DTOs ─────────────────────────────────────────────────────────────────────

type MetaVinculoRequest struct {
	IndustriaID       int            `json:"industria_id"`
	TipoMetricaID     int            `json:"tipo_metrica_id"`
	ParametrosValores map[string]any `json:"parametros_valores"`
	Ativo             *bool          `json:"ativo"`
	RecorteUF         string         `json:"recorte_uf"`
	RecorteGGVs       []string       `json:"recorte_ggvs"`
	TiposVendaValidos []string       `json:"tipos_venda_validos"`
}

type MetaVinculoResponse struct {
	ID                int                  `json:"id"`
	IndustriaID       int                  `json:"industria_id"`
	IndustriaNome     string               `json:"industria_nome"`
	TipoMetricaID     int                  `json:"tipo_metrica_id"`
	TipoMetricaNome   string               `json:"tipo_metrica_nome"`
	NivelAgregacao    string               `json:"nivel_agregacao"`
	ParametrosSchema  []ParametroSchemaDTO `json:"parametros_schema"`
	ParametrosValores map[string]any       `json:"parametros_valores"`
	Ativo             bool                 `json:"ativo"`
	CreatedAt         string               `json:"created_at"`
	RecorteUF         string               `json:"recorte_uf,omitempty"`
	RecorteGGVs       []string             `json:"recorte_ggvs"`
	TiposVendaValidos []string             `json:"tipos_venda_validos"`
}

// ─── Validação ────────────────────────────────────────────────────────────────

// buscarTipoMetricaSchema carrega o parametros_schema de um Tipo de Métrica
// (empresa-scoped) — usado tanto pra validar o vínculo quanto pra enriquecer
// a resposta.
func buscarTipoMetricaSchema(db querier, empresaID string, tipoMetricaID int) (nome, nivel string, schema []ParametroSchemaDTO, err error) {
	var raw []byte
	err = db.QueryRow(`
		SELECT nome, nivel_agregacao, parametros_schema
		FROM farol.tipos_metrica WHERE id = $1 AND empresa_id = $2
	`, tipoMetricaID, empresaID).Scan(&nome, &nivel, &raw)
	if err != nil {
		return "", "", nil, err
	}
	schema = []ParametroSchemaDTO{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &schema)
	}
	return nome, nivel, schema, nil
}

// querier abstrai *sql.DB e *sql.Tx pro QueryRow usado em buscarTipoMetricaSchema.
type querier interface {
	QueryRow(query string, args ...any) *sql.Row
}

// validarParametrosValores garante que todo parâmetro exigido pelo
// parametros_schema do Tipo de Métrica tem valor preenchido no vínculo —
// sem isso o motor de apuração (Épico 4) não teria como calcular.
func validarParametrosValores(schema []ParametroSchemaDTO, valores map[string]any) string {
	for _, p := range schema {
		v, ok := valores[p.Key]
		if !ok || v == nil || v == "" {
			return "parâmetro obrigatório não preenchido: " + p.Key + " (" + p.Label + ")"
		}
	}
	return ""
}

func scanMetaVinculo(row interface{ Scan(...any) error }) (*MetaVinculoResponse, error) {
	var (
		v             MetaVinculoResponse
		industriaNome string
		tipoNome      string
		nivel         string
		schemaRaw     []byte
		parametrosRaw []byte
		recorteUF     sql.NullString
	)
	if err := row.Scan(&v.ID, &v.IndustriaID, &industriaNome, &v.TipoMetricaID, &tipoNome, &nivel, &schemaRaw, &parametrosRaw, &v.Ativo, &v.CreatedAt, &recorteUF, pq.Array(&v.RecorteGGVs), pq.Array(&v.TiposVendaValidos)); err != nil {
		return nil, err
	}
	v.IndustriaNome = industriaNome
	v.TipoMetricaNome = tipoNome
	v.NivelAgregacao = nivel
	v.RecorteUF = recorteUF.String
	if v.RecorteGGVs == nil {
		v.RecorteGGVs = []string{}
	}
	if v.TiposVendaValidos == nil {
		v.TiposVendaValidos = []string{}
	}
	v.ParametrosSchema = []ParametroSchemaDTO{}
	if len(schemaRaw) > 0 {
		_ = json.Unmarshal(schemaRaw, &v.ParametrosSchema)
	}
	v.ParametrosValores = map[string]any{}
	if len(parametrosRaw) > 0 {
		_ = json.Unmarshal(parametrosRaw, &v.ParametrosValores)
	}
	return &v, nil
}

const metaVinculoSelectBase = `
	SELECT mv.id, mv.industria_id, i.nome, mv.tipo_metrica_id, tm.nome, tm.nivel_agregacao,
	       tm.parametros_schema, mv.parametros_valores, mv.ativo, mv.created_at,
	       mv.recorte_uf, mv.recorte_ggvs, mv.tipos_venda_validos
	FROM farol.metas_vinculos mv
	JOIN farol.industrias i ON i.id = mv.industria_id
	JOIN farol.tipos_metrica tm ON tm.id = mv.tipo_metrica_id
`

// ─── MetasVinculosHandler — GET/POST /api/farol/metas-vinculos ────────────

func MetasVinculosHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		switch r.Method {
		case http.MethodGet:
			rows, err := db.Query(metaVinculoSelectBase+`
				WHERE mv.empresa_id = $1
				ORDER BY i.nome ASC, tm.nome ASC
			`, spCtx.EmpresaID)
			if err != nil {
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
			defer rows.Close()

			vinculos := []MetaVinculoResponse{}
			for rows.Next() {
				v, err := scanMetaVinculo(rows)
				if err != nil {
					http.Error(w, "Database error", http.StatusInternalServerError)
					return
				}
				vinculos = append(vinculos, *v)
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(vinculos)

		case http.MethodPost:
			if !hasSpRole(spCtx.SpRole, "gestor_geral") {
				http.Error(w, "Forbidden: gestor_geral necessário", http.StatusForbidden)
				return
			}
			var req MetaVinculoRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "corpo da requisição inválido", http.StatusBadRequest)
				return
			}
			if req.IndustriaID == 0 || req.TipoMetricaID == 0 {
				http.Error(w, "industria_id e tipo_metrica_id são obrigatórios", http.StatusBadRequest)
				return
			}

			_, _, schema, err := buscarTipoMetricaSchema(db, spCtx.EmpresaID, req.TipoMetricaID)
			if err != nil {
				if err == sql.ErrNoRows {
					http.Error(w, "Tipo de Métrica não encontrado", http.StatusBadRequest)
					return
				}
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
			if req.ParametrosValores == nil {
				req.ParametrosValores = map[string]any{}
			}
			if msg := validarParametrosValores(schema, req.ParametrosValores); msg != "" {
				http.Error(w, msg, http.StatusBadRequest)
				return
			}
			var industriaExiste bool
			db.QueryRow(`SELECT EXISTS(SELECT 1 FROM farol.industrias WHERE id = $1 AND empresa_id = $2)`, req.IndustriaID, spCtx.EmpresaID).Scan(&industriaExiste)
			if !industriaExiste {
				http.Error(w, "Indústria não encontrada", http.StatusBadRequest)
				return
			}

			ativo := true
			if req.Ativo != nil {
				ativo = *req.Ativo
			}
			if req.RecorteGGVs == nil {
				req.RecorteGGVs = []string{}
			}
			if req.TiposVendaValidos == nil {
				req.TiposVendaValidos = []string{}
			}
			valoresJSON, _ := json.Marshal(req.ParametrosValores)

			tx, err := db.Begin()
			if err != nil {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
			defer tx.Rollback()

			var id int
			err = tx.QueryRow(`
				INSERT INTO farol.metas_vinculos (empresa_id, industria_id, tipo_metrica_id, parametros_valores, ativo, recorte_uf, recorte_ggvs, tipos_venda_validos)
				VALUES ($1, $2, $3, $4, $5, NULLIF($6,''), $7, $8)
				RETURNING id
			`, spCtx.EmpresaID, req.IndustriaID, req.TipoMetricaID, valoresJSON, ativo, req.RecorteUF, pq.Array(req.RecorteGGVs), pq.Array(req.TiposVendaValidos)).Scan(&id)
			if err != nil {
				if strings.Contains(err.Error(), "uq_farol_metas_vinculos_empresa_industria_tipo") {
					http.Error(w, "Já existe um vínculo dessa indústria com esse Tipo de Métrica", http.StatusConflict)
					return
				}
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}

			auditPayload := map[string]any{
				"industria_id": req.IndustriaID, "tipo_metrica_id": req.TipoMetricaID,
				"parametros_valores": req.ParametrosValores, "ativo": ativo,
				"recorte_uf": req.RecorteUF, "recorte_ggvs": req.RecorteGGVs, "tipos_venda_validos": req.TiposVendaValidos,
			}
			if err := writeAuditLogTx(tx, spCtx.EmpresaID, spCtx.UserID, "metas_vinculos", strconv.Itoa(id), "criar", auditPayload); err != nil {
				http.Error(w, "Erro ao gravar auditoria", http.StatusInternalServerError)
				return
			}
			if err := tx.Commit(); err != nil {
				http.Error(w, "Commit error", http.StatusInternalServerError)
				return
			}
			log.Printf("MetasVinculos: criado vínculo %d (industria=%d, tipo=%d) empresa %s por %s", id, req.IndustriaID, req.TipoMetricaID, spCtx.EmpresaID, spCtx.UserID)
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]int{"id": id})

		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

// ─── MetaVinculoItemHandler — PUT/DELETE /api/farol/metas-vinculos/{id} ───

func MetaVinculoItemHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		idStr := pathSegment(r.URL.Path, "/api/farol/metas-vinculos/")
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
			var req MetaVinculoRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "corpo da requisição inválido", http.StatusBadRequest)
				return
			}

			tx, err := db.Begin()
			if err != nil {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
			defer tx.Rollback()

			antesRow := tx.QueryRow(metaVinculoSelectBase+` WHERE mv.id = $1 AND mv.empresa_id = $2`, id, spCtx.EmpresaID)
			antes, err := scanMetaVinculo(antesRow)
			if err != nil {
				if err == sql.ErrNoRows {
					http.Error(w, "Vínculo não encontrado", http.StatusNotFound)
					return
				}
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}

			tipoMetricaID := antes.TipoMetricaID
			if req.TipoMetricaID != 0 {
				tipoMetricaID = req.TipoMetricaID
			}
			_, _, schema, err := buscarTipoMetricaSchema(tx, spCtx.EmpresaID, tipoMetricaID)
			if err != nil {
				http.Error(w, "Tipo de Métrica não encontrado", http.StatusBadRequest)
				return
			}
			if req.ParametrosValores == nil {
				req.ParametrosValores = antes.ParametrosValores
			}
			if msg := validarParametrosValores(schema, req.ParametrosValores); msg != "" {
				http.Error(w, msg, http.StatusBadRequest)
				return
			}
			ativo := antes.Ativo
			if req.Ativo != nil {
				ativo = *req.Ativo
			}
			recorteUF := antes.RecorteUF
			if req.RecorteUF != "" {
				recorteUF = req.RecorteUF
			}
			recorteGGVs := req.RecorteGGVs
			if recorteGGVs == nil {
				recorteGGVs = antes.RecorteGGVs
			}
			tiposVendaValidos := req.TiposVendaValidos
			if tiposVendaValidos == nil {
				tiposVendaValidos = antes.TiposVendaValidos
			}
			valoresJSON, _ := json.Marshal(req.ParametrosValores)

			_, err = tx.Exec(`
				UPDATE farol.metas_vinculos
				SET tipo_metrica_id = $1, parametros_valores = $2, ativo = $3, updated_at = now(),
				    recorte_uf = NULLIF($4,''), recorte_ggvs = $5, tipos_venda_validos = $6
				WHERE id = $7 AND empresa_id = $8
			`, tipoMetricaID, valoresJSON, ativo, recorteUF, pq.Array(recorteGGVs), pq.Array(tiposVendaValidos), id, spCtx.EmpresaID)
			if err != nil {
				if strings.Contains(err.Error(), "uq_farol_metas_vinculos_empresa_industria_tipo") {
					http.Error(w, "Já existe um vínculo dessa indústria com esse Tipo de Métrica", http.StatusConflict)
					return
				}
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}

			auditPayload := map[string]any{
				"antes": antes,
				"depois": map[string]any{
					"tipo_metrica_id": tipoMetricaID, "parametros_valores": req.ParametrosValores, "ativo": ativo,
					"recorte_uf": recorteUF, "recorte_ggvs": recorteGGVs, "tipos_venda_validos": tiposVendaValidos,
				},
			}
			if err := writeAuditLogTx(tx, spCtx.EmpresaID, spCtx.UserID, "metas_vinculos", strconv.Itoa(id), "editar", auditPayload); err != nil {
				http.Error(w, "Erro ao gravar auditoria", http.StatusInternalServerError)
				return
			}
			if err := tx.Commit(); err != nil {
				http.Error(w, "Commit error", http.StatusInternalServerError)
				return
			}
			log.Printf("MetasVinculos: atualizado vínculo %d empresa %s por %s", id, spCtx.EmpresaID, spCtx.UserID)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"message": "Vínculo atualizado"})

		case http.MethodDelete:
			if !hasSpRole(spCtx.SpRole, "gestor_geral") {
				http.Error(w, "Forbidden: gestor_geral necessário", http.StatusForbidden)
				return
			}
			res, err := db.Exec(`DELETE FROM farol.metas_vinculos WHERE id = $1 AND empresa_id = $2`, id, spCtx.EmpresaID)
			if err != nil {
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
			if n, _ := res.RowsAffected(); n == 0 {
				http.Error(w, "Vínculo não encontrado", http.StatusNotFound)
				return
			}
			log.Printf("MetasVinculos: removido vínculo %d empresa %s por %s", id, spCtx.EmpresaID, spCtx.UserID)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"message": "Vínculo removido"})

		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}
