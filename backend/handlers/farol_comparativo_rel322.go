package handlers

// farol_comparativo_rel322.go — Comparativo REL 322 (WinThor) x Farol.
//
// POR QUE ELE EXISTE. O gestor precisa validar se o Farol bate com o
// relatório oficial do WinThor ("322 — Venda Por Departamento", por
// supervisor), mas de onde a comparação é feita não se alcança nem o Oracle
// de origem (BD VM) nem o Postgres de produção do Farol — só o PDF que o
// WinThor já exporta está disponível. O usuário sobe esse PDF; o backend
// extrai as linhas por supervisor, lê o período do próprio cabeçalho e busca
// no Farol o Bruto e o Líquido para os mesmos cod_supervisor no mesmo
// intervalo. Nada é persistido — cada upload é independente.
//
// LAYOUT DO TEXTO EXTRAÍDO. github.com/ledongthuc/pdf.GetPlainText devolve
// um "token" por linha (uma palavra ou um número por linha, na ordem em que
// foram desenhados no PDF) — não uma linha de texto por linha visual da
// tabela. Cada bloco de cabeçalho (repetido a cada página) começa na linha
// "Código" e termina na linha "Todos os Períodos". Cada linha de dado é:
// código (dígitos) + descrição (um ou mais tokens não-numéricos, com espaços
// e hífens) + exatamente 11 tokens numéricos, dos quais o PRIMEIRO é o
// Vl.Vendido — confirmado batendo a soma de todas as linhas contra o valor
// declarado após "N Supervisores Listados" nos 4 PDFs de exemplo em
// /home/claudio/uploads/*.pdf (bruto: 129.957.080,67 / 1.886.956.234,67 /
// 1.269.150.898,17 / 154.725.962,23 — todos batem exatos).

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/johnfercher/maroto/v2"
	"github.com/johnfercher/maroto/v2/pkg/components/col"
	"github.com/johnfercher/maroto/v2/pkg/components/image"
	"github.com/johnfercher/maroto/v2/pkg/components/page"
	"github.com/johnfercher/maroto/v2/pkg/components/row"
	"github.com/johnfercher/maroto/v2/pkg/components/text"
	"github.com/johnfercher/maroto/v2/pkg/config"
	"github.com/johnfercher/maroto/v2/pkg/consts/align"
	"github.com/johnfercher/maroto/v2/pkg/consts/extension"
	"github.com/johnfercher/maroto/v2/pkg/consts/fontstyle"
	"github.com/johnfercher/maroto/v2/pkg/consts/orientation"
	"github.com/johnfercher/maroto/v2/pkg/props"
	"github.com/ledongthuc/pdf"
	"github.com/lib/pq"
)

// ─── Tipos ──────────────────────────────────────────────────────────────────

// linhaExtraidaRel322 — uma linha de supervisor lida do PDF, antes de cruzar
// com o Farol.
type linhaExtraidaRel322 struct {
	CodSupervisor string
	Descricao     string
	VlVendido     float64
}

// rel322Parsed — resultado da extração do PDF: período + linhas de supervisor.
type rel322Parsed struct {
	PeriodoTexto string // "01/08/2026 a 26/08/2026", como impresso no PDF
	DataInicio   time.Time
	DataFim      time.Time
	// Filiais — códigos do cabeçalho "Filiai(s) :" (ex.: ["10","11","13"]).
	// Vazio = PDF rodado para "Todas as Filiais" (ou o campo não foi
	// encontrado no cabeçalho) — nenhum filtro de filial é aplicado nesse
	// caso. O comparativo tem que recortar pela MESMA filial do relatório:
	// sem isso, um REL 322 gerado só pra um subconjunto de filiais seria
	// comparado contra o Farol da empresa INTEIRA, divergindo de forma
	// enganosa (achado do Blind Hunter em spec-comparativo-rel322.md).
	Filiais []string
	Linhas  []linhaExtraidaRel322
}

// linhaComparativoRel322 — uma linha da resposta da API, já cruzada com o
// Farol e (quando disponível) com a base de origem (VM). Ponteiros
// distinguem "0 apurado" de "não existe deste lado". Só Líquido — Bruto foi
// removido da tela (decisão do Claudio em 28/08/2026): o gestor trabalha só
// com Líquido, que é o número que efetivamente importa pra conferência.
type linhaComparativoRel322 struct {
	CodSupervisor string   `json:"cod_supervisor"`
	Supervisor    string   `json:"supervisor"`
	VlVendidoPDF  *float64 `json:"vl_vendido_pdf"`
	LiquidoFarol  *float64 `json:"liquido_farol"`
	// LiquidoVM — Líquido consultado AO VIVO na base Oracle de origem (a
	// mesma que o JC lê todo dia, ver farol_comparativo_rel322_vm.go). nil
	// quando a VM está indisponível (ver VMIndisponivel na resposta) OU essa
	// linha simplesmente não apareceu na consulta à VM.
	LiquidoVM *float64 `json:"liquido_vm"`
	// Três diferenças percentuais — cada uma isola uma causa possível de
	// divergência: 322×VM aponta se o PRÓPRIO relatório do WinThor já diverge
	// da base de origem; 322×Farol é a comparação histórica (WinThor x
	// Farol); VM×Farol isola perdas/erros especificamente na importação pro
	// Farol (VM é a origem; Farol é o que chegou depois do import).
	DiferencaPDFxVMPct    *float64 `json:"diferenca_pdf_vm_pct"`
	DiferencaPDFxFarolPct *float64 `json:"diferenca_pdf_farol_pct"`
	DiferencaVMxFarolPct  *float64 `json:"diferenca_vm_farol_pct"`
	// Status — sempre decidido por PDF×Farol (tolerância 0,5%), nunca pela
	// VM: a VM é diagnóstico adicional, não substitui o Farol como a base
	// contra a qual o gestor decide "bateu ou não bateu".
	Status string `json:"status"` // ok | divergencia | orfao
	Origem string `json:"origem"` // pdf | farol | ambos
}

