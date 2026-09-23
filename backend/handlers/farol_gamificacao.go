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
	"sort"
	"strconv"
	"strings"

	"github.com/lib/pq"
)

// ─── DTOs ─────────────────────────────────────────────────────────────────────

type GamifRegraResponse struct {
	ID         int      `json:"id"`
	CampanhaID int      `json:"campanha_id"`
	Tipo       string   `json:"tipo"` // cobertura_atingida | sortimento_atingido (por CLIENTE/loja) | rede_completa_atingida (por Rede, 100% das lojas) | rca_completo (100% de TODAS as Redes do RCA) | produto_especifico (por RCA)
	Descricao  string   `json:"descricao"`
	VinculoID  *int     `json:"vinculo_id,omitempty"`
	VigenciaID *int     `json:"vigencia_id,omitempty"`
	CodProds   []string `json:"cod_prods,omitempty"`
	QtdMinima  float64  `json:"qtd_minima,omitempty"`
	Fluxo      string   `json:"fluxo"`
	Pontos     float64  `json:"pontos"`
	ValorBonus float64  `json:"valor_bonus"`
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
	// Niveis — a escala de pagamento desta campanha (editável, ver
	// GamifNiveisHandler e migration 246). Vem junto no GET da campanha
	// pra frontend montar a tela de edição sem precisar de outra chamada.
	Niveis []GamifNivelResponse `json:"niveis,omitempty"`
}

type GamifCampanhaRequest struct {
	IndustriaID int    `json:"industria_id"`
	Nome        string `json:"nome"`
	DataInicio  string `json:"data_inicio"`
	DataFim     string `json:"data_fim"`
	Status      string `json:"status"`
}

// GamifNivelResponse/Request — 1 nível da escala de pagamento (ver
// gamifNivelConfig no motor de cálculo e migration 246). Ordem não vem no
// request: é sempre recalculada como a posição no array enviado, ordenado
// por percentual_minimo (ver GamifNiveisHandler).
type GamifNivelResponse struct {
	ID               int     `json:"id"`
	Nome             string  `json:"nome"`
	PercentualMinimo float64 `json:"percentual_minimo"`
	Multiplicador    float64 `json:"multiplicador"`
	Ordem            int     `json:"ordem"`
}

