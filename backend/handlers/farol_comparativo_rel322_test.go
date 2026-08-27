package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"math"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/johnfercher/maroto/v2"
	"github.com/johnfercher/maroto/v2/pkg/components/col"
	"github.com/johnfercher/maroto/v2/pkg/components/page"
	"github.com/johnfercher/maroto/v2/pkg/components/row"
	"github.com/johnfercher/maroto/v2/pkg/components/text"
	"github.com/johnfercher/maroto/v2/pkg/config"
	"github.com/johnfercher/maroto/v2/pkg/consts/extension"
	"github.com/johnfercher/maroto/v2/pkg/props"
	"github.com/lib/pq"
)

// farol_comparativo_rel322_test.go — cobre a I/O Matrix do parser do REL 322.
//
// O texto usado nos testes reproduz o formato REAL que
// github.com/ledongthuc/pdf.GetPlainText devolve para o relatório WinThor
// "322 - Venda Por Departamento" — um token por linha, na ordem em que o PDF
// desenha (não uma linha de texto por linha visual da tabela). Confirmado
// batendo os 4 PDFs de exemplo em /home/claudio/uploads/*.pdf: a soma do
// primeiro número de cada linha de supervisor bate exata com o valor
// declarado após "N Supervisores Listados".

// cabecalhoRel322 — bloco de cabeçalho de UMA página, do jeito que o PDF o
// desenha (repetido a cada página). periodo e tipo são parametrizados para
// os testes de layout inesperado.
func cabecalhoRel322(periodo, tipo string) string {
	return strings.Join([]string{
		"Código",
		"Supervisor",
		"Vl.Vendido",
		"%",
		"322 - Venda Por Departamento",
		"27/08/2026 12:36:16",
		"Pagina : 001",
		"Departamento :",
		"Todos os Departamentos",
		"Supervisor :",
		"Todos os Supervisores",
		"Periodo :",
		periodo,
		"Tipo :",
		tipo,
		"Filiai(s) :",
		"10,11,13,15,16,18,20,22,23,24,28,29,30,31,32,33",
		"RCA :",
		"Todos os RCAs",
		"Qt.Peso",
		"Seção :",
		"Todas as Seções",
		"Qt. Vendida",
		"Volume",
		"Todos os Clientes",
		"Cliente :",
		"Qt.Cli.Pos.",
		"Distribuição :",
		"Todas as Distribuições",
		"Fornecedor :",
		"Todos os Fornecedores",
		"Vl. Meta",
		"%",
		"Qt. Meta",
		"Venda",
		"Meta",
		"Qt.Cli.Ativos",
		"% Pos.",
		"Ramo de Atividade:",
		"Todos os Ramos de Atividade",
		"Cliente Principal:",
		"Todos os Clientes",
		"Praça:",
		"Todas as Praças",
		"Região:",
		"Todas as Regiões",
		"Produtos:",
		"Todos",
		"Emitente:",
		"Todos os Emitentes",
		"Comprador:",
		"Todos os Compradores",
		"Período de Prev. de Fat.:",
		"Todos os Períodos",
	}, "\n")
}

// linhaRel322Fixture — uma linha de dado válida (código + descrição + 11
// números), no formato de tokens do PDF.
func linhaRel322Fixture(codigo, descricao, vlVendido string, resto ...string) string {
	nums := append([]string{vlVendido}, resto...)
	for len(nums) < 11 {
		nums = append(nums, "0,00")
	}
	linhas := append([]string{codigo, descricao}, nums...)
	return strings.Join(linhas, "\n")
}

const totaisRel322Fixture = "Supervisores Listados\n2\n999,99\nTotal Geral:\n100,00\n0,00\n0,00\n0,00\n0,00\n0\n0\n0"

func TestComparativoRel322_Parse_LinhaNormal(t *testing.T) {
	texto := strings.Join([]string{
		cabecalhoRel322("01/08/2026 a 26/08/2026", "14-Por Supervisor"),
		linhaRel322Fixture("124", "GO - VALE SAO PATRICIO - LUCAS", "9.223.911,54", "7,10"),
		linhaRel322Fixture("240", "GO - NORTE - JOSENILTON", "8.597.443,46", "6,62"),
		totaisRel322Fixture,
	}, "\n")

	parsed, err := parseRel322Texto(texto)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if parsed.PeriodoTexto != "01/08/2026 a 26/08/2026" {
		t.Errorf("periodo = %q", parsed.PeriodoTexto)
	}
	if got, want := parsed.DataInicio.Format("2006-01-02"), "2026-08-01"; got != want {
		t.Errorf("data_inicio = %s, want %s", got, want)
	}
	if got, want := parsed.DataFim.Format("2006-01-02"), "2026-08-26"; got != want {
		t.Errorf("data_fim = %s, want %s", got, want)
	}
	if len(parsed.Linhas) != 2 {
		t.Fatalf("esperava 2 linhas, veio %d: %+v", len(parsed.Linhas), parsed.Linhas)
	}
	if parsed.Linhas[0].CodSupervisor != "124" || parsed.Linhas[0].VlVendido != 9223911.54 {
		t.Errorf("linha 0 = %+v", parsed.Linhas[0])
	}
	if parsed.Linhas[1].CodSupervisor != "240" || parsed.Linhas[1].VlVendido != 8597443.46 {
		t.Errorf("linha 1 = %+v", parsed.Linhas[1])
	}
}

