package handlers

// farol_gamificacao.go — MVP de Gamificação (pedido do José Costa, CEO da
// JC, via Claudio 22/09/2026): "a informação de última venda por si só não
// vai fazer o RCA vender mais" — o pedido real é criar competição/prêmio no
// estilo Mercado Livre/iFood — pontos, ranking e bônus (inclusive em R$,
// ex. R$300 por vender microgarrafas de vodka pra hotéis, um incentivo por
// PRODUTO específico, diferente de Cobertura/Sortimento de Rede).
//
// Acesso restrito a admin_fbtax por enquanto (só o Claudio) — ver
// FbtaxAdminRoute no frontend e withSP(..., "admin_fbtax") em main.go.
//
// Decisões fechadas com o Claudio (22/09/2026):
//   - Bônus pode ser em R$ de verdade (não só pontos) — mas o MVP só
//     CALCULA e MOSTRA; não paga nada automaticamente (RCAs são autônomos,
//     pagamento seria avulso/fora do sistema — decisão de processo em
//     aberto, fora de escopo aqui).
//   - Ranking mostra a POSIÇÃO do RCA, nunca quem está na frente dele —
//     ver GamifRankingHandler, modo "visão do RCA" (?cod_rca=).
//   - Base de pontuação inicial: reaproveita Cobertura/Sortimento já
//     calculados (obterOuCongelarRealizado) — sem motor novo pra isso.
//   - Granularidade é CLIENTE (loja), não Rede: "cada RCA é responsável
//     pelo cliente e pela Rede dele... esse acompanhamento tem que ser no
//     último nível" — premiar pela média da Rede recompensaria/puniria o
//     RCA por lojas que ele não visita naquele momento. Nada de visão
//     agregada de fechamento de Redes da Indústria como um todo por
//     enquanto.
//
// 3 tabelas novas (migration 240): gamif_campanhas, gamif_regras,
// gamif_pontuacao — ver comentário da migration pro desenho completo (o
// comentário da migration ainda descreve a 1ª versão, por Rede — a
// granularidade real por Cliente está aqui e em CalcularPontuacaoCampanha).

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/lib/pq"
)

// ─── DTOs ─────────────────────────────────────────────────────────────────────

type GamifRegraResponse struct {
	ID          int      `json:"id"`
	CampanhaID  int      `json:"campanha_id"`
	Tipo        string   `json:"tipo"` // cobertura_atingida | sortimento_atingido (por CLIENTE/loja) | produto_especifico (por RCA)
	Descricao   string   `json:"descricao"`
	VinculoID   *int     `json:"vinculo_id,omitempty"`
	VigenciaID  *int     `json:"vigencia_id,omitempty"`
	CodProds    []string `json:"cod_prods,omitempty"`
	QtdMinima   float64  `json:"qtd_minima,omitempty"`
	Fluxo       string   `json:"fluxo"`
	Pontos      float64  `json:"pontos"`
	ValorBonus  float64  `json:"valor_bonus"`
}

type GamifRegraRequest struct {
	CampanhaID int      `json:"campanha_id"`
	Tipo       string   `json:"tipo"`
	Descricao  string   `json:"descricao"`
	VinculoID  int      `json:"vinculo_id"`
	VigenciaID int      `json:"vigencia_id"`
	CodProds   []string `json:"cod_prods"`
	QtdMinima  float64  `json:"qtd_minima"`
	Fluxo      string   `json:"fluxo"`
	Pontos     float64  `json:"pontos"`
	ValorBonus float64  `json:"valor_bonus"`
}

type GamifCampanhaResponse struct {
	ID            int                  `json:"id"`
	IndustriaID   int                  `json:"industria_id"`
	IndustriaNome string               `json:"industria_nome"`
	Nome          string               `json:"nome"`
	DataInicio    string               `json:"data_inicio"`
	DataFim       string               `json:"data_fim"`
	Status        string               `json:"status"`
	CreatedAt     string               `json:"created_at"`
	Regras        []GamifRegraResponse `json:"regras,omitempty"`
}

