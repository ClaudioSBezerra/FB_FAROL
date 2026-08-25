package handlers

import (
	"sync"
	"testing"
	"time"
)

// O cache de agg_mes existe para ser COMPARTILHADO entre usuários — é o que faz
// a tela abrir em milissegundos para o segundo gestor que entra. Mas fetchCards
// grava positivados/baseCli nas entradas do mapa que recebe. Se o cache devolver
// a própria referência, dois usuários simultâneos escrevem e iteram o MESMO
// mapa, e o Go aborta com "fatal error: concurrent map iteration and map write"
// — que não é panic recuperável: derruba o processo inteiro.
//
// Aconteceu em produção em 23/08/2026 10:08:17, com o servidor no ar.

func semearCacheAgg(t *testing.T) (string, map[string]aggResult) {
	t.Helper()
	const emp = "11111111-1111-1111-1111-111111111111"

	key := aggMesCacheKey(emp, "faturado", "farol.agg_fat_v01_l0_mes", "cod_fornec", 202601, 202608, nil, nil)
	original := map[string]aggResult{
		"1": {label: "IND A", valor: 100},
		"2": {label: "IND B", valor: 200},
		"3": {label: "IND C", valor: 300},
	}

	aggMesCacheMu.Lock()
	aggMesCache = map[string]aggMesCacheEntry{key: {data: original, at: time.Now()}}
	aggMesCacheMu.Unlock()

	return emp, original
}

func lerDoCache(emp string) map[string]aggResult {
	return cachedAggregatedMes(nil, emp, "faturado", "farol.agg_fat_v01_l0_mes",
		"cod_fornec", "nome_fornec", 202601, 202608, "", "", nil, nil, nil)
}

// Escrever no mapa devolvido não pode alterar o que ficou guardado — senão a
// positivação calculada para UM período fica gravada no cache e é servida para
// o período seguinte, que deveria calcular a sua.
func TestCachedAggregatedMesDevolveCopia(t *testing.T) {
	emp, original := semearCacheAgg(t)

	got := lerDoCache(emp)

	// Exatamente o que fetchCards faz depois de receber o mapa.
	for k, r := range got {
		r.positivados = 42
		r.baseCli = 99
		got[k] = r
	}

	for k, r := range original {
		if r.positivados != 0 || r.baseCli != 0 {
			t.Fatalf("o mapa guardado no cache foi alterado pelo chamador em %q: positivados=%d baseCli=%d",
				k, r.positivados, r.baseCli)
		}
	}
	if len(got) != len(original) {
		t.Fatalf("cópia com %d entradas, original com %d", len(got), len(original))
	}
}

// Reproduz a queda de 23/08: vários usuários na mesma chave de cache, cada um
// gravando no que recebeu. Sem a cópia, isto aborta o processo de teste (ou
// acusa corrida sob -race). Com a cópia, cada um mexe no seu.
func TestCachedAggregatedMesUsoConcorrente(t *testing.T) {
	emp, _ := semearCacheAgg(t)

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m := lerDoCache(emp)
			for k, r := range m {
				r.positivados++
				r.baseCli++
				m[k] = r
			}
		}()
	}
	wg.Wait()
}

// O cache de Q1 (vendasPeriodoCache) tem o mesmo contrato: queryAggregatedVendas
// grava baseCli nas entradas do mapa que recebe. É o cache das telas de 7 e 30
// dias — passou despercebido na primeira auditoria porque não é acessado por uma
// função cached*, e sim direto dentro de vendasPeriodoQ1.
func TestVendasPeriodoQ1DevolveCopia(t *testing.T) {
	const emp = "11111111-1111-1111-1111-111111111111"
	fat := fluxoCtx{name: "faturado"}
	ini := time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC)
	fim := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)

	key := vendasPeriodoCacheKey(emp, fat.name, "cod_fornec", ini, fim, nil, nil)
	original := map[string]aggResult{
		"1": {label: "IND A", valor: 100},
		"2": {label: "IND B", valor: 200},
	}

	vendasPeriodoCacheMu.Lock()
	vendasPeriodoCache = map[string]vendasPeriodoCacheEntry{key: {data: original, at: time.Now()}}
	vendasPeriodoCacheMu.Unlock()

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out := vendasPeriodoQ1(nil, emp, fat, "cod_fornec", "nome_fornec", ini, fim, nil, nil)
			if !out.cached {
				t.Error("esperado acerto de cache — com miss a consulta tocaria o banco (db nil)")
				return
			}
			// Exatamente o que queryAggregatedVendas faz com o que recebe.
			for k, r := range out.result {
				r.baseCli = 7
				out.result[k] = r
			}
		}()
	}
	wg.Wait()

	for k, r := range original {
		if r.baseCli != 0 {
			t.Fatalf("o mapa guardado no cache foi alterado pelo chamador em %q: baseCli=%d", k, r.baseCli)
		}
	}
}