// comparativoRel322Resposta — payload devolvido ao front.
type comparativoRel322Resposta struct {
	Periodo    string `json:"periodo"`
	DataInicio string `json:"data_inicio"`
	DataFim    string `json:"data_fim"`
	// Fluxo — qual base foi comparada ("faturado" ou "transmitido"). O REL 322
	// do WinThor não se autodeclara qual base foi usada pra gerá-lo (o PDF é
	// idêntico nos dois casos) — é sempre escolha explícita do usuário no
	// upload, nunca inferida do conteúdo do PDF. Ver spec-comparativo-rel322-fluxo.md.
	Fluxo string `json:"fluxo"`
	// Filiais — códigos do cabeçalho "Filiai(s) :" do PDF, aplicados como
	// filtro na consulta ao Farol e à VM. Vazio = todas as filiais (PDF sem
	// recorte, ou o campo não foi encontrado no cabeçalho).
	Filiais               []string                 `json:"filiais"`
	Linhas                []linhaComparativoRel322 `json:"linhas"`
	TotalVlVendidoPDF     float64                  `json:"total_vl_vendido_pdf"`
	TotalLiquidoFarol     float64                  `json:"total_liquido_farol"`
	TotalLiquidoVM        *float64                 `json:"total_liquido_vm"`
	QtdSupervisoresPDF    int                      `json:"qtd_supervisores_pdf"`
	QtdDivergencias       int                      `json:"qtd_divergencias"`
	QtdOrfaos             int                      `json:"qtd_orfaos"`
	SemDadoFarolNoPeriodo bool                     `json:"sem_dado_farol_no_periodo"`
	// VMIndisponivel — a consulta à base de origem falhou (Oracle
	// inalcançável, credenciais ausentes, timeout). O comparativo PDF×Farol
	// continua valendo — a VM é só um diagnóstico a mais, nunca bloqueia o
	// resultado principal (mesmo princípio de resiliência do projeto: "se
	// algum indicador secundário falhar, isso ainda precisa funcionar").
	VMIndisponivel bool   `json:"vm_indisponivel"`
	VMErro         string `json:"vm_erro,omitempty"`
}

// tipoVendaReal — códigos que entram na venda real (mesma classificação do
// painel faturado — ver spec-venda-liquida-composicao.md e a migration
// 187_tipo_venda_faturado.sql). Bonificação(5)/Transferência(10)/Remessa(13)
// ficam de fora.
var tipoVendaReal = []string{"1", "4", "7", "8", "9", "11", "14", "20"}

// ─── Extração de texto do PDF ─────────────────────────────────────────────

// extrairTextoPDF — github.com/ledongthuc/pdf é lib de terceiros com
// histórico de panic em entrada malformada (não só error). Sem o recover,
// um PDF corrompido derrubaria a goroutine da requisição inteira em vez de
// virar um 422 normal — o defer converte qualquer panic em erro comum,
// mantendo a mesma mensagem que o handler já usa para esse caminho.
func extrairTextoPDF(dados []byte) (texto string, err error) {
	defer func() {
		if r := recover(); r != nil {
			texto = ""
			err = fmt.Errorf("falha ao processar PDF (arquivo corrompido?): %v", r)
		}
	}()

	reader, err := pdf.NewReader(bytes.NewReader(dados), int64(len(dados)))
	if err != nil {
		return "", fmt.Errorf("arquivo não é um PDF válido: %w", err)
	}
	txt, err := reader.GetPlainText()
	if err != nil {
		return "", fmt.Errorf("falha ao extrair texto do PDF (PDF sem texto real / escaneado?): %w", err)
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, txt); err != nil {
		return "", fmt.Errorf("falha ao ler texto extraído: %w", err)
	}
	return buf.String(), nil
}

// ─── Parser do REL 322 ─────────────────────────────────────────────────────
//
// Como a descrição do supervisor tem espaços e hífens
// ("GO - VALE SAO PATRICIO - LUCAS"), não dá para dividir por espaço.
// Percorre-se a sequência de tokens: um código (linha só de dígitos) inicia
// a linha; tudo que não é numérico até o próximo bloco de 11 números é
// descrição; os 11 números seguintes são Qt.Cli.Ativos, Qt.Cli.Pos., etc, na
// ordem em que o PDF os desenha — o primeiro deles é sempre o Vl.Vendido
// (confirmado empiricamente, ver comentário no topo do arquivo).

var (
	reNumTokenRel322 = regexp.MustCompile(`^-?\d[\d.]*(,\d+)?$`)
	rePaginaRel322   = regexp.MustCompile(`^Pagina\s*:\s*\d+$`)
	reCodigoRel322   = regexp.MustCompile(`^\d+$`)
	rePeriodoRel322  = regexp.MustCompile(`^(\d{2}/\d{2}/\d{4})\s+a\s+(\d{2}/\d{2}/\d{4})$`)
)

