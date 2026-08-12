package handlers

import (
	"testing"
	"time"
)

// A carga diária toca só o mês corrente. O histórico já aquecido (2025, início
// de 2026) precisa SOBREVIVER — reconstruí-lo custa 10-25s por view. Mas tudo
// que encosta no período carregado tem de cair, senão o painel serve número
// velho.
func TestInvalidateBaseCacheMesesPreservaHistorico(t *testing.T) {
	const emp = "11111111-1111-1111-1111-111111111111"
	const outra = "22222222-2222-2222-2222-222222222222"

	// (chave, deve sobreviver a uma carga de julho/2026)
	casos := []struct {
		key       string
		sobrevive bool
		porque    string
	}{
		{emp + "|faturado|V01|cod_fornec|202501-202512||", true, "ano de 2025 não é tocado por carga de jul/2026"},
		{emp + "|faturado|V01|cod_fornec|202601-202605||", true, "jan-mai/2026 fecha antes de julho"},
		{emp + "|faturado|V01|cod_fornec|202606-202606||", true, "junho fechado, anterior a julho"},
		{emp + "|faturado|V01|cod_fornec|202601-202607||", false, "YTD inclui julho"},
		{emp + "|faturado|V01|cod_fornec|202607-202607||", false, "é o próprio mês carregado"},
		// Positivados é COUNT(DISTINCT cnpj) escopado ao período: uma entrada de
		// agosto lê só linhas de agosto, que a carga de julho não tocou.
		{emp + "|faturado|V01|cod_fornec|202608-202608||", true, "agosto não é afetado por carga de julho"},
		{emp + "|faturado|V01|cod_fornec|0-999912||", false, "histórico completo sempre inclui o mês novo"},
		{emp + "|faturado|V01|cod_fornec|lixo||", false, "chave ilegível → invalida por precaução"},
		{outra + "|faturado|V01|cod_fornec|202607-202607||", true, "outra empresa não é afetada"},
	}

	baseCacheMu.Lock()
	baseCache = map[string]baseCacheEntry{}
	for _, c := range casos {
		baseCache[c.key] = baseCacheEntry{data: map[string]int{"x": 1}, at: time.Now()}
	}
	baseCacheMu.Unlock()

	invalidateBaseCacheMeses(emp, 202607, 202607)

	baseCacheMu.RLock()
	defer baseCacheMu.RUnlock()
	for _, c := range casos {
		_, existe := baseCache[c.key]
		if existe != c.sobrevive {
			t.Errorf("chave %q: sobreviveu=%v, esperado %v (%s)", c.key, existe, c.sobrevive, c.porque)
		}
	}
}

// Mesmo raciocínio de TestInvalidateBaseCacheMesesPreservaHistorico, mas para
// o cache de queryAggregatedMes — a chave tem um campo a mais (aggName), mas
// a posição do ymRange (índice 4) é a mesma.
func TestInvalidateAggMesCacheMesesPreservaHistorico(t *testing.T) {
	const emp = "11111111-1111-1111-1111-111111111111"
	const outra = "22222222-2222-2222-2222-222222222222"

	casos := []struct {
		key       string
		sobrevive bool
		porque    string
	}{
		{emp + "|faturado|farol.agg_fat_v06_l0_mes|cod_cliprinc|202501-202512||", true, "2025 não é tocado por carga de jul/2026"},
		{emp + "|faturado|farol.agg_fat_v06_l0_mes|cod_cliprinc|202601-202607||", false, "YTD inclui julho"},
		{emp + "|faturado|farol.agg_fat_v06_l0_mes|cod_cliprinc|202607-202607||", false, "é o próprio mês carregado"},
		{emp + "|faturado|farol.agg_fat_v06_l0_mes|cod_cliprinc|202608-202608||", true, "agosto não é afetado por carga de julho"},
		// filtros com VALOR na chave (diferente de baseCacheKey) — a posição
		// do ymRange não muda, então a invalidação segue funcionando igual.
		{emp + "|faturado|farol.agg_fat_v01_l0_mes|cod_fornec|202501-202512||cod_fornec=F01;", true, "2025 não é tocado, com filtro aplicado"},
		{emp + "|faturado|farol.agg_fat_v01_l0_mes|cod_fornec|202607-202607||cod_fornec=F01;", false, "mês carregado, com filtro aplicado"},
		{outra + "|faturado|farol.agg_fat_v06_l0_mes|cod_cliprinc|202607-202607||", true, "outra empresa não é afetada"},
	}

	aggMesCacheMu.Lock()
	aggMesCache = map[string]aggMesCacheEntry{}
	for _, c := range casos {
		aggMesCache[c.key] = aggMesCacheEntry{data: map[string]aggResult{}, at: time.Now()}
	}
	aggMesCacheMu.Unlock()

	invalidateAggMesCacheMeses(emp, 202607, 202607)

	aggMesCacheMu.RLock()
	defer aggMesCacheMu.RUnlock()
	for _, c := range casos {
		_, existe := aggMesCache[c.key]
		if existe != c.sobrevive {
			t.Errorf("chave %q: sobreviveu=%v, esperado %v (%s)", c.key, existe, c.sobrevive, c.porque)
		}
	}
}

