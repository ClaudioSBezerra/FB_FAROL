-- 143_mv_farol_hierarquia.sql
-- Múltiplas views materializadas — uma por nível de drill.
--
-- PRINCÍPIO:
--   "Os dados já estão pré-agregados. A API deve só ler e carregar."
--
-- Para cada nível de drill (Fornecedor / Gerente / Supervisor / RCA / Cliente),
-- existe uma view materializada com as métricas JÁ CALCULADAS.
-- A API faz apenas: SELECT ... WHERE empresa_id=$1 AND tipo_base=... AND ano=... AND mes=...
-- Sem GROUP BY, sem COUNT(DISTINCT), sem CTEs — leitura direta de linhas prontas.
--
-- VIEWS CRIADAS:
--   farol.mv_farol_cli   — base: por cliente (170K linhas/período)
--   farol.mv_v01_l0      — V01 nível 0: por Fornecedor      (~17 linhas/período)
--   farol.mv_v01_l1      — V01 nível 1: por Gerente         (~170 linhas/período)
--   farol.mv_v01_l2      — V01 nível 2: por Supervisor      (~3.400 linhas/período)
--   farol.mv_v01_l3      — V01 nível 3: por RCA             (~8.500 linhas/período)
--   farol.mv_v02_l0      — V02 nível 0: por Supervisor      (~20 linhas/período)
--   farol.mv_v02_l1      — V02 nível 1: por RCA             (~200 linhas/período)
--   farol.mv_v02_l2      — V02 nível 2: por Fornecedor      (~3.400 linhas/período)
--
-- REFRESH: base primeiro, depois views de resumo em paralelo.
-- (ver farol_v2_import.go e farol_v2_api.go — RefreshViewsHandler)

DROP MATERIALIZED VIEW IF EXISTS farol.mv_farol_resumo CASCADE;

-- ═══════════════════════════════════════════════════════════════════════════════
-- VIEW BASE: farol.mv_farol_cli
-- Uma linha por (empresa, período, fornecedor, hierarquia, cliente).
-- Pré-calcula faturado, transmitido, mix (produtos distintos por cliente).
-- Compatível com summary views: tem base_cli=1, positivados=0/1, mix=num_prod.
-- ═══════════════════════════════════════════════════════════════════════════════

CREATE MATERIALIZED VIEW farol.mv_farol_cli AS
SELECT
    empresa_id, tipo_base, ano, mes,
    COALESCE(cod_fornec,     '') AS cod_fornec,
    COALESCE(cod_gerente,    '') AS cod_gerente,
    COALESCE(cod_supervisor, '') AS cod_supervisor,
    COALESCE(cod_rca,        '') AS cod_rca,
    COALESCE(cod_cli,        '') AS cod_cli,
    COALESCE(empresa,        '') AS empresa,
    COALESCE(uf,             '') AS uf,
    MAX(COALESCE(nome_fornec,     '')) AS nome_fornec,
    MAX(COALESCE(nome_gerente,    '')) AS nome_gerente,
    MAX(COALESCE(nome_supervisor, '')) AS nome_supervisor,
    MAX(COALESCE(nome_rca,        '')) AS nome_rca,
    MAX(COALESCE(nome_cli,        '')) AS nome_cli,
    MAX(qtcli_rca)                     AS qtcli_rca,
    SUM(pvenda)                        AS pvenda,
    SUM(CASE WHEN estado = 'FATURADO'  THEN pvenda ELSE 0 END) AS faturado,
    SUM(CASE WHEN estado != 'FATURADO' THEN pvenda ELSE 0 END) AS transmitido,
    SUM(qt)                            AS qt,
    -- Métricas de compatibilidade com views de resumo:
    1::int                                                          AS base_cli,
    (CASE WHEN SUM(qt) > 0 THEN 1 ELSE 0 END)::int                AS positivados,
    COUNT(DISTINCT NULLIF(cod_prod, ''))::float                    AS mix
FROM vendas_importadas
GROUP BY
    empresa_id, tipo_base, ano, mes,
    cod_fornec, cod_gerente, cod_supervisor, cod_rca, cod_cli, empresa, uf
WITH NO DATA;

CREATE UNIQUE INDEX idx_mvcli_pk ON farol.mv_farol_cli (
    empresa_id, tipo_base, ano, mes,
    cod_fornec, cod_gerente, cod_supervisor, cod_rca, cod_cli, empresa, uf
);
CREATE INDEX idx_mvcli_filter ON farol.mv_farol_cli (empresa_id, tipo_base, ano, mes);

REFRESH MATERIALIZED VIEW farol.mv_farol_cli;
ANALYZE farol.mv_farol_cli;

-- ═══════════════════════════════════════════════════════════════════════════════
-- V01: Por Indústria — Fornecedor → Gerente → Supervisor → RCA → Cliente
-- ═══════════════════════════════════════════════════════════════════════════════