// parseRel322Texto — recebe o texto já extraído do PDF (uma "palavra"/número
// por linha) e devolve o período + as linhas de supervisor. Erro claro
// sempre que o layout não bate com o esperado — nunca um comparativo parcial
// silencioso (constraint do spec).
func parseRel322Texto(texto string) (*rel322Parsed, error) {
	var lines []string
	for _, l := range strings.Split(texto, "\n") {
		l = strings.TrimSpace(l)
		if l != "" {
			lines = append(lines, l)
		}
	}

	achouTitulo := false
	achouTipo := false
	for _, l := range lines {
		if l == "322 - Venda Por Departamento" {
			achouTitulo = true
		}
		if l == "14-Por Supervisor" {
			achouTipo = true
		}
	}
	if !achouTitulo {
		return nil, fmt.Errorf("layout não reconhecido: o PDF não é o relatório \"322 - Venda Por Departamento\"")
	}
	if !achouTipo {
		return nil, fmt.Errorf("layout não reconhecido: o PDF não está no formato \"14-Por Supervisor\"")
	}

	// Remove os blocos de cabeçalho (repetidos a cada página, de "Código" até
	// "Todos os Períodos"), capturando o valor de "Periodo :" e "Filiai(s) :"
	// pelo caminho.
	periodoTexto := ""
	filiaisTexto := ""
	var out []string
	i := 0
	for i < len(lines) {
		if lines[i] == "Código" {
			j := i
			fechou := false
			for j < len(lines) {
				if lines[j] == "Periodo :" && j+1 < len(lines) && periodoTexto == "" {
					periodoTexto = lines[j+1]
				}
				if lines[j] == "Filiai(s) :" && j+1 < len(lines) && filiaisTexto == "" {
					filiaisTexto = lines[j+1]
				}
				if lines[j] == "Todos os Períodos" {
					fechou = true
					break
				}
				j++
			}
			if !fechou {
				return nil, fmt.Errorf("layout não reconhecido: cabeçalho do PDF incompleto")
			}
			i = j + 1
			continue
		}
		if rePaginaRel322.MatchString(lines[i]) {
			i++
			continue
		}
		out = append(out, lines[i])
		i++
	}

	if periodoTexto == "" {
		return nil, fmt.Errorf(`layout não reconhecido: não encontrei a linha "Periodo :" no cabeçalho`)
	}
	m := rePeriodoRel322.FindStringSubmatch(periodoTexto)
	if m == nil {
		return nil, fmt.Errorf("período %q em formato inesperado (esperado DD/MM/AAAA a DD/MM/AAAA)", periodoTexto)
	}
	dataInicio, err1 := time.Parse("02/01/2006", m[1])
	dataFim, err2 := time.Parse("02/01/2006", m[2])
	if err1 != nil || err2 != nil {
		return nil, fmt.Errorf("período %q com datas inválidas", periodoTexto)
	}

	// Percorre as linhas de dado até "Supervisores Listados" (início do
	// bloco de totais, que não é uma linha de supervisor).
	var linhas []linhaExtraidaRel322
	vistos := map[string]bool{}
	i = 0
	n := len(out)
	for i < n {
		l := out[i]
		if l == "Supervisores Listados" {
			break
		}
		if !reCodigoRel322.MatchString(l) {
			return nil, fmt.Errorf("layout não reconhecido: esperava um código de supervisor e encontrei %q", l)
		}
		codigo := l
		if vistos[codigo] {
			// PDF corrompido/editado: o mesmo supervisor apareceria duas vezes
			// e o Farol dele seria somado em dobro nos totais, silenciosamente.
			return nil, fmt.Errorf("layout não reconhecido: código de supervisor %s aparece duplicado no PDF", codigo)
		}
		vistos[codigo] = true
		i++

		var desc []string
		for i < n && !reNumTokenRel322.MatchString(out[i]) {
			desc = append(desc, out[i])
			i++
		}

		var nums []string
		for i < n && reNumTokenRel322.MatchString(out[i]) && len(nums) < 11 {
			nums = append(nums, out[i])
			i++
		}
		if len(nums) != 11 {
			return nil, fmt.Errorf("layout não reconhecido: supervisor %s tem %d valor(es) numérico(s), esperava 11", codigo, len(nums))
		}

		vlVendido, err := parseNumeroBR(nums[0])
		if err != nil {
			return nil, fmt.Errorf("layout não reconhecido: Vl.Vendido inválido (%q) para o supervisor %s", nums[0], codigo)
		}

		linhas = append(linhas, linhaExtraidaRel322{
			CodSupervisor: codigo,
			Descricao:     strings.TrimSpace(strings.Join(desc, " ")),
			VlVendido:     vlVendido,
		})
	}

	if len(linhas) == 0 {
		return nil, fmt.Errorf("layout não reconhecido: nenhuma linha de supervisor encontrada no PDF")
	}

	return &rel322Parsed{
		PeriodoTexto: periodoTexto,
		DataInicio:   dataInicio,
		DataFim:      dataFim,
		Filiais:      parseFiliaisRel322(filiaisTexto),
		Linhas:       linhas,
	}, nil
}

// parseFiliaisRel322 — "10,11,13" → ["10","11","13"]. "Todas as Filiais" (ou
// o campo ausente do cabeçalho, filiaisTexto=="") devolve nil — sem filtro,
// mesmo comportamento de "Todos os Supervisores"/"Todos os Departamentos"
// nos outros campos deste cabeçalho.
func parseFiliaisRel322(filiaisTexto string) []string {
	ft := strings.TrimSpace(filiaisTexto)
	if ft == "" || strings.HasPrefix(strings.ToUpper(ft), "TODAS") || strings.HasPrefix(strings.ToUpper(ft), "TODOS") {
		return nil
	}
	var filiais []string
	for _, p := range strings.Split(ft, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			filiais = append(filiais, p)
		}
	}
	return filiais
}

// parseNumeroBR — "9.223.911,54" → 9223911.54 (separador de milhar '.',
// decimal ',', como todo número do WinThor).
func parseNumeroBR(s string) (float64, error) {
	s = strings.TrimSpace(s)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	s = strings.ReplaceAll(s, ".", "")
	s = strings.ReplaceAll(s, ",", ".")
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	if neg {
		v = -v
	}
	return v, nil
}

// ─── Cruzamento com o Farol ─────────────────────────────────────────────────

// escopoCondRel322 — monta a cláusula SQL que aplica o recorte por persona
// (farol_escopo.go) às duas CTEs de farolBrutoLiquidoPorSupervisor. Extraída
// à parte para ser testável sem *sql.DB.
//
// REGRA DE OURO (mesma de farol_escopo.go): o escopo SOBRESCREVE. Se por
// algum motivo esta função for chamada com Negar=true — não deveria, o
// handler já bloqueia antes com 403 — devolve uma condição impossível
// (1=0) em vez de nenhum filtro: falha fechado, nunca aberto.
func escopoCondRel322(escopo escopoRecorte, args []any) (string, []any) {
	if !escopo.restrito() {
		return "", args
	}
	if escopo.Negar {
		return " AND 1=0", args
	}
	args = append(args, pq.Array(escopo.Vals))
	return fmt.Sprintf(" AND %s = ANY($%d)", escopo.Col, len(args)), args
}

// filialCondRel322 — filtro por filial (coluna `empresa`, presente em
// vendas_faturadas/vendas_transmitidas/vendas_ccd) a partir do cabeçalho
// "Filiai(s) :" do PDF. Sem isso um REL 322 rodado só pra um subconjunto de
// filiais compararia contra o Farol da empresa INTEIRA — divergência
// enganosa, achado do Blind Hunter em spec-comparativo-rel322.md. Vazio (PDF
// pra "Todas as Filiais") não filtra.
func filialCondRel322(filiais []string, args []any) (string, []any) {
	if len(filiais) == 0 {
		return "", args
	}
	args = append(args, pq.Array(filiais))
	return fmt.Sprintf(" AND empresa = ANY($%d)", len(args)), args
}

// tipoVendaSelecionado — códigos vindos do filtro "Tipo de Venda" da tela
// (multi-seleção, ?tipo_venda=1,4,7). Vazio (usuário não mexeu no filtro)
// cai no default de cada fluxo: Faturado usa tipoVendaReal (a definição de
// "venda real" já em produção, inalterada); Transmitido fica sem filtro —
// nil, soma incondicional — preservando o comportamento atual de quem não
// toca no filtro (Transmitido nunca teve o conceito de venda_real).
func tipoVendaSelecionado(selecao []string, fluxo fluxoCtx) []string {
	if len(selecao) > 0 {
		return selecao
	}
	if fluxo.name == "transmitido" {
		return nil
	}
	return tipoVendaReal
}

