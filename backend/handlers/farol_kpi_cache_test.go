package handlers

import (
	"testing"
	"time"
)

// O totalizador lido do cache não pode tocar o banco (db nil estouraria) e a
// chave precisa manter o período no 5º campo, que é o que
// invalidateBaseCacheMeses usa para decidir o que derrubar.
func TestCachedKPITotalServeDoCacheEInvalidaPorMes(t *testing.T) {
	emp := "emp-teste-kpi"
	fl := resolveFluxo("faturado")
	key := baseCacheKey(emp, fl.name, "V01", "kpi_total:cod_fornec", 202601, 202610, nil, nil)
	hist := baseCacheKey(emp, fl.name, "V01", "kpi_mix:cod_fornec", 202501, 202512, nil, nil)
	baseCacheMu.Lock()
	baseCache[key] = baseCacheEntry{data: map[string]int{"v": 39510}, at: time.Now()}
	baseCache[hist] = baseCacheEntry{data: map[string]int{"v": 2644536}, at: time.Now()}
	baseCacheMu.Unlock()

	if v, hit := cachedKPITotal(nil, emp, fl, "V01", "cod_fornec", "kpi_total", 202601, 202610, nil, multiFilters{}); !hit || v != 39510 {
		t.Fatalf("esperava 39510 do cache, veio %d hit=%v", v, hit)
	}
	if v, hit := cachedKPITotal(nil, emp, fl, "V01", "cod_fornec", "kpi_mix", 202501, 202512, nil, nil); !hit || float64(v)/kpiMixEscala != 2.644536 {
		t.Fatalf("mix do cache errado: %d hit=%v", v, hit)
	}

	baseCacheMu.Lock()
	for k := range baseCache {
		parts := splitKey(k)
		if len(parts) >= 5 && parts[0] == emp {
			if ini, fim, ok := parseYMRangeField(parts[4]); !ok || ymOverlapsKeyRange(ini, fim, 202610, 202610) {
				delete(baseCache, k)
			}
		}
	}
	_, sobrouAtual := baseCache[key]
	_, sobrouHist := baseCache[hist]
	delete(baseCache, hist)
	baseCacheMu.Unlock()
	if sobrouAtual || !sobrouHist {
		t.Fatalf("invalidação por mês: atual removido=%v (quer true), histórico preservado=%v (quer true)", !sobrouAtual, sobrouHist)
	}
}

func splitKey(k string) []string {
	var out []string
	ini := 0
	for i := 0; i < len(k); i++ {
		if k[i] == '|' {
			out = append(out, k[ini:i])
			ini = i + 1
		}
	}
	return append(out, k[ini:])
}
