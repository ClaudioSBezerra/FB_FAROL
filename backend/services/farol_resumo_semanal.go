// farol_resumo_semanal.go — o e-mail de segunda-feira.
//
// Descritivo, não avaliativo. O sistema não sabe de férias, licença, troca de
// território nem cliente que fechou as portas — o gestor sabe. Então o e-mail
// mostra ONDE está o dinheiro e por quê, e não afirma que fulano trabalhou mal.
// A diferença parece sutil e não é: um resumo que erra sobre número o gestor
// corrige; um que erra sobre pessoa faz ele desligar a notificação.
package services

import (
	"database/sql"
	"encoding/base64"
	"fmt"
	"html"
	"log"
	"os"
	"strings"
	"time"
)

// TopN — quantos RCAs aparecem nominalmente. O resto vai agregado numa linha.
//
// Cinco porque é o que cabe numa tela de celular sem rolagem e o que um
// supervisor consegue tratar numa semana. Lista de 40 não é priorização.
const TopN = 5

// baseURL do painel, para os links do corpo do e-mail.
func baseURLFarol() string {
	if v := strings.TrimSpace(os.Getenv("APP_URL")); v != "" {
		return strings.TrimRight(v, "/")
	}
	return "https://farol.fbtax.cloud"
}

// linkEquipe monta o link que abre o painel já filtrado.
//
// Depende do PARAM_PARA_COL do FarolExecutivo.tsx: `gerente` vira cod_gerente,
// `supervisor` vira cod_supervisor. Se um dos dois lados mudar sozinho, o link
// abre o painel sem filtro — degrada, não quebra.
func linkEquipe(param, cod string) string {
	if cod == "" {
		return baseURLFarol() + "/farol/v2"
	}
	return fmt.Sprintf("%s/farol/v2?%s=%s", baseURLFarol(), param, cod)
}

// GrupoResumo — um GGV (ou supervisor) na visão de quem recebe.
type GrupoResumo struct {
	Cod        string    `json:"cod"`
	Nome       string    `json:"nome"`
	Link       string    `json:"link"`
	TotalMesa  float64   `json:"total_mesa"`
	Rcas       int       `json:"rcas"`
	Vermelhos  int       `json:"vermelhos"`
	PioresRcas []RcaMesa `json:"piores_rcas"`
}

// ResumoUsuario — tudo que o e-mail de uma pessoa precisa.
type ResumoUsuario struct {
	Nome       string        `json:"nome"`
	Persona    string        `json:"persona"`
	Escopo     string        `json:"escopo"` // "toda a empresa", "sua equipe"
	Mes        string        `json:"mes"`
	Cobertura  Cobertura     `json:"cobertura"`
	TotalMesa  float64       `json:"total_mesa"`
	Realizado  float64       `json:"realizado"` // o que o escopo vendeu no mês
	Ritmo      float64       `json:"ritmo"`     // o que deveria ter vendido
	Vermelho   int           `json:"vermelho"`
	Amarelo    int           `json:"amarelo"`
	Verde      int           `json:"verde"`
	Grupos     []GrupoResumo `json:"grupos"` // por GGV, ou por supervisor
	TopGeral   []RcaMesa     `json:"top_geral"`
	RestoRcas  int           `json:"resto_rcas"`
	RestoValor float64       `json:"resto_valor"`
	LinkPainel string        `json:"link_painel"`
	LinkQuadro string        `json:"link_quadro"`
}