// Duas seleções de filtro diferentes não podem colidir na mesma chave —
// diferente de baseCacheKey/vendasPeriodoCacheKey, que só logam nomes.
func TestAggMesCacheKeyDistingueValorDoFiltro(t *testing.T) {
	k1 := aggMesCacheKey("emp", "faturado", "farol.agg_fat_v01_l0_mes", "cod_gerente",
		202601, 202608, nil, multiFilters{"cod_fornec": {"F01"}})
	k2 := aggMesCacheKey("emp", "faturado", "farol.agg_fat_v01_l0_mes", "cod_gerente",
		202601, 202608, nil, multiFilters{"cod_fornec": {"F02"}})
	if k1 == k2 {
		t.Errorf("chaves iguais para filtros diferentes: %q", k1)
	}
}

// baseCacheKey/vendasPeriodoCacheKey usavam filters.names() (só a coluna),
// então Filial=11 e Filial=20 caíam na MESMA entrada — a segunda leitura
// devolvia o resultado da primeira. Motivou prewarmFilialCache: aquecer com
// uma filial só faria sentido se o cache diferenciasse por valor.
func TestBaseCacheKeyDistingueValorDoFiltro(t *testing.T) {
	k1 := baseCacheKey("emp", "faturado", "V01", "cod_fornec",
		0, 999912, nil, multiFilters{"empresa": {"11"}})
	k2 := baseCacheKey("emp", "faturado", "V01", "cod_fornec",
		0, 999912, nil, multiFilters{"empresa": {"20"}})
	if k1 == k2 {
		t.Errorf("chaves iguais para filiais diferentes: %q", k1)
	}
}

func TestVendasPeriodoCacheKeyDistingueValorDoFiltro(t *testing.T) {
	ini := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	fim := time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC)
	k1 := vendasPeriodoCacheKey("emp", "faturado", "cod_fornec", ini, fim, nil, multiFilters{"empresa": {"11"}})
	k2 := vendasPeriodoCacheKey("emp", "faturado", "cod_fornec", ini, fim, nil, multiFilters{"empresa": {"20"}})
	if k1 == k2 {
		t.Errorf("chaves iguais para filiais diferentes: %q", k1)
	}
}

