package handlers

import (
	"testing"
	"time"
)

// abrirGateFilial / fecharGateFilial — controlam aggFilialReady sem tocar no
// banco. Com o gate fechado é preciso marcar CheckedAt, senão a função tenta a
// sonda e panica com db nil.
func abrirGateFilial(t *testing.T) {
	t.Helper()
	aggFilialReadyMu.Lock()
	aggFilialReadyVal = true
	aggFilialReadyMu.Unlock()
	t.Cleanup(func() { fecharGateFilial(t) })
}

func fecharGateFilial(t *testing.T) {
	t.Helper()
	aggFilialReadyMu.Lock()
	aggFilialReadyVal = false
	aggFilialReadyCheckedAt = time.Now()
	aggFilialReadyMu.Unlock()
}

// Roteamento do filtro cruzado de FILIAL para as aggs V10/V11 (mig 199).
//
// O caso que mais importa aqui é o de DUAS filiais: queryAggregatedMes SOMA
// positivados entre as linhas do grão, e como 23% dos clientes compram de mais
// de uma filial, essa soma contaria o mesmo CNPJ duas vezes. Com 2+ filiais o
// pickAgg tem que RECUSAR a agg e deixar a query cair no scan de vendas_*.
func TestPickAggForCrossFilterFilial(t *testing.T) {
	fat := fluxoCtx{name: "faturado"}

	// V08/V09 fechadas: isola o roteamento de filial do de UF.
	aggUFReadyMu.Lock()
	aggUFReadyVal = false
	aggUFReadyCheckedAt = time.Now()
	aggUFReadyMu.Unlock()

	abrirGateFilial(t)

	umaFilial := multiFilters{"empresa": {"11"}}

	casos := []struct {
		nome     string
		groupCol string
		drill    []drillStep
		filters  multiFilters
		want     string
		wantOK   bool
	}{
		{"Filial + Indústrias", "cod_fornec", nil, umaFilial, "farol.agg_fat_v11_l1_mes", true},
		{"Filial + Por Gerência", "cod_gerente", nil, umaFilial, "farol.agg_fat_v10_l1_mes", true},
		{"Filial + Por Equipe (supervisor)", "cod_supervisor", nil, umaFilial, "farol.agg_fat_v10_l2_mes", true},
		{"Filial + Por RCA", "cod_rca", nil, umaFilial, "farol.agg_fat_v10_l3_mes", true},
		{"Filial + drill fornec → gerente", "cod_gerente",
			[]drillStep{{Level: "cod_fornec", Value: "F1"}}, umaFilial, "farol.agg_fat_v11_l2_mes", true},
		// Nenhuma V10/V11 tem cod_cli → segue no scan.
		{"Filial + cliente → sem agg", "cod_cli", nil, umaFilial, "", false},
		// ── O guard do double-count ──────────────────────────────────────────
		{"DUAS filiais → tem que recusar a agg", "cod_fornec", nil,
			multiFilters{"empresa": {"11", "20"}}, "", false},
		{"TRÊS filiais → idem", "cod_gerente", nil,
			multiFilters{"empresa": {"11", "20", "32"}}, "", false},
	}

	for _, c := range casos {
		got, ok := pickAggForCrossFilter(nil, fat, c.groupCol, c.drill, c.filters)
		if got != c.want || ok != c.wantOK {
			t.Errorf("%s: pickAgg = (%q, %v), esperado (%q, %v)", c.nome, got, ok, c.want, c.wantOK)
		}
	}

	// Gate fechado (tabelas ainda vazias): nem com uma filial pode rotear.
	fecharGateFilial(t)
	if got, ok := pickAggForCrossFilter(nil, fat, "cod_fornec", nil, umaFilial); ok {
		t.Errorf("gate fechado: não podia rotear, veio %q", got)
	}
}

// leafForPositivados (mig 200) — QUAL folha conta CNPJ distinto.
//
// O bug que motivou isto: com filtro de filial nenhuma folha de V01-V07 tem
// `empresa`, a resolução falhava, o override de "Clientes Ativos" não rodava e
// a tela mostrava a CARTEIRA SOMADA — 166.572 numa base de 37.719 clientes,
// com positivação 0% em tudo.
func TestLeafForPositivadosComFiltroDeFilial(t *testing.T) {
	fat := fluxoCtx{name: "faturado"}
	tr := fluxoCtx{name: "transmitido"}
	filial := multiFilters{"empresa": {"11"}}

	casos := []struct {
		nome     string
		fluxo    fluxoCtx
		view     string
		groupCol string
		filters  multiFilters
		want     string
		wantOK   bool
	}{
		// Sem filtro de filial nada muda: continua na folha da própria view.
		{"V01 sem filtro", fat, "V01", "cod_fornec", nil, "farol.agg_fat_v01_l4_mes", true},
		// Com filial, cai na folha da V11 — que tem `empresa` E `cnpj`.
		{"V01 + filial", fat, "V01", "cod_fornec", filial, "farol.agg_fat_v11_l5_mes", true},
		{"Por Gerência + filial", fat, "V03", "cod_gerente", filial, "farol.agg_fat_v11_l5_mes", true},
		{"Por Equipe + filial", fat, "V02", "cod_supervisor", filial, "farol.agg_fat_v11_l5_mes", true},
		{"nível Cliente + filial", fat, "V01", "cod_cli", filial, "farol.agg_fat_v11_l5_mes", true},
		{"transmitido + filial", tr, "V01", "cod_fornec", filial, "farol.agg_trans_v11_l5_mes", true},
		// DUAS filiais também servem: COUNT(DISTINCT cnpj) deduplica sozinho.
		// (o guard de uma-filial-só é do pickAgg, que SOMA linhas do grão.)
		{"duas filiais ainda contam certo", fat, "V01", "cod_fornec",
			multiFilters{"empresa": {"11", "20"}}, "farol.agg_fat_v11_l5_mes", true},
		// A V11 não tem `uf`: filtro combinado não tem folha que sirva.
		{"filial + UF → nenhuma folha", fat, "V01", "cod_fornec",
			multiFilters{"empresa": {"11"}, "uf": {"BA"}}, "", false},
		// V06/V07 não têm positivação.
		{"V06 não tem positivação", fat, "V06", "cod_cliprinc", filial, "", false},
	}

	for _, c := range casos {
		got, ok := leafForPositivados(c.fluxo, c.view, c.groupCol, nil, c.filters)
		if got != c.want || ok != c.wantOK {
			t.Errorf("%s: leafForPositivados = (%q, %v), esperado (%q, %v)", c.nome, got, ok, c.want, c.wantOK)
		}
	}
}

// A expressão de positivados precisa SOMAR entre as linhas do grão nas V10/V11,
// como já faz nas V08/V09 — o AVG dividiria pelo número de filiais. É o que
// torna o guard de uma-filial-só obrigatório: sem ele, a soma infla.
func TestPositivadosSomaNasAggsDeFilial(t *testing.T) {
	for _, tbl := range []string{
		"agg_fat_v10_l0_mes", "agg_fat_v11_l1_mes",
		"agg_trans_v10_l3_mes", "agg_trans_v11_l4_mes",
	} {
		if !aggHasMixTotal[tbl] {
			t.Errorf("%s: precisa estar em aggHasMixTotal, senão pickAgg nunca a escolhe", tbl)
		}
	}
}