// MontarResumo recorta o ranking para uma pessoa e agrupa do jeito que ela lê.
//
// O agrupamento muda com o papel, de propósito. Quem enxerga a empresa inteira
// não quer 1.200 RCAs: quer saber QUAL GGV está pesando. Quem é GGV quer saber
// qual supervisor. Cada nível olha o nível imediatamente abaixo, e só desce ao
// RCA na lista curta dos piores.
func MontarResumo(todos []RcaMesa, cob Cobertura, nome, persona, codRef string,
	nomes map[string]string, mes string) ResumoUsuario {

	rs := FiltrarEscopo(todos, persona, codRef)
	r := ResumoUsuario{
		Nome: nome, Persona: persona, Mes: mes, Cobertura: cob,
		TotalMesa:  TotalNaMesa(rs),
		LinkPainel: baseURLFarol() + "/farol/v2",
		LinkQuadro: baseURLFarol() + "/farol/dinheiro-na-mesa",
	}
	r.Vermelho, r.Amarelo, r.Verde = ContarFaixas(rs)
	for _, x := range rs {
		r.Realizado += x.Realizado
		r.Ritmo += x.RitmoEsperado
	}

	var chave func(RcaMesa) string
	var param string
	switch persona {
	case "ggv":
		chave, param, r.Escopo = func(x RcaMesa) string { return x.CodSupervisor }, "supervisor", "sua equipe"
	case "supervisor":
		r.Escopo = "sua equipe"
	default:
		chave, param, r.Escopo = func(x RcaMesa) string { return x.CodGerente }, "gerente", "toda a empresa"
	}

	if chave != nil {
		idx := map[string]*GrupoResumo{}
		var ordem []string
		for _, x := range rs {
			k := chave(x)
			g := idx[k]
			if g == nil {
				nm := nomes[k]
				if nm == "" {
					nm = k
				}
				g = &GrupoResumo{Cod: k, Nome: nm, Link: linkEquipe(param, k)}
				idx[k] = g
				ordem = append(ordem, k)
			}
			g.TotalMesa += x.DinheiroMesa
			g.Rcas++
			if x.Faixa == "R" {
				g.Vermelhos++
			}
			if len(g.PioresRcas) < 3 && x.DinheiroMesa > 0 {
				g.PioresRcas = append(g.PioresRcas, x)
			}
		}
		for _, k := range ordem {
			r.Grupos = append(r.Grupos, *idx[k])
		}
		for i := 1; i < len(r.Grupos); i++ {
			for j := i; j > 0 && r.Grupos[j].TotalMesa > r.Grupos[j-1].TotalMesa; j-- {
				r.Grupos[j], r.Grupos[j-1] = r.Grupos[j-1], r.Grupos[j]
			}
		}
	}

	// Lista nominal curta + agregado do resto. O agregado existe para o gestor
	// saber o tamanho do que NÃO está vendo — sem ele, os cinco viram "o
	// problema todo" e o resto some da cabeça dele.
	for _, x := range rs {
		if x.DinheiroMesa <= 0 {
			continue
		}
		if len(r.TopGeral) < TopN {
			r.TopGeral = append(r.TopGeral, x)
			continue
		}
		r.RestoRcas++
		r.RestoValor += x.DinheiroMesa
	}
	return r
}

// ─── Texto ───────────────────────────────────────────────────────────────────

func brl(v float64) string {
	s := fmt.Sprintf("%.2f", v)
	inteiro, dec, _ := strings.Cut(s, ".")
	neg := strings.HasPrefix(inteiro, "-")
	inteiro = strings.TrimPrefix(inteiro, "-")
	var out []string
	for len(inteiro) > 3 {
		out = append([]string{inteiro[len(inteiro)-3:]}, out...)
		inteiro = inteiro[:len(inteiro)-3]
	}
	out = append([]string{inteiro}, out...)
	sinal := ""
	if neg {
		sinal = "-"
	}
	return "R$ " + sinal + strings.Join(out, ".") + "," + dec
}

var rotuloMotivo = map[string]string{
	"POSITIVACAO": "positivação abaixo da equipe",
	"MIX":         "mix abaixo da equipe",
}

func motivoTexto(m string) string {
	if t, ok := rotuloMotivo[m]; ok {
		return t
	}
	return "volume geral"
}

