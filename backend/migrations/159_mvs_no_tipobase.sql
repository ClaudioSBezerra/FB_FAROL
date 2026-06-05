-- 159_mvs_no_tipobase.sql
-- Recria as 28 MVs (14 FAT + 14 TRANS) sem tipo_base na chave.
-- Estrutura idêntica à 157, apenas remove tipo_base do GROUP BY e dos índices.

-- ═══════════════════════════════════════════════════════════════════════════════
-- VENDAS FATURADAS — 14 MVs
-- ═══════════════════════════════════════════════════════════════════════════════

CREATE MATERIALIZED VIEW farol.mv_fat_cli AS
SELECT
    empresa_id, data_faturamento,
    EXTRACT(YEAR  FROM data_faturamento)::int AS ano,
    EXTRACT(MONTH FROM data_faturamento)::int AS mes,
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
    MAX(qtcli_rca)        AS qtcli_rca,
    MAX(qtrca_supervisor) AS qtrca_supervisor,
    SUM(pvenda) AS pvenda, SUM(plucro) AS plucro, SUM(qt) AS qt,
    1::int                                          AS base_cli,
    (CASE WHEN SUM(qt) > 0 THEN 1 ELSE 0 END)::int  AS positivados,
    COUNT(DISTINCT NULLIF(cod_prod, ''))::float     AS mix
FROM vendas_faturadas
GROUP BY empresa_id, data_faturamento,
    cod_fornec, cod_gerente, cod_supervisor, cod_rca, cod_cli, empresa, uf
WITH NO DATA;

CREATE UNIQUE INDEX idx_mvfatcli_pk ON farol.mv_fat_cli
    (empresa_id, data_faturamento, cod_fornec, cod_gerente, cod_supervisor, cod_rca, cod_cli, empresa, uf);
CREATE INDEX idx_mvfatcli_data   ON farol.mv_fat_cli (empresa_id, data_faturamento);
CREATE INDEX idx_mvfatcli_anomes ON farol.mv_fat_cli (empresa_id, ano, mes);
CREATE INDEX idx_mvfatcli_inativos ON farol.mv_fat_cli (empresa_id, ano, mes, cod_cli) WHERE positivados = 0;
REFRESH MATERIALIZED VIEW farol.mv_fat_cli;
ANALYZE farol.mv_fat_cli;

CREATE MATERIALIZED VIEW farol.mv_fat_v01_l0 AS
SELECT empresa_id, data_faturamento, ano, mes, cod_fornec, MAX(nome_fornec) AS nome_fornec,
    CASE WHEN MAX(qtcli_rca) > 0 THEN MAX(qtcli_rca)::int ELSE COUNT(DISTINCT cod_cli)::int END AS base_cli,
    COUNT(DISTINCT cod_cli) FILTER (WHERE qt > 0) AS positivados,
    COALESCE(AVG(mix) FILTER (WHERE qt > 0), 0) AS mix,
    SUM(pvenda) AS pvenda, SUM(plucro) AS plucro, SUM(qt) AS qt
FROM farol.mv_fat_cli WHERE cod_fornec != ''
GROUP BY empresa_id, data_faturamento, ano, mes, cod_fornec WITH NO DATA;
CREATE UNIQUE INDEX idx_mvfatv01l0_pk ON farol.mv_fat_v01_l0 (empresa_id, data_faturamento, cod_fornec);
CREATE INDEX idx_mvfatv01l0_anomes ON farol.mv_fat_v01_l0 (empresa_id, ano, mes);
REFRESH MATERIALIZED VIEW farol.mv_fat_v01_l0;

CREATE MATERIALIZED VIEW farol.mv_fat_v01_l1 AS
SELECT empresa_id, data_faturamento, ano, mes, cod_fornec, cod_gerente, MAX(nome_gerente) AS nome_gerente,
    CASE WHEN MAX(qtcli_rca) > 0 THEN MAX(qtcli_rca)::int ELSE COUNT(DISTINCT cod_cli)::int END AS base_cli,
    COUNT(DISTINCT cod_cli) FILTER (WHERE qt > 0) AS positivados,
    COALESCE(AVG(mix) FILTER (WHERE qt > 0), 0) AS mix,
    SUM(pvenda) AS pvenda, SUM(plucro) AS plucro, SUM(qt) AS qt
FROM farol.mv_fat_cli WHERE cod_gerente != ''
GROUP BY empresa_id, data_faturamento, ano, mes, cod_fornec, cod_gerente WITH NO DATA;
CREATE UNIQUE INDEX idx_mvfatv01l1_pk ON farol.mv_fat_v01_l1 (empresa_id, data_faturamento, cod_fornec, cod_gerente);
CREATE INDEX idx_mvfatv01l1_anomes ON farol.mv_fat_v01_l1 (empresa_id, ano, mes);
REFRESH MATERIALIZED VIEW farol.mv_fat_v01_l1;

