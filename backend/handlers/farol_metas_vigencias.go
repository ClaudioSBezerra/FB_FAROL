package handlers

// farol_metas_vigencias.go — Vigências e Faixas de meta (Épico 2, Story 2.2,
// módulo Painel de Gestão de Metas por Indústria)
//
// Uma Vigência é um período (ex.: um mês) em que um conjunto de metas por
// faixa vale, pra um Vínculo (farol_metas_vinculos.go). Ver
// migrations/217_metas_vigencias.sql pro modelo (histórico de vigências,
// EXCLUDE contra sobreposição, status aberta/fechada = congelamento FR17).
//
// Rotas (sp_role >= gestor_geral):
//   GET/POST     /api/farol/metas-vigencias                (?vinculo_id= no GET)
//   PUT/DELETE   /api/farol/metas-vigencias/{id}
//   POST         /api/farol/metas-vigencias/{id}/fechar     (congela a vigência)

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
)

// ─── DTOs ─────────────────────────────────────────────────────────────────────

type FaixaDTO struct {
	Faixa     int     `json:"faixa"`
	ValorMeta float64 `json:"valor_meta"`
}

type VigenciaRequest struct {
	VinculoID  int        `json:"vinculo_id"`
	DataInicio string     `json:"data_inicio"` // "YYYY-MM-DD"
	DataFim    string     `json:"data_fim"`
	Faixas     []FaixaDTO `json:"faixas"`
}

type VigenciaResponse struct {
	ID         int        `json:"id"`
	VinculoID  int        `json:"vinculo_id"`
	DataInicio string     `json:"data_inicio"`
	DataFim    string     `json:"data_fim"`
	Status     string     `json:"status"`
	Faixas     []FaixaDTO `json:"faixas"`
	CreatedAt  string     `json:"created_at"`
}

// ─── Validação ────────────────────────────────────────────────────────────────

func validarVigenciaRequest(req VigenciaRequest) string {
	if strings.TrimSpace(req.DataInicio) == "" || strings.TrimSpace(req.DataFim) == "" {
		return "data_inicio e data_fim são obrigatórias"
	}
	if len(req.Faixas) == 0 {
		return "toda vigência exige pelo menos 1 faixa de meta"
	}
	vistos := map[int]bool{}
	for _, f := range req.Faixas {
		if f.Faixa <= 0 {
			return "número de faixa inválido (precisa ser positivo)"
		}
		if vistos[f.Faixa] {
			return "faixa duplicada na mesma vigência"
		}
		vistos[f.Faixa] = true
	}
	return ""
}

func scanVigencia(row interface{ Scan(...any) error }) (*VigenciaResponse, error) {
	var v VigenciaResponse
	if err := row.Scan(&v.ID, &v.VinculoID, &v.DataInicio, &v.DataFim, &v.Status, &v.CreatedAt); err != nil {
		return nil, err
	}
	v.Faixas = []FaixaDTO{}
	return &v, nil
}