// CorpoHTML monta o e-mail. Tabela simples e estilo inline: cliente de e-mail
// corporativo ignora <style> no head e não tem flexbox confiável.
func CorpoHTML(r ResumoUsuario) string {
	var b strings.Builder
	esc := html.EscapeString

	fmt.Fprintf(&b, `<div style="font-family:Arial,Helvetica,sans-serif;color:#1a1a1a;max-width:680px">`)
	fmt.Fprintf(&b, `<p style="margin:0 0 4px;font-size:13px;color:#667">Farol de Vendas · resumo semanal</p>`)
	fmt.Fprintf(&b, `<h2 style="margin:0 0 6px;font-size:20px">Dinheiro na mesa · %s</h2>`, esc(r.Mes))
	fmt.Fprintf(&b, `<p style="margin:0 0 18px;color:#556;font-size:14px">%s, este é o quadro de <b>%s</b>.</p>`,
		esc(r.Nome), esc(r.Escopo))

	fmt.Fprintf(&b, `<div style="background:#f6f7f6;border-left:3px solid #1B6660;padding:14px 18px;margin-bottom:18px">
<div style="font-size:26px;font-weight:bold">%s</div>
<div style="font-size:13px;color:#556;margin-top:2px">deixados de faturar em relação ao ritmo, no mês até agora</div>
<div style="font-size:13px;color:#556;margin-top:8px">
<span style="color:#A34A1B;font-weight:bold">%d</span> abaixo de 70%% ·
<span style="color:#8a6a2e;font-weight:bold">%d</span> entre 70%% e 90%% ·
<span style="color:#2C6E49;font-weight:bold">%d</span> no ritmo</div></div>`,
		brl(r.TotalMesa), r.Vermelho, r.Amarelo, r.Verde)

	// A POSIÇÃO DO CONJUNTO, logo abaixo do destaque.
	//
	// Sem ela o e-mail engana por omissão. Em 22/08/2026 o valor na mesa era de
	// R$ 12,88 mi e a empresa estava R$ 17,77 mi ACIMA do ritmo — 119%. Quem
	// lê só o número grande entende "estamos R$ 12,9 milhões atrasados", que é
	// o oposto do que estava acontecendo.
	//
	// Os dois números são verdadeiros e não se contradizem: o da mesa soma
	// apenas quem está atrás, porque vendedor que vai bem não compensa o que
	// vai mal na hora de agir. Mas o gestor precisa dos dois para saber se está
	// corrigindo rota ou buscando margem extra.
	if r.Ritmo > 0 {
		pct := r.Realizado / r.Ritmo * 100
		saldo := r.Realizado - r.Ritmo
		cor, verbo := "#2C6E49", "acima do"
		if saldo < 0 {
			cor, verbo, saldo = "#A34A1B", "abaixo do", -saldo
		}
		fmt.Fprintf(&b, `<p style="margin:-8px 0 20px;font-size:14px;color:#556">
No conjunto, %s está em <b style="color:%s">%.0f%%</b> do ritmo — %s <b>%s</b> do esperado para esta altura do mês.</p>`,
			esc(r.Escopo), cor, pct, verbo, brl(saldo))
	}

	// O código vai junto do nome de propósito. Em 22/08/2026 a prévia mostrou
	// duas linhas idênticas — "GGV - GO - GILSON FLORES" nos códigos 3 e 350 —
	// porque o 350 trocou de dono no WinThor e a origem ainda devolve o nome
	// antigo. Sem o código não dá para saber qual link é qual.
	if len(r.Grupos) > 0 {
		titulo := "Por GGV"
		if r.Persona == "ggv" {
			titulo = "Por supervisor"
		}
		fmt.Fprintf(&b, `<h3 style="font-size:15px;margin:22px 0 8px">%s</h3>`, titulo)
		b.WriteString(`<table cellpadding="0" cellspacing="0" style="width:100%;border-collapse:collapse;font-size:14px">`)
		for _, g := range r.Grupos {
			fmt.Fprintf(&b, `<tr>
<td style="padding:10px 0;border-bottom:1px solid #e3e6e5">
  <a href="%s" style="color:#1B6660;font-weight:bold;text-decoration:none">%s</a>
  <span style="color:#99a;font-size:12px"> · %s</span>
  <div style="color:#667;font-size:12.5px;margin-top:2px">%d RCAs · %d abaixo de 70%%</div>
</td>
<td style="padding:10px 0;border-bottom:1px solid #e3e6e5;text-align:right;white-space:nowrap;font-weight:bold">%s</td>
</tr>`, g.Link, esc(g.Nome), esc(g.Cod), g.Rcas, g.Vermelhos, brl(g.TotalMesa))
		}
		b.WriteString(`</table>`)
	}

	if len(r.TopGeral) > 0 {
		fmt.Fprintf(&b, `<h3 style="font-size:15px;margin:24px 0 8px">Onde está o dinheiro</h3>`)
		fmt.Fprintf(&b, `<p style="margin:0 0 8px;color:#667;font-size:12.5px">Ordenado por reais, não por percentual: o RCA grande a 90%% do ritmo pesa mais que o pequeno a 50%%.</p>`)
		b.WriteString(`<table cellpadding="0" cellspacing="0" style="width:100%;border-collapse:collapse;font-size:14px">`)
		for i, x := range r.TopGeral {
			fmt.Fprintf(&b, `<tr>
<td style="padding:9px 0;border-bottom:1px solid #eef1f0;color:#889;width:22px">%d</td>
<td style="padding:9px 0;border-bottom:1px solid #eef1f0">%s
  <div style="color:#667;font-size:12.5px;margin-top:2px">%s · %.0f%% do ritmo</div></td>
<td style="padding:9px 0;border-bottom:1px solid #eef1f0;text-align:right;white-space:nowrap;font-weight:bold">%s</td>
</tr>`, i+1, esc(x.NomeRca), motivoTexto(x.Motivo), x.Atingimento, brl(x.DinheiroMesa))
		}
		b.WriteString(`</table>`)
		if r.RestoRcas > 0 {
			fmt.Fprintf(&b, `<p style="margin:10px 0 0;color:#667;font-size:13px;border:1px dashed #d8dcdb;padding:10px 14px">
+ <b>%d RCAs</b> abaixo do ritmo, individualmente menores — <b>%s</b> somados.</p>`,
				r.RestoRcas, brl(r.RestoValor))
		}
	}

	// Dois botões, com hierarquia clara. O quadro é o destino natural de quem
	// leu o e-mail e quer o detalhe; o painel é para quem já sabe o que
	// investigar. Invertidos, o gestor cairia na tela genérica e teria que
	// remontar o raciocínio que o e-mail acabou de dar.
	fmt.Fprintf(&b, `<p style="margin:24px 0 0">
<a href="%s" style="background:#1B6660;color:#fff;padding:11px 20px;border-radius:4px;text-decoration:none;font-size:14px;display:inline-block">Ver o quadro completo</a>
<a href="%s" style="color:#1B6660;padding:11px 16px;text-decoration:none;font-size:14px;display:inline-block">Abrir o painel</a></p>`,
		r.LinkQuadro, r.LinkPainel)

	// A metodologia vai no rodapé de propósito: quem age lê o topo, quem
	// questiona o número lê aqui. Esconder a régua é o caminho mais rápido
	// para o gestor desconfiar do sistema inteiro na primeira divergência.
	fmt.Fprintf(&b, `<hr style="border:0;border-top:1px solid #e3e6e5;margin:26px 0 12px">
<p style="color:#889;font-size:12px;line-height:1.6;margin:0">
Ritmo esperado = alvo × (dias úteis decorridos ÷ dias úteis do mês).
Alvo: <b>%s</b>. Dias úteis contados pelo faturamento real (%d de %d, fonte: %s) — considera sábado e feriado sem precisar de calendário.
Valores em venda líquida. %d dos %d RCAs com venda no mês entraram no cálculo; os demais não têm base de comparação.
Este resumo descreve onde está o gap, não avalia desempenho: férias, licença e troca de território não são conhecidos pelo sistema.
</p></div>`,
		esc(r.Cobertura.Baseline.Rotulo()), r.Cobertura.DiasDecorridos, r.Cobertura.DiasTotais,
		esc(r.Cobertura.FonteDiasTotal), r.Cobertura.RcasComMeta, r.Cobertura.RcasComVenda)

	return b.String()
}

