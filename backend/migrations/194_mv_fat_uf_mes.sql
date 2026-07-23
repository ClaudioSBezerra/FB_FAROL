-- Migration 194 — MV de faturado LÍQUIDO por UF (do cliente) × mês
--
-- O Painel BI (War Room do CEO) ganha um bloco "Faturado por UF". UF não é
-- dimensão de nenhuma agg_*, então até aqui só existia via scan de vendas_*.
-- O problema: o painel exibe LÍQUIDO (gauges e donut usam agg.liquido), e um
-- scan de SUM(pvenda) traz o BRUTO — o total por UF não fecharia com o headline.
--
-- Esta MV precomputa o líquido por (empresa, uf, ano, mês) com a MESMA fórmula
-- da mig 190 (upsert_venda_liquida), garantindo que a soma por UF bate com o
-- resto do painel:
--   liquido = Σ pvenda (evento='' E tipo_venda ∈ {1,4,7,8,9,11,14,20})
--             − Σ pvenda (evento='DEVOLVIDO') − Σ pvenda (evento='CANCELADO')
--
-- Fonte: vendas_faturadas (evento vazio) ∪ vendas_ccd (DEVOLVIDO/CANCELADO),
-- ambas com coluna `uf`. CORTADO fica de fora (não entra no líquido), igual
-- à mig 190. `bruto` é guardado para referência/depuração.
--
-- Refresh: junto do upsert das agg (processImportJob + RefreshViewsHandler),
-- via REFRESH MATERIALIZED VIEW CONCURRENTLY (por isso o índice único abaixo).
-- Aditiva: não altera nenhuma agg nem o upsert existente.

CREATE MATERIALIZED VIEW IF NOT EXISTS farol.mv_fat_uf_mes AS
WITH un AS (
    SELECT empresa_id,
           COALESCE(NULLIF(uf, ''), '—')            AS uf,
           EXTRACT(YEAR  FROM data_faturamento)::int AS ano,
           EXTRACT(MONTH FROM data_faturamento)::int AS mes,
           tipo_venda,
           ''::text                                 AS evento,
           pvenda
      FROM vendas_faturadas
    UNION ALL
    SELECT empresa_id,
           COALESCE(NULLIF(uf, ''), '—'),
           EXTRACT(YEAR  FROM data_evento)::int,
           EXTRACT(MONTH FROM data_evento)::int,
           ''::text                                 AS tipo_venda,
           evento,
           pvenda
      FROM vendas_ccd
     WHERE evento IN ('DEVOLVIDO', 'CANCELADO')
)
SELECT
    empresa_id, uf, ano, mes,
    COALESCE(SUM(pvenda) FILTER (WHERE evento = '' AND tipo_venda IN ('1','4','7','8','9','11','14','20')), 0)
      - COALESCE(SUM(pvenda) FILTER (WHERE evento = 'DEVOLVIDO'), 0)
      - COALESCE(SUM(pvenda) FILTER (WHERE evento = 'CANCELADO'), 0) AS liquido,
    COALESCE(SUM(pvenda) FILTER (WHERE evento = ''), 0)              AS bruto
FROM un
GROUP BY empresa_id, uf, ano, mes;

-- Índice único → habilita REFRESH ... CONCURRENTLY (não bloqueia leitura).
CREATE UNIQUE INDEX IF NOT EXISTS mv_fat_uf_mes_pk
    ON farol.mv_fat_uf_mes (empresa_id, uf, ano, mes);

COMMENT ON MATERIALIZED VIEW farol.mv_fat_uf_mes IS
  'Faturado líquido por UF (do cliente) × mês, mesma fórmula da mig 190. Fonte do bloco "Faturado por UF" do Painel BI. Refresh junto do upsert das agg.';