// somaComTipoVendaRel322 — expressão SQL pra somar `pvenda`, opcionalmente
// recortada pelo filtro "Tipo de Venda" da tela. tiposVenda vazio = soma
// incondicional (nenhum filtro pedido) — é o caminho do Transmitido quando
// o usuário não seleciona nada.
func somaComTipoVendaRel322(tiposVenda []string, args []any) (string, []any) {
	if len(tiposVenda) == 0 {
		return "SUM(pvenda)", args
	}
	args = append(args, pq.Array(tiposVenda))
	return fmt.Sprintf("SUM(pvenda) FILTER (WHERE tipo_venda = ANY($%d))", len(args)), args
}

// parseListaCSVRel322 — "1, 4 ,7" → ["1","4","7"]. Usada tanto pro filtro de
// Tipo de Venda (?tipo_venda=) quanto em qualquer outro parâmetro CSV futuro
// desta rota. Vazio ou só espaços devolve nil.
func parseListaCSVRel322(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// farolLiquidoPorSupervisor — Líquido por cod_supervisor no range de datas,
// já recortado pelo escopo do usuário logado (ggv/supervisor/rca só veem a
// própria equipe — mesma regra de farol_relatorios.go, ver farol_escopo.go)
// e pela(s) filial(is) declarada(s) no cabeçalho do PDF. Só Líquido — Bruto
// foi removido da tela (decisão do Claudio em 28/08/2026).
//
// fluxo decide a fonte: Faturado lê vendas_faturadas e calcula Líquido como
// venda real menos DEVOLVIDO/CANCELADO de vendas_ccd (inalterado).
// Transmitido lê vendas_transmitidas e calcula Líquido como Bruto menos
// CORTADO de vendas_ccd — sem filtro de tipo_venda, porque Transmitido nunca
// teve o conceito de venda_real (renegociado em 2026-08-27, ver
// spec-comparativo-rel322-liquido-transmitido.md). O REL 322 do WinThor não
// se autodeclara qual base gerou o PDF, então fluxo vem sempre de escolha
// explícita do usuário (nunca inferido).
//
// Consulta as tabelas DIÁRIAS, não os agregados mensais: o período do PDF
// pode ser parcial (mês em andamento). tiposVenda vem de
// tipoVendaSelecionado — já resolvido pro default de cada fluxo quando o
// usuário não mexe no filtro "Tipo de Venda" da tela.
func farolLiquidoPorSupervisor(ctx context.Context, db *sql.DB, empresaID string, dataInicio, dataFim time.Time, filiais, tiposVenda []string, escopo escopoRecorte, fluxo fluxoCtx) (map[string]float64, error) {
	if fluxo.name == "transmitido" {
		return liquidoPorSupervisorTransmitido(ctx, db, empresaID, dataInicio, dataFim, filiais, tiposVenda, escopo, fluxo)
	}
	return liquidoPorSupervisorFaturado(ctx, db, empresaID, dataInicio, dataFim, filiais, tiposVenda, escopo, fluxo)
}

// liquidoPorSupervisorTransmitido — Líquido = Bruto (SUM(pvenda) de
// vendas_transmitidas, opcionalmente recortado pelo filtro "Tipo de Venda"
// da tela — sem seleção, soma incondicional, pois Transmitido nunca teve o
// conceito de venda_real) − Cortado (evento CORTADO de vendas_ccd, via
// resolveFluxo("cortado")). Pode dar negativo (Cortado > Bruto) sem clamp
// nem erro — mesma paridade não-protegida que o Faturado já tem pro próprio
// Líquido.
func liquidoPorSupervisorTransmitido(ctx context.Context, db *sql.DB, empresaID string, dataInicio, dataFim time.Time, filiais, tiposVenda []string, escopo escopoRecorte, fluxo fluxoCtx) (map[string]float64, error) {
	args := []any{empresaID, dataInicio, dataFim}
	filialCond, args := filialCondRel322(filiais, args)
	escopoCond, args := escopoCondRel322(escopo, args)
	somaTotal, args := somaComTipoVendaRel322(tiposVenda, args)

	// Evento CORTADO em vendas_ccd do lado transmitido. resolveFluxo não é
	// reimplementado — só reaproveitado pelo eventoFilter que ele já monta
	// (assume a tabela aliada `v`, ver comentário na CTE ccd abaixo).
	cortado := resolveFluxo("cortado")

	q := fmt.Sprintf(`
WITH trans AS (
    SELECT trim(cod_supervisor) AS cod_supervisor,
           %s AS total
      FROM %s
     WHERE empresa_id = $1 AND %s BETWEEN $2 AND $3%s%s
     GROUP BY trim(cod_supervisor)
), ccd AS (
    SELECT trim(v.cod_supervisor) AS cod_supervisor,
           SUM(v.pvenda) AS cortado
      FROM vendas_ccd v
     WHERE v.empresa_id = $1 AND v.data_evento BETWEEN $2 AND $3
       %s%s%s
     GROUP BY trim(v.cod_supervisor)
)
SELECT COALESCE(trans.cod_supervisor, ccd.cod_supervisor) AS cod_supervisor,
       (COALESCE(trans.total, 0) - COALESCE(ccd.cortado, 0))::float8 AS liquido
  FROM trans
  FULL OUTER JOIN ccd ON ccd.cod_supervisor = trans.cod_supervisor`,
		somaTotal, fluxo.tableName, fluxo.dateCol, filialCond, escopoCond,
		cortado.eventoFilter, filialCond, escopoCond)

	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]float64{}
	for rows.Next() {
		var cod string
		var liquido float64
		if err := rows.Scan(&cod, &liquido); err != nil {
			return nil, err
		}
		out[cod] = liquido
	}
	return out, rows.Err()
}