CREATE MATERIALIZED VIEW farol.mv_fat_v01_l2 AS
SELECT empresa_id, data_faturamento, ano, mes, cod_fornec, cod_gerente, cod_supervisor, MAX(nome_supervisor) AS nome_supervisor,
    CASE WHEN MAX(qtcli_rca) > 0 THEN MAX(qtcli_rca)::int ELSE COUNT(DISTINCT cod_cli)::int END AS base_cli,
    COUNT(DISTINCT cod_cli) FILTER (WHERE qt > 0) AS positivados,
    COALESCE(AVG(mix) FILTER (WHERE qt > 0), 0) AS mix,
    SUM(pvenda) AS pvenda, SUM(plucro) AS plucro, SUM(qt) AS qt
FROM farol.mv_fat_cli WHERE cod_supervisor != ''
GROUP BY empresa_id, data_faturamento, ano, mes, cod_fornec, cod_gerente, cod_supervisor WITH NO DATA;
CREATE UNIQUE INDEX idx_mvfatv01l2_pk ON farol.mv_fat_v01_l2 (empresa_id, data_faturamento, cod_fornec, cod_gerente, cod_supervisor);
CREATE INDEX idx_mvfatv01l2_anomes ON farol.mv_fat_v01_l2 (empresa_id, ano, mes);
REFRESH MATERIALIZED VIEW farol.mv_fat_v01_l2;

CREATE MATERIALIZED VIEW farol.mv_fat_v01_l3 AS
SELECT empresa_id, data_faturamento, ano, mes, cod_fornec, cod_gerente, cod_supervisor, cod_rca, MAX(nome_rca) AS nome_rca,
    CASE WHEN MAX(qtcli_rca) > 0 THEN MAX(qtcli_rca)::int ELSE COUNT(DISTINCT cod_cli)::int END AS base_cli,
    COUNT(DISTINCT cod_cli) FILTER (WHERE qt > 0) AS positivados,
    COALESCE(AVG(mix) FILTER (WHERE qt > 0), 0) AS mix,
    SUM(pvenda) AS pvenda, SUM(plucro) AS plucro, SUM(qt) AS qt
FROM farol.mv_fat_cli WHERE cod_rca != ''
GROUP BY empresa_id, data_faturamento, ano, mes, cod_fornec, cod_gerente, cod_supervisor, cod_rca WITH NO DATA;
CREATE UNIQUE INDEX idx_mvfatv01l3_pk ON farol.mv_fat_v01_l3 (empresa_id, data_faturamento, cod_fornec, cod_gerente, cod_supervisor, cod_rca);
CREATE INDEX idx_mvfatv01l3_anomes ON farol.mv_fat_v01_l3 (empresa_id, ano, mes);
REFRESH MATERIALIZED VIEW farol.mv_fat_v01_l3;

CREATE MATERIALIZED VIEW farol.mv_fat_v02_l0 AS
SELECT empresa_id, data_faturamento, ano, mes, cod_supervisor, MAX(nome_supervisor) AS nome_supervisor,
    CASE WHEN MAX(qtcli_rca) > 0 THEN MAX(qtcli_rca)::int ELSE COUNT(DISTINCT cod_cli)::int END AS base_cli,
    COUNT(DISTINCT cod_cli) FILTER (WHERE qt > 0) AS positivados,
    COALESCE(AVG(mix) FILTER (WHERE qt > 0), 0) AS mix,
    SUM(pvenda) AS pvenda, SUM(plucro) AS plucro, SUM(qt) AS qt
FROM farol.mv_fat_cli WHERE cod_supervisor != ''
GROUP BY empresa_id, data_faturamento, ano, mes, cod_supervisor WITH NO DATA;
CREATE UNIQUE INDEX idx_mvfatv02l0_pk ON farol.mv_fat_v02_l0 (empresa_id, data_faturamento, cod_supervisor);
CREATE INDEX idx_mvfatv02l0_anomes ON farol.mv_fat_v02_l0 (empresa_id, ano, mes);
REFRESH MATERIALIZED VIEW farol.mv_fat_v02_l0;

CREATE MATERIALIZED VIEW farol.mv_fat_v02_l1 AS
SELECT empresa_id, data_faturamento, ano, mes, cod_supervisor, cod_rca, MAX(nome_rca) AS nome_rca,
    CASE WHEN MAX(qtcli_rca) > 0 THEN MAX(qtcli_rca)::int ELSE COUNT(DISTINCT cod_cli)::int END AS base_cli,
    COUNT(DISTINCT cod_cli) FILTER (WHERE qt > 0) AS positivados,
    COALESCE(AVG(mix) FILTER (WHERE qt > 0), 0) AS mix,
    SUM(pvenda) AS pvenda, SUM(plucro) AS plucro, SUM(qt) AS qt
FROM farol.mv_fat_cli WHERE cod_supervisor != '' AND cod_rca != ''
GROUP BY empresa_id, data_faturamento, ano, mes, cod_supervisor, cod_rca WITH NO DATA;
CREATE UNIQUE INDEX idx_mvfatv02l1_pk ON farol.mv_fat_v02_l1 (empresa_id, data_faturamento, cod_supervisor, cod_rca);
CREATE INDEX idx_mvfatv02l1_anomes ON farol.mv_fat_v02_l1 (empresa_id, ano, mes);
REFRESH MATERIALIZED VIEW farol.mv_fat_v02_l1;