-- Nível 0: Fornecedor
CREATE MATERIALIZED VIEW farol.mv_v01_l0 AS
SELECT
    empresa_id, tipo_base, ano, mes,
    cod_fornec,  MAX(nome_fornec) AS nome_fornec,
    CASE WHEN MAX(qtcli_rca) > 0 THEN MAX(qtcli_rca)::int
         ELSE COUNT(DISTINCT cod_cli)::int END         AS base_cli,
    COUNT(DISTINCT cod_cli) FILTER (WHERE qt > 0)      AS positivados,
    COALESCE(AVG(mix) FILTER (WHERE qt > 0), 0)        AS mix,
    SUM(pvenda) AS pvenda, SUM(faturado) AS faturado, SUM(transmitido) AS transmitido
FROM farol.mv_farol_cli
WHERE cod_fornec != ''
GROUP BY empresa_id, tipo_base, ano, mes, cod_fornec
WITH NO DATA;

CREATE UNIQUE INDEX idx_mvv01l0_pk ON farol.mv_v01_l0 (empresa_id, tipo_base, ano, mes, cod_fornec);
REFRESH MATERIALIZED VIEW farol.mv_v01_l0;

-- Nível 1: Gerente (dentro de Fornecedor)
CREATE MATERIALIZED VIEW farol.mv_v01_l1 AS
SELECT
    empresa_id, tipo_base, ano, mes,
    cod_fornec, cod_gerente, MAX(nome_gerente) AS nome_gerente,
    CASE WHEN MAX(qtcli_rca) > 0 THEN MAX(qtcli_rca)::int
         ELSE COUNT(DISTINCT cod_cli)::int END         AS base_cli,
    COUNT(DISTINCT cod_cli) FILTER (WHERE qt > 0)      AS positivados,
    COALESCE(AVG(mix) FILTER (WHERE qt > 0), 0)        AS mix,
    SUM(pvenda) AS pvenda, SUM(faturado) AS faturado, SUM(transmitido) AS transmitido
FROM farol.mv_farol_cli
WHERE cod_gerente != ''
GROUP BY empresa_id, tipo_base, ano, mes, cod_fornec, cod_gerente
WITH NO DATA;

CREATE UNIQUE INDEX idx_mvv01l1_pk ON farol.mv_v01_l1 (empresa_id, tipo_base, ano, mes, cod_fornec, cod_gerente);
REFRESH MATERIALIZED VIEW farol.mv_v01_l1;

-- Nível 2: Supervisor (dentro de Fornecedor + Gerente)
CREATE MATERIALIZED VIEW farol.mv_v01_l2 AS
SELECT
    empresa_id, tipo_base, ano, mes,
    cod_fornec, cod_gerente, cod_supervisor, MAX(nome_supervisor) AS nome_supervisor,
    CASE WHEN MAX(qtcli_rca) > 0 THEN MAX(qtcli_rca)::int
         ELSE COUNT(DISTINCT cod_cli)::int END         AS base_cli,
    COUNT(DISTINCT cod_cli) FILTER (WHERE qt > 0)      AS positivados,
    COALESCE(AVG(mix) FILTER (WHERE qt > 0), 0)        AS mix,
    SUM(pvenda) AS pvenda, SUM(faturado) AS faturado, SUM(transmitido) AS transmitido
FROM farol.mv_farol_cli
WHERE cod_supervisor != ''
GROUP BY empresa_id, tipo_base, ano, mes, cod_fornec, cod_gerente, cod_supervisor
WITH NO DATA;

CREATE UNIQUE INDEX idx_mvv01l2_pk ON farol.mv_v01_l2 (empresa_id, tipo_base, ano, mes, cod_fornec, cod_gerente, cod_supervisor);
REFRESH MATERIALIZED VIEW farol.mv_v01_l2;

-- Nível 3: RCA (dentro de Fornecedor + Gerente + Supervisor)
CREATE MATERIALIZED VIEW farol.mv_v01_l3 AS
SELECT
    empresa_id, tipo_base, ano, mes,
    cod_fornec, cod_gerente, cod_supervisor, cod_rca, MAX(nome_rca) AS nome_rca,
    CASE WHEN MAX(qtcli_rca) > 0 THEN MAX(qtcli_rca)::int
         ELSE COUNT(DISTINCT cod_cli)::int END         AS base_cli,
    COUNT(DISTINCT cod_cli) FILTER (WHERE qt > 0)      AS positivados,
    COALESCE(AVG(mix) FILTER (WHERE qt > 0), 0)        AS mix,
    SUM(pvenda) AS pvenda, SUM(faturado) AS faturado, SUM(transmitido) AS transmitido
