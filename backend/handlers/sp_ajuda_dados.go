package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"

	"fb_farol/services"
)

// SpAjudaDadosHandler — POST /api/sp/ajuda/dados
//
// Body: { "pergunta": "..." }
// Resposta: { reply, sql, columns, rows, truncado }
//
// Pipeline:
//   1. Valida usuário e empresa
//   2. Z.AI gera SQL (system prompt com schema das views)
//   3. Validador rejeita SQL inseguro
//   4. Filtra por empresa_id automaticamente
//   5. Executa em transação READ ONLY com statement_timeout=5s, LIMIT 100
//   6. Z.AI gera narrativa curta sobre o resultado
func SpAjudaDadosHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		spCtx := GetSpContext(r)
		if spCtx == nil || spCtx.EmpresaID == "" {
			http.Error(w, `{"error":"empresa não identificada no contexto"}`, http.StatusUnauthorized)
			return
		}

		var body struct {
			Pergunta  string                   `json:"pergunta"`
			Historico []services.HistoricoMsg `json:"historico,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Pergunta == "" {
			http.Error(w, `{"error":"pergunta obrigatória"}`, http.StatusBadRequest)
			return
		}

		out, err := services.ResponderPerguntaDados(db, body.Pergunta, spCtx.EmpresaID, body.Historico)
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			fmt.Fprintf(w, `{"error":%q}`, err.Error())
			return
		}

		json.NewEncoder(w).Encode(out)
	}
}