CREATE MATERIALIZED VIEW farol.mv_fat_v02_l2 AS
SELECT empresa_id, data_faturamento, ano, mes, cod_supervisor, cod_rca, cod_fornec, MAX(nome_fornec) AS nome_fornec,
    CASE WHEN MAX(qtcli_rca) > 0 THEN MAX(qtcli_rca)::int ELSE COUNT(DISTINCT cod_cli)::int END AS base_cli,
    COUNT(DISTINCT cod_cli) FILTER (WHERE qt > 0) AS positivados,
    COALESCE(AVG(mix) FILTER (WHERE qt > 0), 0) AS mix,
    SUM(pvenda) AS pvenda, SUM(plucro) AS plucro, SUM(qt) AS qt
FROM farol.mv_fat_cli WHERE cod_supervisor != '' AND cod_rca != '' AND cod_fornec != ''
GROUP BY empresa_id, data_faturamento, ano, mes, cod_supervisor, cod_rca, cod_fornec WITH NO DATA;
CREATE UNIQUE INDEX idx_mvfatv02l2_pk ON farol.mv_fat_v02_l2 (empresa_id, data_faturamento, cod_supervisor, cod_rca, cod_fornec);
CREATE INDEX idx_mvfatv02l2_anomes ON farol.mv_fat_v02_l2 (empresa_id, ano, mes);
REFRESH MATERIALIZED VIEW farol.mv_fat_v02_l2;

CREATE MATERIALIZED VIEW farol.mv_fat_v03_l0 AS
SELECT empresa_id, data_faturamento, ano, mes, cod_gerente, MAX(nome_gerente) AS nome_gerente,
    CASE WHEN MAX(qtcli_rca) > 0 THEN MAX(qtcli_rca)::int ELSE COUNT(DISTINCT cod_cli)::int END AS base_cli,
    COUNT(DISTINCT cod_cli) FILTER (WHERE qt > 0) AS positivados,
    COALESCE(AVG(mix) FILTER (WHERE qt > 0), 0) AS mix,
    SUM(pvenda) AS pvenda, SUM(plucro) AS plucro, SUM(qt) AS qt
FROM farol.mv_fat_cli WHERE cod_gerente != ''
GROUP BY empresa_id, data_faturamento, ano, mes, cod_gerente WITH NO DATA;
CREATE UNIQUE INDEX idx_mvfatv03l0_pk ON farol.mv_fat_v03_l0 (empresa_id, data_faturamento, cod_gerente);
CREATE INDEX idx_mvfatv03l0_anomes ON farol.mv_fat_v03_l0 (empresa_id, ano, mes);
REFRESH MATERIALIZED VIEW farol.mv_fat_v03_l0;

CREATE MATERIALIZED VIEW farol.mv_fat_v03_l1 AS
SELECT empresa_id, data_faturamento, ano, mes, cod_gerente, cod_supervisor, MAX(nome_supervisor) AS nome_supervisor,
    CASE WHEN MAX(qtcli_rca) > 0 THEN MAX(qtcli_rca)::int ELSE COUNT(DISTINCT cod_cli)::int END AS base_cli,
    COUNT(DISTINCT cod_cli) FILTER (WHERE qt > 0) AS positivados,
    COALESCE(AVG(mix) FILTER (WHERE qt > 0), 0) AS mix,
    SUM(pvenda) AS pvenda, SUM(plucro) AS plucro, SUM(qt) AS qt
FROM farol.mv_fat_cli WHERE cod_gerente != '' AND cod_supervisor != ''
GROUP BY empresa_id, data_faturamento, ano, mes, cod_gerente, cod_supervisor WITH NO DATA;
CREATE UNIQUE INDEX idx_mvfatv03l1_pk ON farol.mv_fat_v03_l1 (empresa_id, data_faturamento, cod_gerente, cod_supervisor);
CREATE INDEX idx_mvfatv03l1_anomes ON farol.mv_fat_v03_l1 (empresa_id, ano, mes);
REFRESH MATERIALIZED VIEW farol.mv_fat_v03_l1;

CREATE MATERIALIZED VIEW farol.mv_fat_v03_l2 AS
SELECT empresa_id, data_faturamento, ano, mes, cod_gerente, cod_supervisor, cod_rca, MAX(nome_rca) AS nome_rca,
    CASE WHEN MAX(qtcli_rca) > 0 THEN MAX(qtcli_rca)::int ELSE COUNT(DISTINCT cod_cli)::int END AS base_cli,
    COUNT(DISTINCT cod_cli) FILTER (WHERE qt > 0) AS positivados,
    COALESCE(AVG(mix) FILTER (WHERE qt > 0), 0) AS mix,
    SUM(pvenda) AS pvenda, SUM(plucro) AS plucro, SUM(qt) AS qt