type GamifCampanhaRequest struct {
	IndustriaID int    `json:"industria_id"`
	Nome        string `json:"nome"`
	DataInicio  string `json:"data_inicio"`
	DataFim     string `json:"data_fim"`
	Status      string `json:"status"`
}

// ─── GamifCampanhasHandler — GET/POST /api/farol/gamif-campanhas ──────────────

func GamifCampanhasHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		switch r.Method {
		case http.MethodGet:
			rows, err := db.Query(`
				SELECT c.id, c.industria_id, i.nome, c.nome, c.data_inicio::text, c.data_fim::text, c.status, c.created_at::text
				FROM farol.gamif_campanhas c
				JOIN farol.industrias i ON i.id = c.industria_id
				WHERE c.empresa_id = $1
				ORDER BY c.created_at DESC
			`, spCtx.EmpresaID)
			if err != nil {
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
			defer rows.Close()
			campanhas := []GamifCampanhaResponse{}
			for rows.Next() {
				var c GamifCampanhaResponse
				if err := rows.Scan(&c.ID, &c.IndustriaID, &c.IndustriaNome, &c.Nome, &c.DataInicio, &c.DataFim, &c.Status, &c.CreatedAt); err != nil {
					http.Error(w, "Database error", http.StatusInternalServerError)
					return
				}
				campanhas = append(campanhas, c)
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(campanhas)

		case http.MethodPost:
			var req GamifCampanhaRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "corpo da requisição inválido", http.StatusBadRequest)
				return
			}
			if req.IndustriaID == 0 || strings.TrimSpace(req.Nome) == "" || req.DataInicio == "" || req.DataFim == "" {
				http.Error(w, "industria_id, nome, data_inicio e data_fim são obrigatórios", http.StatusBadRequest)
				return
			}
			var industriaExiste bool
			db.QueryRow(`SELECT EXISTS(SELECT 1 FROM farol.industrias WHERE id = $1 AND empresa_id = $2)`, req.IndustriaID, spCtx.EmpresaID).Scan(&industriaExiste)
			if !industriaExiste {
				http.Error(w, "Indústria não encontrada", http.StatusBadRequest)
				return
			}
			var id int
			err := db.QueryRow(`
				INSERT INTO farol.gamif_campanhas (empresa_id, industria_id, nome, data_inicio, data_fim, created_by)
				VALUES ($1, $2, $3, $4, $5, $6) RETURNING id
			`, spCtx.EmpresaID, req.IndustriaID, req.Nome, req.DataInicio, req.DataFim, spCtx.UserID).Scan(&id)
			if err != nil {
				http.Error(w, "Database error: "+err.Error(), http.StatusInternalServerError)
				return
			}
			writeAuditLog(db, spCtx.EmpresaID, spCtx.UserID, "gamif_campanhas", strconv.Itoa(id), "criar", map[string]any{
				"industria_id": req.IndustriaID, "nome": req.Nome, "data_inicio": req.DataInicio, "data_fim": req.DataFim,
			})
			log.Printf("Gamificacao: criada campanha %d (%s) empresa %s por %s", id, req.Nome, spCtx.EmpresaID, spCtx.UserID)
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]int{"id": id})

		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

// ─── GamifCampanhaItemHandler — GET/PUT/DELETE /api/farol/gamif-campanhas/{id} ─

func GamifCampanhaItemHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		id, err := strconv.Atoi(pathSegment(r.URL.Path, "/api/farol/gamif-campanhas/"))
		if err != nil {
			http.Error(w, "ID inválido", http.StatusBadRequest)
			return
		}

		switch r.Method {
		case http.MethodGet:
			var c GamifCampanhaResponse
			err := db.QueryRow(`
				SELECT c.id, c.industria_id, i.nome, c.nome, c.data_inicio::text, c.data_fim::text, c.status, c.created_at::text
				FROM farol.gamif_campanhas c
				JOIN farol.industrias i ON i.id = c.industria_id
				WHERE c.id = $1 AND c.empresa_id = $2
			`, id, spCtx.EmpresaID).Scan(&c.ID, &c.IndustriaID, &c.IndustriaNome, &c.Nome, &c.DataInicio, &c.DataFim, &c.Status, &c.CreatedAt)
			if err == sql.ErrNoRows {
				http.Error(w, "Campanha não encontrada", http.StatusNotFound)
				return
			} else if err != nil {
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
			c.Regras, err = listarGamifRegras(db, id)
			if err != nil {
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(c)

		case http.MethodPut:
			var req GamifCampanhaRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "corpo da requisição inválido", http.StatusBadRequest)
				return
			}
			if req.Status != "" && req.Status != "ativa" && req.Status != "encerrada" {
				http.Error(w, "status inválido (use ativa ou encerrada)", http.StatusBadRequest)
				return
			}
			res, err := db.Exec(`
				UPDATE farol.gamif_campanhas SET
					nome = COALESCE(NULLIF($3,''), nome),
					data_inicio = COALESCE(NULLIF($4,'')::date, data_inicio),
					data_fim = COALESCE(NULLIF($5,'')::date, data_fim),
					status = COALESCE(NULLIF($6,''), status),
					updated_at = now()
				WHERE id = $1 AND empresa_id = $2
			`, id, spCtx.EmpresaID, req.Nome, req.DataInicio, req.DataFim, req.Status)
			if err != nil {
				http.Error(w, "Database error: "+err.Error(), http.StatusInternalServerError)
				return
			}
			if n, _ := res.RowsAffected(); n == 0 {
				http.Error(w, "Campanha não encontrada", http.StatusNotFound)
				return
			}
			writeAuditLog(db, spCtx.EmpresaID, spCtx.UserID, "gamif_campanhas", strconv.Itoa(id), "editar", req)
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]bool{"ok": true})

		case http.MethodDelete:
			res, err := db.Exec(`DELETE FROM farol.gamif_campanhas WHERE id = $1 AND empresa_id = $2`, id, spCtx.EmpresaID)
			if err != nil {
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
			if n, _ := res.RowsAffected(); n == 0 {
				http.Error(w, "Campanha não encontrada", http.StatusNotFound)
				return
			}
			writeAuditLog(db, spCtx.EmpresaID, spCtx.UserID, "gamif_campanhas", strconv.Itoa(id), "excluir", nil)
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]bool{"ok": true})

		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func listarGamifRegras(db *sql.DB, campanhaID int) ([]GamifRegraResponse, error) {
	rows, err := db.Query(`
		SELECT id, campanha_id, tipo, descricao, vinculo_id, vigencia_id, cod_prods, qtd_minima, fluxo, pontos, valor_bonus
		FROM farol.gamif_regras WHERE campanha_id = $1 ORDER BY id
	`, campanhaID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	regras := []GamifRegraResponse{}
	for rows.Next() {
		var r GamifRegraResponse
		var vinculoID, vigenciaID sql.NullInt64
		if err := rows.Scan(&r.ID, &r.CampanhaID, &r.Tipo, &r.Descricao, &vinculoID, &vigenciaID, pq.Array(&r.CodProds), &r.QtdMinima, &r.Fluxo, &r.Pontos, &r.ValorBonus); err != nil {
			return nil, err
		}
		if vinculoID.Valid {
			v := int(vinculoID.Int64)
			r.VinculoID = &v
		}
		if vigenciaID.Valid {
			v := int(vigenciaID.Int64)
			r.VigenciaID = &v
		}
		regras = append(regras, r)
	}
	return regras, rows.Err()
}

// ─── GamifRegrasHandler — GET/POST /api/farol/gamif-regras?campanha_id= ───────

func GamifRegrasHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		switch r.Method {
		case http.MethodGet:
			campanhaID, err := strconv.Atoi(r.URL.Query().Get("campanha_id"))
			if err != nil {
				http.Error(w, "campanha_id é obrigatório", http.StatusBadRequest)
				return
			}
			var pertence bool
			db.QueryRow(`SELECT EXISTS(SELECT 1 FROM farol.gamif_campanhas WHERE id=$1 AND empresa_id=$2)`, campanhaID, spCtx.EmpresaID).Scan(&pertence)
			if !pertence {
				http.Error(w, "Campanha não encontrada", http.StatusNotFound)
				return
			}
			regras, err := listarGamifRegras(db, campanhaID)
			if err != nil {
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(regras)

		case http.MethodPost:
			var req GamifRegraRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "corpo da requisição inválido", http.StatusBadRequest)
				return
			}
			if msg := validarGamifRegra(db, spCtx.EmpresaID, req); msg != "" {
				http.Error(w, msg, http.StatusBadRequest)
				return
			}
			if req.Fluxo == "" {
				req.Fluxo = "faturado"
			}
			if req.CodProds == nil {
				// pq.Array(nil) grava NULL, não '{}' — a coluna é NOT
				// NULL (default '{}' só vale quando a coluna nem aparece
				// no INSERT). Regra sem cod_prods no JSON (cobertura/
				// sortimento) cairia em erro 500 sem isto.
				req.CodProds = []string{}
			}
			var vinculoID, vigenciaID any
			if req.VinculoID != 0 {
				vinculoID = req.VinculoID
			}
			if req.VigenciaID != 0 {
				vigenciaID = req.VigenciaID
			}
			var id int
			err := db.QueryRow(`
				INSERT INTO farol.gamif_regras (campanha_id, tipo, descricao, vinculo_id, vigencia_id, cod_prods, qtd_minima, fluxo, pontos, valor_bonus)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id
			`, req.CampanhaID, req.Tipo, req.Descricao, vinculoID, vigenciaID, pq.Array(req.CodProds), req.QtdMinima, req.Fluxo, req.Pontos, req.ValorBonus).Scan(&id)
			if err != nil {
				http.Error(w, "Database error: "+err.Error(), http.StatusInternalServerError)
				return
			}
			writeAuditLog(db, spCtx.EmpresaID, spCtx.UserID, "gamif_regras", strconv.Itoa(id), "criar", req)
			log.Printf("Gamificacao: criada regra %d (tipo=%s) campanha=%d empresa %s por %s", id, req.Tipo, req.CampanhaID, spCtx.EmpresaID, spCtx.UserID)
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]int{"id": id})

		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