// TestParseRel322_TotalGeralNaoViraLinha — a linha "N Supervisores Listados —
// Total Geral:" é o total de conferência, não uma linha de supervisor: não
// pode aparecer em parsed.Linhas nem quebrar o parser.
func TestComparativoRel322_Parse_TotalGeralNaoViraLinha(t *testing.T) {
	texto := strings.Join([]string{
		cabecalhoRel322("01/08/2026 a 26/08/2026", "14-Por Supervisor"),
		linhaRel322Fixture("124", "GO - VALE SAO PATRICIO - LUCAS", "9.223.911,54", "7,10"),
		totaisRel322Fixture,
	}, "\n")

	parsed, err := parseRel322Texto(texto)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(parsed.Linhas) != 1 {
		t.Fatalf("esperava 1 linha de supervisor (Total Geral não conta), veio %d", len(parsed.Linhas))
	}
	for _, l := range parsed.Linhas {
		if l.CodSupervisor == "2" || strings.Contains(l.Descricao, "Total Geral") {
			t.Errorf("Total Geral vazou como linha de supervisor: %+v", l)
		}
	}
}

// TestParseRel322_MultiplasPaginasComCabecalhoRepetido — o cabeçalho se
// repete a cada página (com "Pagina : NNN" mudando); as linhas de dado das
// duas páginas devem se juntar numa lista só.
func TestComparativoRel322_Parse_MultiplasPaginasComCabecalhoRepetido(t *testing.T) {
	pagina1 := strings.ReplaceAll(cabecalhoRel322("01/08/2026 a 26/08/2026", "14-Por Supervisor"), "Pagina : 001", "Pagina : 001")
	pagina2 := strings.ReplaceAll(cabecalhoRel322("01/08/2026 a 26/08/2026", "14-Por Supervisor"), "Pagina : 001", "Pagina : 002")

	texto := strings.Join([]string{
		pagina1,
		linhaRel322Fixture("124", "GO - VALE SAO PATRICIO - LUCAS", "9.223.911,54", "7,10"),
		pagina2,
		linhaRel322Fixture("1011", "SITE COMPRAS - JC", "710.991,11", "0,55"),
		totaisRel322Fixture,
	}, "\n")

	parsed, err := parseRel322Texto(texto)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(parsed.Linhas) != 2 {
		t.Fatalf("esperava 2 linhas (uma por página), veio %d: %+v", len(parsed.Linhas), parsed.Linhas)
	}
	if parsed.Linhas[0].CodSupervisor != "124" || parsed.Linhas[1].CodSupervisor != "1011" {
		t.Errorf("códigos fora de ordem: %+v", parsed.Linhas)
	}
}

// TestParseRel322_DescricaoComHifen — "GO - VALE SAO PATRICIO - LUCAS" tem
// espaços E hífens, não dá para separar a descrição por espaço.
func TestComparativoRel322_Parse_DescricaoComHifen(t *testing.T) {
	texto := strings.Join([]string{
		cabecalhoRel322("01/08/2026 a 26/08/2026", "14-Por Supervisor"),
		linhaRel322Fixture("124", "GO - VALE SAO PATRICIO - LUCAS", "9.223.911,54", "7,10"),
		totaisRel322Fixture,
	}, "\n")

	parsed, err := parseRel322Texto(texto)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if got, want := parsed.Linhas[0].Descricao, "GO - VALE SAO PATRICIO - LUCAS"; got != want {
		t.Errorf("descricao = %q, want %q", got, want)
	}
}

// TestParseRel322_LayoutSemTitulo — PDF que não é o 322 (falta o título):
// aborta com erro claro, não gera comparativo parcial.
func TestComparativoRel322_Parse_LayoutSemTitulo(t *testing.T) {
	cab := strings.Replace(cabecalhoRel322("01/08/2026 a 26/08/2026", "14-Por Supervisor"),
		"322 - Venda Por Departamento", "999 - Outro Relatório Qualquer", 1)
	texto := strings.Join([]string{
		cab,
		linhaRel322Fixture("124", "GO - VALE SAO PATRICIO - LUCAS", "9.223.911,54", "7,10"),
	}, "\n")

	_, err := parseRel322Texto(texto)
	if err == nil {
		t.Fatal("esperava erro para PDF que não é o REL 322, veio nil")
	}
}

// TestParseRel322_TipoDiferenteDePorSupervisor — mesmo relatório 322, mas
// não é o formato "Por Supervisor" (ex.: por RCA, por fornecedor).
func TestComparativoRel322_Parse_TipoDiferenteDePorSupervisor(t *testing.T) {
	texto := strings.Join([]string{
		cabecalhoRel322("01/08/2026 a 26/08/2026", "01-Por Fornecedor"),
		linhaRel322Fixture("124", "GO - VALE SAO PATRICIO - LUCAS", "9.223.911,54", "7,10"),
	}, "\n")

	_, err := parseRel322Texto(texto)
	if err == nil {
		t.Fatal("esperava erro para layout que não é 'Por Supervisor', veio nil")
	}
}

// TestParseRel322_PeriodoFormatoInesperado — a linha "Periodo :" precisa vir
// no formato DD/MM/AAAA a DD/MM/AAAA; qualquer outra coisa aborta.
func TestComparativoRel322_Parse_PeriodoFormatoInesperado(t *testing.T) {
	texto := strings.Join([]string{
		cabecalhoRel322("Agosto/2026", "14-Por Supervisor"),
		linhaRel322Fixture("124", "GO - VALE SAO PATRICIO - LUCAS", "9.223.911,54", "7,10"),
	}, "\n")

	_, err := parseRel322Texto(texto)
	if err == nil {
		t.Fatal("esperava erro para período em formato inesperado, veio nil")
	}
}