FROM farol.mv_fat_cli WHERE cod_gerente != '' AND cod_supervisor != '' AND cod_rca != ''
GROUP BY empresa_id, data_faturamento, ano, mes, cod_gerente, cod_supervisor, cod_rca WITH NO DATA;
CREATE UNIQUE INDEX idx_mvfatv03l2_pk ON farol.mv_fat_v03_l2 (empresa_id, data_faturamento, cod_gerente, cod_supervisor, cod_rca);
CREATE INDEX idx_mvfatv03l2_anomes ON farol.mv_fat_v03_l2 (empresa_id, ano, mes);
REFRESH MATERIALIZED VIEW farol.mv_fat_v03_l2;

CREATE MATERIALIZED VIEW farol.mv_fat_v03_l3 AS
SELECT empresa_id, data_faturamento, ano, mes, cod_gerente, cod_supervisor, cod_rca, cod_cli, MAX(nome_cli) AS nome_cli,
    1::int                                            AS base_cli,
    (CASE WHEN SUM(qt) > 0 THEN 1 ELSE 0 END)::int    AS positivados,
    SUM(mix) AS mix,
    SUM(pvenda) AS pvenda, SUM(plucro) AS plucro, SUM(qt) AS qt
FROM farol.mv_fat_cli WHERE cod_gerente != '' AND cod_supervisor != '' AND cod_rca != '' AND cod_cli != ''
GROUP BY empresa_id, data_faturamento, ano, mes, cod_gerente, cod_supervisor, cod_rca, cod_cli WITH NO DATA;
CREATE UNIQUE INDEX idx_mvfatv03l3_pk ON farol.mv_fat_v03_l3 (empresa_id, data_faturamento, cod_gerente, cod_supervisor, cod_rca, cod_cli);
CREATE INDEX idx_mvfatv03l3_anomes ON farol.mv_fat_v03_l3 (empresa_id, ano, mes);
REFRESH MATERIALIZED VIEW farol.mv_fat_v03_l3;

CREATE MATERIALIZED VIEW farol.mv_fat_mkt_produto AS
SELECT empresa_id, data_faturamento,
    EXTRACT(YEAR  FROM data_faturamento)::int AS ano,
    EXTRACT(MONTH FROM data_faturamento)::int AS mes,
    COALESCE(cod_fornec, '')  AS cod_fornec, MAX(COALESCE(nome_fornec, '')) AS nome_fornec,
    COALESCE(cod_prod, '')    AS cod_prod,   MAX(COALESCE(nome_prod, ''))   AS nome_prod,
    MAX(COALESCE(ean, ''))    AS ean,
    COUNT(DISTINCT NULLIF(cod_cli, ''))                AS qt_clientes,
    COUNT(DISTINCT CASE WHEN qt > 0 THEN cod_cli END)  AS qt_positivados,
    SUM(pvenda) AS pvenda, SUM(plucro) AS plucro, SUM(qt) AS qt
FROM vendas_faturadas WHERE cod_prod != ''
GROUP BY empresa_id, data_faturamento, cod_fornec, cod_prod WITH NO DATA;
CREATE UNIQUE INDEX idx_mvfatmktp_pk ON farol.mv_fat_mkt_produto (empresa_id, data_faturamento, cod_fornec, cod_prod);
CREATE INDEX idx_mvfatmktp_anomes ON farol.mv_fat_mkt_produto (empresa_id, ano, mes);
REFRESH MATERIALIZED VIEW farol.mv_fat_mkt_produto;

CREATE MATERIALIZED VIEW farol.mv_fat_mkt_prod_pen AS
SELECT empresa_id, data_faturamento,
    EXTRACT(YEAR  FROM data_faturamento)::int AS ano,
    EXTRACT(MONTH FROM data_faturamento)::int AS mes,
    COALESCE(cod_prod, '')  AS cod_prod, MAX(COALESCE(nome_prod,  ''))  AS nome_prod,
    MAX(COALESCE(cod_fornec, '')) AS cod_fornec, MAX(COALESCE(nome_fornec,'')) AS nome_fornec,
    MAX(COALESCE(ean, '')) AS ean,
    COUNT(DISTINCT CASE WHEN qt > 0 THEN cod_cli END) AS qt_positivados,
    SUM(pvenda) AS pvenda, SUM(plucro) AS plucro, SUM(qt) AS qt
FROM vendas_faturadas WHERE cod_prod != ''
GROUP BY empresa_id, data_faturamento, cod_prod WITH NO DATA;
CREATE UNIQUE INDEX idx_mvfatmktpp_pk ON farol.mv_fat_mkt_prod_pen (empresa_id, data_faturamento, cod_prod);
CREATE INDEX idx_mvfatmktpp_anomes ON farol.mv_fat_mkt_prod_pen (empresa_id, ano, mes);
REFRESH MATERIALIZED VIEW farol.mv_fat_mkt_prod_pen;

-- ═══════════════════════════════════════════════════════════════════════════════
-- VENDAS TRANSMITIDAS — 14 MVs
-- ═══════════════════════════════════════════════════════════════════════════════