// validarGamifRegra garante que a regra tem os campos certos pro tipo
// escolhido — cobertura_atingida/sortimento_atingido exigem vinculo_id +
// vigencia_id de um vínculo REAL da mesma empresa (e cujo formula_codigo
// bate com o tipo da regra, senão "atingiu" nunca teria o sentido certo);
// produto_especifico exige cod_prods + qtd_minima > 0.
func validarGamifRegra(db *sql.DB, empresaID string, req GamifRegraRequest) string {
	if req.CampanhaID == 0 {
		return "campanha_id é obrigatório"
	}
	var campanhaExiste bool
	db.QueryRow(`SELECT EXISTS(SELECT 1 FROM farol.gamif_campanhas WHERE id=$1 AND empresa_id=$2)`, req.CampanhaID, empresaID).Scan(&campanhaExiste)
	if !campanhaExiste {
		return "campanha não encontrada"
	}
	if req.Fluxo != "" && req.Fluxo != "faturado" && req.Fluxo != "transmitido" {
		return "fluxo inválido (use faturado ou transmitido)"
	}
	switch req.Tipo {
	case "cobertura_atingida", "sortimento_atingido":
		if req.VinculoID == 0 || req.VigenciaID == 0 {
			return "vinculo_id e vigencia_id são obrigatórios pra este tipo de regra"
		}
		formulaEsperada := "cobertura_rede"
		if req.Tipo == "sortimento_atingido" {
			formulaEsperada = "sortimento_rede"
		}
		var formulaCodigo string
		err := db.QueryRow(`
			SELECT tm.formula_codigo FROM farol.metas_vinculos mv
			JOIN farol.tipos_metrica tm ON tm.id = mv.tipo_metrica_id
			JOIN farol.metas_vigencias v ON v.vinculo_id = mv.id
			WHERE mv.id = $1 AND v.id = $2 AND mv.empresa_id = $3
		`, req.VinculoID, req.VigenciaID, empresaID).Scan(&formulaCodigo)
		if err == sql.ErrNoRows {
			return "vínculo/vigência não encontrado"
		} else if err != nil {
			return "erro ao validar vínculo/vigência"
		}
		if formulaCodigo != formulaEsperada {
			return fmt.Sprintf("o vínculo escolhido é do tipo %q, mas a regra é %q", formulaCodigo, req.Tipo)
		}
	case "produto_especifico":
		if len(req.CodProds) == 0 {
			return "cod_prods é obrigatório pra regra de produto específico"
		}
		if req.QtdMinima <= 0 {
			return "qtd_minima precisa ser maior que zero"
		}
	default:
		return "tipo inválido (use cobertura_atingida, sortimento_atingido ou produto_especifico)"
	}
	if req.Pontos <= 0 && req.ValorBonus <= 0 {
		return "a regra precisa premiar algo — preencha pontos e/ou valor_bonus"
	}
	return ""
}