// liquidoPorSupervisorFaturado — Líquido = venda real (tipo_venda no filtro
// "Tipo de Venda" da tela — default tipoVendaReal quando o usuário não
// mexe) menos DEVOLVIDO/CANCELADO de vendas_ccd — mesma classificação do
// painel faturado (spec-venda-liquida-composicao.md) quando o filtro fica
// no default.
func liquidoPorSupervisorFaturado(ctx context.Context, db *sql.DB, empresaID string, dataInicio, dataFim time.Time, filiais, tiposVenda []string, escopo escopoRecorte, fluxo fluxoCtx) (map[string]float64, error) {
	args := []any{empresaID, dataInicio, dataFim}
	filialCond, args := filialCondRel322(filiais, args)
	escopoCond, args := escopoCondRel322(escopo, args)
	somaVendaReal, args := somaComTipoVendaRel322(tiposVenda, args)

	// Evento negativo em vendas_ccd do lado faturado. resolveFluxo não é
	// reimplementado — só reaproveitado pelo eventoFilter que ele já monta
	// (assume a tabela aliada `v`, ver comentário na CTE ccd abaixo).
	negativo := resolveFluxo("cancdev")

	q := fmt.Sprintf(`
WITH fat AS (
    SELECT trim(cod_supervisor) AS cod_supervisor,
           %s AS venda_real
      FROM %s
     WHERE empresa_id = $1 AND %s BETWEEN $2 AND $3%s%s
     GROUP BY trim(cod_supervisor)
), ccd AS (
    SELECT trim(v.cod_supervisor) AS cod_supervisor,
           SUM(v.pvenda) AS descontos
      FROM vendas_ccd v
     WHERE v.empresa_id = $1 AND v.data_evento BETWEEN $2 AND $3
       %s%s%s
     GROUP BY trim(v.cod_supervisor)
)
SELECT COALESCE(fat.cod_supervisor, ccd.cod_supervisor) AS cod_supervisor,
       (COALESCE(fat.venda_real, 0) - COALESCE(ccd.descontos, 0))::float8 AS liquido
  FROM fat
  FULL OUTER JOIN ccd ON ccd.cod_supervisor = fat.cod_supervisor`,
		somaVendaReal, fluxo.tableName, fluxo.dateCol, filialCond, escopoCond,
		negativo.eventoFilter, filialCond, escopoCond)

	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]float64{}
	for rows.Next() {
		var cod string
		var liquido float64
		if err := rows.Scan(&cod, &liquido); err != nil {
			return nil, err
		}
		out[cod] = liquido
	}
	return out, rows.Err()
}

// montarComparativo — busca o Farol e a VM (já recortados pelo escopo do
// usuário, pelo fluxo escolhido no upload, pela(s) filial(is) do cabeçalho
// do PDF e pelo filtro "Tipo de Venda" da tela) e cruza os três com as
// linhas do PDF. Fina: toda a regra de status/diferenças vive em
// cruzarComRel322 (pura, testável sem banco/Oracle).
//
// A falha da VM (Oracle inalcançável, credenciais ausentes, timeout) NUNCA
// aborta o comparativo — só marca VMIndisponivel na resposta. PDF×Farol é o
// resultado principal; a VM é diagnóstico a mais.
func montarComparativo(ctx context.Context, db *sql.DB, empresaID string, parsed *rel322Parsed, tiposVendaSelecao []string, escopo escopoRecorte, fluxo fluxoCtx) (*comparativoRel322Resposta, error) {
	tiposVenda := tipoVendaSelecionado(tiposVendaSelecao, fluxo)

	farol, err := farolLiquidoPorSupervisor(ctx, db, empresaID, parsed.DataInicio, parsed.DataFim, parsed.Filiais, tiposVenda, escopo, fluxo)
	if err != nil {
		return nil, err
	}

	vm, vmErr := vmLiquidoPorSupervisor(ctx, empresaID, parsed.DataInicio, parsed.DataFim, parsed.Filiais, tiposVenda, escopo, fluxo)
	if vmErr != nil {
		log.Printf("[comparativo322] VM indisponível: %v", vmErr)
	}

	resp := cruzarComRel322(farol, vm, vmErr, parsed)
	resp.Fluxo = fluxo.name
	return resp, nil
}