CREATE MATERIALIZED VIEW farol.mv_trans_cli AS
SELECT
    empresa_id, data_transmissao,
    EXTRACT(YEAR  FROM data_transmissao)::int AS ano,
    EXTRACT(MONTH FROM data_transmissao)::int AS mes,
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
    MAX(qtcli_rca)        AS qtcli_rca,
    MAX(qtrca_supervisor) AS qtrca_supervisor,
    SUM(pvenda) AS pvenda, SUM(plucro) AS plucro, SUM(qt) AS qt,
    1::int                                          AS base_cli,
    (CASE WHEN SUM(qt) > 0 THEN 1 ELSE 0 END)::int  AS positivados,
    COUNT(DISTINCT NULLIF(cod_prod, ''))::float     AS mix
FROM vendas_transmitidas
GROUP BY empresa_id, data_transmissao,
    cod_fornec, cod_gerente, cod_supervisor, cod_rca, cod_cli, empresa, uf
WITH NO DATA;

CREATE UNIQUE INDEX idx_mvtranscli_pk ON farol.mv_trans_cli
    (empresa_id, data_transmissao, cod_fornec, cod_gerente, cod_supervisor, cod_rca, cod_cli, empresa, uf);
CREATE INDEX idx_mvtranscli_data   ON farol.mv_trans_cli (empresa_id, data_transmissao);
CREATE INDEX idx_mvtranscli_anomes ON farol.mv_trans_cli (empresa_id, ano, mes);
CREATE INDEX idx_mvtranscli_inativos ON farol.mv_trans_cli (empresa_id, ano, mes, cod_cli) WHERE positivados = 0;
REFRESH MATERIALIZED VIEW farol.mv_trans_cli;
ANALYZE farol.mv_trans_cli;

CREATE MATERIALIZED VIEW farol.mv_trans_v01_l0 AS
SELECT empresa_id, data_transmissao, ano, mes, cod_fornec, MAX(nome_fornec) AS nome_fornec,
    CASE WHEN MAX(qtcli_rca) > 0 THEN MAX(qtcli_rca)::int ELSE COUNT(DISTINCT cod_cli)::int END AS base_cli,
    COUNT(DISTINCT cod_cli) FILTER (WHERE qt > 0) AS positivados,
    COALESCE(AVG(mix) FILTER (WHERE qt > 0), 0) AS mix,
    SUM(pvenda) AS pvenda, SUM(plucro) AS plucro, SUM(qt) AS qt
FROM farol.mv_trans_cli WHERE cod_fornec != ''
GROUP BY empresa_id, data_transmissao, ano, mes, cod_fornec WITH NO DATA;
CREATE UNIQUE INDEX idx_mvtransv01l0_pk ON farol.mv_trans_v01_l0 (empresa_id, data_transmissao, cod_fornec);
CREATE INDEX idx_mvtransv01l0_anomes ON farol.mv_trans_v01_l0 (empresa_id, ano, mes);
REFRESH MATERIALIZED VIEW farol.mv_trans_v01_l0;

CREATE MATERIALIZED VIEW farol.mv_trans_v01_l1 AS
SELECT empresa_id, data_transmissao, ano, mes, cod_fornec, cod_gerente, MAX(nome_gerente) AS nome_gerente,
    CASE WHEN MAX(qtcli_rca) > 0 THEN MAX(qtcli_rca)::int ELSE COUNT(DISTINCT cod_cli)::int END AS base_cli,
    COUNT(DISTINCT cod_cli) FILTER (WHERE qt > 0) AS positivados,
    COALESCE(AVG(mix) FILTER (WHERE qt > 0), 0) AS mix,
    SUM(pvenda) AS pvenda, SUM(plucro) AS plucro, SUM(qt) AS qt
FROM farol.mv_trans_cli WHERE cod_gerente != ''
GROUP BY empresa_id, data_transmissao, ano, mes, cod_fornec, cod_gerente WITH NO DATA;
CREATE UNIQUE INDEX idx_mvtransv01l1_pk ON farol.mv_trans_v01_l1 (empresa_id, data_transmissao, cod_fornec, cod_gerente);
CREATE INDEX idx_mvtransv01l1_anomes ON farol.mv_trans_v01_l1 (empresa_id, ano, mes);
REFRESH MATERIALIZED VIEW farol.mv_trans_v01_l1;

CREATE MATERIALIZED VIEW farol.mv_trans_v01_l2 AS
SELECT empresa_id, data_transmissao, ano, mes, cod_fornec, cod_gerente, cod_supervisor, MAX(nome_supervisor) AS nome_supervisor,
    CASE WHEN MAX(qtcli_rca) > 0 THEN MAX(qtcli_rca)::int ELSE COUNT(DISTINCT cod_cli)::int END AS base_cli,
    COUNT(DISTINCT cod_cli) FILTER (WHERE qt > 0) AS positivados,
    COALESCE(AVG(mix) FILTER (WHERE qt > 0), 0) AS mix,
    SUM(pvenda) AS pvenda, SUM(plucro) AS plucro, SUM(qt) AS qt
FROM farol.mv_trans_cli WHERE cod_supervisor != ''
GROUP BY empresa_id, data_transmissao, ano, mes, cod_fornec, cod_gerente, cod_supervisor WITH NO DATA;
CREATE UNIQUE INDEX idx_mvtransv01l2_pk ON farol.mv_trans_v01_l2 (empresa_id, data_transmissao, cod_fornec, cod_gerente, cod_supervisor);
CREATE INDEX idx_mvtransv01l2_anomes ON farol.mv_trans_v01_l2 (empresa_id, ano, mes);
REFRESH MATERIALIZED VIEW farol.mv_trans_v01_l2;