// ─── GamifRegraItemHandler — DELETE /api/farol/gamif-regras/{id} ──────────────

func GamifRegraItemHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		id, err := strconv.Atoi(pathSegment(r.URL.Path, "/api/farol/gamif-regras/"))
		if err != nil {
			http.Error(w, "ID inválido", http.StatusBadRequest)
			return
		}

		switch r.Method {
		case http.MethodPut:
			var req GamifRegraRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "corpo da requisição inválido", http.StatusBadRequest)
				return
			}
			// campanha_id não vem no corpo de um PUT (a regra já pertence a
			// uma campanha, não se move de campanha) — pega do registro
			// existente pra validarGamifRegra funcionar igual ao POST.
			if err := db.QueryRow(`
				SELECT campanha_id FROM farol.gamif_regras WHERE id = $1 AND campanha_id IN (SELECT id FROM farol.gamif_campanhas WHERE empresa_id = $2)
			`, id, spCtx.EmpresaID).Scan(&req.CampanhaID); err != nil {
				if err == sql.ErrNoRows {
					http.Error(w, "Regra não encontrada", http.StatusNotFound)
					return
				}
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
			if msg := validarGamifRegra(db, spCtx.EmpresaID, req); msg != "" {
				http.Error(w, msg, http.StatusBadRequest)
				return
			}
			if req.Fluxo == "" {
				req.Fluxo = "faturado"
			}
			if req.CodProds == nil {
				req.CodProds = []string{}
			}
			var vinculoID, vigenciaID any
			if req.VinculoID != 0 {
				vinculoID = req.VinculoID
			}
			if req.VigenciaID != 0 {
				vigenciaID = req.VigenciaID
			}
			res, err := db.Exec(`
				UPDATE farol.gamif_regras SET
					tipo = $2, descricao = $3, vinculo_id = $4, vigencia_id = $5,
					cod_prods = $6, qtd_minima = $7, fluxo = $8, pontos = $9, valor_bonus = $10
				WHERE id = $1
			`, id, req.Tipo, req.Descricao, vinculoID, vigenciaID, pq.Array(req.CodProds), req.QtdMinima, req.Fluxo, req.Pontos, req.ValorBonus)
			if err != nil {
				http.Error(w, "Database error: "+err.Error(), http.StatusInternalServerError)
				return
			}
			if n, _ := res.RowsAffected(); n == 0 {
				http.Error(w, "Regra não encontrada", http.StatusNotFound)
				return
			}
			writeAuditLog(db, spCtx.EmpresaID, spCtx.UserID, "gamif_regras", strconv.Itoa(id), "editar", req)
			log.Printf("Gamificacao: editada regra %d empresa %s por %s", id, spCtx.EmpresaID, spCtx.UserID)
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]bool{"ok": true})

		case http.MethodDelete:
			res, err := db.Exec(`
				DELETE FROM farol.gamif_regras WHERE id = $1 AND campanha_id IN (SELECT id FROM farol.gamif_campanhas WHERE empresa_id = $2)
			`, id, spCtx.EmpresaID)
			if err != nil {
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
			if n, _ := res.RowsAffected(); n == 0 {
				http.Error(w, "Regra não encontrada", http.StatusNotFound)
				return
			}
			writeAuditLog(db, spCtx.EmpresaID, spCtx.UserID, "gamif_regras", strconv.Itoa(id), "excluir", nil)
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]bool{"ok": true})

		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

