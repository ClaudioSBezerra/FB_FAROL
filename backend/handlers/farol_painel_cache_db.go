package handlers

// farol_painel_cache_db.go — camada L2 (persistida no banco) do baseCache
// do Painel Geral (decisão do Claudio, 11/09/2026).
//
// "Clientes Ativos" (queryDistinctPositivados, farol_v2_api.go) é um
// COUNT(DISTINCT cnpj) na folha agg_*_l4/l3_mes — 3 a 11s por miss, medido
// em produção. Até aqui só existia cache EM MEMÓRIA (baseCache): zera a
// cada deploy, e este projeto redeploya várias vezes por dia — todo
// redeploy, os primeiros usuários reais pagam o recálculo, às vezes
// competindo com o próprio prewarm de boot (5,1s medidos num fetchCards
// real durante o prewarm de outra empresa, mesmo minuto).
//
// migration 235 (farol.painel_cache_snapshot) é o mesmo princípio do
// snapshot do Painel de Objetivos (migration 234, farol_metas_congelamento.go):
// L1 (memória, rápido, mas frágil a deploy) por cima de L2 (banco,
// sobrevive a deploy). Miss em memória consulta o banco antes de
// recalcular; qualquer cálculo novo (prewarm OU auto-cura de um miss ao
// vivo) grava nos dois níveis. `cache_name` deixa a tabela genérica pra
// outros caches no futuro sem migration nova.

import (
	"database/sql"
	"encoding/json"
	"log"
)

// painelCacheGet lê uma linha já calculada da L2. Não devolve erro pro
// chamador — qualquer falha (linha não existe, JSON inválido, banco fora)
// vira miss normal, que cai no cálculo ao vivo (mesma auto-cura do
// snapshot de Objetivos): nunca pode quebrar uma resposta real por causa
// do cache.
func painelCacheGet(db *sql.DB, empresaID, cacheName, cacheKey string) (map[string]int, bool) {
	var raw []byte
	err := db.QueryRow(`
		SELECT resultado_json FROM farol.painel_cache_snapshot
		WHERE empresa_id = $1 AND cache_name = $2 AND cache_key = $3
	`, empresaID, cacheName, cacheKey).Scan(&raw)
	if err != nil {
		return nil, false
	}
	var data map[string]int
	if jerr := json.Unmarshal(raw, &data); jerr != nil {
		return nil, false
	}
	return data, true
}

// painelCacheSet grava (UPSERT) o resultado — chamado tanto pelo prewarm
// quanto pela auto-cura de um miss ao vivo. Erro aqui só loga: gravar
// cache nunca pode derrubar a resposta real do usuário.
func painelCacheSet(db *sql.DB, empresaID, cacheName, cacheKey string, ymIni, ymFim int, data map[string]int) {
	raw, err := json.Marshal(data)
	if err != nil {
		return
	}
	if _, err := db.Exec(`
		INSERT INTO farol.painel_cache_snapshot (empresa_id, cache_name, cache_key, ym_ini, ym_fim, resultado_json, calculado_em)
		VALUES ($1, $2, $3, $4, $5, $6, now())
		ON CONFLICT (empresa_id, cache_name, cache_key)
		DO UPDATE SET resultado_json = $6, ym_ini = $4, ym_fim = $5, calculado_em = now()
	`, empresaID, cacheName, cacheKey, ymIni, ymFim, raw); err != nil {
		log.Printf("[farol:cache] painelCacheSet falhou (cache=%s empresa=%s): %v", cacheName, empresaID, err)
	}
}

// painelCacheInvalidateMeses remove da L2 as linhas cujo período SOBREPÕE
// os meses recém-carregados — espelha invalidateBaseCacheMeses (memória),
// pra a L2 nunca continuar servindo, depois de uma carga, um número que a
// carga acabou de corrigir. Mesmo teste de sobreposição (ymOverlapsKeyRange)
// já usado em memória, só que como WHERE de SQL.
func painelCacheInvalidateMeses(db *sql.DB, empresaID, cacheName string, ymIni, ymFim int) {
	if db == nil { // testes puros de lógica de invalidação em memória não passam DB
		return
	}
	res, err := db.Exec(`
		DELETE FROM farol.painel_cache_snapshot
		WHERE empresa_id = $1 AND cache_name = $2 AND ym_ini <= $4 AND $3 <= ym_fim
	`, empresaID, cacheName, ymIni, ymFim)
	if err != nil {
		log.Printf("[farol:cache] painelCacheInvalidateMeses falhou (cache=%s empresa=%s): %v", cacheName, empresaID, err)
		return
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		log.Printf("[farol:cache] painelCacheSnapshot (L2, cache=%s) invalidado p/ meses %d..%d — %d linha(s)", cacheName, ymIni, ymFim, n)
	}
}

// painelCacheInvalidateAll remove TODAS as linhas de uma empresa/cache —
// espelha invalidateBaseCache (memória), pra reconstrução total (meses
// desconhecidos ou dados apagados em bloco).
func painelCacheInvalidateAll(db *sql.DB, empresaID, cacheName string) {
	if db == nil {
		return
	}
	if _, err := db.Exec(`
		DELETE FROM farol.painel_cache_snapshot WHERE empresa_id = $1 AND cache_name = $2
	`, empresaID, cacheName); err != nil {
		log.Printf("[farol:cache] painelCacheInvalidateAll falhou (cache=%s empresa=%s): %v", cacheName, empresaID, err)
	}
}
