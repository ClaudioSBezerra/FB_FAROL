package handlers

// sp_ignorados.go — CRUD de produtos ignorados na calibragem
//
// GET    /api/sp/ignorados/tipos   → lista tipos de ignorado ativos
// GET    /api/sp/ignorados?cd_id=X → lista produtos ignorados do CD
// DELETE /api/sp/ignorados/{id}    → reativa o produto (remove da lista)

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

type IgnoradoResponse struct {
	ID           int64   `json:"id"`
	CdID         int     `json:"cd_id"`
	CodProd      int     `json:"codprod"`
	CodFilial    int     `json:"cod_filial"`
	Produto      *string `json:"produto"`
	TipoDescricao *string `json:"tipo_descricao"`
	IgnoradoPor  *string `json:"ignorado_por,omitempty"`
	CreatedAt    string  `json:"created_at"`
}

type TipoIgnoradoResponse struct {
	ID       int    `json:"id"`
	Codigo   int    `json:"codigo"`
	Descricao string `json:"descricao"`
}

// SpIgnoradosTiposHandler lista os tipos de ignorado ativos.
// GET /api/sp/ignorados/tipos
func SpIgnoradosTiposHandler(db *sql.DB) http.HandlerFunc {
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

		rows, err := db.Query(`
			SELECT id, codigo, descricao FROM farol.sp_tipo_ignorado
			WHERE ativo = true ORDER BY codigo
		`)
		if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var tipos []TipoIgnoradoResponse
		for rows.Next() {
			var t TipoIgnoradoResponse
			if rows.Scan(&t.ID, &t.Codigo, &t.Descricao) == nil {
				tipos = append(tipos, t)
			}
		}
		if tipos == nil {
			tipos = []TipoIgnoradoResponse{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tipos)
	}
}

// SpIgnoradosHandler lista e remove produtos ignorados.
// GET    /api/sp/ignorados?cd_id=X
// DELETE /api/sp/ignorados/{id}
func SpIgnoradosHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Verifica se há ID no path → DELETE (reativar)
		rawPath := strings.TrimPrefix(r.URL.Path, "/api/sp/ignorados")
		rawPath = strings.TrimPrefix(rawPath, "/")
		if rawPath != "" {
			if r.Method != http.MethodDelete {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			if !spCtx.CanApprove() {
				http.Error(w, "Forbidden: gestor_geral+ para reativar produto", http.StatusForbidden)
				return
			}
			ignoradoID, err := strconv.ParseInt(rawPath, 10, 64)
			if err != nil {
				http.Error(w, "ID inválido", http.StatusBadRequest)
				return
			}

			// Em transação: deleta o registro de ignorados E
			// volta o status das propostas correspondentes de 'ignorado' para 'pendente'
			tx, err := db.Begin()
			if err != nil {
				http.Error(w, "Erro ao iniciar transação", http.StatusInternalServerError)
				return
			}
			defer tx.Rollback()

			// Captura a chave (cd_id, codprod, cod_filial) antes do delete
			var cdID, codprod, codFilial int
			err = tx.QueryRow(`
				SELECT cd_id, codprod, cod_filial
				  FROM farol.sp_ignorados
				 WHERE id = $1 AND empresa_id = $2
			`, ignoradoID, spCtx.EmpresaID).Scan(&cdID, &codprod, &codFilial)
			if err == sql.ErrNoRows {
				http.Error(w, "Produto ignorado não encontrado", http.StatusNotFound)
				return
			}
			if err != nil {
				http.Error(w, "Erro ao buscar registro: "+err.Error(), http.StatusInternalServerError)
				return
			}

			if _, err = tx.Exec(`
				DELETE FROM farol.sp_ignorados
				 WHERE id = $1 AND empresa_id = $2
			`, ignoradoID, spCtx.EmpresaID); err != nil {
				http.Error(w, "Erro ao reativar produto: "+err.Error(), http.StatusInternalServerError)
				return
			}

			// Restaura status das propostas com status='ignorado' do mesmo produto/CD/filial
			// para 'pendente' e limpa os campos de aprovação que foram preenchidos no ignorar.
			if _, err = tx.Exec(`
				UPDATE farol.sp_propostas
				   SET status = 'pendente',
				       aprovado_por = NULL,
				       aprovado_em  = NULL
				 WHERE empresa_id = $1
				   AND cd_id      = $2
				   AND codprod    = $3
				   AND cod_filial = $4
				   AND status     = 'ignorado'
			`, spCtx.EmpresaID, cdID, codprod, codFilial); err != nil {
				http.Error(w, "Erro ao restaurar propostas: "+err.Error(), http.StatusInternalServerError)
				return
			}

			if err := tx.Commit(); err != nil {
				http.Error(w, "Erro ao confirmar operação", http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"message": "Produto reativado com sucesso"})
			return
		}

		// GET — lista
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		cdIDStr := r.URL.Query().Get("cd_id")
		query := `
			SELECT i.id, i.cd_id, i.codprod, i.cod_filial, i.produto,
			       t.descricao,
			       u.full_name,
			       TO_CHAR(i.created_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"')
			FROM farol.sp_ignorados i
			LEFT JOIN farol.sp_tipo_ignorado t ON t.id = i.tipo_ignorado_id
			LEFT JOIN public.users u ON u.id = i.ignorado_por
			WHERE i.empresa_id = $1
		`
		args := []interface{}{spCtx.EmpresaID}
		if cdIDStr != "" {
			query += " AND i.cd_id = $2"
			args = append(args, cdIDStr)
		}
		query += " ORDER BY i.created_at DESC"

		rows, err := db.Query(query, args...)
		if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var lista []IgnoradoResponse
		for rows.Next() {
			var item IgnoradoResponse
			if err := rows.Scan(
				&item.ID, &item.CdID, &item.CodProd, &item.CodFilial,
				&item.Produto, &item.TipoDescricao, &item.IgnoradoPor, &item.CreatedAt,
			); err != nil {
				continue
			}
			lista = append(lista, item)
		}
		if lista == nil {
			lista = []IgnoradoResponse{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(lista)
	}
}
