package handlers

// farol_gamificacao_jc_email.go — resumo diário do ranking de Gamificação
// pro CEO José Costa, mesmo padrão já validado em farol_jc_email.go
// (objetivos-industria-email): o FAROL monta e manda o HTML sozinho —
// nada de anexo pelo agente do Paperclip (mecanismo instável, já
// abandonado uma vez, ver farol_jc_email.go). Pedido do Claudio
// 23/09/2026: "enviar ao CEO todos os dias um resumo... do ranking por
// campanha" — uma seção por campanha ATIVA, sem precisar informar
// campanha_id (o agente do Paperclip só chama a rota, sem saber IDs).
//
// Reaproveita resolverRankingCampanha (mesmo motor do painel admin) — não
// recalcula nada de novo, só lê farol.gamif_pontuacao, que já é mantida em
// dia pelo prewarm diário (RecalcularGamificacaoAtivas, farol_gamificacao.go).

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"fb_farol/services"
)

type gamifResumoCampanha struct {
	CampanhaID    int                 `json:"campanha_id"`
	Nome          string              `json:"nome"`
	IndustriaNome string              `json:"industria_nome"`
	DataInicio    string              `json:"data_inicio"`
	DataFim       string              `json:"data_fim"`
	TotalRCAs     int                 `json:"total_rcas_pontuando"`
	Ranking       []GamifRankingLinha `json:"ranking_top10"`
}

