package handlers

// farol_comparativo_rel322_vm_test.go — testes SEM Oracle (o ambiente de dev
// não tem JC_ORACLE_USER/PASS): cobrem só a montagem da query
// (montarQueryVM), que é pura.
//
// TestMontarQueryVM_PlaceholdersEmOrdemCrescente é o teste que teria
// capturado o bug real de produção (ORA-00932: "expression (:1) is of data
// type TIMESTAMP WITH TIME ZONE, which is incompatible with expected data
// type NUMBER") ANTES do deploy: o driver go-ora, com args posicionais
// simples, não remapeia bind por nome — associa cada ":N" ao N-ésimo valor
// de args NA ORDEM EM QUE OS PLACEHOLDERS APARECEM NO TEXTO. Qualquer
// bind() chamado fora de ordem (ex.: os binds de data, calculados antes no
// código Go, mas aparecendo DEPOIS no texto final, dentro do WHERE) quebra
// essa invariante silenciosamente — só estoura em runtime, contra o Oracle
// real, com um erro de tipo que não aponta pra causa.

import (
	"regexp"
	"strconv"
	"testing"
	"time"
)

var reBindVM = regexp.MustCompile(`:(\d+)`)

// placeholdersEmOrdem — extrai todos os ":N" do texto, na ordem em que
// aparecem, e confere que formam a sequência estritamente crescente
// 1,2,3,...,len(args) — a única ordem que o driver interpreta corretamente
// pra args posicionais simples (ver comentário grande em montarQueryVM).
func placeholdersEmOrdem(t *testing.T, q string, args []any) {
	t.Helper()
	matches := reBindVM.FindAllStringSubmatch(q, -1)
	if len(matches) != len(args) {
		t.Fatalf("query tem %d placeholders ':N', mas args tem %d valores — precisam ser iguais: %s", len(matches), len(args), q)
	}
	for i, m := range matches {
		n, err := strconv.Atoi(m[1])
		if err != nil {
			t.Fatalf("placeholder %q não é numérico: %v", m[0], err)
		}
		want := i + 1
		if n != want {
			t.Errorf("placeholder na posição textual %d é :%d, want :%d (ordem de aparição no texto tem que bater com a ordem de args) — query: %s", i, n, want, q)
		}
	}
}

func TestMontarQueryVM_PlaceholdersEmOrdemCrescente(t *testing.T) {
	dataInicio := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	dataFim := time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC)

	casos := []struct {
		nome       string
		fluxo      fluxoCtx
		filiais    []string
		tiposVenda []string
		escopo     escopoRecorte
	}{
		{"faturado, sem filtro nenhum", resolveFluxo("faturado"), nil, tipoVendaReal, escopoRecorte{}},
		{"faturado, com filial e escopo", resolveFluxo("faturado"), []string{"10", "11"}, tipoVendaReal, escopoRecorte{Col: "cod_supervisor", Vals: []string{"124"}}},
		{"faturado, tipo_venda selecionado pelo usuário", resolveFluxo("faturado"), nil, []string{"5", "10"}, escopoRecorte{}},
		{"transmitido, sem filtro nenhum (default: sem tipo_venda)", resolveFluxo("transmitido"), nil, nil, escopoRecorte{}},
		{"transmitido, com filial", resolveFluxo("transmitido"), []string{"20"}, nil, escopoRecorte{}},
		{"transmitido, tipo_venda selecionado pelo usuário", resolveFluxo("transmitido"), nil, []string{"1", "4", "7"}, escopoRecorte{}},
		{"transmitido, tudo junto (filial + tipo_venda + escopo)", resolveFluxo("transmitido"), []string{"10", "11", "13"}, []string{"1"}, escopoRecorte{Col: "cod_gerente", Vals: []string{"55", "56"}}},
		{"escopo restrito sem nenhum valor liberado (falha fechado)", resolveFluxo("faturado"), nil, tipoVendaReal, escopoRecorte{Col: "cod_rca", Vals: nil}},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			q, args := montarQueryVM("IAUSER.COMPRAS_FAROL_VW", dataInicio, dataFim, c.filiais, c.tiposVenda, c.escopo, c.fluxo)
			placeholdersEmOrdem(t, q, args)
		})
	}
}