// CorpoTexto — alternativa em texto puro. Alguns clientes corporativos
// bloqueiam HTML, e um e-mail vazio some sem ninguém reclamar.
func CorpoTexto(r ResumoUsuario) string {
	var b strings.Builder
	fmt.Fprintf(&b, "FAROL DE VENDAS — RESUMO SEMANAL\nDinheiro na mesa · %s\n\n", r.Mes)
	fmt.Fprintf(&b, "%s, este é o quadro de %s.\n\n", r.Nome, r.Escopo)
	fmt.Fprintf(&b, "%s deixados de faturar em relação ao ritmo.\n", brl(r.TotalMesa))
	fmt.Fprintf(&b, "%d abaixo de 70%% · %d entre 70%% e 90%% · %d no ritmo\n", r.Vermelho, r.Amarelo, r.Verde)
	if r.Ritmo > 0 {
		pct := r.Realizado / r.Ritmo * 100
		saldo := r.Realizado - r.Ritmo
		verbo := "acima do"
		if saldo < 0 {
			verbo, saldo = "abaixo do", -saldo
		}
		fmt.Fprintf(&b, "No conjunto: %.0f%% do ritmo, %s %s esperado.\n", pct, verbo, brl(saldo))
	}

	if len(r.Grupos) > 0 {
		b.WriteString("\n")
		for _, g := range r.Grupos {
			fmt.Fprintf(&b, "%-30s [%s] %14s  (%d RCAs, %d vermelhos)\n  %s\n",
				g.Nome, g.Cod, brl(g.TotalMesa), g.Rcas, g.Vermelhos, g.Link)
		}
	}
	if len(r.TopGeral) > 0 {
		b.WriteString("\nONDE ESTÁ O DINHEIRO\n")
		for i, x := range r.TopGeral {
			fmt.Fprintf(&b, "%d. %-32s %14s  %s, %.0f%% do ritmo\n",
				i+1, x.NomeRca, brl(x.DinheiroMesa), motivoTexto(x.Motivo), x.Atingimento)
		}
		if r.RestoRcas > 0 {
			fmt.Fprintf(&b, "+ %d RCAs menores, %s somados\n", r.RestoRcas, brl(r.RestoValor))
		}
	}
	fmt.Fprintf(&b, "\nQuadro completo: %s\nPainel: %s\n", r.LinkQuadro, r.LinkPainel)
	fmt.Fprintf(&b, "\n--\nRitmo = alvo x (dias úteis decorridos / dias do mês). Alvo: %s.\n"+
		"Dias úteis pelo faturamento real (%d de %d, %s). Venda líquida.\n"+
		"%d dos %d RCAs com venda entraram no cálculo.\n"+
		"Descreve onde está o gap; não avalia desempenho.\n",
		r.Cobertura.Baseline.Rotulo(), r.Cobertura.DiasDecorridos, r.Cobertura.DiasTotais,
		r.Cobertura.FonteDiasTotal, r.Cobertura.RcasComMeta, r.Cobertura.RcasComVenda)
	return b.String()
}