// ─── Motor de pontuação ────────────────────────────────────────────────────

type gamifRegraInterna struct {
	ID                    int
	Tipo, Descricao       string
	VinculoID, VigenciaID sql.NullInt64
	CodProds              []string
	QtdMinima             float64
	Fluxo                 string
	Pontos, ValorBonus    float64
}

type gamifAcumuladorRCA struct {
	NomeRCA string
	Pontos  float64
	Bonus   float64
	Detalhe []map[string]any
}

// CalcularPontuacaoCampanha recalcula (replace total) a pontuação de TODOS
// os RCAs de uma campanha — soma o resultado de cada regra:
//   - cobertura_atingida/sortimento_atingido: reaproveita
//     obterOuCongelarRealizado (o Realizado já calculado/congelado do
//     módulo de Objetivos por Indústria — sem calcular nada de novo) e
//     premia cada CLIENTE (loja) que bateu o próprio limiar — não a Rede
//     como um todo (decisão do Claudio 22/09/2026: o RCA responde pelo
//     cliente e pela Rede dele, não por uma média de várias lojas que ele
//     não controla loja a loja). O dono de cada loja vem de
//     lerClientesValidos (pode divergir do "dono aproximado" da Rede).
//   - produto_especifico: soma quantidade vendida dos cod_prods, por
//     cod_rca, dentro do período da CAMPANHA (não de uma vigência — direto
//     em vendas_faturadas/vendas_transmitidas, que já carregam cod_rca e
//     nome_rca por linha) — premia quem bateu qtd_minima.
func CalcularPontuacaoCampanha(db *sql.DB, empresaID string, campanhaID int) error {
	var dataInicio, dataFim string
	if err := db.QueryRow(`SELECT data_inicio::text, data_fim::text FROM farol.gamif_campanhas WHERE id = $1 AND empresa_id = $2`,
		campanhaID, empresaID).Scan(&dataInicio, &dataFim); err != nil {
		return fmt.Errorf("campanha não encontrada: %w", err)
	}

	rows, err := db.Query(`
		SELECT id, tipo, descricao, vinculo_id, vigencia_id, cod_prods, qtd_minima, fluxo, pontos, valor_bonus
		FROM farol.gamif_regras WHERE campanha_id = $1
	`, campanhaID)
	if err != nil {
		return err
	}
	var regras []gamifRegraInterna
	for rows.Next() {
		var rg gamifRegraInterna
		if err := rows.Scan(&rg.ID, &rg.Tipo, &rg.Descricao, &rg.VinculoID, &rg.VigenciaID, pq.Array(&rg.CodProds), &rg.QtdMinima, &rg.Fluxo, &rg.Pontos, &rg.ValorBonus); err != nil {
			rows.Close()
			return err
		}
		regras = append(regras, rg)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	porRCA := map[string]*gamifAcumuladorRCA{}
	somar := func(codRCA, nomeRCA string, rg gamifRegraInterna) {
		if codRCA == "" {
			return
		}
		a, ok := porRCA[codRCA]
		if !ok {
			a = &gamifAcumuladorRCA{}
			porRCA[codRCA] = a
		}
		if nomeRCA != "" {
			a.NomeRCA = nomeRCA
		}
		a.Pontos += rg.Pontos
		a.Bonus += rg.ValorBonus
		a.Detalhe = append(a.Detalhe, map[string]any{
			"regra_id": rg.ID, "tipo": rg.Tipo, "descricao": rg.Descricao,
			"pontos": rg.Pontos, "bonus": rg.ValorBonus,
		})
	}

	for _, rg := range regras {
		switch rg.Tipo {
		case "cobertura_atingida", "sortimento_atingido":
			if !rg.VinculoID.Valid || !rg.VigenciaID.Valid {
				continue
			}
			realizado, err := obterOuCongelarRealizado(db, empresaID, int(rg.VinculoID.Int64), int(rg.VigenciaID.Int64), rg.Fluxo, "rede")
			if err != nil {
				return fmt.Errorf("regra %d: %w", rg.ID, err)
			}
			// Premia por CLIENTE (loja), não por Rede — decisão do Claudio
			// 22/09/2026: "cada RCA é responsável pelo cliente e pela Rede
			// dele... esse acompanhamento tem que ser no último nível".
			// RealizadoRede.Atingiu é a MÉDIA das lojas da Rede — premiar
			// por aí recompensaria/puniria o RCA por algo que só uma loja
			// "puxa" pra cima ou pra baixo, fora do controle dele numa
			// visita específica. RealizadoRede.CodRCA também é só o "dono
			// aproximado" da Rede inteira (redeRepresentante, primeiro
			// CNPJ) — o dono REAL de cada loja vem de
			// farol.metas_clientes_validos (lerClientesValidos), que pode
			// divergir por CNPJ dentro da mesma Rede (~6 de 134 Redes reais
			// têm isso, achado 2026-09-04).
			clientesValidos, err := lerClientesValidos(db, empresaID, int(rg.VigenciaID.Int64))
			if err != nil {
				return fmt.Errorf("regra %d: %w", rg.ID, err)
			}
			donoPorCNPJ := make(map[string]clienteValido, len(clientesValidos))
			for _, c := range clientesValidos {
				donoPorCNPJ[c.CNPJ] = c
			}
			for _, rede := range realizado.Redes {
				for _, cliente := range rede.Clientes {
					if !cliente.Atingiu {
						continue
					}
					codRCA, nomeRCA := rede.CodRCA, rede.NomeRCA
					if dono, ok := donoPorCNPJ[cliente.CNPJ]; ok && dono.CodRCA != "" {
						codRCA, nomeRCA = dono.CodRCA, dono.NomeRCA
					}
					somar(codRCA, nomeRCA, rg)
				}
			}
		case "produto_especifico":
			if len(rg.CodProds) == 0 || rg.QtdMinima <= 0 {
				continue
			}
			tabela, colData := "vendas_faturadas", "data_faturamento"
			if rg.Fluxo == "transmitido" {
				tabela, colData = "vendas_transmitidas", "data_transmissao"
			}
			query := fmt.Sprintf(`
				SELECT cod_rca, MAX(nome_rca), SUM(qt) FROM %s
				WHERE empresa_id = $1 AND %s BETWEEN $2 AND $3 AND cod_prod = ANY($4) AND cod_rca <> ''
				GROUP BY cod_rca
			`, tabela, colData)
			rows2, err := db.Query(query, empresaID, dataInicio, dataFim, pq.Array(rg.CodProds))
			if err != nil {
				return fmt.Errorf("regra %d: %w", rg.ID, err)
			}
			for rows2.Next() {
				var codRCA, nomeRCA string
				var qtd float64
				if err := rows2.Scan(&codRCA, &nomeRCA, &qtd); err != nil {
					rows2.Close()
					return err
				}
				if qtd >= rg.QtdMinima {
					somar(codRCA, nomeRCA, rg)
				}
			}
			rows2.Close()
			if err := rows2.Err(); err != nil {
				return err
			}
		}
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM farol.gamif_pontuacao WHERE campanha_id = $1`, campanhaID); err != nil {
		return err
	}
	for codRCA, a := range porRCA {
		detalheJSON, _ := json.Marshal(a.Detalhe)
		if _, err := tx.Exec(`
			INSERT INTO farol.gamif_pontuacao (empresa_id, campanha_id, cod_rca, nome_rca, pontos_total, bonus_total, detalhe)
			VALUES ($1,$2,$3,$4,$5,$6,$7)
		`, empresaID, campanhaID, codRCA, a.NomeRCA, a.Pontos, a.Bonus, detalheJSON); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	log.Printf("Gamificacao: pontuação recalculada campanha=%d → %d RCA(s) pontuando", campanhaID, len(porRCA))
	return nil
}

// ─── GamifCalcularHandler — POST /api/farol/gamif-campanhas-calcular?campanha_id= ─

func GamifCalcularHandler(db *sql.DB) http.HandlerFunc {
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
		campanhaID, err := strconv.Atoi(r.URL.Query().Get("campanha_id"))
		if err != nil {
			http.Error(w, "campanha_id é obrigatório", http.StatusBadRequest)
			return
		}
		if err := CalcularPontuacaoCampanha(db, spCtx.EmpresaID, campanhaID); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeAuditLog(db, spCtx.EmpresaID, spCtx.UserID, "gamif_pontuacao", strconv.Itoa(campanhaID), "recalcular", nil)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	}
}

// ─── GamifRankingHandler — GET /api/farol/gamif-ranking?campanha_id=&cod_rca= ─

// GamifRankingLinha — 1 linha do ranking completo (visão admin: nomes de
// todo mundo). O modo "visão do RCA" (cod_rca= preenchido) NUNCA devolve
// esta lista — só GamifMinhaPosicaoResponse, sem nome de ninguém além do
// próprio RCA (decisão do Claudio 22/09/2026: mostra a posição, não quem
// está na frente).
type GamifRankingLinha struct {
	Posicao     int             `json:"posicao"`
	CodRCA      string          `json:"cod_rca"`
	NomeRCA     string          `json:"nome_rca"`
	PontosTotal float64         `json:"pontos_total"`
	BonusTotal  float64         `json:"bonus_total"`
	Detalhe     json.RawMessage `json:"detalhe"`
}

type GamifMinhaPosicaoResponse struct {
	CodRCA      string          `json:"cod_rca"`
	Posicao     int             `json:"posicao"`
	TotalRCAs   int             `json:"total_rcas"`
	PontosTotal float64         `json:"pontos_total"`
	BonusTotal  float64         `json:"bonus_total"`
	Detalhe     json.RawMessage `json:"detalhe"`
}

func GamifRankingHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		campanhaID, err := strconv.Atoi(r.URL.Query().Get("campanha_id"))
		if err != nil {
			http.Error(w, "campanha_id é obrigatório", http.StatusBadRequest)
			return
		}
		var pertence bool
		db.QueryRow(`SELECT EXISTS(SELECT 1 FROM farol.gamif_campanhas WHERE id=$1 AND empresa_id=$2)`, campanhaID, spCtx.EmpresaID).Scan(&pertence)
		if !pertence {
			http.Error(w, "Campanha não encontrada", http.StatusNotFound)
			return
		}

		rows, err := db.Query(`
			SELECT cod_rca, nome_rca, pontos_total, bonus_total, detalhe,
			       RANK() OVER (ORDER BY pontos_total DESC) AS posicao,
			       COUNT(*) OVER () AS total_rcas
			FROM farol.gamif_pontuacao WHERE campanha_id = $1 AND empresa_id = $2
			ORDER BY pontos_total DESC, cod_rca
		`, campanhaID, spCtx.EmpresaID)
		if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		codRCAFiltro := strings.TrimSpace(r.URL.Query().Get("cod_rca"))
		ranking := []GamifRankingLinha{}
		var minhaPosicao *GamifMinhaPosicaoResponse
		totalRCAs := 0
		for rows.Next() {
			var linha GamifRankingLinha
			if err := rows.Scan(&linha.CodRCA, &linha.NomeRCA, &linha.PontosTotal, &linha.BonusTotal, &linha.Detalhe, &linha.Posicao, &totalRCAs); err != nil {
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
			if codRCAFiltro != "" {
				if linha.CodRCA == codRCAFiltro {
					minhaPosicao = &GamifMinhaPosicaoResponse{
						CodRCA: linha.CodRCA, Posicao: linha.Posicao, TotalRCAs: totalRCAs,
						PontosTotal: linha.PontosTotal, BonusTotal: linha.BonusTotal, Detalhe: linha.Detalhe,
					}
				}
				continue
			}
			ranking = append(ranking, linha)
		}
		if err := rows.Err(); err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if codRCAFiltro != "" {
			if minhaPosicao == nil {
				w.WriteHeader(http.StatusNotFound)
				json.NewEncoder(w).Encode(map[string]string{"error": "este RCA ainda não pontuou nesta campanha"})
				return
			}
			json.NewEncoder(w).Encode(minhaPosicao)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"ranking": ranking, "total_rcas": totalRCAs})
	}
}