// A posição do ymRange (índice 4, usada por invalidateBaseCacheMeses) não
// pode mudar mesmo com filtro tendo valor agora — senão a invalidação por
// mês quebra silenciosamente pra qualquer chave com filtro.
func TestBaseCacheKeyComFiltroPreservaPosicaoDoYmRange(t *testing.T) {
	const emp = "11111111-1111-1111-1111-111111111111"
	key := baseCacheKey(emp, "faturado", "V01", "cod_fornec",
		202607, 202607, nil, multiFilters{"empresa": {"11"}})

	baseCacheMu.Lock()
	baseCache = map[string]baseCacheEntry{key: {data: map[string]int{"x": 1}, at: time.Now()}}
	baseCacheMu.Unlock()

	invalidateBaseCacheMeses(emp, 202607, 202607)

	baseCacheMu.RLock()
	_, existe := baseCache[key]
	baseCacheMu.RUnlock()
	if existe {
		t.Errorf("chave com filtro sobreviveu a uma invalidação que deveria pegá-la: %q", key)
	}
}

// O cache de Q1 guarda datas em vez de ym; a conversão precisa bater igual.
func TestInvalidateVendasPeriodoCacheMeses(t *testing.T) {
	const emp = "11111111-1111-1111-1111-111111111111"
	casos := []struct {
		key       string
		sobrevive bool
	}{
		{emp + "|faturado|cod_fornec|2025-01-01|2025-12-31||", true},
		{emp + "|faturado|cod_fornec|2026-01-01|2026-05-31||", true},
		{emp + "|faturado|cod_fornec|2026-01-01|2026-07-27||", false},
		{emp + "|faturado|cod_fornec|2026-07-01|2026-07-31||", false},
	}

	vendasPeriodoCacheMu.Lock()
	vendasPeriodoCache = map[string]vendasPeriodoCacheEntry{}
	for _, c := range casos {
		vendasPeriodoCache[c.key] = vendasPeriodoCacheEntry{at: time.Now()}
	}
	vendasPeriodoCacheMu.Unlock()

	invalidateVendasPeriodoCacheMeses(emp, 202607, 202607)

	vendasPeriodoCacheMu.RLock()
	defer vendasPeriodoCacheMu.RUnlock()
	for _, c := range casos {
		if _, existe := vendasPeriodoCache[c.key]; existe != c.sobrevive {
			t.Errorf("chave %q: sobreviveu=%v, esperado %v", c.key, existe, c.sobrevive)
		}
	}
}

// Os períodos aquecidos têm de bater com os que o painel pede, senão o
// aquecimento diário não vira cache hit. Cobre a virada de ano (janeiro), onde
// "mês anterior" cai no dezembro do ano passado.
func TestPeriodosComuns(t *testing.T) {
	jul := periodosComuns(time.Date(2026, 7, 27, 10, 0, 0, 0, time.UTC))
	esperadosJul := []ymRange{
		{202601, 202607}, // YTD — combinação vista no log de produção
		{202501, 202512}, // comparativo do YTD
		{202607, 202607},
		{202507, 202507},
		{202606, 202606}, // mês fechado que o painel abriu por padrão
		{202506, 202506},
	}
	if len(jul) != len(esperadosJul) {
		t.Fatalf("julho: %d períodos, esperado %d", len(jul), len(esperadosJul))
	}
	for i, e := range esperadosJul {
		if jul[i] != e {
			t.Errorf("julho[%d] = %v, esperado %v", i, jul[i], e)
		}
	}

	jan := periodosComuns(time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC))
	temDez2025 := false
	for _, p := range jan {
		if p == (ymRange{202512, 202512}) {
			temDez2025 = true
		}
		if p.ini > p.fim {
			t.Errorf("janeiro: período invertido %v", p)
		}
	}
	if !temDez2025 {
		t.Error("janeiro: 'mês anterior' deveria ser dez/2025")
	}
}

func TestMesesRangeYM(t *testing.T) {
	if _, _, ok := mesesRangeYM(nil); ok {
		t.Error("lista vazia deveria devolver ok=false (força invalidação total)")
	}
	ini, fim, ok := mesesRangeYM([]aggMesYM{{2026, 7}, {2025, 3}, {2026, 1}})
	if !ok || ini != 202503 || fim != 202607 {
		t.Errorf("mesesRangeYM = (%d, %d, %v), esperado (202503, 202607, true)", ini, fim, ok)
	}
}
