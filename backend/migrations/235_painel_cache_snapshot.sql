-- Migration 235: snapshot persistido do baseCache/aggMesCache do Painel Geral
--
-- Mesmo racional do snapshot do Painel de Objetivos (migration 234): o
-- COUNT(DISTINCT cnpj) de "Clientes Ativos" (queryDistinctPositivados,
-- farol_v2_api.go) e as agregações de agg_mes custam segundos por miss —
-- medido em produção: 3-11s. Hoje só existe cache EM MEMÓRIA (baseCache/
-- aggMesCache), que zera a cada deploy — e este projeto redeploya várias
-- vezes por dia. Todo deploy, os primeiros usuários reais pagam o
-- recálculo até o prewarm de boot terminar (ou pior, competem com ele —
-- 5,1s medidos num fetchCards real durante o prewarm de outra empresa).
--
-- `cache_name` distingue qual cache guardou a linha ('base_positivados' por
-- enquanto — dá pra reusar pra outros caches no futuro sem nova tabela).
-- `cache_key` é a MESMA string que já vira chave do mapa em memória
-- (baseCacheKey) — reaproveita a lógica de chave já existente e testada.
-- `ym_ini`/`ym_fim` ficam em colunas próprias (não só embutidos na string)
-- porque invalidateBaseCacheMeses precisa filtrar por SOBREPOSIÇÃO de mês
-- via SQL (`ym_ini <= :fim AND :ini <= ym_fim`) — extrair isso de dentro de
-- uma string toda vez seria mais lento e mais frágil que uma coluna
-- indexada.

CREATE TABLE IF NOT EXISTS farol.painel_cache_snapshot (
    id             BIGSERIAL    PRIMARY KEY,
    empresa_id     UUID         NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    cache_name     TEXT         NOT NULL,
    cache_key      TEXT         NOT NULL,
    ym_ini         INTEGER      NOT NULL,
    ym_fim         INTEGER      NOT NULL,
    resultado_json JSONB        NOT NULL,
    calculado_em   TIMESTAMPTZ  NOT NULL DEFAULT now(),

    CONSTRAINT uq_farol_painel_cache_snapshot UNIQUE (empresa_id, cache_name, cache_key)
);

CREATE INDEX IF NOT EXISTS idx_farol_painel_cache_snapshot_invalidacao
    ON farol.painel_cache_snapshot (empresa_id, cache_name, ym_ini, ym_fim);