CREATE MATERIALIZED VIEW farol.mv_trans_v01_l3 AS
SELECT empresa_id, data_transmissao, ano, mes, cod_fornec, cod_gerente, cod_supervisor, cod_rca, MAX(nome_rca) AS nome_rca,
    CASE WHEN MAX(qtcli_rca) > 0 THEN MAX(qtcli_rca)::int ELSE COUNT(DISTINCT cod_cli)::int END AS base_cli,
    COUNT(DISTINCT cod_cli) FILTER (WHERE qt > 0) AS positivados,
    COALESCE(AVG(mix) FILTER (WHERE qt > 0), 0) AS mix,
    SUM(pvenda) AS pvenda, SUM(plucro) AS plucro, SUM(qt) AS qt
FROM farol.mv_trans_cli WHERE cod_rca != ''
GROUP BY empresa_id, data_transmissao, ano, mes, cod_fornec, cod_gerente, cod_supervisor, cod_rca WITH NO DATA;
CREATE UNIQUE INDEX idx_mvtransv01l3_pk ON farol.mv_trans_v01_l3 (empresa_id, data_transmissao, cod_fornec, cod_gerente, cod_supervisor, cod_rca);
CREATE INDEX idx_mvtransv01l3_anomes ON farol.mv_trans_v01_l3 (empresa_id, ano, mes);
REFRESH MATERIALIZED VIEW farol.mv_trans_v01_l3;

CREATE MATERIALIZED VIEW farol.mv_trans_v02_l0 AS
SELECT empresa_id, data_transmissao, ano, mes, cod_supervisor, MAX(nome_supervisor) AS nome_supervisor,
    CASE WHEN MAX(qtcli_rca) > 0 THEN MAX(qtcli_rca)::int ELSE COUNT(DISTINCT cod_cli)::int END AS base_cli,
    COUNT(DISTINCT cod_cli) FILTER (WHERE qt > 0) AS positivados,
    COALESCE(AVG(mix) FILTER (WHERE qt > 0), 0) AS mix,
    SUM(pvenda) AS pvenda, SUM(plucro) AS plucro, SUM(qt) AS qt
FROM farol.mv_trans_cli WHERE cod_supervisor != ''
GROUP BY empresa_id, data_transmissao, ano, mes, cod_supervisor WITH NO DATA;
CREATE UNIQUE INDEX idx_mvtransv02l0_pk ON farol.mv_trans_v02_l0 (empresa_id, data_transmissao, cod_supervisor);
CREATE INDEX idx_mvtransv02l0_anomes ON farol.mv_trans_v02_l0 (empresa_id, ano, mes);
REFRESH MATERIALIZED VIEW farol.mv_trans_v02_l0;

CREATE MATERIALIZED VIEW farol.mv_trans_v02_l1 AS
SELECT empresa_id, data_transmissao, ano, mes, cod_supervisor, cod_rca, MAX(nome_rca) AS nome_rca,
    CASE WHEN MAX(qtcli_rca) > 0 THEN MAX(qtcli_rca)::int ELSE COUNT(DISTINCT cod_cli)::int END AS base_cli,
    COUNT(DISTINCT cod_cli) FILTER (WHERE qt > 0) AS positivados,
    COALESCE(AVG(mix) FILTER (WHERE qt > 0), 0) AS mix,
    SUM(pvenda) AS pvenda, SUM(plucro) AS plucro, SUM(qt) AS qt
FROM farol.mv_trans_cli WHERE cod_supervisor != '' AND cod_rca != ''
GROUP BY empresa_id, data_transmissao, ano, mes, cod_supervisor, cod_rca WITH NO DATA;
CREATE UNIQUE INDEX idx_mvtransv02l1_pk ON farol.mv_trans_v02_l1 (empresa_id, data_transmissao, cod_supervisor, cod_rca);
CREATE INDEX idx_mvtransv02l1_anomes ON farol.mv_trans_v02_l1 (empresa_id, ano, mes);
REFRESH MATERIALIZED VIEW farol.mv_trans_v02_l1;

CREATE MATERIALIZED VIEW farol.mv_trans_v02_l2 AS
SELECT empresa_id, data_transmissao, ano, mes, cod_supervisor, cod_rca, cod_fornec, MAX(nome_fornec) AS nome_fornec,
    CASE WHEN MAX(qtcli_rca) > 0 THEN MAX(qtcli_rca)::int ELSE COUNT(DISTINCT cod_cli)::int END AS base_cli,
    COUNT(DISTINCT cod_cli) FILTER (WHERE qt > 0) AS positivados,
    COALESCE(AVG(mix) FILTER (WHERE qt > 0), 0) AS mix,
    SUM(pvenda) AS pvenda, SUM(plucro) AS plucro, SUM(qt) AS qt