// cruzarComRel322 — aplica a regra de status (tolerância 0,5% PDF×Farol),
// órfãos, período sem dado e as três diferenças percentuais (PDF×VM,
// PDF×Farol, VM×Farol) sobre o resultado já buscado do Farol e da VM.
// Separada de montarComparativo para ser testável sem *sql.DB nem Oracle —
// os mapas de entrada já vêm prontos.
//
// Só Líquido — Bruto foi removido da tela (decisão do Claudio em
// 28/08/2026). Líquido pode vir negativo (ex.: Cortado > Bruto no
// Transmitido) sem clamp nem erro. Status é decidido SEMPRE por PDF×Farol,
// nunca pela VM — a VM é diagnóstico adicional.
func cruzarComRel322(farol map[string]float64, vm map[string]float64, vmErr error, parsed *rel322Parsed) *comparativoRel322Resposta {
	// Período sem NENHUM dado no Farol (range futuro ou ainda não
	// importado): reflete a realidade, todas as linhas viram divergência
	// (Farol = 0), não um conjunto de órfãos — é uma condição sistêmica, não
	// um cod_supervisor isolado sem correspondente.
	semDadoNoPeriodo := len(farol) == 0

	usados := map[string]bool{}
	var linhas []linhaComparativoRel322
	var totalPDF, totalLiquido, totalVM float64
	var temVM bool
	var qtdDivergencias, qtdOrfaos int

	aplicarVM := func(linha *linhaComparativoRel322, cod string, vlPDF *float64, liquidoFarol *float64) {
		vmVal, ok := vm[cod]
		if !ok {
			return
		}
		linha.LiquidoVM = f64p(vmVal)
		totalVM += vmVal
		temVM = true
		if vlPDF != nil {
			if pct := percentDiffRel322(vmVal, *vlPDF); !math.IsInf(pct, 0) {
				linha.DiferencaPDFxVMPct = f64p(pct * 100)
			}
		}
		if liquidoFarol != nil {
			if pct := percentDiffRel322(*liquidoFarol, vmVal); !math.IsInf(pct, 0) {
				linha.DiferencaVMxFarolPct = f64p(pct * 100)
			}
		}
	}

	for _, l := range parsed.Linhas {
		totalPDF += l.VlVendido
		vlPDF := l.VlVendido

		liquido, achou := farol[l.CodSupervisor]
		if !achou && !semDadoNoPeriodo {
			// Farol tem dado no período, só não para ESTE supervisor: órfão.
			qtdOrfaos++
			linha := linhaComparativoRel322{
				CodSupervisor: l.CodSupervisor,
				Supervisor:    l.Descricao,
				VlVendidoPDF:  f64p(vlPDF),
				Status:        "orfao",
				Origem:        "pdf",
			}
			aplicarVM(&linha, l.CodSupervisor, &vlPDF, nil)
			linhas = append(linhas, linha)
			continue
		}
		usados[l.CodSupervisor] = true
		totalLiquido += liquido

		pctFarol := percentDiffRel322(liquido, vlPDF)
		status := "divergencia"
		if pctFarol <= 0.005 {
			status = "ok"
		} else {
			qtdDivergencias++
		}

		// PDF=0 e Farol!=0 faz percentDiffRel322 devolver +Inf. encoding/json
		// não sabe serializar Infinity — se esse valor chegasse até aqui,
		// json.NewEncoder(w).Encode falharia e o cliente receberia um 200 com
		// corpo vazio/truncado, o "comparativo parcial silencioso" que o spec
		// proíbe. Vira nil (o front já trata como "—"); o status continua
		// "divergencia" — nunca "ok" nesse caso, já garantido pela tolerância.
		linha := linhaComparativoRel322{
			CodSupervisor: l.CodSupervisor,
			Supervisor:    l.Descricao,
			VlVendidoPDF:  f64p(vlPDF),
			LiquidoFarol:  f64p(liquido),
			Status:        status,
			Origem:        "ambos",
		}
		if !math.IsInf(pctFarol, 0) {
			linha.DiferencaPDFxFarolPct = f64p(pctFarol * 100)
		}
		aplicarVM(&linha, l.CodSupervisor, &vlPDF, &liquido)
		linhas = append(linhas, linha)
	}

	// Supervisores com dado no Farol no período, mas ausentes do PDF —
	// órfãos do outro lado. Só faz sentido quando o Farol TEM dado no
	// período (senão já caímos no caso sistêmico acima, e todo cod_supervisor
	// do Farol estaria "ausente" por definição de len(farol)==0).
	if !semDadoNoPeriodo {
		// Iteração de map não tem ordem estável — sem ordenar, a mesma
		// resposta viria em ordem diferente a cada upload do MESMO PDF.
		var codsOrfaosFarol []string
		for cod := range farol {
			if !usados[cod] {
				codsOrfaosFarol = append(codsOrfaosFarol, cod)
			}
		}
		sort.Strings(codsOrfaosFarol)

		for _, cod := range codsOrfaosFarol {
			liquido := farol[cod]
			qtdOrfaos++
			totalLiquido += liquido
			linha := linhaComparativoRel322{
				CodSupervisor: cod,
				LiquidoFarol:  f64p(liquido),
				Status:        "orfao",
				Origem:        "farol",
			}
			aplicarVM(&linha, cod, nil, &liquido)
			linhas = append(linhas, linha)
		}
	}

	filiais := parsed.Filiais
	if filiais == nil {
		filiais = []string{}
	}

	resp := &comparativoRel322Resposta{
		Periodo:               parsed.PeriodoTexto,
		DataInicio:            parsed.DataInicio.Format("2006-01-02"),
		DataFim:               parsed.DataFim.Format("2006-01-02"),
		Filiais:               filiais,
		Linhas:                linhas,
		TotalVlVendidoPDF:     totalPDF,
		TotalLiquidoFarol:     totalLiquido,
		QtdSupervisoresPDF:    len(parsed.Linhas),
		QtdDivergencias:       qtdDivergencias,
		QtdOrfaos:             qtdOrfaos,
		SemDadoFarolNoPeriodo: semDadoNoPeriodo,
	}
	if vmErr != nil {
		resp.VMIndisponivel = true
		resp.VMErro = "não foi possível consultar a base de origem (VM) agora — tente novamente em instantes"
	} else if temVM {
		resp.TotalLiquidoVM = f64p(totalVM)
	}
	return resp
}

// percentDiffRel322 — distância percentual entre o valor do Farol e o
// Vl.Vendido do PDF. PDF=0 e Farol=0 é 0% (bate); PDF=0 e Farol≠0 é tratado
// como divergência máxima (sem divisão por zero).
func percentDiffRel322(farolValor, pdfValor float64) float64 {
	if pdfValor == 0 {
		if farolValor == 0 {
			return 0
		}
		return math.Inf(1)
	}
	return math.Abs(farolValor-pdfValor) / math.Abs(pdfValor)
}

func f64p(v float64) *float64 { return &v }

// ─── Handler HTTP ───────────────────────────────────────────────────────────

// normalizarFluxoParam — só aceita "transmitido"/"trans" (case-insensitive)
// como fluxo vindo de um cliente HTTP; qualquer outra coisa — vazio, valor
// não reconhecido, ou os usos INTERNOS de resolveFluxo ("cancdev"/"cortado",
// que resolvem só o evento negativo do CCD) — cai no default faturado. O
// resultado desta função é o único valor seguro pra alimentar o resolveFluxo
// genérico quando a entrada vem de fora (ver Boundaries de
// spec-comparativo-rel322-fluxo.md: um request com ?fluxo=cancdev não pode
// fazer a query principal ler de vendas_ccd como se fosse a base comparada).
func normalizarFluxoParam(v string) string {
	v = strings.TrimSpace(v)
	if strings.EqualFold(v, "transmitido") || strings.EqualFold(v, "trans") {
		return "transmitido"
	}
	return "faturado"
}

