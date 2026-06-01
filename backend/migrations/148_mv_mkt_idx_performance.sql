-- 148_mv_mkt_idx_performance.sql
-- Índices de cobertura para o Painel Marketing e populate inicial de mv_mkt_produto.
--
-- Problema 1: mv_mkt_produto foi criada WITH NO DATA → REFRESH CONCURRENTLY
-- falha se a view nunca foi populada antes. Este REFRESH inicial (não-concurrent)
-- resolve o bootstrap. Execuções futuras via "Consolidar view" usam CONCURRENTLY.
--
-- Problema 2: fetchMktKPI faz COUNT(DISTINCT cod_cli) em mv_farol_cli (170K+
-- linhas/período) sem índice cobrindo cod_cli. Índice de cobertura resolve.

-- ── Populate inicial da mv_mkt_produto (não-concurrent: view nunca populada) ──
REFRESH MATERIALIZED VIEW farol.mv_mkt_produto;

-- ── Índice de cobertura para KPI de marketing (COUNT DISTINCT cod_cli) ────────
-- Permite ao planner satisfazer toda a query de KPI apenas com o índice,
-- sem varredura na tabela principal (index-only scan).
CREATE INDEX IF NOT EXISTS idx_mvcli_mkt_kpi
    ON farol.mv_farol_cli (empresa_id, tipo_base, ano, mes)
    INCLUDE (cod_cli, positivados, mix, pvenda, faturado, transmitido);

-- ── Índice de cobertura para GROUP BY cod_cli (visão Por Cliente) ─────────────
CREATE INDEX IF NOT EXISTS idx_mvcli_mkt_cliente
    ON farol.mv_farol_cli (empresa_id, tipo_base, ano, mes, cod_cli)
    INCLUDE (nome_cli, positivados, mix, pvenda, faturado, transmitido);

ANALYZE farol.mv_farol_cli;
ANALYZE farol.mv_mkt_produto;