FROM farol.mv_farol_cli
WHERE cod_rca != ''
GROUP BY empresa_id, tipo_base, ano, mes, cod_fornec, cod_gerente, cod_supervisor, cod_rca
WITH NO DATA;

CREATE UNIQUE INDEX idx_mvv01l3_pk ON farol.mv_v01_l3 (empresa_id, tipo_base, ano, mes, cod_fornec, cod_gerente, cod_supervisor, cod_rca);
REFRESH MATERIALIZED VIEW farol.mv_v01_l3;

-- ═══════════════════════════════════════════════════════════════════════════════
-- V02: Por Força de Vendas — Supervisor → RCA → Fornecedor → Cliente
-- ═══════════════════════════════════════════════════════════════════════════════

-- Nível 0: Supervisor
CREATE MATERIALIZED VIEW farol.mv_v02_l0 AS
SELECT
    empresa_id, tipo_base, ano, mes,
    cod_supervisor, MAX(nome_supervisor) AS nome_supervisor,
    CASE WHEN MAX(qtcli_rca) > 0 THEN MAX(qtcli_rca)::int
         ELSE COUNT(DISTINCT cod_cli)::int END         AS base_cli,
    COUNT(DISTINCT cod_cli) FILTER (WHERE qt > 0)      AS positivados,
    COALESCE(AVG(mix) FILTER (WHERE qt > 0), 0)        AS mix,
    SUM(pvenda) AS pvenda, SUM(faturado) AS faturado, SUM(transmitido) AS transmitido
FROM farol.mv_farol_cli
WHERE cod_supervisor != ''
GROUP BY empresa_id, tipo_base, ano, mes, cod_supervisor
WITH NO DATA;

CREATE UNIQUE INDEX idx_mvv02l0_pk ON farol.mv_v02_l0 (empresa_id, tipo_base, ano, mes, cod_supervisor);
REFRESH MATERIALIZED VIEW farol.mv_v02_l0;

-- Nível 1: RCA (dentro de Supervisor)
CREATE MATERIALIZED VIEW farol.mv_v02_l1 AS
SELECT
    empresa_id, tipo_base, ano, mes,
    cod_supervisor, cod_rca, MAX(nome_rca) AS nome_rca,
    CASE WHEN MAX(qtcli_rca) > 0 THEN MAX(qtcli_rca)::int
         ELSE COUNT(DISTINCT cod_cli)::int END         AS base_cli,
    COUNT(DISTINCT cod_cli) FILTER (WHERE qt > 0)      AS positivados,
    COALESCE(AVG(mix) FILTER (WHERE qt > 0), 0)        AS mix,
    SUM(pvenda) AS pvenda, SUM(faturado) AS faturado, SUM(transmitido) AS transmitido
FROM farol.mv_farol_cli
WHERE cod_supervisor != '' AND cod_rca != ''
GROUP BY empresa_id, tipo_base, ano, mes, cod_supervisor, cod_rca
WITH NO DATA;

CREATE UNIQUE INDEX idx_mvv02l1_pk ON farol.mv_v02_l1 (empresa_id, tipo_base, ano, mes, cod_supervisor, cod_rca);
REFRESH MATERIALIZED VIEW farol.mv_v02_l1;

-- Nível 2: Fornecedor (dentro de Supervisor + RCA)
CREATE MATERIALIZED VIEW farol.mv_v02_l2 AS
SELECT
    empresa_id, tipo_base, ano, mes,
    cod_supervisor, cod_rca, cod_fornec, MAX(nome_fornec) AS nome_fornec,
    CASE WHEN MAX(qtcli_rca) > 0 THEN MAX(qtcli_rca)::int
         ELSE COUNT(DISTINCT cod_cli)::int END         AS base_cli,
    COUNT(DISTINCT cod_cli) FILTER (WHERE qt > 0)      AS positivados,
    COALESCE(AVG(mix) FILTER (WHERE qt > 0), 0)        AS mix,
    SUM(pvenda) AS pvenda, SUM(faturado) AS faturado, SUM(transmitido) AS transmitido
FROM farol.mv_farol_cli
WHERE cod_supervisor != '' AND cod_rca != '' AND cod_fornec != ''
GROUP BY empresa_id, tipo_base, ano, mes, cod_supervisor, cod_rca, cod_fornec
WITH NO DATA;

CREATE UNIQUE INDEX idx_mvv02l2_pk ON farol.mv_v02_l2 (empresa_id, tipo_base, ano, mes, cod_supervisor, cod_rca, cod_fornec);
REFRESH MATERIALIZED VIEW farol.mv_v02_l2;

ANALYZE farol.mv_v01_l0;
ANALYZE farol.mv_v01_l1;
ANALYZE farol.mv_v01_l2;
ANALYZE farol.mv_v01_l3;
ANALYZE farol.mv_v02_l0;
ANALYZE farol.mv_v02_l1;
ANALYZE farol.mv_v02_l2;