// ComparativoRel322Handler — POST /api/v2/farol/relatorio/comparativo-rel322
//
// Multipart, campo "file": o PDF do REL 322 (WinThor, "Por Supervisor").
// Leitura pura (não persiste nada) — permissão somente_leitura, mesmo sendo
// POST, porque o upload é só o transporte da consulta, não uma escrita.
func ComparativoRel322Handler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		// Recorte por persona (farol_escopo.go) — mesma regra que
		// /extrato-produto-cliente já aplica: sem isto, um supervisor logado
		// (sp_role=somente_leitura, liberado por esta rota) enxergaria o
		// Bruto/Líquido de TODOS os supervisores da empresa só fazendo upload
		// de um PDF que os liste. Checado ANTES de gastar trabalho com o
		// upload — negar não depende do PDF.
		escopo := escopoDoUsuario(db, spCtx, "")
		if escopo.restrito() && escopo.Negar {
			http.Error(w, "Sem escopo definido para este usuário", http.StatusForbidden)
			return
		}

		// MaxBytesReader antes do ParseMultipartForm: o segundo só define o
		// limiar de MEMÓRIA (acima disso o multipart transborda pra um
		// arquivo temporário em disco, sem teto de tamanho nenhum). 25MB dá
		// folga sobre os 20MB do ParseMultipartForm.
		r.Body = http.MaxBytesReader(w, r.Body, 25<<20)

		if err := r.ParseMultipartForm(20 << 20); err != nil {
			http.Error(w, jsonErrorRel322("falha ao ler o upload: "+err.Error()), http.StatusBadRequest)
			return
		}
		// Upload grande transborda pra disco (arquivo temporário do
		// multipart) — sem isto, ele nunca seria removido depois do request.
		if r.MultipartForm != nil {
			defer r.MultipartForm.RemoveAll()
		}
		file, _, ferr := r.FormFile("file")
		if ferr != nil {
			http.Error(w, jsonErrorRel322(`arquivo não enviado (campo "file")`), http.StatusBadRequest)
			return
		}
		defer file.Close()

		dados, rerr := io.ReadAll(file)
		if rerr != nil {
			http.Error(w, jsonErrorRel322("falha ao ler o arquivo enviado"), http.StatusBadRequest)
			return
		}

		texto, perr := extrairTextoPDF(dados)
		if perr != nil {
			log.Printf("[comparativo322] extração PDF ERRO: %v", perr)
			http.Error(w, jsonErrorRel322("não foi possível ler o PDF: "+perr.Error()), http.StatusUnprocessableEntity)
			return
		}

		parsed, perr2 := parseRel322Texto(texto)
		if perr2 != nil {
			http.Error(w, jsonErrorRel322(perr2.Error()), http.StatusUnprocessableEntity)
			return
		}

		// ?fluxo= — mesmo padrão de ?formato= logo abaixo. O REL 322 do
		// WinThor não se autodeclara sobre qual base (Faturado/Transmitido)
		// foi gerado, então é sempre escolha explícita do usuário no upload
		// (nunca inferida do PDF); vazio/não reconhecido cai no default
		// faturado. NÃO repassa o valor bruto da querystring pra
		// resolveFluxo: ele também resolve "cancdev"/"cortado" (uso INTERNO,
		// só pro evento negativo do CCD) — um cliente HTTP mandando isso
		// aqui não pode fazer a query principal ler de vendas_ccd como se
		// fosse a base comparada (normalizarFluxoParam fecha essa porta).
		fluxo := resolveFluxo(normalizarFluxoParam(r.URL.Query().Get("fluxo")))

		// ?tipo_venda= — filtro "Tipo de Venda" da tela (multi-seleção, ex.:
		// "1,4,7"). Vazio cai no default de cada fluxo (tipoVendaSelecionado).
		tiposVendaSelecao := parseListaCSVRel322(r.URL.Query().Get("tipo_venda"))

		resultado, qerr := montarComparativo(r.Context(), db, spCtx.EmpresaID, parsed, tiposVendaSelecao, escopo, fluxo)
		if qerr != nil {
			log.Printf("[comparativo322] consulta Farol ERRO: %v", qerr)
			http.Error(w, jsonErrorRel322("falha ao consultar o Farol"), http.StatusInternalServerError)
			return
		}

		// ?formato=pdf reprocessa o mesmo upload no mesmo request e devolve o
		// PDF do RESULTADO do comparativo — nunca do PDF do WinThor enviado.
		// Sem persistência, igual ao resto da feature. Default (json) mantém
		// o contrato já em produção intocado.
		if strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("formato")), "pdf") {
			logo, ext := logoRelatorio(db, spCtx.EmpresaID)
			pdfBytes, gerr := comparativoRel322PDF(resultado, logo, ext)
			if gerr != nil {
				log.Printf("[comparativo322] PDF ERRO: %v", gerr)
				http.Error(w, jsonErrorRel322("falha ao gerar PDF"), http.StatusInternalServerError)
				return
			}
			nome := fmt.Sprintf("comparativo-rel322_%s_a_%s", resultado.DataInicio, resultado.DataFim)
			w.Header().Set("Content-Type", "application/pdf")
			w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.pdf"`, nome))
			if _, werr := w.Write(pdfBytes); werr != nil {
				log.Printf("[comparativo322] falha ao escrever resposta PDF: %v", werr)
			}
			return
		}

		// Defesa em profundidade: depois do fix do DiferencaPct (+Inf → nil)
		// isto não deveria mais falhar, mas se algum outro campo algum dia
		// carregar um valor não serializável, não pode ficar invisível — sem
		// o log, o cliente recebe 200 com corpo vazio/truncado e ninguém sabe.
		if err := json.NewEncoder(w).Encode(resultado); err != nil {
			log.Printf("[comparativo322] falha ao serializar resposta: %v", err)
		}
	}
}

func jsonErrorRel322(msg string) string {
	b, _ := json.Marshal(map[string]string{"error": msg})
	return string(b)
}

// ─── PDF ──────────────────────────────────────────────────────────────────────