// NomesGerentesSupervisores devolve cod → nome para rotular os grupos.
func NomesGerentesSupervisores(db *sql.DB, empresaID string, ano, mes int) map[string]string {
	out := map[string]string{}
	rows, err := db.Query(`
		SELECT cod_gerente, MAX(nome_gerente) FROM farol.agg_fat_v03_l0_mes
		 WHERE empresa_id=$1 AND ano=$2 AND mes=$3 GROUP BY cod_gerente
		 UNION ALL
		SELECT cod_supervisor, MAX(nome_supervisor) FROM farol.agg_fat_v02_l0_mes
		 WHERE empresa_id=$1 AND ano=$2 AND mes=$3 GROUP BY cod_supervisor`,
		empresaID, ano, mes)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var cod, nome string
		if rows.Scan(&cod, &nome) == nil && cod != "" {
			out[cod] = nome
		}
	}
	return out
}

var mesPtBR = [...]string{"", "janeiro", "fevereiro", "março", "abril", "maio", "junho",
	"julho", "agosto", "setembro", "outubro", "novembro", "dezembro"}

// RotuloMes — "agosto/2026".
func RotuloMes(ano, mes int) string {
	if mes < 1 || mes > 12 {
		return fmt.Sprintf("%02d/%04d", mes, ano)
	}
	return fmt.Sprintf("%s/%d", mesPtBR[mes], ano)
}