type GamifNivelRequest struct {
	Nome             string  `json:"nome"`
	PercentualMinimo float64 `json:"percentual_minimo"`
	Multiplicador    float64 `json:"multiplicador"`
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
			// Escala de pagamento padrão — sem isso, a campanha nasceria sem
			// NENHUM nível configurado (ninguém pontuaria); o admin edita ou
			// substitui depois em "Editar escala de pagamento".
			if err := criarNiveisPadrao(db, id); err != nil {
				log.Printf("Gamificacao: erro ao criar níveis padrão da campanha %d: %v", id, err)
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
			c.Niveis, err = listarGamifNiveis(db, id)
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

func listarGamifNiveis(db *sql.DB, campanhaID int) ([]GamifNivelResponse, error) {
	rows, err := db.Query(`
		SELECT id, nome, percentual_minimo, multiplicador, ordem
		FROM farol.gamif_niveis WHERE campanha_id = $1 ORDER BY percentual_minimo ASC
	`, campanhaID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	niveis := []GamifNivelResponse{}
	for rows.Next() {
		var n GamifNivelResponse
		if err := rows.Scan(&n.ID, &n.Nome, &n.PercentualMinimo, &n.Multiplicador, &n.Ordem); err != nil {
			return nil, err
		}
		niveis = append(niveis, n)
	}
	return niveis, rows.Err()
}

// ─── GamifNiveisHandler — GET/PUT /api/farol/gamif-niveis?campanha_id= ────────

// Escala de pagamento EDITÁVEL por campanha (pedido do Claudio 23/09/2026).
// PUT substitui a lista INTEIRA (DELETE + INSERT, mesmo padrão de
// CalcularPontuacaoCampanha) — mais simples que CRUD item a item pra uma
// lista pequena e reordenável; a ordem enviada no array define a "ordem"
// gravada (1, 2, 3...), usada só pra escolher a cor do troféu no frontend.
// NÃO recalcula pontuação sozinho — decisão do Claudio 23/09/2026: só
// aplica no próximo "Recalcular pontuação" clicado explicitamente.
func GamifNiveisHandler(db *sql.DB) http.HandlerFunc {
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
		w.Header().Set("Content-Type", "application/json")

		switch r.Method {
		case http.MethodGet:
			niveis, err := listarGamifNiveis(db, campanhaID)
			if err != nil {
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
			json.NewEncoder(w).Encode(niveis)

		case http.MethodPut:
			var req struct {
				Niveis []GamifNivelRequest `json:"niveis"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "corpo da requisição inválido", http.StatusBadRequest)
				return
			}
			if len(req.Niveis) == 0 {
				http.Error(w, "a escala precisa ter pelo menos 1 nível", http.StatusBadRequest)
				return
			}
			for _, n := range req.Niveis {
				if strings.TrimSpace(n.Nome) == "" {
					http.Error(w, "todo nível precisa de um nome", http.StatusBadRequest)
					return
				}
				if n.PercentualMinimo < 0 {
					http.Error(w, "percentual_minimo não pode ser negativo", http.StatusBadRequest)
					return
				}
				if n.Multiplicador <= 0 {
					http.Error(w, "multiplicador precisa ser maior que zero", http.StatusBadRequest)
					return
				}
			}
			sort.Slice(req.Niveis, func(i, j int) bool { return req.Niveis[i].PercentualMinimo < req.Niveis[j].PercentualMinimo })

			tx, err := db.Begin()
			if err != nil {
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
			defer tx.Rollback()
			if _, err := tx.Exec(`DELETE FROM farol.gamif_niveis WHERE campanha_id = $1`, campanhaID); err != nil {
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
			for i, n := range req.Niveis {
				if _, err := tx.Exec(`
					INSERT INTO farol.gamif_niveis (campanha_id, nome, percentual_minimo, multiplicador, ordem)
					VALUES ($1,$2,$3,$4,$5)
				`, campanhaID, n.Nome, n.PercentualMinimo, n.Multiplicador, i+1); err != nil {
					http.Error(w, "Database error: "+err.Error(), http.StatusInternalServerError)
					return
				}
			}
			if err := tx.Commit(); err != nil {
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
			writeAuditLog(db, spCtx.EmpresaID, spCtx.UserID, "gamif_niveis", strconv.Itoa(campanhaID), "editar", req.Niveis)
			log.Printf("Gamificacao: escala de pagamento da campanha %d atualizada (%d níveis) por %s", campanhaID, len(req.Niveis), spCtx.UserID)
			niveis, err := listarGamifNiveis(db, campanhaID)
			if err != nil {
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
			json.NewEncoder(w).Encode(niveis)

		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
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
	case "rede_completa_atingida", "rca_completo":
		// rede_completa_atingida: bônus extra por Rede 100% coberta (todas
		// as lojas DAQUELA Rede bateram) — pedido do Claudio 22/09/2026,
		// pra separar "Loja Individual" de "Rede Completa" como campanhas
		// distintas. rca_completo: mesma ideia, mas pro PORTFÓLIO INTEIRO
		// do RCA (todas as Redes dele, não só uma) — pedido do Claudio
		// 22/09/2026, mostra progresso ("faltam N lojas") mesmo antes de
		// completar. Ambas aceitam vínculo de Cobertura OU Sortimento
		// (formula-agnóstico — "100%" faz sentido pras duas métricas).
		if req.VinculoID == 0 || req.VigenciaID == 0 {
			return "vinculo_id e vigencia_id são obrigatórios pra este tipo de regra"
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
		if formulaCodigo != "cobertura_rede" && formulaCodigo != "sortimento_rede" {
			return fmt.Sprintf("vínculo do tipo %q não serve pra Cobertura nem Sortimento", formulaCodigo)
		}
	case "produto_especifico":
		if len(req.CodProds) == 0 {
			return "cod_prods é obrigatório pra regra de produto específico"
		}
		if req.QtdMinima <= 0 {
			return "qtd_minima precisa ser maior que zero"
		}
	default:
		return "tipo inválido (use cobertura_atingida, sortimento_atingido, rede_completa_atingida, rca_completo ou produto_especifico)"
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

// gamifNivelConfig — 1 nível da escala de pagamento de UMA campanha (ver
// migration 246). Substituiu a função fixa gamifNivelPagamento (pedido do
// Claudio 23/09/2026: "podemos precisar editar a configuração da
// premiação... não está acessível na tela") — agora vem do banco,
// editável por campanha, com número variável de níveis.
type gamifNivelConfig struct {
	Nome             string
	PercentualMinimo float64
	Multiplicador    float64
	Ordem            int
}

// carregarNiveisCampanha lê a escala de pagamento de uma campanha, do MAIOR
// corte pro menor — classificarNivel percorre nessa ordem e para no
// primeiro que o percentual alcança.
func carregarNiveisCampanha(db *sql.DB, campanhaID int) ([]gamifNivelConfig, error) {
	rows, err := db.Query(`
		SELECT nome, percentual_minimo, multiplicador, ordem
		FROM farol.gamif_niveis WHERE campanha_id = $1 ORDER BY percentual_minimo DESC
	`, campanhaID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var niveis []gamifNivelConfig
	for rows.Next() {
		var n gamifNivelConfig
		if err := rows.Scan(&n.Nome, &n.PercentualMinimo, &n.Multiplicador, &n.Ordem); err != nil {
			return nil, err
		}
		niveis = append(niveis, n)
	}
	return niveis, rows.Err()
}

// classificarNivel acha o nível mais alto que percentual alcança (niveis já
// vem ordenado do maior corte pro menor — ver carregarNiveisCampanha).
// ordem=0 quando não bateu nenhum nível (abaixo do menor corte configurado).
func classificarNivel(niveis []gamifNivelConfig, percentual float64) (nome string, multiplicador float64, ordem int) {
	for _, n := range niveis {
		if percentual >= n.PercentualMinimo {
			return n.Nome, n.Multiplicador, n.Ordem
		}
	}
	return "", 0, 0
}

// niveisPadraoGamif — escala aplicada por padrão a toda campanha NOVA (o
// mesmo que era fixo no código antes de virar editável) — o admin edita ou
// substitui depois via GamifNiveisHandler; sem isso, uma campanha recém
// criada nasceria sem nenhum nível e ninguém pontuaria.
var niveisPadraoGamif = []gamifNivelConfig{
	{Nome: "Bronze", PercentualMinimo: 60, Multiplicador: 0.30, Ordem: 1},
	{Nome: "Prata", PercentualMinimo: 75, Multiplicador: 0.50, Ordem: 2},
	{Nome: "Ouro", PercentualMinimo: 100, Multiplicador: 1.00, Ordem: 3},
	{Nome: "Diamante", PercentualMinimo: 120, Multiplicador: 1.20, Ordem: 4},
}

func criarNiveisPadrao(db *sql.DB, campanhaID int) error {
	for _, n := range niveisPadraoGamif {
		if _, err := db.Exec(`
			INSERT INTO farol.gamif_niveis (campanha_id, nome, percentual_minimo, multiplicador, ordem)
			VALUES ($1,$2,$3,$4,$5)
		`, campanhaID, n.Nome, n.PercentualMinimo, n.Multiplicador, n.Ordem); err != nil {
			return err
		}
	}
	return nil
}

type gamifAcumuladorRCA struct {
	NomeRCA string
	Pontos  float64
	Bonus   float64
	// Volume — pedido do Claudio 22/09/2026 ("ranking estranho... curva
	// ABC de vendas"): critério de desempate no ranking (curva ABC),
	// nunca usado no cálculo de pontos/bônus em si.
	Volume float64
	// NivelPrincipal/PercentualPrincipal/NivelOrdem — pedido do Claudio
	// 23/09/2026: o maior % entre as regras desta campanha pra este RCA,
	// usado pro selo/cor ÚNICO mostrado no ranking e no mobile (uma
	// campanha com várias regras mostra o melhor nível batido, não a
	// soma/média). NivelOrdem é a posição do nível na escala DESSA
	// campanha — como o nome agora é livre ("Platina" etc.), a cor do
	// troféu no frontend é escolhida pela ordem, não mais pelo nome.
	NivelPrincipal      string
	PercentualPrincipal float64
	NivelOrdem          int
	// RealizadoPrincipal/MetaPrincipal — pedido do Claudio 23/09/2026: "no
	// extrato do RCA colocar a quantidade objetivo e o que o RCA vendeu".
	// O par (realizado, meta) da MESMA regra que gerou o percentual
	// principal — genérico o bastante pra qualquer tipo (lojas cobertas/
	// total, redes cobertas/total, ou qtd vendida/qtd mínima).
	RealizadoPrincipal float64
	MetaPrincipal      float64
	Detalhe            []map[string]any
}

// obterAcumulador acha ou cria o acumulador do RCA — usado por toda regra
// que precisa registrar progresso, MESMO quando o RCA ainda não bateu nada
// (percentual < 60%, sem nível): é o que permite a tarja vermelha/progresso
// aparecer pro RCA na "visão dele" (mobile) antes de qualquer prêmio.
func obterAcumulador(porRCA map[string]*gamifAcumuladorRCA, codRCA, nomeRCA string) *gamifAcumuladorRCA {
	if codRCA == "" {
		return nil
	}
	a, ok := porRCA[codRCA]
	if !ok {
		a = &gamifAcumuladorRCA{}
		porRCA[codRCA] = a
	}
	if nomeRCA != "" {
		a.NomeRCA = nomeRCA
	}
	return a
}

// aplicarNivel classifica percentual na escala de pagamento DESSA campanha
// (niveis) e credita pontos/bônus JÁ ajustados pela fração do nível (ex.:
// Bronze paga 30% do rg.Pontos/rg.ValorBonus configurado) — rg.Pontos/
// rg.ValorBonus é sempre o valor "cheio" (100% do nível mais alto = tudo).
// realizado/meta são o par bruto que gerou percentual (ex.: qtd vendida/
// qtd_minima, ou lojas cobertas/total) — guardados só da regra PRINCIPAL,
// pro extrato mostrar "vendeu X de um objetivo Y" (pedido do Claudio
// 23/09/2026). extra são campos específicos do tipo de regra (ex.:
// cobertos/total, qtd/qtd_minima) anexados ao detalhe.
func (a *gamifAcumuladorRCA) aplicarNivel(niveis []gamifNivelConfig, rg gamifRegraInterna, percentual, realizado, meta, volume float64, extra map[string]any) {
	nivel, mult, ordem := classificarNivel(niveis, percentual)
	pontosPagos := rg.Pontos * mult
	bonusPagos := rg.ValorBonus * mult
	a.Pontos += pontosPagos
	a.Bonus += bonusPagos
	a.Volume += volume
	if percentual > a.PercentualPrincipal {
		a.PercentualPrincipal = percentual
		a.NivelPrincipal = nivel
		a.NivelOrdem = ordem
		a.RealizadoPrincipal = realizado
		a.MetaPrincipal = meta
	}
	item := map[string]any{
		"regra_id": rg.ID, "tipo": rg.Tipo, "descricao": rg.Descricao,
		"percentual": percentual, "nivel": nivel, "nivel_ordem": ordem, "completo": percentual >= 100,
		"pontos": pontosPagos, "bonus": bonusPagos, "volume": volume,
	}
	for k, v := range extra {
		item[k] = v
	}
	a.Detalhe = append(a.Detalhe, item)
}

// donoRealPorCNPJ resolve o dono (CodRCA/NomeRCA) REAL de cada CNPJ na
// vigência, a partir de farol.metas_clientes_validos — não a aproximação
// "1 dono pra Rede inteira" (redeRepresentante) que RealizadoRede.CodRCA
// carrega. Usado por qualquer regra que precise atribuir crédito por
// CLIENTE (loja), não por Rede.
func donoRealPorCNPJ(db *sql.DB, empresaID string, vigenciaID int) (map[string]clienteValido, error) {
	clientesValidos, err := lerClientesValidos(db, empresaID, vigenciaID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]clienteValido, len(clientesValidos))
	for _, c := range clientesValidos {
		out[c.CNPJ] = c
	}
	return out, nil
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

	niveis, err := carregarNiveisCampanha(db, campanhaID)
	if err != nil {
		return fmt.Errorf("carregar escala de pagamento: %w", err)
	}

	porRCA := map[string]*gamifAcumuladorRCA{}

	for _, rg := range regras {
		switch rg.Tipo {
		case "cobertura_atingida", "sortimento_atingido":
			// Escala de pagamento (pedido do Claudio 23/09/2026): virou
			// percentual do PRÓPRIO portfólio do RCA (clientes dele que
			// atingiram ÷ total de clientes dele no vínculo), não mais 1
			// pontuação fixa por loja isolada — mesma ideia de rca_completo
			// abaixo, só que a nível de CLIENTE em vez de REDE. A atribuição
			// de dono continua por CNPJ (donoPorCNPJ), preservando a decisão
			// de 22/09/2026 de não usar a média/aproximação da Rede — só o
			// jeito de PAGAR mudou de "fixo por loja" pra "gradual por %".
			if !rg.VinculoID.Valid || !rg.VigenciaID.Valid {
				continue
			}
			realizado, err := obterOuCongelarRealizado(db, empresaID, int(rg.VinculoID.Int64), int(rg.VigenciaID.Int64), rg.Fluxo, "rede")
			if err != nil {
				return fmt.Errorf("regra %d: %w", rg.ID, err)
			}
			donoPorCNPJ, err := donoRealPorCNPJ(db, empresaID, int(rg.VigenciaID.Int64))
			if err != nil {
				return fmt.Errorf("regra %d: %w", rg.ID, err)
			}
			type progressoRCA struct {
				NomeRCA          string
				Total, Atingiram int
			}
			progressoPorRCA := map[string]*progressoRCA{}
			for _, rede := range realizado.Redes {
				for _, cliente := range rede.Clientes {
					codRCA, nomeRCA := rede.CodRCA, rede.NomeRCA
					if dono, ok := donoPorCNPJ[cliente.CNPJ]; ok && dono.CodRCA != "" {
						codRCA, nomeRCA = dono.CodRCA, dono.NomeRCA
					}
					if codRCA == "" {
						continue
					}
					p, ok := progressoPorRCA[codRCA]
					if !ok {
						p = &progressoRCA{}
						progressoPorRCA[codRCA] = p
					}
					if nomeRCA != "" {
						p.NomeRCA = nomeRCA
					}
					p.Total++
					if cliente.Atingiu {
						p.Atingiram++
					}
				}
			}
			for codRCA, p := range progressoPorRCA {
				if p.Total == 0 {
					continue
				}
				percentual := float64(p.Atingiram) / float64(p.Total) * 100
				a := obterAcumulador(porRCA, codRCA, p.NomeRCA)
				a.aplicarNivel(niveis, rg, percentual, float64(p.Atingiram), float64(p.Total), float64(p.Atingiram), map[string]any{
					"cobertos": p.Atingiram, "total": p.Total, "faltam": p.Total - p.Atingiram,
				})
			}
		case "rede_completa_atingida":
			// Escala de pagamento por Rede (pedido do Claudio 23/09/2026):
			// cada Rede do vínculo paga pelo seu PRÓPRIO % de cobertura
			// (lojas atingidas ÷ total de lojas daquela Rede), não mais só
			// tudo-ou-nada — uma Rede 100% completa é Ouro, uma 70% é
			// Bronze. RCA com várias Redes acumula uma avaliação POR Rede
			// (a soma de todas), igual o desenho binário anterior.
			if !rg.VinculoID.Valid || !rg.VigenciaID.Valid {
				continue
			}
			realizado, err := obterOuCongelarRealizado(db, empresaID, int(rg.VinculoID.Int64), int(rg.VigenciaID.Int64), rg.Fluxo, "rede")
			if err != nil {
				return fmt.Errorf("regra %d: %w", rg.ID, err)
			}
			donoPorCNPJ, err := donoRealPorCNPJ(db, empresaID, int(rg.VigenciaID.Int64))
			if err != nil {
				return fmt.Errorf("regra %d: %w", rg.ID, err)
			}
			for _, rede := range realizado.Redes {
				if len(rede.Clientes) == 0 {
					continue
				}
				atingiram := 0
				rcasEnvolvidos := map[string]string{} // codRCA -> nomeRCA
				for _, cliente := range rede.Clientes {
					codRCA, nomeRCA := rede.CodRCA, rede.NomeRCA
					if dono, ok := donoPorCNPJ[cliente.CNPJ]; ok && dono.CodRCA != "" {
						codRCA, nomeRCA = dono.CodRCA, dono.NomeRCA
					}
					if codRCA != "" {
						rcasEnvolvidos[codRCA] = nomeRCA
					}
					if cliente.Atingiu {
						atingiram++
					}
				}
				percentual := float64(atingiram) / float64(len(rede.Clientes)) * 100
				for codRCA, nomeRCA := range rcasEnvolvidos {
					a := obterAcumulador(porRCA, codRCA, nomeRCA)
					a.aplicarNivel(niveis, rg, percentual, float64(atingiram), float64(len(rede.Clientes)), float64(atingiram), map[string]any{
						"rede": rede.Fantasia, "cod_princ": rede.CodPrinc, "cobertos": atingiram, "total": len(rede.Clientes),
					})
				}
			}
		case "rca_completo":
			// "Bater 100% de TODAS as Redes do RCA" — pedido do Claudio
			// 22/09/2026: mais amplo que rede_completa_atingida (que é por
			// UMA Rede). Junta TODOS os clientes de TODAS as Redes do
			// vínculo/vigência, agrupa pelo dono real (CNPJ a CNPJ, não a
			// aproximação da Rede) e só paga pro RCA cujo portfólio
			// inteiro bateu. Diferente das outras regras: toca o
			// acumulador de QUALQUER RCA com pelo menos 1 cliente no
			// escopo, mesmo sem completar — pra guardar o PROGRESSO
			// ("faltam N de M") no detalhe, visível na "visão do RCA"
			// mesmo antes de bater o 100%. Sem isso a regra só existiria
			// como resultado binário no fim do mês, sem servir de
			// motivação no meio do caminho.
			if !rg.VinculoID.Valid || !rg.VigenciaID.Valid {
				continue
			}
			realizado, err := obterOuCongelarRealizado(db, empresaID, int(rg.VinculoID.Int64), int(rg.VigenciaID.Int64), rg.Fluxo, "rede")
			if err != nil {
				return fmt.Errorf("regra %d: %w", rg.ID, err)
			}
			donoPorCNPJ, err := donoRealPorCNPJ(db, empresaID, int(rg.VigenciaID.Int64))
			if err != nil {
				return fmt.Errorf("regra %d: %w", rg.ID, err)
			}
			type progressoRCA struct {
				NomeRCA          string
				Total, Atingiram int
			}
			progressoPorRCA := map[string]*progressoRCA{}
			for _, rede := range realizado.Redes {
				for _, cliente := range rede.Clientes {
					codRCA, nomeRCA := rede.CodRCA, rede.NomeRCA
					if dono, ok := donoPorCNPJ[cliente.CNPJ]; ok && dono.CodRCA != "" {
						codRCA, nomeRCA = dono.CodRCA, dono.NomeRCA
					}
					if codRCA == "" {
						continue
					}
					p, ok := progressoPorRCA[codRCA]
					if !ok {
						p = &progressoRCA{}
						progressoPorRCA[codRCA] = p
					}
					if nomeRCA != "" {
						p.NomeRCA = nomeRCA
					}
					p.Total++
					if cliente.Atingiu {
						p.Atingiram++
					}
				}
			}
			for codRCA, p := range progressoPorRCA {
				if p.Total == 0 {
					continue
				}
				percentual := float64(p.Atingiram) / float64(p.Total) * 100
				a := obterAcumulador(porRCA, codRCA, p.NomeRCA)
				a.aplicarNivel(niveis, rg, percentual, float64(p.Atingiram), float64(p.Total), float64(p.Atingiram), map[string]any{
					"cobertos": p.Atingiram, "total": p.Total, "faltam": p.Total - p.Atingiram,
				})
			}
		case "produto_especifico":
			// Escala de pagamento (pedido do Claudio 23/09/2026): percentual
			// = qtd vendida ÷ qtd_minima — pode passar de 100% (Diamante,
			// >=120%), é o cenário mais natural pra essa fórmula (ex.: Black
			// Label, mínimo 10 garrafas, quem vende 12+ já é Diamante).
			// Continua tocando o RCA mesmo com qtd < mínimo (sem nível, mas
			// com progresso visível — mesma lógica de rca_completo).
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
				if qtd <= 0 {
					continue
				}
				percentual := qtd / rg.QtdMinima * 100
				a := obterAcumulador(porRCA, codRCA, nomeRCA)
				a.aplicarNivel(niveis, rg, percentual, qtd, rg.QtdMinima, qtd, map[string]any{
					"qtd": qtd, "qtd_minima": rg.QtdMinima,
				})
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
		var nivelPrincipal any
		var nivelOrdem any
		if a.NivelPrincipal != "" {
			nivelPrincipal = a.NivelPrincipal
			nivelOrdem = a.NivelOrdem
		}
		if _, err := tx.Exec(`
			INSERT INTO farol.gamif_pontuacao (empresa_id, campanha_id, cod_rca, nome_rca, pontos_total, bonus_total, volume_desempate, detalhe, nivel_principal, percentual_principal, nivel_ordem, realizado_principal, meta_principal)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		`, empresaID, campanhaID, codRCA, a.NomeRCA, a.Pontos, a.Bonus, a.Volume, detalheJSON, nivelPrincipal, a.PercentualPrincipal, nivelOrdem, a.RealizadoPrincipal, a.MetaPrincipal); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	log.Printf("Gamificacao: pontuação recalculada campanha=%d → %d RCA(s) pontuando", campanhaID, len(porRCA))
	return nil
}

// RecalcularGamificacaoAtivas recalcula TODAS as campanhas ativas de uma
// empresa — pedido do Claudio 23/09/2026: até aqui a Gamificação só
// recalculava quando alguém clicava em "Recalcular pontuação" (ou gerava
// um extrato), diferente do resto do Farol (Cobertura/Sortimento), que já
// atualiza sozinho todo dia. Chamada de dentro do mesmo aquecimento diário
// (PrewarmDiario, ver farol_v2_api.go) — assim o ranking que o RCA vê no
// celular reflete a venda de ontem sem precisar de ninguém clicar em nada.
// Erro em 1 campanha não impede as outras (loga e segue).
func RecalcularGamificacaoAtivas(db *sql.DB, empresaID string) {
	rows, err := db.Query(`SELECT id FROM farol.gamif_campanhas WHERE empresa_id = $1 AND status = 'ativa'`, empresaID)
	if err != nil {
		log.Printf("[farol:gamif] recalculo diário: falha ao listar campanhas ativas empresa=%s: %v", empresaID, err)
		return
	}
	var campanhaIDs []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			log.Printf("[farol:gamif] recalculo diário: falha ao ler campanha empresa=%s: %v", empresaID, err)
			return
		}
		campanhaIDs = append(campanhaIDs, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		log.Printf("[farol:gamif] recalculo diário: erro ao percorrer campanhas empresa=%s: %v", empresaID, err)
		return
	}
	for _, id := range campanhaIDs {
		if err := CalcularPontuacaoCampanha(db, empresaID, id); err != nil {
			log.Printf("[farol:gamif] recalculo diário: falha campanha=%d empresa=%s: %v", id, empresaID, err)
			continue
		}
	}
	if len(campanhaIDs) > 0 {
		log.Printf("[farol:gamif] recalculo diário: %d campanha(s) ativa(s) recalculada(s) empresa=%s", len(campanhaIDs), empresaID)
	}
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
	Posicao             int     `json:"posicao"`
	CodRCA              string  `json:"cod_rca"`
	NomeRCA             string  `json:"nome_rca"`
	PontosTotal         float64 `json:"pontos_total"`
	BonusTotal          float64 `json:"bonus_total"`
	VolumeDesempate     float64 `json:"volume_desempate"`
	NivelPrincipal      string  `json:"nivel_principal,omitempty"`
	PercentualPrincipal float64 `json:"percentual_principal"`
	// NivelOrdem — posição (1,2,3...) do NivelPrincipal na escala DESSA
	// campanha (farol.gamif_niveis), pra frontend/mobile escolherem a cor
	// do troféu sem precisar de mais uma consulta (nomes de nível agora
	// são livres, editáveis por campanha — não dá mais pra colorir só
	// pelo texto "bronze"/"prata"). 0 quando não bateu nenhum nível.
	NivelOrdem int `json:"nivel_ordem"`
	// RealizadoPrincipal/MetaPrincipal — pedido do Claudio 23/09/2026: "no
	// extrato do RCA colocar a quantidade objetivo e o que o RCA vendeu".
	RealizadoPrincipal float64         `json:"realizado_principal"`
	MetaPrincipal      float64         `json:"meta_principal"`
	Detalhe            json.RawMessage `json:"detalhe"`
}

type GamifMinhaPosicaoResponse struct {
	CodRCA              string          `json:"cod_rca"`
	Posicao             int             `json:"posicao"`
	TotalRCAs           int             `json:"total_rcas"`
	PontosTotal         float64         `json:"pontos_total"`
	BonusTotal          float64         `json:"bonus_total"`
	VolumeDesempate     float64         `json:"volume_desempate"`
	NivelPrincipal      string          `json:"nivel_principal,omitempty"`
	PercentualPrincipal float64         `json:"percentual_principal"`
	NivelOrdem          int             `json:"nivel_ordem"`
	RealizadoPrincipal  float64         `json:"realizado_principal"`
	MetaPrincipal       float64         `json:"meta_principal"`
	Detalhe             json.RawMessage `json:"detalhe"`
}

// resolverRankingCampanha roda a query de ranking (com desempate por
// volume — curva ABC, ver comentário no SQL) e devolve tanto o ranking
// GERAL (só quem tem pontos/bônus > 0 — achado do Claudio 22/09/2026, "o
// ranking ficou estranho": regra rca_completo toca todo RCA do vínculo pra
// rastrear progresso, inundando a lista de zerados) quanto a "minha
// posição" de um cod_rca específico, se pedido — essa última SEMPRE acha o
// RCA, mesmo zerado, porque é onde o progresso ("faltam N") precisa
// aparecer. Extraído do handler admin pra ser reaproveitado pelo endpoint
// público (visão do RCA de verdade, não só simulação).
func resolverRankingCampanha(db *sql.DB, empresaID string, campanhaID int, codRCAFiltro string) (ranking []GamifRankingLinha, minhaPosicao *GamifMinhaPosicaoResponse, err error) {
	rows, err := db.Query(`
		SELECT cod_rca, nome_rca, pontos_total, bonus_total, volume_desempate,
		       COALESCE(nivel_principal, ''), percentual_principal, COALESCE(nivel_ordem, 0),
		       COALESCE(realizado_principal, 0), COALESCE(meta_principal, 0), detalhe,
		       RANK() OVER (ORDER BY pontos_total DESC, volume_desempate DESC) AS posicao,
		       COUNT(*) OVER () AS total_rcas
		FROM farol.gamif_pontuacao WHERE campanha_id = $1 AND empresa_id = $2
		ORDER BY pontos_total DESC, volume_desempate DESC, cod_rca
	`, campanhaID, empresaID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	ranking = []GamifRankingLinha{}
	totalRCAsComProgresso := 0
	for rows.Next() {
		var linha GamifRankingLinha
		if err := rows.Scan(&linha.CodRCA, &linha.NomeRCA, &linha.PontosTotal, &linha.BonusTotal, &linha.VolumeDesempate, &linha.NivelPrincipal, &linha.PercentualPrincipal, &linha.NivelOrdem, &linha.RealizadoPrincipal, &linha.MetaPrincipal, &linha.Detalhe, &linha.Posicao, &totalRCAsComProgresso); err != nil {
			return nil, nil, err
		}
		if codRCAFiltro != "" {
			if linha.CodRCA == codRCAFiltro {
				minhaPosicao = &GamifMinhaPosicaoResponse{
					CodRCA: linha.CodRCA, Posicao: linha.Posicao, TotalRCAs: totalRCAsComProgresso,
					PontosTotal: linha.PontosTotal, BonusTotal: linha.BonusTotal, VolumeDesempate: linha.VolumeDesempate,
					NivelPrincipal: linha.NivelPrincipal, PercentualPrincipal: linha.PercentualPrincipal, NivelOrdem: linha.NivelOrdem,
					RealizadoPrincipal: linha.RealizadoPrincipal, MetaPrincipal: linha.MetaPrincipal, Detalhe: linha.Detalhe,
				}
			}
			continue
		}
		if linha.PontosTotal > 0 || linha.BonusTotal > 0 {
			ranking = append(ranking, linha)
		}
	}
	return ranking, minhaPosicao, rows.Err()
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

		codRCAFiltro := strings.TrimSpace(r.URL.Query().Get("cod_rca"))
		ranking, minhaPosicao, err := resolverRankingCampanha(db, spCtx.EmpresaID, campanhaID, codRCAFiltro)
		if err != nil {
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
		// total_rcas aqui é a contagem de quem tem pontos/bônus > 0 (o
		// "ranking" de verdade) — não o total de linhas com progresso
		// (que inclui zerados de regras tipo rca_completo).
		json.NewEncoder(w).Encode(map[string]any{"ranking": ranking, "total_rcas": len(ranking)})
	}
}

// ─── GamifExtratosHandler — GET/POST /api/farol/gamif-extratos?campanha_id= ───

// Extrato de pagamento — pedido do Claudio 23/09/2026: "cada campanha
// precisa ser rastreável e teremos que ter um extrato para enviar aos
// gestores e RH para o pagamento... para documentação no jurídico também".
// GET lista os extratos JÁ GERADOS (histórico, cada um imutável desde a
// criação). POST gera um NOVO — recalcula a campanha primeiro (pega o
// estado mais atual das vendas) e SÓ ENTÃO congela uma cópia em
// farol.gamif_extratos; gerar de novo no futuro não apaga nem altera os
// extratos anteriores, criando um novo ao lado (histórico de pagamentos).
type GamifExtratoLinha struct {
	CodRCA              string  `json:"cod_rca"`
	NomeRCA             string  `json:"nome_rca"`
	PontosTotal         float64 `json:"pontos_total"`
	BonusTotal          float64 `json:"bonus_total"`
	VolumeDesempate     float64 `json:"volume_desempate"`
	NivelPrincipal      string  `json:"nivel_principal,omitempty"`
	PercentualPrincipal float64 `json:"percentual_principal"`
	NivelOrdem          int     `json:"nivel_ordem"`
	// RealizadoPrincipal/MetaPrincipal — pedido do Claudio 23/09/2026: "no
	// extrato do RCA colocar a quantidade objetivo e o que o RCA vendeu".
	RealizadoPrincipal float64         `json:"realizado_principal"`
	MetaPrincipal      float64         `json:"meta_principal"`
	Detalhe            json.RawMessage `json:"detalhe"`
}

type GamifExtratoResponse struct {
	ID         int                 `json:"id"`
	CampanhaID int                 `json:"campanha_id"`
	GeradoEm   string              `json:"gerado_em"`
	GeradoPor  string              `json:"gerado_por"`
	Linhas     []GamifExtratoLinha `json:"linhas"`
}

func listarGamifExtratos(db *sql.DB, empresaID string, campanhaID int) ([]GamifExtratoResponse, error) {
	rows, err := db.Query(`
		SELECT id, campanha_id, gerado_em::text, gerado_por, linhas
		FROM farol.gamif_extratos WHERE campanha_id = $1 AND empresa_id = $2
		ORDER BY gerado_em DESC
	`, campanhaID, empresaID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	extratos := []GamifExtratoResponse{}
	for rows.Next() {
		var e GamifExtratoResponse
		var linhasJSON []byte
		if err := rows.Scan(&e.ID, &e.CampanhaID, &e.GeradoEm, &e.GeradoPor, &linhasJSON); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(linhasJSON, &e.Linhas); err != nil {
			return nil, err
		}
		extratos = append(extratos, e)
	}
	return extratos, rows.Err()
}

func GamifExtratosHandler(db *sql.DB) http.HandlerFunc {
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
		w.Header().Set("Content-Type", "application/json")

		switch r.Method {
		case http.MethodGet:
			extratos, err := listarGamifExtratos(db, spCtx.EmpresaID, campanhaID)
			if err != nil {
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
			json.NewEncoder(w).Encode(extratos)

		case http.MethodPost:
			if err := CalcularPontuacaoCampanha(db, spCtx.EmpresaID, campanhaID); err != nil {
				http.Error(w, "Erro ao recalcular: "+err.Error(), http.StatusInternalServerError)
				return
			}
			rows, err := db.Query(`
				SELECT cod_rca, nome_rca, pontos_total, bonus_total, volume_desempate, COALESCE(nivel_principal,''), percentual_principal, COALESCE(nivel_ordem,0),
				       COALESCE(realizado_principal,0), COALESCE(meta_principal,0), detalhe
				FROM farol.gamif_pontuacao WHERE campanha_id = $1 AND empresa_id = $2
				ORDER BY pontos_total DESC, volume_desempate DESC, cod_rca
			`, campanhaID, spCtx.EmpresaID)
			if err != nil {
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
			linhas := []GamifExtratoLinha{}
			for rows.Next() {
				var l GamifExtratoLinha
				if err := rows.Scan(&l.CodRCA, &l.NomeRCA, &l.PontosTotal, &l.BonusTotal, &l.VolumeDesempate, &l.NivelPrincipal, &l.PercentualPrincipal, &l.NivelOrdem, &l.RealizadoPrincipal, &l.MetaPrincipal, &l.Detalhe); err != nil {
					rows.Close()
					http.Error(w, "Database error", http.StatusInternalServerError)
					return
				}
				linhas = append(linhas, l)
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
			linhasJSON, _ := json.Marshal(linhas)
			var id int
			var geradoEm string
			err = db.QueryRow(`
				INSERT INTO farol.gamif_extratos (empresa_id, campanha_id, gerado_por, linhas)
				VALUES ($1,$2,$3,$4) RETURNING id, gerado_em::text
			`, spCtx.EmpresaID, campanhaID, spCtx.UserID, linhasJSON).Scan(&id, &geradoEm)
			if err != nil {
				http.Error(w, "Database error: "+err.Error(), http.StatusInternalServerError)
				return
			}
			writeAuditLog(db, spCtx.EmpresaID, spCtx.UserID, "gamif_extratos", strconv.Itoa(id), "gerar", map[string]any{"campanha_id": campanhaID, "linhas": len(linhas)})
			log.Printf("Gamificacao: extrato %d gerado campanha=%d (%d RCAs) empresa %s por %s", id, campanhaID, len(linhas), spCtx.EmpresaID, spCtx.UserID)
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(GamifExtratoResponse{ID: id, CampanhaID: campanhaID, GeradoEm: geradoEm, GeradoPor: spCtx.UserID, Linhas: linhas})

		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

// ─── GamifPublicMinhasCampanhasHandler — GET /api/farol/public/gamif-minhas-campanhas ─

// GamifCampanhaComPosicao — 1 campanha ativa + a posição/progresso do RCA
// nela, pro celular do RCA em campo (mesma URL pública /m/.../metas-industria
// que ele já usa, decisão do Claudio 22/09/2026: "mesmo padrão" de
// segurança do resto da tela — sem login, só quem tem o link/CNPJ+cod
// entra, igual Cobertura/Sortimento já são hoje).
type GamifCampanhaComPosicao struct {
	CampanhaID    int    `json:"campanha_id"`
	Nome          string `json:"nome"`
	IndustriaNome string `json:"industria_nome"`
	DataInicio    string `json:"data_inicio"`
	DataFim       string `json:"data_fim"`
	GamifMinhaPosicaoResponse
}

func GamifPublicMinhasCampanhasHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		q := r.URL.Query()
		empresaID := resolveEmpresaCNPJ(db, q.Get("cnpj"))
		if empresaID == "" {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "empresa não encontrada para este CNPJ"})
			return
		}
		codRCA := strings.TrimSpace(q.Get("cod_rca"))
		if codRCA == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "cod_rca é obrigatório"})
			return
		}

		// Só campanhas ATIVAS em que este RCA tem alguma linha de
		// pontuação (pontuando ou só progresso) — nunca lista campanhas
		// de outros RCAs nem detalhe de quem está na frente (mesma regra
		// de resolverRankingCampanha).
		rows, err := db.Query(`
			SELECT c.id, c.nome, i.nome, c.data_inicio::text, c.data_fim::text
			FROM farol.gamif_campanhas c
			JOIN farol.industrias i ON i.id = c.industria_id
			WHERE c.empresa_id = $1 AND c.status = 'ativa'
			  AND EXISTS (SELECT 1 FROM farol.gamif_pontuacao p WHERE p.campanha_id = c.id AND p.cod_rca = $2)
			ORDER BY c.created_at DESC
		`, empresaID, codRCA)
		if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		type campanhaBase struct {
			id                               int
			nome, industriaNome, inicio, fim string
		}
		var campanhas []campanhaBase
		for rows.Next() {
			var c campanhaBase
			if err := rows.Scan(&c.id, &c.nome, &c.industriaNome, &c.inicio, &c.fim); err != nil {
				rows.Close()
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
			campanhas = append(campanhas, c)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}

		out := make([]GamifCampanhaComPosicao, 0, len(campanhas))
		for _, c := range campanhas {
			_, minhaPosicao, err := resolverRankingCampanha(db, empresaID, c.id, codRCA)
			if err != nil {
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
			if minhaPosicao == nil {
				continue // não deveria acontecer (o EXISTS acima já garantiu), defensivo
			}
			out = append(out, GamifCampanhaComPosicao{
				CampanhaID: c.id, Nome: c.nome, IndustriaNome: c.industriaNome,
				DataInicio: c.inicio, DataFim: c.fim,
				GamifMinhaPosicaoResponse: *minhaPosicao,
			})
		}
		json.NewEncoder(w).Encode(map[string]any{"campanhas": out})
	}
}
