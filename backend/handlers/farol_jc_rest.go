package handlers

// farol_jc_rest.go — API REST simples (GET + querystring + Bearer token)
// pros mesmos dois recursos do servidor MCP (farol_mcp.go), pro agente
// "Gerador de Painéis"/CEO/CFO no Paperclip.
//
// Decisão de 15/09/2026: o padrão real dos agentes do Paperclip (adapter
// Claude Code) é chamar API REST direto durante a execução, documentada em
// texto no AGENTS.md do agente (mesmo padrão já em uso lá pra
// cerebro-jc-api.fbtechia.com) — não existe sessão MCP com handshake
// mantida entre chamadas de bash/curl. Confirmado no próprio ecossistema
// Paperclip: "Agents talk to the REST API directly during their heartbeat"
// (README do paperclip-mcp, projeto do board operator). Por isso este
// endpoint existe AO LADO do servidor MCP (farol_mcp.go) — mesmo escopo de
// empresa e mesmo token, reusando as mesmas funções de cálculo
// (calcularObjetivosIndustria/calcularComparativoFechamento), só troca o
// transporte (JSON puro na resposta, sem envelope MCP/JSON-RPC).

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
)

// NewFarolJCRestHandler monta as rotas REST simples, escopadas à mesma
// empresa e token do MCP (FAROL_MCP_EMPRESA_ID/FAROL_MCP_TOKEN) — mesmo
// racional de fail-safe do NewMCPHandler: sem as env vars, não monta nada.
func NewFarolJCRestHandler(getDB func() *sql.DB) (h http.Handler, ok bool) {
	empresaID := strings.TrimSpace(os.Getenv("FAROL_MCP_EMPRESA_ID"))
	token := strings.TrimSpace(os.Getenv("FAROL_MCP_TOKEN"))
	if empresaID == "" || token == "" {
		return nil, false
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/industrias", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSONErro(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		nomes, err := listarNomesIndustrias(getDB(), empresaID)
		if err != nil {
			writeJSONErro(w, http.StatusInternalServerError, "erro ao listar indústrias")
			return
		}
		writeJSON(w, map[string]any{"industrias": nomes})
	})

	mux.HandleFunc("/objetivos-industria", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSONErro(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		q := r.URL.Query()
		industria := q.Get("industria")
		periodo := q.Get("periodo")
		if industria == "" || periodo == "" {
			writeJSONErro(w, http.StatusBadRequest, "parâmetros obrigatórios: industria, periodo (AAAA-MM)")
			return
		}
		out, err := calcularObjetivosIndustria(getDB(), empresaID, industria, periodo, q.Get("fluxo"))
		if err != nil {
			writeJSONErro(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, out)
	})

	mux.HandleFunc("/comparativo-fechamento", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSONErro(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		q := r.URL.Query()
		industria := q.Get("industria")
		periodo := q.Get("periodo")
		if industria == "" || periodo == "" {
			writeJSONErro(w, http.StatusBadRequest, "parâmetros obrigatórios: industria, periodo (AAAA-MM)")
			return
		}
		out, err := calcularComparativoFechamento(getDB(), empresaID, industria, periodo)
		if err != nil {
			writeJSONErro(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, out)
	})

	registrarRotaEmailObjetivosIndustria(mux, getDB, empresaID)

	log.Printf("[farol:mcp] rotas REST /api/farol-jc/{objetivos-industria,comparativo-fechamento,objetivos-industria-email} habilitadas")
	return authMiddleware(token, mux), true
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeJSONErro(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