// SegundaDaSemana — a segunda-feira da semana de `t`, chave do log de envio.
func SegundaDaSemana(t time.Time) time.Time {
	d := int(t.Weekday())
	if d == 0 {
		d = 7
	}
	t = t.AddDate(0, 0, -(d - 1))
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// ─── Envio ───────────────────────────────────────────────────────────────────

// sendHTMLReport — multipart/alternative com texto puro e HTML.
//
// As duas partes de propósito: cliente corporativo que bloqueia HTML mostra a
// alternativa em texto em vez de um e-mail em branco, e o filtro de spam vê um
// e-mail bem formado em vez de HTML solto.
func sendHTMLReport(to []string, subject, texto, htmlBody string) error {
	if len(to) == 0 {
		return fmt.Errorf("nenhum destinatário informado")
	}
	cfg := GetEmailConfig()
	if cfg.Username == "" || cfg.Password == "" {
		return fmt.Errorf("SMTP não configurado (SMTP_USER/SMTP_PASSWORD ausentes)")
	}
	b := "fb-farol-" + fmt.Sprint(time.Now().UnixNano())
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: =?UTF-8?B?%s?=\r\n"+
		"MIME-Version: 1.0\r\nContent-Type: multipart/alternative; boundary=%q\r\n\r\n"+
		"--%s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s\r\n"+
		"--%s\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n%s\r\n--%s--\r\n",
		cfg.From, strings.Join(to, ", "), b64Assunto(subject), b, b, texto, b, htmlBody, b)
	return sendMailSSL(cfg, to, []byte(msg))
}

// ResultadoEnvio — uma linha do relatório do disparo.
type ResultadoEnvio struct {
	Email     string  `json:"email"`
	Nome      string  `json:"nome"`
	Persona   string  `json:"persona"`
	Rcas      int     `json:"rcas"`
	TotalMesa float64 `json:"total_mesa"`
	Enviado   bool    `json:"enviado"`
	Pulado    string  `json:"pulado,omitempty"`
	Erro      string  `json:"erro,omitempty"`
	HTML      string  `json:"html,omitempty"` // só no modo prévia
}