FROM farol.mv_trans_cli WHERE cod_supervisor != '' AND cod_rca != '' AND cod_fornec != ''
GROUP BY empresa_id, data_transmissao, ano, mes, cod_supervisor, cod_rca, cod_fornec WITH NO DATA;
CREATE UNIQUE INDEX idx_mvtransv02l2_pk ON farol.mv_trans_v02_l2 (empresa_id, data_transmissao, cod_supervisor, cod_rca, cod_fornec);
CREATE INDEX idx_mvtransv02l2_anomes ON farol.mv_trans_v02_l2 (empresa_id, ano, mes);
REFRESH MATERIALIZED VIEW farol.mv_trans_v02_l2;

CREATE MATERIALIZED VIEW farol.mv_trans_v03_l0 AS
SELECT empresa_id, data_transmissao, ano, mes, cod_gerente, MAX(nome_gerente) AS nome_gerente,
    CASE WHEN MAX(qtcli_rca) > 0 THEN MAX(qtcli_rca)::int ELSE COUNT(DISTINCT cod_cli)::int END AS base_cli,
    COUNT(DISTINCT cod_cli) FILTER (WHERE qt > 0) AS positivados,
    COALESCE(AVG(mix) FILTER (WHERE qt > 0), 0) AS mix,
    SUM(pvenda) AS pvenda, SUM(plucro) AS plucro, SUM(qt) AS qt
FROM farol.mv_trans_cli WHERE cod_gerente != ''
GROUP BY empresa_id, data_transmissao, ano, mes, cod_gerente WITH NO DATA;
CREATE UNIQUE INDEX idx_mvtransv03l0_pk ON farol.mv_trans_v03_l0 (empresa_id, data_transmissao, cod_gerente);
CREATE INDEX idx_mvtransv03l0_anomes ON farol.mv_trans_v03_l0 (empresa_id, ano, mes);
REFRESH MATERIALIZED VIEW farol.mv_trans_v03_l0;

CREATE MATERIALIZED VIEW farol.mv_trans_v03_l1 AS
SELECT empresa_id, data_transmissao, ano, mes, cod_gerente, cod_supervisor, MAX(nome_supervisor) AS nome_supervisor,
    CASE WHEN MAX(qtcli_rca) > 0 THEN MAX(qtcli_rca)::int ELSE COUNT(DISTINCT cod_cli)::int END AS base_cli,
    COUNT(DISTINCT cod_cli) FILTER (WHERE qt > 0) AS positivados,
    COALESCE(AVG(mix) FILTER (WHERE qt > 0), 0) AS mix,
    SUM(pvenda) AS pvenda, SUM(plucro) AS plucro, SUM(qt) AS qt
FROM farol.mv_trans_cli WHERE cod_gerente != '' AND cod_supervisor != ''
GROUP BY empresa_id, data_transmissao, ano, mes, cod_gerente, cod_supervisor WITH NO DATA;
CREATE UNIQUE INDEX idx_mvtransv03l1_pk ON farol.mv_trans_v03_l1 (empresa_id, data_transmissao, cod_gerente, cod_supervisor);
CREATE INDEX idx_mvtransv03l1_anomes ON farol.mv_trans_v03_l1 (empresa_id, ano, mes);
REFRESH MATERIALIZED VIEW farol.mv_trans_v03_l1;

CREATE MATERIALIZED VIEW farol.mv_trans_v03_l2 AS
SELECT empresa_id, data_transmissao, ano, mes, cod_gerente, cod_supervisor, cod_rca, MAX(nome_rca) AS nome_rca,
    CASE WHEN MAX(qtcli_rca) > 0 THEN MAX(qtcli_rca)::int ELSE COUNT(DISTINCT cod_cli)::int END AS base_cli,
    COUNT(DISTINCT cod_cli) FILTER (WHERE qt > 0) AS positivados,
    COALESCE(AVG(mix) FILTER (WHERE qt > 0), 0) AS mix,
    SUM(pvenda) AS pvenda, SUM(plucro) AS plucro, SUM(qt) AS qt
FROM farol.mv_trans_cli WHERE cod_gerente != '' AND cod_supervisor != '' AND cod_rca != ''
GROUP BY empresa_id, data_transmissao, ano, mes, cod_gerente, cod_supervisor, cod_rca WITH NO DATA;
CREATE UNIQUE INDEX idx_mvtransv03l2_pk ON farol.mv_trans_v03_l2 (empresa_id, data_transmissao, cod_gerente, cod_supervisor, cod_rca);
CREATE INDEX idx_mvtransv03l2_anomes ON farol.mv_trans_v03_l2 (empresa_id, ano, mes);
REFRESH MATERIALIZED VIEW farol.mv_trans_v03_l2;

CREATE MATERIALIZED VIEW farol.mv_trans_v03_l3 AS
SELECT empresa_id, data_transmissao, ano, mes, cod_gerente, cod_supervisor, cod_rca, cod_cli, MAX(nome_cli) AS nome_cli,
    1::int                                            AS base_cli,
    (CASE WHEN SUM(qt) > 0 THEN 1 ELSE 0 END)::int    AS positivados,
    SUM(mix) AS mix,
    SUM(pvenda) AS pvenda, SUM(plucro) AS plucro, SUM(qt) AS qt
