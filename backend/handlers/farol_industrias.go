package handlers

// farol_industrias.go — Cadastro de Indústrias (mapeamento de fornecedores)
//
// Mapeia N cod_fornec (WinThor) pra 1 indústria canônica — o mesmo fabricante
// às vezes tem cod_fornec diferente por filial. Ver
// spec-cadastro-industria.md. Só o cadastro (CRUD); nenhum filtro cruzado ou
// tela existente consome esta tabela ainda.
//
// Rotas:
//   GET/POST         /api/farol/industrias
//   PUT/DELETE       /api/farol/industrias/{id}

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
)

// ─── DTOs ─────────────────────────────────────────────────────────────────────

type IndustriaFornecedorDTO struct {
	CodFornec string `json:"cod_fornec"`
	Rotulo    string `json:"rotulo,omitempty"`
}

type IndustriaRequest struct {
	Nome         string                   `json:"nome"`
	RazaoSocial  string                   `json:"razao_social"`
	Ativo        *bool                    `json:"ativo"`
	Fornecedores []IndustriaFornecedorDTO `json:"fornecedores"`
}

type IndustriaResponse struct {
	ID           int                      `json:"id"`
	Nome         string                   `json:"nome"`
	RazaoSocial  string                   `json:"razao_social,omitempty"`
	Ativo        bool                     `json:"ativo"`
	CreatedAt    string                   `json:"created_at"`
	Fornecedores []IndustriaFornecedorDTO `json:"fornecedores"`
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// conflitoIndustriaFornecedor identifica, quando um INSERT em
// industria_fornecedores viola uq_farol_industria_fornecedores_empresa_cod,
// qual indústria já usa aquele cod_fornec — pra devolver 409 com mensagem
// clara em vez de erro genérico.
func conflitoIndustriaFornecedor(db *sql.DB, empresaID, codFornec string) string {
	var nomeExistente string
	db.QueryRow(`
		SELECT i.nome FROM farol.industria_fornecedores fo
		JOIN farol.industrias i ON i.id = fo.industria_id
		WHERE fo.empresa_id = $1 AND fo.cod_fornec = $2
	`, empresaID, codFornec).Scan(&nomeExistente)
	return nomeExistente
}

func scanIndustrias(rows *sql.Rows) ([]IndustriaResponse, error) {
	porID := map[int]*IndustriaResponse{}
	var ordem []int
	for rows.Next() {
		var (
			id                int
			nome              string
			razaoSocial       sql.NullString
			ativo             bool
			createdAt         string
			codFornec, rotulo sql.NullString
		)
		if err := rows.Scan(&id, &nome, &razaoSocial, &ativo, &createdAt, &codFornec, &rotulo); err != nil {
			return nil, err
		}
		ind, ok := porID[id]
		if !ok {
			ind = &IndustriaResponse{
				ID: id, Nome: nome, RazaoSocial: razaoSocial.String,
				Ativo: ativo, CreatedAt: createdAt, Fornecedores: []IndustriaFornecedorDTO{},
			}
			porID[id] = ind
			ordem = append(ordem, id)
		}
		if codFornec.Valid {
			ind.Fornecedores = append(ind.Fornecedores, IndustriaFornecedorDTO{
				CodFornec: codFornec.String, Rotulo: rotulo.String,
			})
		}
	}
	out := make([]IndustriaResponse, 0, len(ordem))
	for _, id := range ordem {
		out = append(out, *porID[id])
	}
	return out, nil
}

// ─── IndustriasHandler — GET/POST /api/farol/industrias ────────────────────

func IndustriasHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		switch r.Method {
		case http.MethodGet:
			rows, err := db.Query(`
				SELECT i.id, i.nome, i.razao_social, i.ativo, i.created_at,
				       fo.cod_fornec, fo.rotulo
				FROM farol.industrias i
				LEFT JOIN farol.industria_fornecedores fo ON fo.industria_id = i.id
				WHERE i.empresa_id = $1
				ORDER BY i.nome ASC, fo.cod_fornec ASC
			`, spCtx.EmpresaID)
			if err != nil {
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
			defer rows.Close()
			industrias, err := scanIndustrias(rows)
			if err != nil {
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(industrias)

		case http.MethodPost:
			if !RequireWrite(spCtx, w) {
				return
			}
			var req IndustriaRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Nome) == "" {
				http.Error(w, "nome é obrigatório", http.StatusBadRequest)
				return
			}
			ativo := true
			if req.Ativo != nil {
				ativo = *req.Ativo
			}

			tx, err := db.Begin()
			if err != nil {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
			defer tx.Rollback()

			var id int
			err = tx.QueryRow(`
				INSERT INTO farol.industrias (empresa_id, nome, razao_social, ativo)
				VALUES ($1, $2, NULLIF($3,''), $4)
				RETURNING id
			`, spCtx.EmpresaID, req.Nome, req.RazaoSocial, ativo).Scan(&id)
			if err != nil {
				if strings.Contains(err.Error(), "uq_farol_industrias_empresa_nome") {
					http.Error(w, "Já existe uma indústria com esse nome", http.StatusConflict)
					return
				}
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}

			for _, f := range req.Fornecedores {
				cod := strings.TrimSpace(f.CodFornec)
				if cod == "" {
					continue
				}
				if _, err := tx.Exec(`
					INSERT INTO farol.industria_fornecedores (empresa_id, industria_id, cod_fornec, rotulo)
					VALUES ($1, $2, $3, NULLIF($4,''))
				`, spCtx.EmpresaID, id, cod, f.Rotulo); err != nil {
					if strings.Contains(err.Error(), "uq_farol_industria_fornecedores_empresa_cod") {
						conflitante := conflitoIndustriaFornecedor(db, spCtx.EmpresaID, cod)
						http.Error(w, "cod_fornec "+cod+" já está vinculado à indústria \""+conflitante+"\"", http.StatusConflict)
						return
					}
					http.Error(w, "Database error", http.StatusInternalServerError)
					return
				}
			}

			if err := tx.Commit(); err != nil {
				http.Error(w, "Commit error", http.StatusInternalServerError)
				return
			}
			log.Printf("Industrias: criada indústria %d (%s) empresa %s por %s", id, req.Nome, spCtx.EmpresaID, spCtx.UserID)
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]int{"id": id})

		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

// ─── IndustriaItemHandler — PUT/DELETE /api/farol/industrias/{id} ──────────

func IndustriaItemHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		idStr := pathSegment(r.URL.Path, "/api/farol/industrias/")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			http.Error(w, "ID inválido", http.StatusBadRequest)
			return
		}

		switch r.Method {
		case http.MethodPut:
			if !RequireWrite(spCtx, w) {
				return
			}
			var req IndustriaRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Nome) == "" {
				http.Error(w, "nome é obrigatório", http.StatusBadRequest)
				return
			}
			ativo := true
			if req.Ativo != nil {
				ativo = *req.Ativo
			}

			tx, err := db.Begin()
			if err != nil {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
			defer tx.Rollback()

			res, err := tx.Exec(`
				UPDATE farol.industrias
				SET nome = $1, razao_social = NULLIF($2,''), ativo = $3, updated_at = now()
				WHERE id = $4 AND empresa_id = $5
			`, req.Nome, req.RazaoSocial, ativo, id, spCtx.EmpresaID)
			if err != nil {
				if strings.Contains(err.Error(), "uq_farol_industrias_empresa_nome") {
					http.Error(w, "Já existe uma indústria com esse nome", http.StatusConflict)
					return
				}
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
			if n, _ := res.RowsAffected(); n == 0 {
				http.Error(w, "Indústria não encontrada", http.StatusNotFound)
				return
			}

			// PUT faz REPLACE total do conjunto de fornecedores.
			if _, err := tx.Exec(`DELETE FROM farol.industria_fornecedores WHERE industria_id = $1 AND empresa_id = $2`, id, spCtx.EmpresaID); err != nil {
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
			for _, f := range req.Fornecedores {
				cod := strings.TrimSpace(f.CodFornec)
				if cod == "" {
					continue
				}
				if _, err := tx.Exec(`
					INSERT INTO farol.industria_fornecedores (empresa_id, industria_id, cod_fornec, rotulo)
					VALUES ($1, $2, $3, NULLIF($4,''))
				`, spCtx.EmpresaID, id, cod, f.Rotulo); err != nil {
					if strings.Contains(err.Error(), "uq_farol_industria_fornecedores_empresa_cod") {
						conflitante := conflitoIndustriaFornecedor(db, spCtx.EmpresaID, cod)
						http.Error(w, "cod_fornec "+cod+" já está vinculado à indústria \""+conflitante+"\"", http.StatusConflict)
						return
					}
					http.Error(w, "Database error", http.StatusInternalServerError)
					return
				}
			}

			if err := tx.Commit(); err != nil {
				http.Error(w, "Commit error", http.StatusInternalServerError)
				return
			}
			log.Printf("Industrias: atualizada indústria %d empresa %s por %s", id, spCtx.EmpresaID, spCtx.UserID)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"message": "Indústria atualizada"})

		case http.MethodDelete:
			if !RequireWrite(spCtx, w) {
				return
			}
			res, err := db.Exec(`DELETE FROM farol.industrias WHERE id = $1 AND empresa_id = $2`, id, spCtx.EmpresaID)
			if err != nil {
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
			if n, _ := res.RowsAffected(); n == 0 {
				http.Error(w, "Indústria não encontrada", http.StatusNotFound)
				return
			}
			log.Printf("Industrias: removida indústria %d empresa %s por %s", id, spCtx.EmpresaID, spCtx.UserID)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"message": "Indústria removida"})

		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}
