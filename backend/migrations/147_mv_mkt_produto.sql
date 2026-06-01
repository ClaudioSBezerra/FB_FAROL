-- 147_mv_mkt_produto.sql
-- Painel Marketing: penetração de produto (clientes únicos por produto/período).
-- Fonte: vendas_importadas — uma linha por (empresa, período, fornecedor, produto).
-- qt_clientes  = clientes distintos que tiveram qualquer quantidade deste produto.
-- qt_positivados = clientes que efetivamente FATURARAM (estado='FATURADO').

CREATE MATERIALIZED VIEW IF NOT EXISTS farol.mv_mkt_produto AS
SELECT
    empresa_id,
    tipo_base,
    ano,
    mes,
    COALESCE(cod_fornec, '')  AS cod_fornec,
    MAX(COALESCE(nome_fornec, '')) AS nome_fornec,
    COALESCE(cod_prod, '')    AS cod_prod,
    MAX(COALESCE(nome_prod, ''))   AS nome_prod,
    COUNT(DISTINCT NULLIF(cod_cli, ''))                                          AS qt_clientes,
    COUNT(DISTINCT CASE WHEN estado = 'FATURADO' AND qt > 0 THEN cod_cli END)   AS qt_positivados,
    SUM(pvenda)                                                                   AS pvenda,
    SUM(CASE WHEN estado = 'FATURADO'  THEN pvenda ELSE 0 END)                  AS faturado,
    SUM(CASE WHEN estado != 'FATURADO' THEN pvenda ELSE 0 END)                  AS transmitido
FROM vendas_importadas
WHERE cod_prod != ''
GROUP BY empresa_id, tipo_base, ano, mes, cod_fornec, cod_prod
WITH NO DATA;

CREATE UNIQUE INDEX IF NOT EXISTS idx_mv_mkt_produto_pk
    ON farol.mv_mkt_produto (empresa_id, tipo_base, ano, mes, cod_fornec, cod_prod);

CREATE INDEX IF NOT EXISTS idx_mv_mkt_produto_filter
    ON farol.mv_mkt_produto (empresa_id, tipo_base, ano, mes);

-- ANALYZE será executado automaticamente após o primeiro REFRESH MATERIALIZED VIEW
