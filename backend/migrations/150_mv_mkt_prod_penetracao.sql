-- 150_mv_mkt_prod_penetracao.sql
-- MV de penetração de produto com contagem correta de clientes distintos.
-- Agrupa por cod_prod apenas (não por cod_fornec×cod_prod), evitando dupla
-- contagem quando o mesmo produto aparece sob múltiplos fornecedores.
-- Substitui a query direta em vendas_importadas no fetchMktProduto (que
-- causava queries de 3 min para 3008 produtos × 2M+ linhas).

CREATE MATERIALIZED VIEW IF NOT EXISTS farol.mv_mkt_prod_pen AS
SELECT
    empresa_id,
    tipo_base,
    ano,
    mes,
    COALESCE(cod_prod, '')  AS cod_prod,
    MAX(COALESCE(nome_prod,  ''))  AS nome_prod,
    MAX(COALESCE(cod_fornec, ''))  AS cod_fornec,
    MAX(COALESCE(nome_fornec,''))  AS nome_fornec,
    COUNT(DISTINCT CASE WHEN estado = 'FATURADO' AND qt > 0 THEN cod_cli END) AS qt_positivados,
    SUM(pvenda)                                                                AS pvenda,
    SUM(CASE WHEN estado = 'FATURADO'  THEN pvenda ELSE 0 END)                AS faturado,
    SUM(CASE WHEN estado != 'FATURADO' THEN pvenda ELSE 0 END)                AS transmitido
FROM vendas_importadas
WHERE cod_prod != ''
GROUP BY empresa_id, tipo_base, ano, mes, cod_prod
WITH NO DATA;

CREATE UNIQUE INDEX IF NOT EXISTS idx_mv_mkt_prod_pen_pk
    ON farol.mv_mkt_prod_pen (empresa_id, tipo_base, ano, mes, cod_prod);

CREATE INDEX IF NOT EXISTS idx_mv_mkt_prod_pen_filter
    ON farol.mv_mkt_prod_pen (empresa_id, tipo_base, ano, mes);

-- Índice parcial para fetchClientesInativos — quando empresa tem 100% positivação,
-- a query varria a tabela inteira para retornar 0 linhas.
-- Com índice parcial WHERE positivados=0, o planner retorna vazio em O(1).
CREATE INDEX IF NOT EXISTS idx_mvcli_inativos
    ON farol.mv_farol_cli (empresa_id, tipo_base, ano, mes, cod_cli)
    WHERE positivados = 0;

-- Populate inicial (não-concurrent — view nunca foi populada)
REFRESH MATERIALIZED VIEW farol.mv_mkt_prod_pen;

ANALYZE farol.mv_mkt_prod_pen;
ANALYZE farol.mv_farol_cli;
