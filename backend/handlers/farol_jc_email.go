package handlers

// farol_jc_email.go — envia o Painel de Objetivos por Indústria por
// e-mail, montado no próprio servidor (não pelo agente de IA) — decisão
// de 15/09/2026 depois de o agente do Paperclip gerar o HTML duas vezes
// e nunca conseguir anexá-lo de fato na issue (ferramenta de anexo
// instável lá, ver farol_mcp.go). Reusa a MESMA infra de e-mail já
// validada em produção (services.SendHTMLReport, multipart/alternative,
// mesmo estilo visual do resumo semanal/executivo) — nenhuma peça nova
// de infraestrutura, só um HTML novo.

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"sort"
	"strings"

	"fb_farol/services"
)

func esc(s string) string { return html.EscapeString(s) }

func brlSimples(v float64) string {
	return fmt.Sprintf("R$ %s", formatarMilhar(v))
}

// formatarMilhar formata "12345.6" como "12.345,60" (padrão BR) sem
// depender de lib de i18n — mesmo grão de formatação já usado nos outros
// e-mails do Farol, reimplementado aqui pra não criar dependência cruzada
// desnecessária entre pacotes internos por uma função tão pequena.
func formatarMilhar(v float64) string {
	neg := v < 0
	if neg {
		v = -v
	}
	inteiro := int64(v)
	centavos := int64((v-float64(inteiro))*100 + 0.5)
	s := fmt.Sprintf("%d", inteiro)
	var partes []string
	for len(s) > 3 {
		partes = append([]string{s[len(s)-3:]}, partes...)
		s = s[:len(s)-3]
	}
	partes = append([]string{s}, partes...)
	out := strings.Join(partes, ".") + fmt.Sprintf(",%02d", centavos)
	if neg {
		return "-" + out
	}
	return out
}