// EnviarResumoSemanal monta e envia para todos os usuários com a flag ligada.
//
// `previa=true` NÃO envia e NÃO grava log: devolve o HTML para conferência.
// Existe porque a primeira versão de um e-mail que vai para diretoria não deve
// estrear na caixa da diretoria.
//
// `forcar=true` ignora o log da semana. Sem ele, rodar duas vezes na segunda
// não manda dois e-mails — que é o comportamento certo do worker e o errado
// para quem está testando.
func EnviarResumoSemanal(db *sql.DB, empresaID string, ano, mes int, ate time.Time,
	base Baseline, previa, forcar bool) ([]ResultadoEnvio, error) {

	todos, cob, err := ColetarDinheiroNaMesa(db, empresaID, ano, mes, ate, base)
	if err != nil {
		return nil, err
	}
	nomes := NomesGerentesSupervisores(db, empresaID, ano, mes)
	rotulo := RotuloMes(ano, mes)
	semana := SegundaDaSemana(time.Now())

	rows, err := db.Query(`
		SELECT id, email, COALESCE(NULLIF(full_name,''), email),
		       COALESCE(tipo_persona,''), COALESCE(cod_referencia,'')
		  FROM users
		 WHERE farol_resumo_semanal = TRUE
		 ORDER BY tipo_persona, cod_referencia, email`)
	if err != nil {
		return nil, fmt.Errorf("listar destinatários: %w", err)
	}
	defer rows.Close()

	type dest struct{ id, email, nome, persona, codRef string }
	var lista []dest
	for rows.Next() {
		var d dest
		if rows.Scan(&d.id, &d.email, &d.nome, &d.persona, &d.codRef) == nil {
			lista = append(lista, d)
		}
	}

	out := make([]ResultadoEnvio, 0, len(lista))
	for _, d := range lista {
		res := ResultadoEnvio{Email: d.email, Nome: d.nome, Persona: d.persona}

		if !previa && !forcar {
			var existe bool
			_ = db.QueryRow(`SELECT EXISTS(SELECT 1 FROM farol.resumo_semanal_log
			                                WHERE user_id=$1 AND semana=$2)`,
				d.id, semana).Scan(&existe)
			if existe {
				res.Pulado = "já enviado nesta semana"
				out = append(out, res)
				continue
			}
		}

		r := MontarResumo(todos, cob, d.nome, d.persona, d.codRef, nomes, rotulo)
		res.Rcas = len(r.TopGeral) + r.RestoRcas
		res.TotalMesa = r.TotalMesa

		// Escopo vazio não vira e-mail em branco: um resumo sem nada dentro
		// ensina o gestor a ignorar o remetente.
		if r.TotalMesa <= 0 && len(r.TopGeral) == 0 {
			res.Pulado = "nada a reportar no escopo"
			out = append(out, res)
			continue
		}

		htmlBody := CorpoHTML(r)
		if previa {
			res.HTML = htmlBody
			out = append(out, res)
			continue
		}

		assunto := fmt.Sprintf("[FAROL] Dinheiro na mesa · %s · %s", rotulo, brl(r.TotalMesa))
		if err := sendHTMLReport([]string{d.email}, assunto, CorpoTexto(r), htmlBody); err != nil {
			res.Erro = err.Error()
		} else {
			res.Enviado = true
		}

		// Log inclusive do erro: "não recebi" é a primeira frase, e sem
		// registro não dá para saber se falhou aqui ou no servidor deles.
		_, _ = db.Exec(`
			INSERT INTO farol.resumo_semanal_log
			    (empresa_id, user_id, semana, destinatario, rcas, total_mesa, baseline, erro)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
			ON CONFLICT (user_id, semana) DO UPDATE SET
			    enviado_em = NOW(), rcas = EXCLUDED.rcas,
			    total_mesa = EXCLUDED.total_mesa, erro = EXCLUDED.erro`,
			empresaID, d.id, semana, d.email, res.Rcas, r.TotalMesa, string(base), res.Erro)

		out = append(out, res)
	}
	return out, nil
}

// b64Assunto — RFC 2047. Sem isso, acento no assunto vira lixo em vários clientes.
func b64Assunto(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}

// ─── Worker ──────────────────────────────────────────────────────────────────

