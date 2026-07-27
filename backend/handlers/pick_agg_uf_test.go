package handlers

import (
	"testing"
	"time"
)

// nome_cliprinc (V06 "Por Rede") não existe em vendas_*; é derivada na
// consolidação. Sem a tradução, o filtro cruzado em Por Rede quebrava a query
// e devolvia zero cards em silêncio (produção 27/07/2026).
func TestScanLabelExpr(t *testing.T) {
	if got := scanLabelExpr("nome_cliprinc"); got != `COALESCE(NULLIF(MAX(v.fantasia), ''), MAX(v.nome_cli))` {
		t.Errorf("nome_cliprinc deve virar a expressão derivada, veio %q", got)
	}
	for _, col := range []string{"nome_fornec", "nome_rca", "nome_cli", "depto"} {
		if got, want := scanLabelExpr(col), "MAX(v."+col+")"; got != want {
			t.Errorf("scanLabelExpr(%q) = %q, esperado %q", col, got, want)
		}
	}
}

// Cobre o roteamento do filtro cruzado de UF para as aggs V08/V09 (mig 197):
// com o gate aberto, cada agrupamento comum deve cair na tabela certa; sem
// UF no filtro, nada muda; com o gate fechado, V08/V09 ficam invisíveis.
func TestPickAggForCrossFilterUF(t *testing.T) {
	fat := fluxoCtx{name: "faturado"}

	setGate := func(open bool) {
		aggUFReadyMu.Lock()
		aggUFReadyVal = open
		if !open {
			aggUFReadyCheckedAt = time.Now() // evita probe com db nil
		}
		aggUFReadyMu.Unlock()
	}
	setGate(true)
	defer setGate(false)

	uf := multiFilters{"uf": {"BA", "GO"}}

	cases := []struct {
		nome     string
		groupCol string
		drill    []drillStep
		filters  multiFilters
		want     string
		wantOK   bool
	}{
		{"UF + Indústrias (o caso nº1)", "cod_fornec", nil, uf, "farol.agg_fat_v09_l1_mes", true},
		{"UF + Por Gerência", "cod_gerente", nil, uf, "farol.agg_fat_v08_l1_mes", true},
		{"UF + Por Equipe (supervisor)", "cod_supervisor", nil, uf, "farol.agg_fat_v08_l2_mes", true},
		{"UF + Por RCA", "cod_rca", nil, uf, "farol.agg_fat_v08_l3_mes", true},
		{"UF + drill fornec → gerente", "cod_gerente",
			[]drillStep{{Level: "cod_fornec", Value: "F1"}}, uf, "farol.agg_fat_v09_l2_mes", true},
		{"UF + drill fornec/ger/sup → rca", "cod_rca",
			[]drillStep{{Level: "cod_fornec", Value: "F1"}, {Level: "cod_gerente", Value: "G1"}, {Level: "cod_supervisor", Value: "S1"}},
			uf, "farol.agg_fat_v09_l4_mes", true},
		// Sem UF: rota pré-existente intacta (fornec em Por Equipe → v01_l2,
		// a tabela com folha=supervisor que contém cod_fornec)
		{"fornec + supervisor (rota antiga)", "cod_supervisor", nil,
			multiFilters{"cod_fornec": {"F1"}}, "farol.agg_fat_v01_l2_mes", true},
		// UF + cliente: nenhuma V08/V09 tem cod_cli → segue sem agg (scan)
		{"UF + cliente → sem agg", "cod_cli", nil, uf, "", false},
	}

	for _, c := range cases {
		got, ok := pickAggForCrossFilter(nil, fat, c.groupCol, c.drill, c.filters)
		if got != c.want || ok != c.wantOK {
			t.Errorf("%s: pickAgg = (%q, %v), esperado (%q, %v)", c.nome, got, ok, c.want, c.wantOK)
		}
	}

	// Gate fechado (tabelas ainda vazias): UF não pode rotear pra V08/V09.
	setGate(false)
	if got, ok := pickAggForCrossFilter(nil, fat, "cod_fornec", nil, uf); ok {
		t.Errorf("gate fechado: esperado fallback pro scan, veio %q", got)
	}
}