// listarResumoGamificacaoAtivas monta o resumo de TODAS as campanhas ativas
// da empresa, top 10 do ranking de cada uma.
func listarResumoGamificacaoAtivas(db *sql.DB, empresaID string) ([]gamifResumoCampanha, error) {
	rows, err := db.Query(`
		SELECT c.id, c.nome, i.nome, c.data_inicio::text, c.data_fim::text
		FROM farol.gamif_campanhas c
		JOIN farol.industrias i ON i.id = c.industria_id
		WHERE c.empresa_id = $1 AND c.status = 'ativa'
		ORDER BY c.created_at DESC
	`, empresaID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var campanhas []gamifResumoCampanha
	for rows.Next() {
		var c gamifResumoCampanha
		if err := rows.Scan(&c.CampanhaID, &c.Nome, &c.IndustriaNome, &c.DataInicio, &c.DataFim); err != nil {
			return nil, err
		}
		campanhas = append(campanhas, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range campanhas {
		ranking, _, err := resolverRankingCampanha(db, empresaID, campanhas[i].CampanhaID, "")
		if err != nil {
			return nil, fmt.Errorf("campanha %d: %w", campanhas[i].CampanhaID, err)
		}
		campanhas[i].TotalRCAs = len(ranking)
		if len(ranking) > 10 {
			ranking = ranking[:10]
		}
		campanhas[i].Ranking = ranking
	}
	return campanhas, nil
}

// construirEmailResumoGamificacao monta o HTML — mesma paleta visual dos
// outros e-mails do Farol pro José Costa (teal #1B6660 de destaque, verde
// #2C6E49 pra pontos/bônus, cinzas #667/#556/#889 pra texto secundário,
// ver farol_jc_email.go).
func construirEmailResumoGamificacao(campanhas []gamifResumoCampanha) (assunto, texto, htmlBody string) {
	assunto = fmt.Sprintf("Farol — Gamificação: resumo diário (%d campanha(s) ativa(s))", len(campanhas))

	var b strings.Builder
	b.WriteString(`<div style="font-family:Arial,Helvetica,sans-serif;color:#1a1a1a;max-width:680px">`)
	b.WriteString(`<p style="margin:0 0 4px;font-size:13px;color:#667">Farol de Vendas · Gamificação</p>`)
	b.WriteString(`<h2 style="margin:0 0 18px;font-size:20px">Resumo diário do ranking</h2>`)

	if len(campanhas) == 0 {
		b.WriteString(`<p style="color:#556;font-size:14px">Nenhuma campanha ativa no momento.</p>`)
	}

	for _, c := range campanhas {
		fmt.Fprintf(&b, `<div style="background:#f6f7f6;border-left:3px solid #1B6660;padding:14px 18px;margin-bottom:12px">
<div style="font-size:16px;font-weight:bold">%s</div>
<div style="font-size:13px;color:#556;margin-top:2px">%s · %s a %s · <b style="color:#2C6E49">%d RCA(s)</b> pontuando</div>
</div>`, esc(c.Nome), esc(c.IndustriaNome), esc(c.DataInicio), esc(c.DataFim), c.TotalRCAs)

		if len(c.Ranking) == 0 {
			b.WriteString(`<p style="color:#889;font-size:13px;margin:0 0 22px">Ninguém pontuou ainda nesta campanha.</p>`)
			continue
		}

		b.WriteString(`<table cellpadding="0" cellspacing="0" style="width:100%;border-collapse:collapse;font-size:14px;margin-bottom:22px">`)
		for _, l := range c.Ranking {
			nivel := l.NivelPrincipal
			if nivel == "" {
				nivel = "—"
			}
			fmt.Fprintf(&b, `<tr>
<td style="padding:8px 0;border-bottom:1px solid #eef1f0;color:#889;width:28px">%dº</td>
<td style="padding:8px 0;border-bottom:1px solid #eef1f0">%s
  <div style="color:#667;font-size:12.5px;margin-top:2px">%s</div></td>
<td style="padding:8px 0;border-bottom:1px solid #eef1f0;text-align:right;white-space:nowrap;font-weight:bold;color:#2C6E49">%.0f pts</td>
<td style="padding:8px 0;border-bottom:1px solid #eef1f0;text-align:right;white-space:nowrap;color:#556">%s</td>
</tr>`, l.Posicao, esc(l.NomeRCA), esc(nivel), l.PontosTotal, brlSimples(l.BonusTotal))
		}
		b.WriteString(`</table>`)
	}

	b.WriteString(`<hr style="border:0;border-top:1px solid #e3e6e5;margin:26px 0 12px">
<p style="color:#889;font-size:12px;line-height:1.6;margin:0">
Dado direto do motor de Gamificação do Farol, recalculado automaticamente todo dia às 07:00 — não é estimativa.
</p></div>`)

	htmlBody = b.String()

	var t strings.Builder
	fmt.Fprintf(&t, "Gamificacao - Resumo diario (%d campanha(s) ativa(s))\n\n", len(campanhas))
	for _, c := range campanhas {
		fmt.Fprintf(&t, "%s (%s): %d RCA(s) pontuando\n", c.Nome, c.IndustriaNome, c.TotalRCAs)
	}
	texto = t.String()

	return assunto, texto, htmlBody
}

// enviarEmailResumoGamificacao monta e envia o resumo — ponta a ponta,
// sem depender de nenhuma ferramenta de anexo de terceiro.
func enviarEmailResumoGamificacao(db *sql.DB, empresaID, email string) error {
	campanhas, err := listarResumoGamificacaoAtivas(db, empresaID)
	if err != nil {
		return err
	}
	assunto, texto, htmlBody := construirEmailResumoGamificacao(campanhas)
	return services.SendHTMLReport([]string{email}, assunto, texto, htmlBody)
}

// registrarRotasGamificacaoJC adiciona GET /gamif-ranking (JSON cru, pro
// agente formatar se quiser) e GET /gamif-resumo-email (monta e manda o
// HTML sozinho) ao mux do farol_jc_rest.go — mesma auth/escopo dos outros
// endpoints REST desse arquivo.
func registrarRotasGamificacaoJC(mux *http.ServeMux, getDB func() *sql.DB, empresaID string) {
	mux.HandleFunc("/gamif-ranking", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSONErro(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		campanhas, err := listarResumoGamificacaoAtivas(getDB(), empresaID)
		if err != nil {
			writeJSONErro(w, http.StatusInternalServerError, "erro ao montar ranking: "+err.Error())
			return
		}
		writeJSON(w, map[string]any{"campanhas": campanhas})
	})

	mux.HandleFunc("/gamif-resumo-email", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSONErro(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		email := strings.TrimSpace(r.URL.Query().Get("email"))
		if email == "" {
			writeJSONErro(w, http.StatusBadRequest, "parâmetro obrigatório: email")
			return
		}
		permitidos := emailsPermitidos()
		if len(permitidos) == 0 {
			writeJSONErro(w, http.StatusServiceUnavailable, "EMAILS_PERMITIDOS não configurado neste serviço")
			return
		}
		if !emailPermitido(email, permitidos) {
			writeJSONErro(w, http.StatusForbidden, "destinatário não autorizado")
			return
		}
		if err := enviarEmailResumoGamificacao(getDB(), empresaID, email); err != nil {
			writeJSONErro(w, http.StatusBadRequest, err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"enviado": true, "para": email})
	})
}