// construirEmailObjetivosIndustria monta assunto + corpo (texto e HTML)
// do painel — mesma paleta visual do resumo semanal (services/farol_resumo_semanal.go:
// CorpoHTML): teal #1B6660 de destaque, verde #2C6E49/vermelho #A34A1B
// pro farol atingiu/não atingiu, cinzas #667/#556/#889 pra texto
// secundário. Redes que JÁ atingem não entram nas tabelas — só as que
// precisam de atenção (mesmo princípio de "Onde está o dinheiro" do
// resumo semanal: a lista é pra agir, não pra conferir tudo).
func construirEmailObjetivosIndustria(out objetivosIndustriaOutput, comp *comparativoFechamentoOutput) (assunto, texto, htmlBody string) {
	assunto = fmt.Sprintf("Farol — Objetivos por Indústria: %s (%s)", out.Industria, out.Periodo)

	var faltandoCobertura, faltandoSortimento []redeObjetivo
	for _, r := range out.Redes {
		if r.CoberturaAtingiu != nil && !*r.CoberturaAtingiu {
			faltandoCobertura = append(faltandoCobertura, r)
		}
		if r.SortimentoAtingiu != nil && !*r.SortimentoAtingiu {
			faltandoSortimento = append(faltandoSortimento, r)
		}
	}
	sort.Slice(faltandoCobertura, func(i, j int) bool {
		vi, vj := 0.0, 0.0
		if faltandoCobertura[i].CoberturaValorTotal != nil {
			vi = *faltandoCobertura[i].CoberturaValorTotal
		}
		if faltandoCobertura[j].CoberturaValorTotal != nil {
			vj = *faltandoCobertura[j].CoberturaValorTotal
		}
		return vi > vj
	})
	sort.Slice(faltandoSortimento, func(i, j int) bool {
		vi, vj := 0.0, 0.0
		if faltandoSortimento[i].SortimentoMedioEans != nil {
			vi = *faltandoSortimento[i].SortimentoMedioEans
		}
		if faltandoSortimento[j].SortimentoMedioEans != nil {
			vj = *faltandoSortimento[j].SortimentoMedioEans
		}
		return vi > vj
	})

	var b strings.Builder
	fmt.Fprintf(&b, `<div style="font-family:Arial,Helvetica,sans-serif;color:#1a1a1a;max-width:680px">`)
	fmt.Fprintf(&b, `<p style="margin:0 0 4px;font-size:13px;color:#667">Farol de Vendas · Objetivos por Indústria</p>`)
	fmt.Fprintf(&b, `<h2 style="margin:0 0 6px;font-size:20px">%s — %s</h2>`, esc(out.Industria), esc(out.Periodo))
	fmt.Fprintf(&b, `<p style="margin:0 0 18px;color:#556;font-size:14px">%d Redes no programa, fluxo %s.</p>`,
		out.TotalRedes, esc(out.Fluxo))

	cardHTML := func(titulo string, atingindo, faltando int) string {
		total := atingindo + faltando
		pct := 0.0
		if total > 0 {
			pct = float64(atingindo) / float64(total) * 100
		}
		return fmt.Sprintf(`<div style="background:#f6f7f6;border-left:3px solid #1B6660;padding:14px 18px;margin-bottom:12px">
<div style="font-size:14px;color:#556">%s</div>
<div style="font-size:26px;font-weight:bold;margin-top:2px"><span style="color:#2C6E49">%d atingindo</span> <span style="color:#889;font-size:16px;font-weight:normal">/ %d</span></div>
<div style="font-size:13px;color:#556;margin-top:4px"><span style="color:#A34A1B;font-weight:bold">%d</span> faltando atingir · <b>%.0f%%</b> de cobertura do programa</div></div>`,
			esc(titulo), atingindo, total, faltando, pct)
	}
	b.WriteString(cardHTML("Cobertura por Rede", out.CoberturaRedesAtingindo, out.CoberturaRedesFaltando))
	b.WriteString(cardHTML("Sortimento por Rede", out.SortimentoRedesAtingindo, out.SortimentoRedesFaltando))

	tabelaRedes := func(titulo, coluna string, redes []redeObjetivo, valorDe func(redeObjetivo) (string, bool)) {
		if len(redes) == 0 {
			return
		}
		limite := redes
		resto := 0
		if len(limite) > 15 {
			resto = len(limite) - 15
			limite = limite[:15]
		}
		fmt.Fprintf(&b, `<h3 style="font-size:15px;margin:22px 0 8px">%s</h3>`, esc(titulo))
		b.WriteString(`<table cellpadding="0" cellspacing="0" style="width:100%;border-collapse:collapse;font-size:14px">`)
		for _, r := range limite {
			valor, ok := valorDe(r)
			if !ok {
				valor = "—"
			}
			fmt.Fprintf(&b, `<tr>
<td style="padding:9px 0;border-bottom:1px solid #eef1f0">%s
  <div style="color:#667;font-size:12.5px;margin-top:2px">Rede %s · %d loja(s) · %s</div></td>
<td style="padding:9px 0;border-bottom:1px solid #eef1f0;text-align:right;white-space:nowrap;font-weight:bold;color:#A34A1B">%s</td>
</tr>`, esc(r.Fantasia), esc(r.CodPrinc), r.QtLojas, esc(r.NomeGGV), valor)
		}
		b.WriteString(`</table>`)
		if resto > 0 {
			fmt.Fprintf(&b, `<p style="margin:8px 0 0;color:#889;font-size:12.5px">+ %d Redes adicionais faltando atingir (consulte a API pra lista completa).</p>`, resto)
		}
	}
	tabelaRedes("Redes faltando atingir Cobertura", "Valor", faltandoCobertura, func(r redeObjetivo) (string, bool) {
		if r.CoberturaValorTotal == nil {
			return "", false
		}
		return brlSimples(*r.CoberturaValorTotal), true
	})
	tabelaRedes("Redes faltando atingir Sortimento", "EANs", faltandoSortimento, func(r redeObjetivo) (string, bool) {
		if r.SortimentoMedioEans == nil {
			return "", false
		}
		return fmt.Sprintf("%.1f EANs/loja", *r.SortimentoMedioEans), true
	})

	if comp != nil && comp.Divergentes > 0 {
		var divergentes []comparativoLinha
		for _, l := range comp.Linhas {
			if l.Status == "DIVERGE" {
				divergentes = append(divergentes, l)
			}
		}
		sort.Slice(divergentes, func(i, j int) bool {
			return absFloat(divergentes[i].DiferencaValorP) > absFloat(divergentes[j].DiferencaValorP)
		})
		limite := divergentes
		resto := 0
		if len(limite) > 15 {
			resto = len(limite) - 15
			limite = limite[:15]
		}
		fmt.Fprintf(&b, `<h3 style="font-size:15px;margin:24px 0 8px">Divergências vs. fechamento reportado (%d de %d Redes)</h3>`,
			comp.Divergentes, comp.Total)
		b.WriteString(`<table cellpadding="0" cellspacing="0" style="width:100%;border-collapse:collapse;font-size:14px">`)
		for _, l := range limite {
			fmt.Fprintf(&b, `<tr>
<td style="padding:9px 0;border-bottom:1px solid #eef1f0">%s
  <div style="color:#667;font-size:12.5px;margin-top:2px">Rede %s</div></td>
<td style="padding:9px 0;border-bottom:1px solid #eef1f0;text-align:right;white-space:nowrap;font-weight:bold;color:#A34A1B">%s (%.1f%%)</td>
</tr>`, esc(l.Fantasia), esc(l.CodPrinc), brlSimples(l.DiferencaValor), l.DiferencaValorP)
		}
		b.WriteString(`</table>`)
		if resto > 0 {
			fmt.Fprintf(&b, `<p style="margin:8px 0 0;color:#889;font-size:12.5px">+ %d Redes adicionais divergentes.</p>`, resto)
		}
	}

	fmt.Fprintf(&b, `<hr style="border:0;border-top:1px solid #e3e6e5;margin:26px 0 12px">
<p style="color:#889;font-size:12px;line-height:1.6;margin:0">
Período: %s a %s. Dado direto do motor de apuração do Farol (mesmo cálculo do Painel de Objetivos por Indústria) — não é estimativa.
</p></div>`, esc(out.DataInicio), esc(out.DataFim))

	htmlBody = b.String()

	var t strings.Builder
	fmt.Fprintf(&t, "Objetivos por Industria - %s (%s)\n\n", out.Industria, out.Periodo)
	fmt.Fprintf(&t, "Cobertura: %d atingindo / %d faltando (de %d Redes)\n", out.CoberturaRedesAtingindo, out.CoberturaRedesFaltando, out.TotalRedes)
	fmt.Fprintf(&t, "Sortimento: %d atingindo / %d faltando (de %d Redes)\n", out.SortimentoRedesAtingindo, out.SortimentoRedesFaltando, out.TotalRedes)
	if comp != nil {
		fmt.Fprintf(&t, "Comparativo de fechamento: %d divergentes de %d Redes\n", comp.Divergentes, comp.Total)
	}
	texto = t.String()

	return assunto, texto, htmlBody
}