// StartResumoSemanalFarol — dispara o resumo na segunda de manhã.
//
// Tick de hora em hora em vez de dormir até a próxima segunda: se o container
// reiniciar às 7h30 de uma segunda, um sleep longo perderia a semana inteira em
// silêncio. Com o tick, o próximo acorda às 8h e manda.
//
// A idempotência não depende do horário e sim do UNIQUE (user_id, semana) do
// log: rodar cinco vezes na mesma segunda envia uma vez só.
//
// Janela das 7h às 9h (BRT): antes disso a carga diária das 04:30 ainda pode
// estar consolidando agregados — o resumo sairia sobre número parcial.
func StartResumoSemanalFarol(getDB func() *sql.DB) {
	empresaID := strings.TrimSpace(os.Getenv("JC_EMPRESA_ID"))
	if empresaID == "" {
		log.Printf("[farol:resumo] worker desativado — JC_EMPRESA_ID ausente")
		return
	}
	go func() {
		log.Printf("[farol:resumo] worker ativo — segunda-feira entre 07h e 09h (America/Sao_Paulo)")
		time.Sleep(90 * time.Second) // deixa migrations e prewarm respirarem
		t := time.NewTicker(1 * time.Hour)
		defer t.Stop()

		rodar := func() {
			db := getDB()
			if db == nil {
				return
			}
			loc := tzBrasilResumo()
			agora := time.Now().In(loc)
			if agora.Weekday() != time.Monday || agora.Hour() < 7 || agora.Hour() >= 9 {
				return
			}
			// Ontem = domingo. O mês corrente é o que interessa; na virada de
			// mês a segunda cai no mês novo com pouquíssimo dado, e o resumo
			// sai pequeno em vez de errado.
			ate := time.Date(agora.Year(), agora.Month(), agora.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -1)
			res, err := EnviarResumoSemanal(db, empresaID, agora.Year(), int(agora.Month()),
				ate, BaselineAnoAnterior, false, false)
			if err != nil {
				log.Printf("[farol:resumo] worker FALHOU: %v", err)
				return
			}
			env, pul := 0, 0
			for _, x := range res {
				if x.Enviado {
					env++
				} else if x.Pulado != "" {
					pul++
				}
			}
			if env > 0 || pul < len(res) {
				log.Printf("[farol:resumo] worker: %d destinatário(s), %d enviado(s), %d pulado(s)",
					len(res), env, pul)
			}
		}

		rodar()
		for range t.C {
			rodar()
		}
	}()
}

func tzBrasilResumo() *time.Location {
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		return time.FixedZone("BRT", -3*3600)
	}
	return loc
}

// EnviarResumoTeste manda UMA cópia para um endereço avulso, com o recorte de
// quem pediu. O corpo é idêntico ao do envio real — sem tarja de teste — porque
// o uso previsto é conferir e reencaminhar, e uma tarja obrigaria a explicar o
// que ela significa.
//
// NÃO grava em resumo_semanal_log: aquele log é a prova do envio semanal, e uma
// cópia de teste ali faria o worker pular o envio de verdade na segunda.
//
// A rota só aceita persona sem escopo, e o disparo fica no log da aplicação com
// quem pediu e para onde foi — mandar o quadro da empresa para endereço externo
// é exatamente o tipo de ação que precisa deixar rastro.
func EnviarResumoTeste(db *sql.DB, empresaID string, ano, mes int, ate time.Time,
	base Baseline, nome, persona, codRef, para string) (ResumoUsuario, error) {

	todos, cob, err := ColetarDinheiroNaMesa(db, empresaID, ano, mes, ate, base)
	if err != nil {
		return ResumoUsuario{}, err
	}
	nomes := NomesGerentesSupervisores(db, empresaID, ano, mes)
	r := MontarResumo(todos, cob, nome, persona, codRef, nomes, RotuloMes(ano, mes))

	assunto := fmt.Sprintf("[FAROL] Dinheiro na mesa · %s · %s", r.Mes, brl(r.TotalMesa))
	return r, sendHTMLReport([]string{para}, assunto, CorpoTexto(r), CorpoHTML(r))
}
