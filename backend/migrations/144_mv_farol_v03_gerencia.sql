-- 144_mv_farol_v03_gerencia.sql
-- V03: visão de gerência (organização) — Gerente/GGV → Supervisor → RCA → Cliente.
--
-- É a visão natural do diretor: desce pela estrutura de pessoas, sem passar pelo
-- fornecedor. Mesmo princípio das demais: métricas pré-calculadas, API só faz
-- SELECT/WHERE.
--
-- IMPORTANTE — nível Cliente (mv_v03_l3):
--   No V03 o fornecedor NÃO está no drill path antes do cliente. Um mesmo cliente
--   atendido por um RCA pode ter linhas de vários fornecedores em mv_farol_cli.
--   Por isso o leaf agrega (SUM) por cliente dentro de gerente+supervisor+rca —
--   diferente de V01/V02, onde o fornecedor já fixa a linha e a base serve direto.
--
-- Depende de farol.mv_farol_cli (migration 143). 144 > 143 garante a ordem.

-- Nível 0: Gerente/GGV
CREATE MATERIALIZED VIEW farol.mv_v03_l0 AS
SELECT
    empresa_id, tipo_base, ano, mes,
    cod_gerente, MAX(nome_gerente) AS nome_gerente,
    CASE WHEN MAX(qtcli_rca) > 0 THEN MAX(qtcli_rca)::int
         ELSE COUNT(DISTINCT cod_cli)::int END         AS base_cli,
    COUNT(DISTINCT cod_cli) FILTER (WHERE qt > 0)      AS positivados,
    COALESCE(AVG(mix) FILTER (WHERE qt > 0), 0)        AS mix,
    SUM(pvenda) AS pvenda, SUM(faturado) AS faturado, SUM(transmitido) AS transmitido
FROM farol.mv_farol_cli
WHERE cod_gerente != ''
GROUP BY empresa_id, tipo_base, ano, mes, cod_gerente
WITH NO DATA;

CREATE UNIQUE INDEX idx_mvv03l0_pk ON farol.mv_v03_l0 (empresa_id, tipo_base, ano, mes, cod_gerente);
REFRESH MATERIALIZED VIEW farol.mv_v03_l0;

-- Nível 1: Supervisor (dentro de Gerente)
CREATE MATERIALIZED VIEW farol.mv_v03_l1 AS
SELECT
    empresa_id, tipo_base, ano, mes,
    cod_gerente, cod_supervisor, MAX(nome_supervisor) AS nome_supervisor,
    CASE WHEN MAX(qtcli_rca) > 0 THEN MAX(qtcli_rca)::int
         ELSE COUNT(DISTINCT cod_cli)::int END         AS base_cli,
    COUNT(DISTINCT cod_cli) FILTER (WHERE qt > 0)      AS positivados,
    COALESCE(AVG(mix) FILTER (WHERE qt > 0), 0)        AS mix,
    SUM(pvenda) AS pvenda, SUM(faturado) AS faturado, SUM(transmitido) AS transmitido
FROM farol.mv_farol_cli
WHERE cod_gerente != '' AND cod_supervisor != ''
GROUP BY empresa_id, tipo_base, ano, mes, cod_gerente, cod_supervisor
WITH NO DATA;

CREATE UNIQUE INDEX idx_mvv03l1_pk ON farol.mv_v03_l1 (empresa_id, tipo_base, ano, mes, cod_gerente, cod_supervisor);
REFRESH MATERIALIZED VIEW farol.mv_v03_l1;

-- Nível 2: RCA (dentro de Gerente + Supervisor)
CREATE MATERIALIZED VIEW farol.mv_v03_l2 AS
SELECT
    empresa_id, tipo_base, ano, mes,
    cod_gerente, cod_supervisor, cod_rca, MAX(nome_rca) AS nome_rca,
    CASE WHEN MAX(qtcli_rca) > 0 THEN MAX(qtcli_rca)::int
         ELSE COUNT(DISTINCT cod_cli)::int END         AS base_cli,
    COUNT(DISTINCT cod_cli) FILTER (WHERE qt > 0)      AS positivados,
    COALESCE(AVG(mix) FILTER (WHERE qt > 0), 0)        AS mix,
    SUM(pvenda) AS pvenda, SUM(faturado) AS faturado, SUM(transmitido) AS transmitido
FROM farol.mv_farol_cli
WHERE cod_gerente != '' AND cod_supervisor != '' AND cod_rca != ''
GROUP BY empresa_id, tipo_base, ano, mes, cod_gerente, cod_supervisor, cod_rca
WITH NO DATA;

CREATE UNIQUE INDEX idx_mvv03l2_pk ON farol.mv_v03_l2 (empresa_id, tipo_base, ano, mes, cod_gerente, cod_supervisor, cod_rca);
REFRESH MATERIALIZED VIEW farol.mv_v03_l2;

-- Nível 3: Cliente (dentro de Gerente + Supervisor + RCA) — soma fornecedores
CREATE MATERIALIZED VIEW farol.mv_v03_l3 AS
SELECT
    empresa_id, tipo_base, ano, mes,
    cod_gerente, cod_supervisor, cod_rca, cod_cli, MAX(nome_cli) AS nome_cli,
    1::int                                            AS base_cli,
    (CASE WHEN SUM(qt) > 0 THEN 1 ELSE 0 END)::int    AS positivados,
    SUM(mix)                                          AS mix,
    SUM(pvenda) AS pvenda, SUM(faturado) AS faturado, SUM(transmitido) AS transmitido
FROM farol.mv_farol_cli
WHERE cod_gerente != '' AND cod_supervisor != '' AND cod_rca != '' AND cod_cli != ''
GROUP BY empresa_id, tipo_base, ano, mes, cod_gerente, cod_supervisor, cod_rca, cod_cli
WITH NO DATA;

CREATE UNIQUE INDEX idx_mvv03l3_pk ON farol.mv_v03_l3 (empresa_id, tipo_base, ano, mes, cod_gerente, cod_supervisor, cod_rca, cod_cli);
REFRESH MATERIALIZED VIEW farol.mv_v03_l3;

ANALYZE farol.mv_v03_l0;
ANALYZE farol.mv_v03_l1;
ANALYZE farol.mv_v03_l2;
ANALYZE farol.mv_v03_l3;