// comparativoRel322PDF — PDF do RESULTADO do comparativo (nunca do PDF do
// WinThor enviado). Mesma estrutura de relatorioReceitaPDF
// (farol_cnpj_receita.go): logo do inquilino, cabeçalho, tabela com
// cabeNaColuna/fonteCelula (evita a sobreposição de linha corrigida ali),
// rodapé. O comparativo tem no máximo ~50-90 linhas — bem abaixo do volume
// que já pagina automaticamente, sem paginação manual nova.
func comparativoRel322PDF(resultado *comparativoRel322Resposta, logo []byte, ext extension.Type) ([]byte, error) {
	// Paisagem: com Líquido (Farol) + Líquido (VM) + 3 diferenças percentuais
	// + Status, a tabela não cabe numa página retrato sem espremer os valores
	// em moeda (a mesma limitação que já existia SÓ com Bruto+Líquido).
	cfg := config.NewBuilder().
		WithPageNumber(props.PageNumber{Pattern: "Pág. {current}/{total}", Place: props.RightBottom}).
		WithOrientation(orientation.Horizontal).
		WithLeftMargin(8).WithRightMargin(8).WithTopMargin(10).
		Build()
	mrt := maroto.New(cfg)

	pg := page.New()

	// Cabeçalho com logo
	pg.Add(
		row.New(14).Add(
			col.New(2).Add(image.NewFromBytes(logo, ext, props.Rect{Center: true, Percent: 80})),
			col.New(10).Add(
				text.New("Comparativo REL 322 (WinThor) x Farol",
					props.Text{Size: 13, Style: fontstyle.Bold, Align: align.Left, Top: 2}),
				text.New(fmt.Sprintf("Período do PDF de origem: %s", resultado.Periodo),
					props.Text{Size: 8, Align: align.Left, Top: 9}),
			),
		),
	)

	// Cobertura parcial: o PDF é o que circula por e-mail, descolado do
	// contexto em que foi gerado — a ressalva vai no topo, nunca vira erro
	// (mesmo princípio da ressalva de cobertura em relatorioReceitaPDF).
	if resultado.SemDadoFarolNoPeriodo {
		pg.Add(row.New(6).Add(col.New(12).Add(
			text.New(fmt.Sprintf("PARCIAL — o Farol não tem NENHUM dado importado no período %s. Líquido aparece como R$ 0,00 em todas as linhas.", resultado.Periodo),
				props.Text{Size: 8, Style: fontstyle.BoldItalic, Align: align.Left}),
		)))
	}
	// VM indisponível — mesmo princípio: nunca vira erro, só um aviso visível.
	if resultado.VMIndisponivel {
		pg.Add(row.New(6).Add(col.New(12).Add(
			text.New("VM (base de origem) indisponível nesta consulta — as colunas de Líquido (VM) e as diferenças que dependem dela aparecem como \"—\".",
				props.Text{Size: 8, Style: fontstyle.Italic, Align: align.Left}),
		)))
	}
	if len(resultado.Filiais) > 0 {
		pg.Add(row.New(5).Add(col.New(12).Add(
			text.New("Filial(is) do PDF: "+strings.Join(resultado.Filiais, ", "), props.Text{Size: 7, Align: align.Left}),
		)))
	}

	pg.Add(row.New(5).Add(col.New(12).Add(
		text.New(fmt.Sprintf("%d supervisor(es) no PDF — %d divergência(s), %d órfã(s)",
			resultado.QtdSupervisoresPDF, resultado.QtdDivergencias, resultado.QtdOrfaos),
			props.Text{Size: 9, Style: fontstyle.Bold}),
	)))

	cab := props.Text{Size: 6, Style: fontstyle.Bold}
	cabDir := props.Text{Size: 6, Style: fontstyle.Bold, Align: align.Right}
	pg.Add(row.New(8).Add(
		col.New(1).Add(text.New("Código", cab)),
		col.New(3).Add(text.New("Supervisor", cab)),
		col.New(2).Add(text.New("Vl.Vendido (PDF)", cabDir)),
		col.New(1).Add(text.New("Líquido (Farol)", cabDir)),
		col.New(1).Add(text.New("Líquido (VM)", cabDir)),
		col.New(1).Add(text.New("% PDF×VM", cabDir)),
		col.New(1).Add(text.New("% PDF×Farol", cabDir)),
		col.New(1).Add(text.New("% VM×Farol", cabDir)),
		col.New(1).Add(text.New("Status", cab)),
	))

	cel := props.Text{Size: fonteCelula}
	celDir := props.Text{Size: fonteCelula, Align: align.Right}
	for _, l := range resultado.Linhas {
		pg.Add(row.New(5).Add(
			col.New(1).Add(text.New(l.CodSupervisor, cel)),
			col.New(3).Add(text.New(cabeNaColuna(primeiroNaoVazio(l.Supervisor, "—"), 3), cel)),
			col.New(2).Add(text.New(valorOuTracoRel322(l.VlVendidoPDF), celDir)),
			col.New(1).Add(text.New(valorOuTracoRel322(l.LiquidoFarol), celDir)),
			col.New(1).Add(text.New(valorOuTracoRel322(l.LiquidoVM), celDir)),
			col.New(1).Add(text.New(pctOuTracoRel322(l.DiferencaPDFxVMPct), celDir)),
			col.New(1).Add(text.New(pctOuTracoRel322(l.DiferencaPDFxFarolPct), celDir)),
			col.New(1).Add(text.New(pctOuTracoRel322(l.DiferencaVMxFarolPct), celDir)),
			col.New(1).Add(text.New(statusTextoRel322(l.Status), cel)),
		))
	}

	pg.Add(row.New(10).Add(col.New(12).Add(
		text.New("OK = Líquido (Farol) está a até 0,5% do Vl.Vendido do PDF. DIVERGÊNCIA = fora dessa tolerância. "+
			"ÓRFÃ = supervisor presente em só um dos lados (PDF ou Farol) no período. "+
			"Líquido (VM) vem AO VIVO da base de origem (WinThor/Oracle) — diagnóstico adicional, nunca decide o Status. "+
			"% PDF×VM e % VM×Farol isolam se a divergência já vem do próprio relatório WinThor ou surgiu na importação pro Farol.",
			props.Text{Size: 6, Top: 3}),
	)))

	mrt.AddPages(pg)
	doc, err := mrt.Generate()
	if err != nil {
		return nil, err
	}
	return doc.GetBytes(), nil
}

// valorOuTracoRel322 — "—" para "não existe deste lado" (ponteiro nil),
// nunca R$ 0,00: 0 apurado e ausente são coisas diferentes (mesma distinção
// que o JSON já preserva com ponteiros).
func valorOuTracoRel322(v *float64) string {
	if v == nil {
		return "—"
	}
	return moedaBR(*v)
}

// pctOuTracoRel322 — cruzarComRel322 já garante que DiferencaPct vira nil
// (não +Inf) quando a distância é infinita — é o que alimenta JSON e PDF.
// A checagem de NaN/Inf aqui é defesa em profundidade: se um valor não-nil
// mas não-finito chegar mesmo assim (hoje ou por uma mudança futura), o PDF
// não pode imprimir literalmente "+Inf%"/"NaN%" numa linha de um relatório
// que circula por e-mail.
func pctOuTracoRel322(v *float64) string {
	if v == nil || math.IsNaN(*v) || math.IsInf(*v, 0) {
		return "—"
	}
	return strings.ReplaceAll(strconv.FormatFloat(*v, 'f', 2, 64), ".", ",") + "%"
}

// statusTextoRel322 — o PDF não tem o selo colorido da tela; status em texto.
func statusTextoRel322(s string) string {
	switch s {
	case "ok":
		return "OK"
	case "divergencia":
		return "DIVERGÊNCIA"
	case "orfao":
		return "ÓRFÃ"
	}
	return strings.ToUpper(s)
}
