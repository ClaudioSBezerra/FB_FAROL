-- Migration 130: adiciona cl_ativos, posit_med, ttal_itens às materialized views.
-- cl_ativos  = clientes distintos com objetivo no ano anterior (vl_anterior > 0)
-- posit_med  = clientes distintos com objetivo no ano corrente (vl_corrente > 0)
-- ttal_itens = total de linhas (produto×cliente) com objetivo no ano corrente
-- Remove qtd_clientes (substituído pelas colunas acima).
-- IDEMPOTENTE: derruba e recria as views.

DROP MATERIALIZED VIEW IF EXISTS vw_obj_supervisor;
DROP MATERIALIZED VIEW IF EXISTS vw_obj_rca_fornecedor;

CREATE MATERIALIZED VIEW vw_obj_rca_fornecedor AS
SELECT
    oi.empresa_id,
    oi.tipo_periodo,
    oi.ano,
    oi.periodo_seq,
    oi.cod_supervisor,
    COALESCE(g.nome, 'Supervisor ' || oi.cod_supervisor::text) AS nome_supervisor,
    oi.cod_rca,
    COALESCE(r.nome, 'RCA ' || oi.cod_rca::text)              AS nome_rca,
    oi.cod_fornec,
    MAX(oi.fornecedor)                                                              AS fornecedor,
    COUNT(DISTINCT oi.cod_prod)                                                     AS qtd_produtos,
    COUNT(DISTINCT oi.cod_cli) FILTER (WHERE oi.cod_cli != 0 AND oi.vl_anterior > 0) AS cl_ativos,
    COUNT(DISTINCT oi.cod_cli) FILTER (WHERE oi.cod_cli != 0 AND oi.vl_corrente > 0) AS posit_med,
    COUNT(*)                   FILTER (WHERE oi.cod_cli != 0 AND oi.vl_corrente > 0) AS ttal_itens,
    SUM(oi.vl_anterior)                                                             AS vl_anterior,
    SUM(oi.vl_corrente)                                                             AS vl_corrente
FROM objetivos_importados oi
LEFT JOIN gestores g ON g.empresa_id = oi.empresa_id AND g.cod_supervisor = oi.cod_supervisor
LEFT JOIN rcas     r ON r.empresa_id = oi.empresa_id AND r.cod_rca        = oi.cod_rca
GROUP BY
    oi.empresa_id, oi.tipo_periodo, oi.ano, oi.periodo_seq,
    oi.cod_supervisor, g.nome,
    oi.cod_rca, r.nome,
    oi.cod_fornec
WITH DATA;

CREATE MATERIALIZED VIEW vw_obj_supervisor AS
SELECT
    oi.empresa_id,
    oi.tipo_periodo,
    oi.ano,
    oi.periodo_seq,
    oi.cod_supervisor,
    COALESCE(g.nome, 'Supervisor ' || oi.cod_supervisor::text) AS nome_supervisor,
    oi.cod_fornec,
    MAX(oi.fornecedor)                                                              AS fornecedor,
    COUNT(DISTINCT oi.cod_rca)                                                      AS qtd_rcas,
    COUNT(DISTINCT oi.cod_prod)                                                     AS qtd_produtos,
    COUNT(DISTINCT oi.cod_cli) FILTER (WHERE oi.cod_cli != 0 AND oi.vl_anterior > 0) AS cl_ativos,
    COUNT(DISTINCT oi.cod_cli) FILTER (WHERE oi.cod_cli != 0 AND oi.vl_corrente > 0) AS posit_med,
    COUNT(*)                   FILTER (WHERE oi.cod_cli != 0 AND oi.vl_corrente > 0) AS ttal_itens,
    SUM(oi.vl_anterior)                                                             AS vl_anterior,
    SUM(oi.vl_corrente)                                                             AS vl_corrente
FROM objetivos_importados oi
LEFT JOIN gestores g ON g.empresa_id = oi.empresa_id AND g.cod_supervisor = oi.cod_supervisor
GROUP BY
    oi.empresa_id, oi.tipo_periodo, oi.ano, oi.periodo_seq,
    oi.cod_supervisor, g.nome,
    oi.cod_fornec
WITH DATA;

CREATE INDEX IF NOT EXISTS idx_mv_rca_forn_periodo
    ON vw_obj_rca_fornecedor (empresa_id, tipo_periodo, ano, periodo_seq);

CREATE INDEX IF NOT EXISTS idx_mv_supervisor_periodo
    ON vw_obj_supervisor (empresa_id, tipo_periodo, ano, periodo_seq);