// TestParseRel322_LinhaComContagemNumericaErrada — se uma linha não tiver os
// 11 números esperados (relatório mudou de formato), aborta com erro citando
// o supervisor — nunca gera comparativo parcial.
func TestComparativoRel322_Parse_LinhaComContagemNumericaErrada(t *testing.T) {
	texto := strings.Join([]string{
		cabecalhoRel322("01/08/2026 a 26/08/2026", "14-Por Supervisor"),
		"124",
		"GO - VALE SAO PATRICIO - LUCAS",
		"9.223.911,54",
		"7,10",
		"305.803,00",
		totaisRel322Fixture, // corta a linha em 3 números em vez de 11
	}, "\n")

	_, err := parseRel322Texto(texto)
	if err == nil {
		t.Fatal("esperava erro para linha com contagem numérica errada, veio nil")
	}
	if !strings.Contains(err.Error(), "124") {
		t.Errorf("erro deveria citar o supervisor 124: %v", err)
	}
}

// TestComparativoRel322_Parse_CodigoDuplicado — PDF corrompido/editado com o
// mesmo cod_supervisor duas vezes: sem detecção, o Farol dele seria somado em
// dobro nos totais, silenciosamente (mesmo princípio de "nunca comparativo
// errado sem avisar" que já motiva os outros erros claros do parser).
func TestComparativoRel322_Parse_CodigoDuplicado(t *testing.T) {
	texto := strings.Join([]string{
		cabecalhoRel322("01/08/2026 a 26/08/2026", "14-Por Supervisor"),
		linhaRel322Fixture("124", "GO - VALE SAO PATRICIO - LUCAS", "9.223.911,54", "7,10"),
		linhaRel322Fixture("124", "GO - VALE SAO PATRICIO - LUCAS (dup)", "1.000,00", "0,01"),
		totaisRel322Fixture,
	}, "\n")

	_, err := parseRel322Texto(texto)
	if err == nil {
		t.Fatal("esperava erro para código de supervisor duplicado, veio nil")
	}
	if !strings.Contains(err.Error(), "124") {
		t.Errorf("erro deveria citar o código duplicado 124: %v", err)
	}
}

// TestParseRel322_SomaBateComTotalGeral — replica a checagem manual do spec:
// a soma do Vl.Vendido de todas as linhas bate com o total declarado.
func TestComparativoRel322_Parse_SomaBateComTotalGeral(t *testing.T) {
	texto := strings.Join([]string{
		cabecalhoRel322("01/08/2026 a 26/08/2026", "14-Por Supervisor"),
		linhaRel322Fixture("124", "GO - VALE SAO PATRICIO - LUCAS", "9.223.911,54", "7,10"),
		linhaRel322Fixture("240", "GO - NORTE - JOSENILTON", "8.597.443,46", "6,62"),
		linhaRel322Fixture("586", "PRESTAÇÃO DE SERVIÇOS-FRETES", "81.863,38", "0,06"),
		"Supervisores Listados\n3\n17.903.218,38\nTotal Geral:\n100,00",
	}, "\n")

	parsed, err := parseRel322Texto(texto)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	var soma float64
	for _, l := range parsed.Linhas {
		soma += l.VlVendido
	}
	if math.Abs(soma-17903218.38) > 0.01 {
		t.Errorf("soma = %.2f, want 17903218.38", soma)
	}
}

