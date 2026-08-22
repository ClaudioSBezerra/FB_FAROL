// farol_envio.go — tela de envio do resumo: telefone, token e disparo.
//
// O disparo é MANUAL por wa.me, não integração. Para cinco pessoas uma vez por
// semana, a API oficial da Meta não se paga: exige verificação de empresa,
// número dedicado que sai do WhatsApp normal e template aprovado. O sistema
// prepara a mensagem, uma pessoa toca e envia.
package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"fb_farol/services"
)

type destinatarioResumo struct {
	UserID       string `json:"user_id"`
	Nome         string `json:"nome"`
	Email        string `json:"email"`
	Persona      string `json:"persona"`
	Telefone     string `json:"telefone"`
	Link         string `json:"link"`
	Acessos      int    `json:"acessos"`
	UltimoAcesso string `json:"ultimo_acesso"`
}

// FarolEnvioHandler
//
//	GET  /api/v2/farol/resumo/envio            lista os destinatários
//	POST /api/v2/farol/resumo/envio            {user_id, telefone}      grava o número
//	POST /api/v2/farol/resumo/envio?acao=gerar    {user_id}             cria/renova o token
//	POST /api/v2/farol/resumo/envio?acao=revogar  {user_id}             invalida o link
func FarolEnvioHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		// Esta tela mostra os tokens de TODO MUNDO, e token é credencial
		// portadora. Persona com escopo não entra — pela mesma razão que não
		// dispara o resumo nem usa o text-to-SQL.
		if escopoDoUsuario(db, spCtx, "").restrito() {
			log.Printf("[farol:envio] acesso negado — persona=%s user=%s",
				spCtx.TipoPersona, spCtx.UserID)
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}

		if r.Method == http.MethodPost {
			if !RequireWrite(spCtx, w) {
				return
			}
			var req struct {
				UserID   string `json:"user_id"`
				Telefone string `json:"telefone"`
			}
			if json.NewDecoder(r.Body).Decode(&req) != nil || req.UserID == "" {
				http.Error(w, `{"error":"payload inválido"}`, http.StatusBadRequest)
				return
			}

			switch r.URL.Query().Get("acao") {
			case "gerar":
				tok, err := services.TokenDoQuadro(db, req.UserID)
				if err != nil {
					http.Error(w, `{"error":"falha ao gerar o link"}`, http.StatusInternalServerError)
					return
				}
				log.Printf("[farol:envio] token gerado para user=%s por %s", req.UserID, spCtx.UserID)
				_ = json.NewEncoder(w).Encode(map[string]string{"link": services.LinkQuadroToken(tok)})
				return

			case "revogar":
				_, _ = db.Exec(`UPDATE farol.quadro_token SET revogado=TRUE WHERE user_id=$1`, req.UserID)
				log.Printf("[farol:envio] token REVOGADO de user=%s por %s", req.UserID, spCtx.UserID)
				_ = json.NewEncoder(w).Encode(map[string]bool{"revogado": true})
				return
			}

			// Só dígitos: o número entra numa URL do wa.me, e máscara ("(62)
			// 9 9999-8888") quebraria o link em silêncio — abriria o WhatsApp
			// sem destinatário, e ninguém entenderia por quê.
			tel := strings.Map(func(c rune) rune {
				if c >= '0' && c <= '9' {
					return c
				}
				return -1
			}, req.Telefone)
			if tel != "" && (len(tel) < 12 || len(tel) > 13) {
				http.Error(w, `{"error":"telefone deve ter 12 ou 13 dígitos: 55 + DDD + número"}`,
					http.StatusBadRequest)
				return
			}
			if _, err := db.Exec(`UPDATE users SET telefone=$1 WHERE id=$2`, tel, req.UserID); err != nil {
				http.Error(w, `{"error":"falha ao gravar"}`, http.StatusInternalServerError)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"telefone": tel})
			return
		}

		rows, err := db.Query(`
			SELECT u.id, COALESCE(NULLIF(u.full_name,''), u.email), u.email,
			       COALESCE(u.tipo_persona,''), COALESCE(u.telefone,''),
			       COALESCE(t.token,''), COALESCE(t.acessos,0),
			       COALESCE(to_char(t.ultimo_acesso,'DD/MM/YYYY HH24:MI'),'')
			  FROM users u
			  LEFT JOIN farol.quadro_token t ON t.user_id = u.id AND NOT t.revogado
			 WHERE u.farol_resumo_semanal = TRUE
			 ORDER BY u.full_name`)
		if err != nil {
			http.Error(w, `{"error":"falha ao listar"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		out := []destinatarioResumo{}
		for rows.Next() {
			var d destinatarioResumo
			var tok string
			if rows.Scan(&d.UserID, &d.Nome, &d.Email, &d.Persona, &d.Telefone,
				&tok, &d.Acessos, &d.UltimoAcesso) != nil {
				continue
			}
			if tok != "" {
				d.Link = services.LinkQuadroToken(tok)
			}
			out = append(out, d)
		}
		_ = json.NewEncoder(w).Encode(out)
	}
}