func carregarFaixas(db querierMulti, vigenciaID int) ([]FaixaDTO, error) {
	rows, err := db.Query(`SELECT faixa, valor_meta FROM farol.metas_faixas WHERE vigencia_id = $1 ORDER BY faixa DESC`, vigenciaID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	faixas := []FaixaDTO{}
	for rows.Next() {
		var f FaixaDTO
		if err := rows.Scan(&f.Faixa, &f.ValorMeta); err != nil {
			return nil, err
		}
		faixas = append(faixas, f)
	}
	return faixas, nil
}

// querierMulti abstrai *sql.DB e *sql.Tx pro Query usado em carregarFaixas.
type querierMulti interface {
	Query(query string, args ...any) (*sql.Rows, error)
}

// inserirVigenciaTx insere uma vigência + suas faixas dentro da transação
// do caller. Reaproveitada pelo POST normal e pela importação CSV (Story
// 3.1) — mesma regra de negócio, dois pontos de entrada.
func inserirVigenciaTx(tx *sql.Tx, empresaID string, vinculoID int, dataInicio, dataFim string, faixas []FaixaDTO) (int, error) {
	var id int
	err := tx.QueryRow(`
		INSERT INTO farol.metas_vigencias (empresa_id, vinculo_id, data_inicio, data_fim)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`, empresaID, vinculoID, dataInicio, dataFim).Scan(&id)
	if err != nil {
		return 0, err
	}
	for _, f := range faixas {
		if _, err := tx.Exec(`INSERT INTO farol.metas_faixas (empresa_id, vigencia_id, faixa, valor_meta) VALUES ($1, $2, $3, $4)`,
			empresaID, id, f.Faixa, f.ValorMeta); err != nil {
			return 0, err
		}
	}
	return id, nil
}

// ─── MetasVigenciasHandler — GET/POST /api/farol/metas-vigencias ──────────

func MetasVigenciasHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		switch r.Method {
		case http.MethodGet:
			vinculoIDStr := r.URL.Query().Get("vinculo_id")
			var rows *sql.Rows
			var err error
			if vinculoIDStr != "" {
				rows, err = db.Query(`
					SELECT id, vinculo_id, data_inicio::text, data_fim::text, status, created_at
					FROM farol.metas_vigencias WHERE empresa_id = $1 AND vinculo_id = $2
					ORDER BY data_inicio DESC
				`, spCtx.EmpresaID, vinculoIDStr)
			} else {
				rows, err = db.Query(`
					SELECT id, vinculo_id, data_inicio::text, data_fim::text, status, created_at
					FROM farol.metas_vigencias WHERE empresa_id = $1
					ORDER BY data_inicio DESC
				`, spCtx.EmpresaID)
			}
			if err != nil {
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
			defer rows.Close()

			vigencias := []VigenciaResponse{}
			for rows.Next() {
				v, err := scanVigencia(rows)
				if err != nil {
					http.Error(w, "Database error", http.StatusInternalServerError)
					return
				}
				faixas, err := carregarFaixas(db, v.ID)
				if err != nil {
					http.Error(w, "Database error", http.StatusInternalServerError)
					return
				}
				v.Faixas = faixas
				vigencias = append(vigencias, *v)
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(vigencias)

		case http.MethodPost:
			if !hasSpRole(spCtx.SpRole, "gestor_geral") {
				http.Error(w, "Forbidden: gestor_geral necessário", http.StatusForbidden)
				return
			}
			var req VigenciaRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "corpo da requisição inválido", http.StatusBadRequest)
				return
			}
			if req.VinculoID == 0 {
				http.Error(w, "vinculo_id é obrigatório", http.StatusBadRequest)
				return
			}
			if msg := validarVigenciaRequest(req); msg != "" {
				http.Error(w, msg, http.StatusBadRequest)
				return
			}
			var vinculoExiste bool
			db.QueryRow(`SELECT EXISTS(SELECT 1 FROM farol.metas_vinculos WHERE id = $1 AND empresa_id = $2)`, req.VinculoID, spCtx.EmpresaID).Scan(&vinculoExiste)
			if !vinculoExiste {
				http.Error(w, "Vínculo não encontrado", http.StatusBadRequest)
				return
			}

			tx, err := db.Begin()
			if err != nil {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
			defer tx.Rollback()

			id, err := inserirVigenciaTx(tx, spCtx.EmpresaID, req.VinculoID, req.DataInicio, req.DataFim, req.Faixas)
			if err != nil {
				if strings.Contains(err.Error(), "ex_farol_metas_vigencias_sem_overlap") {
					http.Error(w, "Já existe uma vigência deste vínculo que se sobrepõe a este período", http.StatusConflict)
					return
				}
				http.Error(w, "Database error: "+err.Error(), http.StatusInternalServerError)
				return
			}

			auditPayload := map[string]any{"vinculo_id": req.VinculoID, "data_inicio": req.DataInicio, "data_fim": req.DataFim, "faixas": req.Faixas}
			if err := writeAuditLogTx(tx, spCtx.EmpresaID, spCtx.UserID, "metas_vigencias", strconv.Itoa(id), "criar", auditPayload); err != nil {
				http.Error(w, "Erro ao gravar auditoria", http.StatusInternalServerError)
				return
			}
			if err := tx.Commit(); err != nil {
				http.Error(w, "Commit error", http.StatusInternalServerError)
				return
			}
			log.Printf("MetasVigencias: criada vigência %d (vinculo=%d, %s..%s) empresa %s por %s", id, req.VinculoID, req.DataInicio, req.DataFim, spCtx.EmpresaID, spCtx.UserID)
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]int{"id": id})

		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

// ─── MetaVigenciaItemHandler — PUT/DELETE .../{id}, POST .../{id}/fechar ──

func MetaVigenciaItemHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		idStr := pathSegment(r.URL.Path, "/api/farol/metas-vigencias/")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			http.Error(w, "ID inválido", http.StatusBadRequest)
			return
		}
		fechar := strings.HasSuffix(strings.TrimRight(r.URL.Path, "/"), "/fechar")

		if fechar && r.Method == http.MethodPost {
			if !hasSpRole(spCtx.SpRole, "gestor_geral") {
				http.Error(w, "Forbidden: gestor_geral necessário", http.StatusForbidden)
				return
			}
			res, err := db.Exec(`UPDATE farol.metas_vigencias SET status = 'fechada', updated_at = now() WHERE id = $1 AND empresa_id = $2 AND status = 'aberta'`, id, spCtx.EmpresaID)
			if err != nil {
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
			n, _ := res.RowsAffected()
			if n == 0 {
				http.Error(w, "Vigência não encontrada ou já fechada", http.StatusNotFound)
				return
			}
			writeAuditLog(db, spCtx.EmpresaID, spCtx.UserID, "metas_vigencias", strconv.Itoa(id), "fechar", nil)
			log.Printf("MetasVigencias: fechada vigência %d empresa %s por %s", id, spCtx.EmpresaID, spCtx.UserID)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"message": "Vigência fechada"})
			return
		}

		switch r.Method {
		case http.MethodPut:
			if !hasSpRole(spCtx.SpRole, "gestor_geral") {
				http.Error(w, "Forbidden: gestor_geral necessário", http.StatusForbidden)
				return
			}
			var req VigenciaRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "corpo da requisição inválido", http.StatusBadRequest)
				return
			}
			if msg := validarVigenciaRequest(req); msg != "" {
				http.Error(w, msg, http.StatusBadRequest)
				return
			}

			tx, err := db.Begin()
			if err != nil {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
			defer tx.Rollback()

			var statusAtual string
			err = tx.QueryRow(`SELECT status FROM farol.metas_vigencias WHERE id = $1 AND empresa_id = $2`, id, spCtx.EmpresaID).Scan(&statusAtual)
			if err != nil {
				if err == sql.ErrNoRows {
					http.Error(w, "Vigência não encontrada", http.StatusNotFound)
					return
				}
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
			if statusAtual == "fechada" {
				http.Error(w, "Vigência fechada não pode ser editada por esta tela — exige reprocessamento manual", http.StatusForbidden)
				return
			}

			_, err = tx.Exec(`UPDATE farol.metas_vigencias SET data_inicio = $1, data_fim = $2, updated_at = now() WHERE id = $3 AND empresa_id = $4`,
				req.DataInicio, req.DataFim, id, spCtx.EmpresaID)
			if err != nil {
				if strings.Contains(err.Error(), "ex_farol_metas_vigencias_sem_overlap") {
					http.Error(w, "Já existe uma vigência deste vínculo que se sobrepõe a este período", http.StatusConflict)
					return
				}
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
			if _, err := tx.Exec(`DELETE FROM farol.metas_faixas WHERE vigencia_id = $1`, id); err != nil {
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
			for _, f := range req.Faixas {
				if _, err := tx.Exec(`INSERT INTO farol.metas_faixas (empresa_id, vigencia_id, faixa, valor_meta) VALUES ($1, $2, $3, $4)`,
					spCtx.EmpresaID, id, f.Faixa, f.ValorMeta); err != nil {
					http.Error(w, "Database error ao salvar faixas", http.StatusInternalServerError)
					return
				}
			}

			auditPayload := map[string]any{"data_inicio": req.DataInicio, "data_fim": req.DataFim, "faixas": req.Faixas}
			if err := writeAuditLogTx(tx, spCtx.EmpresaID, spCtx.UserID, "metas_vigencias", strconv.Itoa(id), "editar", auditPayload); err != nil {
				http.Error(w, "Erro ao gravar auditoria", http.StatusInternalServerError)
				return
			}
			if err := tx.Commit(); err != nil {
				http.Error(w, "Commit error", http.StatusInternalServerError)
				return
			}
			log.Printf("MetasVigencias: atualizada vigência %d empresa %s por %s", id, spCtx.EmpresaID, spCtx.UserID)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"message": "Vigência atualizada"})

		case http.MethodDelete:
			if !hasSpRole(spCtx.SpRole, "gestor_geral") {
				http.Error(w, "Forbidden: gestor_geral necessário", http.StatusForbidden)
				return
			}
			res, err := db.Exec(`DELETE FROM farol.metas_vigencias WHERE id = $1 AND empresa_id = $2 AND status = 'aberta'`, id, spCtx.EmpresaID)
			if err != nil {
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
			if n, _ := res.RowsAffected(); n == 0 {
				var existeFechada bool
				db.QueryRow(`SELECT EXISTS(SELECT 1 FROM farol.metas_vigencias WHERE id = $1 AND empresa_id = $2 AND status = 'fechada')`, id, spCtx.EmpresaID).Scan(&existeFechada)
				if existeFechada {
					http.Error(w, "Vigência fechada não pode ser removida", http.StatusForbidden)
					return
				}
				http.Error(w, "Vigência não encontrada", http.StatusNotFound)
				return
			}
			log.Printf("MetasVigencias: removida vigência %d empresa %s por %s", id, spCtx.EmpresaID, spCtx.UserID)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"message": "Vigência removida"})

		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}