func TestComparativoRel322_ParseNumeroBR(t *testing.T) {
	casos := []struct {
		in   string
		want float64
	}{
		{"9.223.911,54", 9223911.54},
		{"0,00", 0},
		{"1.208", 1208},
		{"33352", 33352},
		{"-45,90", -45.90},
	}
	for _, c := range casos {
		got, err := parseNumeroBR(c.in)
		if err != nil {
			t.Fatalf("parseNumeroBR(%q) erro: %v", c.in, err)
		}
		if math.Abs(got-c.want) > 0.001 {
			t.Errorf("parseNumeroBR(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// farolFake — atalho para montar o mapa que farolBrutoLiquidoPorSupervisor
// devolveria, sem precisar de banco.
func farolFake(m map[string][2]float64) map[string]struct{ Bruto, Liquido float64 } {
	out := map[string]struct{ Bruto, Liquido float64 }{}
	for cod, v := range m {
		out[cod] = struct{ Bruto, Liquido float64 }{Bruto: v[0], Liquido: v[1]}
	}
	return out
}

// TestComparativoRel322_Cruzamento_TudoBate — Farol dentro da tolerância de
// 0,5% do Vl.Vendido do PDF em todas as linhas: tudo "ok", zero divergência,
// zero órfã.
func TestComparativoRel322_Cruzamento_TudoBate(t *testing.T) {
	parsed := &rel322Parsed{
		PeriodoTexto: "01/08/2026 a 26/08/2026",
		DataInicio:   mustParseData(t, "2026-08-01"),
		DataFim:      mustParseData(t, "2026-08-26"),
		Linhas: []linhaExtraidaRel322{
			{CodSupervisor: "124", Descricao: "GO - VALE SAO PATRICIO - LUCAS", VlVendido: 9223911.54},
			{CodSupervisor: "240", Descricao: "GO - NORTE - JOSENILTON", VlVendido: 8597443.46},
		},
	}
	farol := farolFake(map[string][2]float64{
		"124": {9223911.54, 9000000.00}, // Bruto bate exato
		"240": {8600000.00, 8597443.46}, // Líquido bate exato
	})

	resp := cruzarComRel322(farol, parsed)

	if resp.SemDadoFarolNoPeriodo {
		t.Error("não deveria marcar sem-dado-no-período: o Farol tem dado para ambos")
	}
	if resp.QtdDivergencias != 0 || resp.QtdOrfaos != 0 {
		t.Fatalf("esperava 0 divergências e 0 órfãs, veio %d/%d", resp.QtdDivergencias, resp.QtdOrfaos)
	}
	for _, l := range resp.Linhas {
		if l.Status != "ok" {
			t.Errorf("linha %s deveria ser ok, veio %s", l.CodSupervisor, l.Status)
		}
	}
}

// TestComparativoRel322_Cruzamento_Orfaos — cobre os dois sentidos: supervisor
// só no PDF (Farol tem dado no período, mas não para esse código) e
// supervisor só no Farol (apareceu na base, mas não no PDF enviado).
func TestComparativoRel322_Cruzamento_Orfaos(t *testing.T) {
	parsed := &rel322Parsed{
		PeriodoTexto: "01/08/2026 a 26/08/2026",
		DataInicio:   mustParseData(t, "2026-08-01"),
		DataFim:      mustParseData(t, "2026-08-26"),
		Linhas: []linhaExtraidaRel322{
			{CodSupervisor: "124", Descricao: "GO - VALE SAO PATRICIO - LUCAS", VlVendido: 9223911.54},
			{CodSupervisor: "705", Descricao: "SUPERVISOR SO NO PDF", VlVendido: 1000.00},
		},
	}
	farol := farolFake(map[string][2]float64{
		"124": {9223911.54, 9223911.54},
		"999": {5000.00, 5000.00}, // só no Farol, não veio no PDF
	})

	resp := cruzarComRel322(farol, parsed)

	if resp.SemDadoFarolNoPeriodo {
		t.Error("não deveria marcar sem-dado-no-período: o Farol tem dado")
	}
	if resp.QtdOrfaos != 2 {
		t.Fatalf("esperava 2 órfãs (uma de cada lado), veio %d", resp.QtdOrfaos)
	}

	var achouSoPDF, achouSoFarol bool
	for _, l := range resp.Linhas {
		if l.CodSupervisor == "705" {
			achouSoPDF = true
			if l.Status != "orfao" || l.Origem != "pdf" {
				t.Errorf("705 deveria ser órfão de origem pdf: %+v", l)
			}
			if l.BrutoFarol != nil || l.LiquidoFarol != nil {
				t.Errorf("705 não deveria ter Bruto/Líquido do Farol: %+v", l)
			}
		}
		if l.CodSupervisor == "999" {
			achouSoFarol = true
			if l.Status != "orfao" || l.Origem != "farol" {
				t.Errorf("999 deveria ser órfão de origem farol: %+v", l)
			}
			if l.VlVendidoPDF != nil {
				t.Errorf("999 não deveria ter Vl.Vendido do PDF: %+v", l)
			}
		}
	}
	if !achouSoPDF || !achouSoFarol {
		t.Fatalf("esperava órfão pdf (705) e órfão farol (999): %+v", resp.Linhas)
	}
}

// TestComparativoRel322_Cruzamento_Divergencia — Farol distante >0,5% do
// Vl.Vendido do PDF em Bruto E Líquido: linha marcada divergência.
func TestComparativoRel322_Cruzamento_Divergencia(t *testing.T) {
	parsed := &rel322Parsed{
		PeriodoTexto: "01/08/2026 a 26/08/2026",
		DataInicio:   mustParseData(t, "2026-08-01"),
		DataFim:      mustParseData(t, "2026-08-26"),
		Linhas: []linhaExtraidaRel322{
			{CodSupervisor: "124", Descricao: "GO - VALE SAO PATRICIO - LUCAS", VlVendido: 100000.00},
		},
	}
	farol := farolFake(map[string][2]float64{
		"124": {90000.00, 89000.00}, // 10%/11% de distância — bem acima de 0,5%
	})

	resp := cruzarComRel322(farol, parsed)

	if resp.QtdDivergencias != 1 {
		t.Fatalf("esperava 1 divergência, veio %d", resp.QtdDivergencias)
	}
	if resp.Linhas[0].Status != "divergencia" {
		t.Errorf("status = %s, want divergencia", resp.Linhas[0].Status)
	}
	if resp.Linhas[0].DiferencaPct == nil || *resp.Linhas[0].DiferencaPct < 10 {
		t.Errorf("diferenca_pct deveria refletir a menor distância (~10%%): %+v", resp.Linhas[0].DiferencaPct)
	}
}

// TestComparativoRel322_Cruzamento_SemDadoNoPeriodo — Farol sem NENHUMA linha
// no período (range futuro / ainda não importado): condição sistêmica, todas
// as linhas do PDF viram divergência (Farol=0), nenhuma vira órfã.
func TestComparativoRel322_Cruzamento_SemDadoNoPeriodo(t *testing.T) {
	parsed := &rel322Parsed{
		PeriodoTexto: "01/01/2099 a 31/01/2099",
		DataInicio:   mustParseData(t, "2099-01-01"),
		DataFim:      mustParseData(t, "2099-01-31"),
		Linhas: []linhaExtraidaRel322{
			{CodSupervisor: "124", Descricao: "GO - VALE SAO PATRICIO - LUCAS", VlVendido: 9223911.54},
			{CodSupervisor: "240", Descricao: "GO - NORTE - JOSENILTON", VlVendido: 8597443.46},
		},
	}
	farol := farolFake(nil) // Farol vazio: nada importado para o período

	resp := cruzarComRel322(farol, parsed)

	if !resp.SemDadoFarolNoPeriodo {
		t.Fatal("esperava sem_dado_farol_no_periodo = true")
	}
	if resp.QtdOrfaos != 0 {
		t.Errorf("período sem dado é condição sistêmica, não deveria gerar órfãs: %d", resp.QtdOrfaos)
	}
	if resp.QtdDivergencias != 2 {
		t.Fatalf("as 2 linhas do PDF deveriam virar divergência (Farol=0), veio %d", resp.QtdDivergencias)
	}
	for _, l := range resp.Linhas {
		if l.Status != "divergencia" {
			t.Errorf("linha %s deveria ser divergencia, veio %s", l.CodSupervisor, l.Status)
		}
		if l.BrutoFarol == nil || *l.BrutoFarol != 0 || l.LiquidoFarol == nil || *l.LiquidoFarol != 0 {
			t.Errorf("linha %s deveria mostrar Bruto/Líquido = 0, não ausente: %+v", l.CodSupervisor, l)
		}
	}
}

// TestComparativoRel322_Cruzamento_PDFZeroFarolNaoZero_NaoQuebraJSON — o bug
// mais grave dos achados dos revisores: PDF=0 e Farol!=0 fazia
// percentDiffRel322 devolver +Inf, que encoding/json não sabe serializar.
// json.NewEncoder(w).Encode falhava (erro descartado) e o cliente recebia
// HTTP 200 com corpo vazio/truncado — o "comparativo parcial silencioso" que
// o spec proíbe. DiferencaPct tem que virar nil, nunca Inf, e a resposta
// inteira tem que serializar sem erro.
func TestComparativoRel322_Cruzamento_PDFZeroFarolNaoZero_NaoQuebraJSON(t *testing.T) {
	parsed := &rel322Parsed{
		PeriodoTexto: "01/08/2026 a 26/08/2026",
		DataInicio:   mustParseData(t, "2026-08-01"),
		DataFim:      mustParseData(t, "2026-08-26"),
		Linhas: []linhaExtraidaRel322{
			{CodSupervisor: "999", Descricao: "TESTE VL VENDIDO ZERO", VlVendido: 0},
		},
	}
	farol := farolFake(map[string][2]float64{
		"999": {1000.00, 900.00}, // Farol tem venda, mas o PDF mostrou 0 pra esse supervisor
	})

	resp := cruzarComRel322(farol, parsed)

	if len(resp.Linhas) != 1 {
		t.Fatalf("esperava 1 linha, veio %d", len(resp.Linhas))
	}
	l := resp.Linhas[0]
	if l.DiferencaPct != nil {
		t.Errorf("DiferencaPct deveria ser nil quando PDF=0 e Farol!=0, veio %v", *l.DiferencaPct)
	}
	if l.Status != "divergencia" {
		t.Errorf("status = %q, want divergencia (nunca ok quando a distância é infinita)", l.Status)
	}

	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("json.Marshal falhou (era exatamente o bug): %v", err)
	}
	if strings.Contains(string(b), "Inf") {
		t.Errorf("JSON contém Inf, deveria ter virado null: %s", b)
	}
}

// TestComparativoRel322_EscopoCond — a cláusula SQL que aplica o recorte por
// persona (farol_escopo.go), testável sem *sql.DB.
func TestComparativoRel322_EscopoCond(t *testing.T) {
	baseArgs := func() []any {
		return []any{"empresa-x", mustParseData(t, "2026-08-01"), mustParseData(t, "2026-08-26"), pq.Array(tipoVendaReal)}
	}

	// Sem restrição (gerente geral, diretor, admin_fbtax, TI...): nenhum
	// filtro extra, nenhum argumento extra.
	cond, args := escopoCondRel322(escopoRecorte{}, baseArgs())
	if cond != "" {
		t.Errorf("sem restrição: cond = %q, want vazio", cond)
	}
	if len(args) != 4 {
		t.Errorf("sem restrição: não deveria acrescentar argumento, veio %d", len(args))
	}

	// Persona supervisor: filtra por cod_supervisor, é o recorte natural
	// deste relatório (grão = supervisor).
	eSup := escopoRecorte{Col: "cod_supervisor", Vals: []string{"124"}, Persona: "supervisor"}
	condSup, argsSup := escopoCondRel322(eSup, baseArgs())
	if want := " AND cod_supervisor = ANY($5)"; condSup != want {
		t.Errorf("persona supervisor: cond = %q, want %q", condSup, want)
	}
	if len(argsSup) != 5 {
		t.Errorf("persona supervisor: esperava 5 argumentos, veio %d", len(argsSup))
	}

	// Persona ggv: mesma lógica, coluna diferente (cod_gerente) — segue o
	// padrão genérico de farol_relatorios.go, sem caso especial por persona.
	eGgv := escopoRecorte{Col: "cod_gerente", Vals: []string{"55"}, Persona: "ggv"}
	condGgv, _ := escopoCondRel322(eGgv, baseArgs())
	if want := " AND cod_gerente = ANY($5)"; condGgv != want {
		t.Errorf("persona ggv: cond = %q, want %q", condGgv, want)
	}

	// Negar (cod_referencia vazio no cadastro): defesa em profundidade — o
	// handler já bloqueia com 403 antes de chegar aqui, mas se algum dia essa
	// blindagem sumir, a query não pode passar a devolver tudo.
	eNegar := escopoRecorte{Col: "cod_supervisor", Negar: true, Persona: "supervisor"}
	condNegar, argsNegar := escopoCondRel322(eNegar, baseArgs())
	if want := " AND 1=0"; condNegar != want {
		t.Errorf("Negar: cond = %q, want %q (falha fechado)", condNegar, want)
	}
	if len(argsNegar) != 4 {
		t.Errorf("Negar: não deveria acrescentar argumento, veio %d", len(argsNegar))
	}
}

// TestComparativoRel322_MontarComparativoRespeitaEscopoSupervisor — teste de
// integração (precisa de banco real, mesmo padrão de biTestDB em
// farol_bi_api_test.go: sem DATABASE_URL, pula). Confirma o achado mais sério
// dos revisores: hoje, sem este teste, um usuário logado como supervisor
// (sp_role=somente_leitura, liberado por esta rota) enxergaria o
// Bruto/Líquido de TODOS os supervisores da empresa só fazendo upload de um
// PDF que os liste — exatamente o vazamento que farol_escopo.go existe pra
// fechar em /extrato-produto-cliente. Testa no nível de montarComparativo (o
// mesmo código que o handler chama) em vez de subir um PDF de verdade: quem
// já cobre o parsing do PDF são os testes de parseRel322Texto acima.
func TestComparativoRel322_MontarComparativoRespeitaEscopoSupervisor(t *testing.T) {
	db, empresaID := biTestDB(t)

	rows, err := db.Query(`
		SELECT DISTINCT cod_supervisor FROM vendas_faturadas
		 WHERE empresa_id = $1 AND cod_supervisor <> '' LIMIT 2`, empresaID)
	if err != nil {
		t.Fatalf("consulta de cod_supervisor: %v", err)
	}
	var cods []string
	for rows.Next() {
		var c string
		if rows.Scan(&c) == nil {
			cods = append(cods, c)
		}
	}
	rows.Close()
	if len(cods) < 2 {
		t.Skip("banco de teste não tem 2 cod_supervisor distintos com venda — pulado")
	}
	meu, outro := cods[0], cods[1]

	parsed := &rel322Parsed{
		PeriodoTexto: "01/01/2020 a 31/12/2030",
		DataInicio:   mustParseData(t, "2020-01-01"),
		DataFim:      mustParseData(t, "2030-12-31"),
		Linhas: []linhaExtraidaRel322{
			{CodSupervisor: meu, Descricao: "MEU", VlVendido: 1},
			{CodSupervisor: outro, Descricao: "OUTRO", VlVendido: 1},
		},
	}
	escopo := escopoRecorte{Col: "cod_supervisor", Vals: []string{meu}, Persona: "supervisor"}

	resp, err := montarComparativo(context.Background(), db, empresaID, parsed, escopo)
	if err != nil {
		t.Fatalf("montarComparativo: %v", err)
	}
	for _, l := range resp.Linhas {
		if l.CodSupervisor != outro {
			continue
		}
		if l.BrutoFarol != nil || l.LiquidoFarol != nil {
			t.Errorf("vazou Bruto/Líquido do supervisor %s, fora do escopo de %s: %+v", outro, meu, l)
		}
	}
}

func mustParseData(t *testing.T, s string) time.Time {
	t.Helper()
	tm, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatalf("data de teste inválida %q: %v", s, err)
	}
	return tm
}

// tinyPNGRel322 — logo mínima válida (1x1) para exercitar
// comparativoRel322PDF sem depender do banco (logoRelatorio lê
// companies.logo_data).
func tinyPNGRel322(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("falha ao gerar PNG de teste: %v", err)
	}
	return buf.Bytes()
}

// TestComparativoRel322_PDF_Normal — I/O Matrix: "PDF do comparativo normal".
// Cobre linha ok, divergência e órfã (dos dois lados) na mesma tabela —
// devolve as mesmas linhas/status que o JSON equivalente, sem erro.
func TestComparativoRel322_PDF_Normal(t *testing.T) {
	resultado := &comparativoRel322Resposta{
		Periodo:            "01/08/2026 a 26/08/2026",
		DataInicio:         "2026-08-01",
		DataFim:            "2026-08-26",
		QtdSupervisoresPDF: 2,
		QtdDivergencias:    1,
		QtdOrfaos:          2,
		Linhas: []linhaComparativoRel322{
			{
				CodSupervisor: "124", Supervisor: "GO - VALE SAO PATRICIO - LUCAS",
				VlVendidoPDF: f64p(9223911.54), BrutoFarol: f64p(9223911.54), LiquidoFarol: f64p(9000000),
				DiferencaPct: f64p(0), Status: "ok", Origem: "ambos",
			},
			{
				CodSupervisor: "240", Supervisor: "GO - NORTE - JOSENILTON",
				VlVendidoPDF: f64p(100000), BrutoFarol: f64p(90000), LiquidoFarol: f64p(89000),
				DiferencaPct: f64p(10), Status: "divergencia", Origem: "ambos",
			},
			{
				CodSupervisor: "705", Supervisor: "SUPERVISOR SO NO PDF",
				VlVendidoPDF: f64p(1000), Status: "orfao", Origem: "pdf",
			},
			{
				CodSupervisor: "999",
				BrutoFarol:    f64p(5000), LiquidoFarol: f64p(5000), Status: "orfao", Origem: "farol",
			},
		},
	}

	b, err := comparativoRel322PDF(resultado, tinyPNGRel322(t), extension.Png)
	if err != nil {
		t.Fatalf("comparativoRel322PDF: erro inesperado: %v", err)
	}
	if len(b) == 0 {
		t.Fatal("PDF vazio")
	}
	if !bytes.HasPrefix(b, []byte("%PDF")) {
		t.Errorf("saída não parece um PDF válido (assinatura ausente): %q", b[:min(20, len(b))])
	}

	// O Verification Gap provou que checar só "gerou sem erro" deixa passar
	// até deletar o loop de linhas inteiro. extrairTextoPDF (a mesma função
	// que lê o PDF do WinThor) lê de volta o PDF que ACABAMOS de gerar —
	// confirmado rodando de verdade que ela funciona nesse sentido também —
	// e o texto extraído precisa refletir cada linha/status do resultado.
	texto, err := extrairTextoPDF(b)
	if err != nil {
		t.Fatalf("extrairTextoPDF no PDF recém-gerado: %v", err)
	}
	for _, cod := range []string{"124", "240", "705", "999"} {
		if !strings.Contains(texto, cod) {
			t.Errorf("texto extraído não contém o código de supervisor %s: %q", cod, texto)
		}
	}
	for _, esperado := range []string{"OK", "DIVERGÊNCIA", "ÓRFÃ"} {
		if !strings.Contains(texto, esperado) {
			t.Errorf("texto extraído não contém o status %q: %q", esperado, texto)
		}
	}
	// Linha 999 (órfã só-farol): VlVendidoPDF e DiferencaPct são nil →
	// precisam aparecer como "—", nunca R$ 0,00 nem "+Inf%"/"NaN%".
	if !strings.Contains(texto, "—") {
		t.Errorf("texto extraído não contém o traço \"—\" esperado para os campos ausentes: %q", texto)
	}
}

// TestComparativoRel322_PDF_SemDadoNoPeriodo — I/O Matrix: "Sem dado do
// Farol no período". O PDF ainda é gerado, com a ressalva no topo — nunca
// vira erro.
func TestComparativoRel322_PDF_SemDadoNoPeriodo(t *testing.T) {
	resultado := &comparativoRel322Resposta{
		Periodo:               "01/01/2099 a 31/01/2099",
		DataInicio:            "2099-01-01",
		DataFim:               "2099-01-31",
		QtdSupervisoresPDF:    2,
		QtdDivergencias:       2,
		QtdOrfaos:             0,
		SemDadoFarolNoPeriodo: true,
		Linhas: []linhaComparativoRel322{
			{
				CodSupervisor: "124", Supervisor: "GO - VALE SAO PATRICIO - LUCAS",
				VlVendidoPDF: f64p(9223911.54), BrutoFarol: f64p(0), LiquidoFarol: f64p(0),
				DiferencaPct: nil, Status: "divergencia", Origem: "ambos",
			},
			{
				CodSupervisor: "240", Supervisor: "GO - NORTE - JOSENILTON",
				VlVendidoPDF: f64p(8597443.46), BrutoFarol: f64p(0), LiquidoFarol: f64p(0),
				DiferencaPct: nil, Status: "divergencia", Origem: "ambos",
			},
		},
	}

	b, err := comparativoRel322PDF(resultado, tinyPNGRel322(t), extension.Png)
	if err != nil {
		t.Fatalf("comparativoRel322PDF: esperava PDF gerado mesmo sem dado do Farol no período, veio erro: %v", err)
	}
	if len(b) == 0 {
		t.Fatal("PDF vazio")
	}
	if !bytes.HasPrefix(b, []byte("%PDF")) {
		t.Errorf("saída não parece um PDF válido (assinatura ausente): %q", b[:min(20, len(b))])
	}

	// Acceptance Criteria da spec: "quando sem_dado_farol_no_periodo, o PDF
	// inclui a ressalva no topo". Sem ler o conteúdo de volta, nada garante
	// que a ressalva realmente foi escrita (só checar "gerou sem erro" não
	// prova isso — é exatamente o gap que o Verification Gap apontou).
	texto, err := extrairTextoPDF(b)
	if err != nil {
		t.Fatalf("extrairTextoPDF no PDF recém-gerado: %v", err)
	}
	if !strings.Contains(texto, "PARCIAL") {
		t.Errorf("texto extraído não contém a ressalva \"PARCIAL\": %q", texto)
	}
	if !strings.Contains(texto, "NENHUM dado importado") {
		t.Errorf("texto extraído não contém a frase da ressalva sobre não haver dado importado: %q", texto)
	}
}

func TestComparativoRel322_PercentDiff(t *testing.T) {
	if got := percentDiffRel322(100, 100); got != 0 {
		t.Errorf("iguais deveria dar 0%%, veio %v", got)
	}
	if got := percentDiffRel322(0, 0); got != 0 {
		t.Errorf("PDF=0 e Farol=0 deveria dar 0%%, veio %v", got)
	}
	if got := percentDiffRel322(100, 0); !math.IsInf(got, 1) {
		t.Errorf("PDF=0 e Farol!=0 deveria ser divergência máxima, veio %v", got)
	}
	if got := percentDiffRel322(100.4, 100); got > 0.005 {
		t.Errorf("0,4%% de diferença deveria estar dentro da tolerância de 0,5%%, veio %v", got)
	}
	if got := percentDiffRel322(101, 100); got <= 0.005 {
		t.Errorf("1%% de diferença deveria estourar a tolerância de 0,5%%, veio %v", got)
	}
}

// TestComparativoRel322_PctOuTraco_InfNaN — defesa em profundidade.
// cruzarComRel322 já garante que DiferencaPct vira nil (não +Inf) quando a
// distância é infinita, mas pctOuTracoRel322 não pode confiar cegamente
// nisso: se um ponteiro não-nil só NaN/Inf chegasse até ela (hoje ou por uma
// mudança futura em cruzarComRel322), formatar direto imprimiria
// literalmente "+Inf%"/"NaN%" numa linha do PDF que circula por e-mail.
func TestComparativoRel322_PctOuTraco_InfNaN(t *testing.T) {
	inf := math.Inf(1)
	if got := pctOuTracoRel322(&inf); got != "—" {
		t.Errorf("pctOuTracoRel322(+Inf) = %q, want \"—\"", got)
	}
	infNeg := math.Inf(-1)
	if got := pctOuTracoRel322(&infNeg); got != "—" {
		t.Errorf("pctOuTracoRel322(-Inf) = %q, want \"—\"", got)
	}
	nan := math.NaN()
	if got := pctOuTracoRel322(&nan); got != "—" {
		t.Errorf("pctOuTracoRel322(NaN) = %q, want \"—\"", got)
	}
	if got := pctOuTracoRel322(nil); got != "—" {
		t.Errorf("pctOuTracoRel322(nil) = %q, want \"—\"", got)
	}
	dez := 10.0
	if got := pctOuTracoRel322(&dez); got != "10,00%" {
		t.Errorf("pctOuTracoRel322(10) = %q, want \"10,00%%\"", got)
	}
}

// ─── PDF de fixture para os testes de handler ──────────────────────────────
//
// O handler precisa de um upload de verdade: extrairTextoPDF lê BYTES DE PDF
// reais (github.com/ledongthuc/pdf.NewReader), não o texto sintético que
// alimenta parseRel322Texto nos testes acima. construirPDFRel322Fixture usa
// o MESMO maroto que comparativoRel322PDF (já confirmado compatível com
// extrairTextoPDF) para desenhar cada linha da fixture como uma linha de PDF
// própria — reproduzindo o "um token por linha" que o parser espera.
func construirPDFRel322Fixture(t *testing.T, linhas []string) []byte {
	t.Helper()
	cfg := config.NewBuilder().Build()
	mrt := maroto.New(cfg)
	pg := page.New()
	for _, l := range linhas {
		pg.Add(row.New(4).Add(col.New(12).Add(text.New(l, props.Text{Size: 8}))))
	}
	mrt.AddPages(pg)
	doc, err := mrt.Generate()
	if err != nil {
		t.Fatalf("construirPDFRel322Fixture: gerar PDF: %v", err)
	}
	return doc.GetBytes()
}

// linhasRel322FixtureCompleta — cabeçalho + uma linha de supervisor + totais,
// já achatado em uma linha por token (mesmo formato que cabecalhoRel322 /
// linhaRel322Fixture / totaisRel322Fixture produzem quando unidos por "\n").
func linhasRel322FixtureCompleta(periodo string) []string {
	texto := strings.Join([]string{
		cabecalhoRel322(periodo, "14-Por Supervisor"),
		linhaRel322Fixture("124", "GO - VALE SAO PATRICIO - LUCAS", "9.223.911,54", "7,10"),
		totaisRel322Fixture,
	}, "\n")
	var linhas []string
	for _, l := range strings.Split(texto, "\n") {
		if strings.TrimSpace(l) != "" {
			linhas = append(linhas, l)
		}
	}
	return linhas
}

// multipartPDFReqRel322 — monta uma request POST multipart de verdade (campo
// "file"), já autenticada via FarolContext injetado direto no contexto
// (mesmo atalho de biReq em farol_bi_api_test.go — evita depender de
// login/JWT real para exercitar o handler).
func multipartPDFReqRel322(t *testing.T, url string, pdfBytes []byte, empresaID string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", "rel322.pdf")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := fw.Write(pdfBytes); err != nil {
		t.Fatalf("escrever bytes do PDF no multipart: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("fechar multipart writer: %v", err)
	}
	r := httptest.NewRequest(http.MethodPost, url, &buf)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	ctx := context.WithValue(r.Context(), SpContextKey, &FarolContext{
		UserID: "teste", SpRole: "gestor_geral", EmpresaID: empresaID,
		AllFiliais: true, Modulos: []string{"vendas"},
	})
	return r.WithContext(ctx)
}

// TestComparativoRel322_Handler_FormatoPDF — ninguém testava o handler HTTP
// de verdade com ?formato=pdf, só a função pura comparativoRel322PDF: trocar
// o nome do parâmetro, inverter a comparação, ou deixar o
// Content-Type: application/json vazando pro caminho do PDF não quebraria
// teste nenhum antes deste. Integração real: precisa de *sql.DB (o handler
// chama logoRelatorio/montarComparativo), mesmo padrão biTestDB — pula sem
// DATABASE_URL.
func TestComparativoRel322_Handler_FormatoPDF(t *testing.T) {
	db, empresaID := biTestDB(t)

	periodo := "01/08/2026 a 26/08/2026"
	pdfBytes := construirPDFRel322Fixture(t, linhasRel322FixtureCompleta(periodo))

	r := multipartPDFReqRel322(t, "/api/v2/farol/relatorio/comparativo-rel322?formato=pdf", pdfBytes, empresaID)
	w := httptest.NewRecorder()
	ComparativoRel322Handler(db)(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, corpo: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/pdf" {
		t.Errorf("Content-Type = %q, want application/pdf", ct)
	}
	if !bytes.HasPrefix(w.Body.Bytes(), []byte("%PDF")) {
		t.Errorf("corpo da resposta não começa com %%PDF")
	}
	cd := w.Header().Get("Content-Disposition")
	if !strings.Contains(cd, "2026-08-01") || !strings.Contains(cd, "2026-08-26") {
		t.Errorf("Content-Disposition sem o período do PDF de origem: %q", cd)
	}
}

// TestComparativoRel322_Handler_FormatoJSONPadrao — o "Never" da spec: sem
// ?formato (ou formato diferente de pdf), o contrato antigo não pode
// quebrar. Mesmo upload do teste acima, sem a querystring.
func TestComparativoRel322_Handler_FormatoJSONPadrao(t *testing.T) {
	db, empresaID := biTestDB(t)

	periodo := "01/08/2026 a 26/08/2026"
	pdfBytes := construirPDFRel322Fixture(t, linhasRel322FixtureCompleta(periodo))

	r := multipartPDFReqRel322(t, "/api/v2/farol/relatorio/comparativo-rel322", pdfBytes, empresaID)
	w := httptest.NewRecorder()
	ComparativoRel322Handler(db)(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, corpo: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json (contrato antigo não pode quebrar)", ct)
	}
	var resp comparativoRel322Resposta
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("corpo não é JSON válido: %v — corpo: %s", err, w.Body.String())
	}
	if resp.Periodo != periodo {
		t.Errorf("periodo = %q, want %q", resp.Periodo, periodo)
	}
	if len(resp.Linhas) == 0 {
		t.Error("esperava ao menos 1 linha no comparativo")
	}
}