FROM farol.mv_trans_cli WHERE cod_gerente != '' AND cod_supervisor != '' AND cod_rca != '' AND cod_cli != ''
GROUP BY empresa_id, data_transmissao, ano, mes, cod_gerente, cod_supervisor, cod_rca, cod_cli WITH NO DATA;
CREATE UNIQUE INDEX idx_mvtransv03l3_pk ON farol.mv_trans_v03_l3 (empresa_id, data_transmissao, cod_gerente, cod_supervisor, cod_rca, cod_cli);
CREATE INDEX idx_mvtransv03l3_anomes ON farol.mv_trans_v03_l3 (empresa_id, ano, mes);
REFRESH MATERIALIZED VIEW farol.mv_trans_v03_l3;

CREATE MATERIALIZED VIEW farol.mv_trans_mkt_produto AS
SELECT empresa_id, data_transmissao,
    EXTRACT(YEAR  FROM data_transmissao)::int AS ano,
    EXTRACT(MONTH FROM data_transmissao)::int AS mes,
    COALESCE(cod_fornec, '')  AS cod_fornec, MAX(COALESCE(nome_fornec, '')) AS nome_fornec,
    COALESCE(cod_prod, '')    AS cod_prod,   MAX(COALESCE(nome_prod, ''))   AS nome_prod,
    MAX(COALESCE(ean, ''))    AS ean,
    COUNT(DISTINCT NULLIF(cod_cli, ''))                AS qt_clientes,
    COUNT(DISTINCT CASE WHEN qt > 0 THEN cod_cli END)  AS qt_positivados,
    SUM(pvenda) AS pvenda, SUM(plucro) AS plucro, SUM(qt) AS qt
FROM vendas_transmitidas WHERE cod_prod != ''
GROUP BY empresa_id, data_transmissao, cod_fornec, cod_prod WITH NO DATA;
CREATE UNIQUE INDEX idx_mvtransmktp_pk ON farol.mv_trans_mkt_produto (empresa_id, data_transmissao, cod_fornec, cod_prod);
CREATE INDEX idx_mvtransmktp_anomes ON farol.mv_trans_mkt_produto (empresa_id, ano, mes);
REFRESH MATERIALIZED VIEW farol.mv_trans_mkt_produto;

CREATE MATERIALIZED VIEW farol.mv_trans_mkt_prod_pen AS
SELECT empresa_id, data_transmissao,
    EXTRACT(YEAR  FROM data_transmissao)::int AS ano,
    EXTRACT(MONTH FROM data_transmissao)::int AS mes,
    COALESCE(cod_prod, '')  AS cod_prod, MAX(COALESCE(nome_prod,  ''))  AS nome_prod,
    MAX(COALESCE(cod_fornec, '')) AS cod_fornec, MAX(COALESCE(nome_fornec,'')) AS nome_fornec,
    MAX(COALESCE(ean, '')) AS ean,
    COUNT(DISTINCT CASE WHEN qt > 0 THEN cod_cli END) AS qt_positivados,
    SUM(pvenda) AS pvenda, SUM(plucro) AS plucro, SUM(qt) AS qt
FROM vendas_transmitidas WHERE cod_prod != ''
GROUP BY empresa_id, data_transmissao, cod_prod WITH NO DATA;
CREATE UNIQUE INDEX idx_mvtransmktpp_pk ON farol.mv_trans_mkt_prod_pen (empresa_id, data_transmissao, cod_prod);
CREATE INDEX idx_mvtransmktpp_anomes ON farol.mv_trans_mkt_prod_pen (empresa_id, ano, mes);
REFRESH MATERIALIZED VIEW farol.mv_trans_mkt_prod_pen;

ANALYZE farol.mv_fat_v01_l0; ANALYZE farol.mv_fat_v01_l1; ANALYZE farol.mv_fat_v01_l2; ANALYZE farol.mv_fat_v01_l3;
ANALYZE farol.mv_fat_v02_l0; ANALYZE farol.mv_fat_v02_l1; ANALYZE farol.mv_fat_v02_l2;
ANALYZE farol.mv_fat_v03_l0; ANALYZE farol.mv_fat_v03_l1; ANALYZE farol.mv_fat_v03_l2; ANALYZE farol.mv_fat_v03_l3;
ANALYZE farol.mv_fat_mkt_produto; ANALYZE farol.mv_fat_mkt_prod_pen;
ANALYZE farol.mv_trans_v01_l0; ANALYZE farol.mv_trans_v01_l1; ANALYZE farol.mv_trans_v01_l2; ANALYZE farol.mv_trans_v01_l3;
ANALYZE farol.mv_trans_v02_l0; ANALYZE farol.mv_trans_v02_l1; ANALYZE farol.mv_trans_v02_l2;
ANALYZE farol.mv_trans_v03_l0; ANALYZE farol.mv_trans_v03_l1; ANALYZE farol.mv_trans_v03_l2; ANALYZE farol.mv_trans_v03_l3;
ANALYZE farol.mv_trans_mkt_produto; ANALYZE farol.mv_trans_mkt_prod_pen;