// enviarEmailObjetivosIndustria calcula (mesma lógica das duas rotas
// existentes) e envia o painel por e-mail — ponta a ponta, sem depender
// de nenhuma ferramenta de anexo de terceiro. O comparativo de fechamento
// é best-effort: indústria sem fechamento externo cadastrado ainda manda
// o e-mail só com Cobertura/Sortimento.
func enviarEmailObjetivosIndustria(db *sql.DB, empresaID, industria, periodo, fluxo, email string) error {
	out, err := calcularObjetivosIndustria(db, empresaID, industria, periodo, fluxo)
	if err != nil {
		return err
	}
	var comp *comparativoFechamentoOutput
	if c, cerr := calcularComparativoFechamento(db, empresaID, industria, periodo); cerr == nil {
		comp = &c
	}
	assunto, texto, htmlBody := construirEmailObjetivosIndustria(out, comp)
	return services.SendHTMLReport([]string{email}, assunto, texto, htmlBody)
}

// registrarRotaEmailObjetivosIndustria adiciona GET /objetivos-industria-email
// ao mux do farol_jc_rest.go — mesma auth/escopo dos outros endpoints REST.
func registrarRotaEmailObjetivosIndustria(mux *http.ServeMux, getDB func() *sql.DB, empresaID string) {
	mux.HandleFunc("/objetivos-industria-email", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSONErro(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		q := r.URL.Query()
		industria, periodo, email := q.Get("industria"), q.Get("periodo"), strings.TrimSpace(q.Get("email"))
		if industria == "" || periodo == "" || email == "" {
			writeJSONErro(w, http.StatusBadRequest, "parâmetros obrigatórios: industria, periodo (AAAA-MM ou DD/MM/AAAA a DD/MM/AAAA), email")
			return
		}
		if err := enviarEmailObjetivosIndustria(getDB(), empresaID, industria, periodo, q.Get("fluxo"), email); err != nil {
			writeJSONErro(w, http.StatusBadRequest, err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"enviado": true, "para": email, "industria": industria, "periodo": periodo})
	})
}
