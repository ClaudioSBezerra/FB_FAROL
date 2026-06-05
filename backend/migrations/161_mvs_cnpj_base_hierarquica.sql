-- 161_mvs_cnpj_base_hierarquica.sql
-- Recria todas as MVs com 2 mudanças:
--   1. positivados = COUNT(DISTINCT cnpj) FILTER (qt > 0)
--      (era COUNT DISTINCT cod_cli — agora usa CNPJ completo)
--   2. base_cli HIERÁRQUICA:
--      • RCA          → qtcli_rca próprio (vindo do WinThor)
--      • Supervisor   → SUM(qtcli_rca dos seus RCAs)
--      • Gerente      → SUM(qtcli_rca dos RCAs do gerente)
--      • Fornecedor   → SUM(qtcli_rca de TODOS RCAs da empresa — denominador fixo)
--
-- A base é estável (vinda do cadastro do WinThor), não varia por escopo
-- (período/fornecedor). É uma propriedade da hierarquia comercial.
--
-- Implementação: MV auxiliar (mv_*_carteira_rca) com 1 linha por RCA;
-- nas MVs derivadas, sub-SELECT pra somar conforme o nível.

DROP MATERIALIZED VIEW IF EXISTS farol.mv_fat_cli CASCADE;
DROP MATERIALIZED VIEW IF EXISTS farol.mv_trans_cli CASCADE;
DROP MATERIALIZED VIEW IF EXISTS farol.mv_fat_mkt_produto;
DROP MATERIALIZED VIEW IF EXISTS farol.mv_fat_mkt_prod_pen;
DROP MATERIALIZED VIEW IF EXISTS farol.mv_trans_mkt_produto;
DROP MATERIALIZED VIEW IF EXISTS farol.mv_trans_mkt_prod_pen;
DROP MATERIALIZED VIEW IF EXISTS farol.mv_fat_carteira_rca;
DROP MATERIALIZED VIEW IF EXISTS farol.mv_trans_carteira_rca;

-- ═══════════════════════════════════════════════════════════════════════════════
-- AUXILIARES: carteira dos RCAs (1 linha por RCA, com sua hierarquia + qtcli)
-- ═══════════════════════════════════════════════════════════════════════════════

CREATE MATERIALIZED VIEW farol.mv_fat_carteira_rca AS
SELECT
    empresa_id,
    cod_rca,
    MAX(cod_gerente)    AS cod_gerente,
    MAX(cod_supervisor) AS cod_supervisor,
    MAX(qtcli_rca)      AS qtcli_rca
FROM vendas_faturadas
WHERE cod_rca <> ''
GROUP BY empresa_id, cod_rca;

CREATE UNIQUE INDEX idx_mvfatcart_pk    ON farol.mv_fat_carteira_rca (empresa_id, cod_rca);
CREATE INDEX        idx_mvfatcart_ger   ON farol.mv_fat_carteira_rca (empresa_id, cod_gerente);
CREATE INDEX        idx_mvfatcart_sup   ON farol.mv_fat_carteira_rca (empresa_id, cod_supervisor);
REFRESH MATERIALIZED VIEW farol.mv_fat_carteira_rca;
ANALYZE farol.mv_fat_carteira_rca;

CREATE MATERIALIZED VIEW farol.mv_trans_carteira_rca AS
SELECT
    empresa_id,
    cod_rca,
    MAX(cod_gerente)    AS cod_gerente,
    MAX(cod_supervisor) AS cod_supervisor,
    MAX(qtcli_rca)      AS qtcli_rca
FROM vendas_transmitidas
WHERE cod_rca <> ''
GROUP BY empresa_id, cod_rca;

CREATE UNIQUE INDEX idx_mvtranscart_pk    ON farol.mv_trans_carteira_rca (empresa_id, cod_rca);
CREATE INDEX        idx_mvtranscart_ger   ON farol.mv_trans_carteira_rca (empresa_id, cod_gerente);
CREATE INDEX        idx_mvtranscart_sup   ON farol.mv_trans_carteira_rca (empresa_id, cod_supervisor);
REFRESH MATERIALIZED VIEW farol.mv_trans_carteira_rca;
ANALYZE farol.mv_trans_carteira_rca;

-- ═══════════════════════════════════════════════════════════════════════════════
-- ════════════════════════════ VENDAS FATURADAS ═══════════════════════════════
-- ═══════════════════════════════════════════════════════════════════════════════

-- BASE: mv_fat_cli — agora com CNPJ na chave (cod_cli permanece pra display)
CREATE MATERIALIZED VIEW farol.mv_fat_cli AS
SELECT
    empresa_id, data_faturamento,
    EXTRACT(YEAR  FROM data_faturamento)::int AS ano,
    EXTRACT(MONTH FROM data_faturamento)::int AS mes,
    COALESCE(cod_fornec,     '') AS cod_fornec,
    COALESCE(cod_gerente,    '') AS cod_gerente,
    COALESCE(cod_supervisor, '') AS cod_supervisor,
    COALESCE(cod_rca,        '') AS cod_rca,
    COALESCE(cnpj,           '') AS cnpj,
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
    cod_fornec, cod_gerente, cod_supervisor, cod_rca, cnpj, cod_cli, empresa, uf
WITH NO DATA;

CREATE UNIQUE INDEX idx_mvfatcli_pk ON farol.mv_fat_cli
    (empresa_id, data_faturamento, cod_fornec, cod_gerente, cod_supervisor, cod_rca, cnpj, cod_cli, empresa, uf);
CREATE INDEX idx_mvfatcli_data   ON farol.mv_fat_cli (empresa_id, data_faturamento);
CREATE INDEX idx_mvfatcli_anomes ON farol.mv_fat_cli (empresa_id, ano, mes);
CREATE INDEX idx_mvfatcli_cnpj   ON farol.mv_fat_cli (empresa_id, cnpj);
REFRESH MATERIALIZED VIEW farol.mv_fat_cli;
ANALYZE farol.mv_fat_cli;

-- V01 Por Indústria — Fornecedor → Gerente → Supervisor → RCA
-- base no nível Fornecedor = SUM total da empresa (denominador fixo)
CREATE MATERIALIZED VIEW farol.mv_fat_v01_l0 AS
SELECT
    m.empresa_id, m.data_faturamento, m.ano, m.mes,
    m.cod_fornec, MAX(m.nome_fornec) AS nome_fornec,
    (SELECT COALESCE(SUM(c.qtcli_rca), 0)
       FROM farol.mv_fat_carteira_rca c
      WHERE c.empresa_id = m.empresa_id)::int             AS base_cli,
    COUNT(DISTINCT m.cnpj) FILTER (WHERE m.qt > 0)::int   AS positivados,
    COALESCE(AVG(m.mix) FILTER (WHERE m.qt > 0), 0)       AS mix,
    SUM(m.pvenda) AS pvenda, SUM(m.plucro) AS plucro, SUM(m.qt) AS qt
FROM farol.mv_fat_cli m
WHERE m.cod_fornec <> ''
GROUP BY m.empresa_id, m.data_faturamento, m.ano, m.mes, m.cod_fornec
WITH NO DATA;
CREATE UNIQUE INDEX idx_mvfatv01l0_pk ON farol.mv_fat_v01_l0 (empresa_id, data_faturamento, cod_fornec);
CREATE INDEX idx_mvfatv01l0_anomes ON farol.mv_fat_v01_l0 (empresa_id, ano, mes);
REFRESH MATERIALIZED VIEW farol.mv_fat_v01_l0;

-- nível Fornec+Gerente — base = SUM dos RCAs do gerente
CREATE MATERIALIZED VIEW farol.mv_fat_v01_l1 AS
SELECT
    m.empresa_id, m.data_faturamento, m.ano, m.mes,
    m.cod_fornec, m.cod_gerente, MAX(m.nome_gerente) AS nome_gerente,
    (SELECT COALESCE(SUM(c.qtcli_rca), 0)
       FROM farol.mv_fat_carteira_rca c
      WHERE c.empresa_id = m.empresa_id AND c.cod_gerente = m.cod_gerente)::int AS base_cli,
    COUNT(DISTINCT m.cnpj) FILTER (WHERE m.qt > 0)::int AS positivados,
    COALESCE(AVG(m.mix) FILTER (WHERE m.qt > 0), 0)     AS mix,
    SUM(m.pvenda) AS pvenda, SUM(m.plucro) AS plucro, SUM(m.qt) AS qt
FROM farol.mv_fat_cli m
WHERE m.cod_gerente <> ''
GROUP BY m.empresa_id, m.data_faturamento, m.ano, m.mes, m.cod_fornec, m.cod_gerente
WITH NO DATA;
CREATE UNIQUE INDEX idx_mvfatv01l1_pk ON farol.mv_fat_v01_l1 (empresa_id, data_faturamento, cod_fornec, cod_gerente);
CREATE INDEX idx_mvfatv01l1_anomes ON farol.mv_fat_v01_l1 (empresa_id, ano, mes);
REFRESH MATERIALIZED VIEW farol.mv_fat_v01_l1;

-- nível Fornec+Gerente+Supervisor — base = SUM dos RCAs do supervisor
CREATE MATERIALIZED VIEW farol.mv_fat_v01_l2 AS
SELECT
    m.empresa_id, m.data_faturamento, m.ano, m.mes,
    m.cod_fornec, m.cod_gerente, m.cod_supervisor, MAX(m.nome_supervisor) AS nome_supervisor,
    (SELECT COALESCE(SUM(c.qtcli_rca), 0)
       FROM farol.mv_fat_carteira_rca c
      WHERE c.empresa_id = m.empresa_id AND c.cod_supervisor = m.cod_supervisor)::int AS base_cli,
    COUNT(DISTINCT m.cnpj) FILTER (WHERE m.qt > 0)::int AS positivados,
    COALESCE(AVG(m.mix) FILTER (WHERE m.qt > 0), 0)     AS mix,
    SUM(m.pvenda) AS pvenda, SUM(m.plucro) AS plucro, SUM(m.qt) AS qt
FROM farol.mv_fat_cli m
WHERE m.cod_supervisor <> ''
GROUP BY m.empresa_id, m.data_faturamento, m.ano, m.mes, m.cod_fornec, m.cod_gerente, m.cod_supervisor
WITH NO DATA;
CREATE UNIQUE INDEX idx_mvfatv01l2_pk ON farol.mv_fat_v01_l2 (empresa_id, data_faturamento, cod_fornec, cod_gerente, cod_supervisor);
CREATE INDEX idx_mvfatv01l2_anomes ON farol.mv_fat_v01_l2 (empresa_id, ano, mes);
REFRESH MATERIALIZED VIEW farol.mv_fat_v01_l2;

-- nível RCA — base = qtcli_rca do RCA
CREATE MATERIALIZED VIEW farol.mv_fat_v01_l3 AS
SELECT
    m.empresa_id, m.data_faturamento, m.ano, m.mes,
    m.cod_fornec, m.cod_gerente, m.cod_supervisor, m.cod_rca, MAX(m.nome_rca) AS nome_rca,
    MAX(m.qtcli_rca)::int                               AS base_cli,
    COUNT(DISTINCT m.cnpj) FILTER (WHERE m.qt > 0)::int AS positivados,
    COALESCE(AVG(m.mix) FILTER (WHERE m.qt > 0), 0)     AS mix,
    SUM(m.pvenda) AS pvenda, SUM(m.plucro) AS plucro, SUM(m.qt) AS qt
FROM farol.mv_fat_cli m
WHERE m.cod_rca <> ''
GROUP BY m.empresa_id, m.data_faturamento, m.ano, m.mes, m.cod_fornec, m.cod_gerente, m.cod_supervisor, m.cod_rca
WITH NO DATA;
CREATE UNIQUE INDEX idx_mvfatv01l3_pk ON farol.mv_fat_v01_l3 (empresa_id, data_faturamento, cod_fornec, cod_gerente, cod_supervisor, cod_rca);
CREATE INDEX idx_mvfatv01l3_anomes ON farol.mv_fat_v01_l3 (empresa_id, ano, mes);
REFRESH MATERIALIZED VIEW farol.mv_fat_v01_l3;

-- V02 Por Equipe — Supervisor → RCA → Fornecedor
CREATE MATERIALIZED VIEW farol.mv_fat_v02_l0 AS
SELECT
    m.empresa_id, m.data_faturamento, m.ano, m.mes,
    m.cod_supervisor, MAX(m.nome_supervisor) AS nome_supervisor,
    (SELECT COALESCE(SUM(c.qtcli_rca), 0)
       FROM farol.mv_fat_carteira_rca c
      WHERE c.empresa_id = m.empresa_id AND c.cod_supervisor = m.cod_supervisor)::int AS base_cli,
    COUNT(DISTINCT m.cnpj) FILTER (WHERE m.qt > 0)::int AS positivados,
    COALESCE(AVG(m.mix) FILTER (WHERE m.qt > 0), 0)     AS mix,
    SUM(m.pvenda) AS pvenda, SUM(m.plucro) AS plucro, SUM(m.qt) AS qt
FROM farol.mv_fat_cli m
WHERE m.cod_supervisor <> ''
GROUP BY m.empresa_id, m.data_faturamento, m.ano, m.mes, m.cod_supervisor
WITH NO DATA;
CREATE UNIQUE INDEX idx_mvfatv02l0_pk ON farol.mv_fat_v02_l0 (empresa_id, data_faturamento, cod_supervisor);
CREATE INDEX idx_mvfatv02l0_anomes ON farol.mv_fat_v02_l0 (empresa_id, ano, mes);
REFRESH MATERIALIZED VIEW farol.mv_fat_v02_l0;

CREATE MATERIALIZED VIEW farol.mv_fat_v02_l1 AS
SELECT
    m.empresa_id, m.data_faturamento, m.ano, m.mes,
    m.cod_supervisor, m.cod_rca, MAX(m.nome_rca) AS nome_rca,
    MAX(m.qtcli_rca)::int                               AS base_cli,
    COUNT(DISTINCT m.cnpj) FILTER (WHERE m.qt > 0)::int AS positivados,
    COALESCE(AVG(m.mix) FILTER (WHERE m.qt > 0), 0)     AS mix,
    SUM(m.pvenda) AS pvenda, SUM(m.plucro) AS plucro, SUM(m.qt) AS qt
FROM farol.mv_fat_cli m
WHERE m.cod_supervisor <> '' AND m.cod_rca <> ''
GROUP BY m.empresa_id, m.data_faturamento, m.ano, m.mes, m.cod_supervisor, m.cod_rca
WITH NO DATA;
CREATE UNIQUE INDEX idx_mvfatv02l1_pk ON farol.mv_fat_v02_l1 (empresa_id, data_faturamento, cod_supervisor, cod_rca);
CREATE INDEX idx_mvfatv02l1_anomes ON farol.mv_fat_v02_l1 (empresa_id, ano, mes);
REFRESH MATERIALIZED VIEW farol.mv_fat_v02_l1;

CREATE MATERIALIZED VIEW farol.mv_fat_v02_l2 AS
SELECT
    m.empresa_id, m.data_faturamento, m.ano, m.mes,
    m.cod_supervisor, m.cod_rca, m.cod_fornec, MAX(m.nome_fornec) AS nome_fornec,
    MAX(m.qtcli_rca)::int                               AS base_cli,
    COUNT(DISTINCT m.cnpj) FILTER (WHERE m.qt > 0)::int AS positivados,
    COALESCE(AVG(m.mix) FILTER (WHERE m.qt > 0), 0)     AS mix,
    SUM(m.pvenda) AS pvenda, SUM(m.plucro) AS plucro, SUM(m.qt) AS qt
FROM farol.mv_fat_cli m
WHERE m.cod_supervisor <> '' AND m.cod_rca <> '' AND m.cod_fornec <> ''
GROUP BY m.empresa_id, m.data_faturamento, m.ano, m.mes, m.cod_supervisor, m.cod_rca, m.cod_fornec
WITH NO DATA;
CREATE UNIQUE INDEX idx_mvfatv02l2_pk ON farol.mv_fat_v02_l2 (empresa_id, data_faturamento, cod_supervisor, cod_rca, cod_fornec);
CREATE INDEX idx_mvfatv02l2_anomes ON farol.mv_fat_v02_l2 (empresa_id, ano, mes);
REFRESH MATERIALIZED VIEW farol.mv_fat_v02_l2;

-- V03 Por Gerência — Gerente → Supervisor → RCA → Cliente
CREATE MATERIALIZED VIEW farol.mv_fat_v03_l0 AS
SELECT
    m.empresa_id, m.data_faturamento, m.ano, m.mes,
    m.cod_gerente, MAX(m.nome_gerente) AS nome_gerente,
    (SELECT COALESCE(SUM(c.qtcli_rca), 0)
       FROM farol.mv_fat_carteira_rca c
      WHERE c.empresa_id = m.empresa_id AND c.cod_gerente = m.cod_gerente)::int AS base_cli,
    COUNT(DISTINCT m.cnpj) FILTER (WHERE m.qt > 0)::int AS positivados,
    COALESCE(AVG(m.mix) FILTER (WHERE m.qt > 0), 0)     AS mix,
    SUM(m.pvenda) AS pvenda, SUM(m.plucro) AS plucro, SUM(m.qt) AS qt
FROM farol.mv_fat_cli m
WHERE m.cod_gerente <> ''
GROUP BY m.empresa_id, m.data_faturamento, m.ano, m.mes, m.cod_gerente
WITH NO DATA;
CREATE UNIQUE INDEX idx_mvfatv03l0_pk ON farol.mv_fat_v03_l0 (empresa_id, data_faturamento, cod_gerente);
CREATE INDEX idx_mvfatv03l0_anomes ON farol.mv_fat_v03_l0 (empresa_id, ano, mes);
REFRESH MATERIALIZED VIEW farol.mv_fat_v03_l0;

CREATE MATERIALIZED VIEW farol.mv_fat_v03_l1 AS
SELECT
    m.empresa_id, m.data_faturamento, m.ano, m.mes,
    m.cod_gerente, m.cod_supervisor, MAX(m.nome_supervisor) AS nome_supervisor,
    (SELECT COALESCE(SUM(c.qtcli_rca), 0)
       FROM farol.mv_fat_carteira_rca c
      WHERE c.empresa_id = m.empresa_id AND c.cod_supervisor = m.cod_supervisor)::int AS base_cli,
    COUNT(DISTINCT m.cnpj) FILTER (WHERE m.qt > 0)::int AS positivados,
    COALESCE(AVG(m.mix) FILTER (WHERE m.qt > 0), 0)     AS mix,
    SUM(m.pvenda) AS pvenda, SUM(m.plucro) AS plucro, SUM(m.qt) AS qt
FROM farol.mv_fat_cli m
WHERE m.cod_gerente <> '' AND m.cod_supervisor <> ''
GROUP BY m.empresa_id, m.data_faturamento, m.ano, m.mes, m.cod_gerente, m.cod_supervisor
WITH NO DATA;
CREATE UNIQUE INDEX idx_mvfatv03l1_pk ON farol.mv_fat_v03_l1 (empresa_id, data_faturamento, cod_gerente, cod_supervisor);
CREATE INDEX idx_mvfatv03l1_anomes ON farol.mv_fat_v03_l1 (empresa_id, ano, mes);
REFRESH MATERIALIZED VIEW farol.mv_fat_v03_l1;

CREATE MATERIALIZED VIEW farol.mv_fat_v03_l2 AS
SELECT
    m.empresa_id, m.data_faturamento, m.ano, m.mes,
    m.cod_gerente, m.cod_supervisor, m.cod_rca, MAX(m.nome_rca) AS nome_rca,
    MAX(m.qtcli_rca)::int                               AS base_cli,
    COUNT(DISTINCT m.cnpj) FILTER (WHERE m.qt > 0)::int AS positivados,
    COALESCE(AVG(m.mix) FILTER (WHERE m.qt > 0), 0)     AS mix,
    SUM(m.pvenda) AS pvenda, SUM(m.plucro) AS plucro, SUM(m.qt) AS qt
FROM farol.mv_fat_cli m
WHERE m.cod_gerente <> '' AND m.cod_supervisor <> '' AND m.cod_rca <> ''
GROUP BY m.empresa_id, m.data_faturamento, m.ano, m.mes, m.cod_gerente, m.cod_supervisor, m.cod_rca
WITH NO DATA;
CREATE UNIQUE INDEX idx_mvfatv03l2_pk ON farol.mv_fat_v03_l2 (empresa_id, data_faturamento, cod_gerente, cod_supervisor, cod_rca);
CREATE INDEX idx_mvfatv03l2_anomes ON farol.mv_fat_v03_l2 (empresa_id, ano, mes);
REFRESH MATERIALIZED VIEW farol.mv_fat_v03_l2;

-- nível Cliente final no V03 — base = 1, positivados = 0 ou 1
CREATE MATERIALIZED VIEW farol.mv_fat_v03_l3 AS
SELECT
    m.empresa_id, m.data_faturamento, m.ano, m.mes,
    m.cod_gerente, m.cod_supervisor, m.cod_rca, m.cnpj,
    MAX(m.cod_cli)  AS cod_cli,
    MAX(m.nome_cli) AS nome_cli,
    1::int                                            AS base_cli,
    (CASE WHEN SUM(m.qt) > 0 THEN 1 ELSE 0 END)::int  AS positivados,
    SUM(m.mix)                                        AS mix,
    SUM(m.pvenda) AS pvenda, SUM(m.plucro) AS plucro, SUM(m.qt) AS qt
FROM farol.mv_fat_cli m
WHERE m.cod_gerente <> '' AND m.cod_supervisor <> '' AND m.cod_rca <> '' AND m.cnpj <> ''
GROUP BY m.empresa_id, m.data_faturamento, m.ano, m.mes, m.cod_gerente, m.cod_supervisor, m.cod_rca, m.cnpj
WITH NO DATA;
CREATE UNIQUE INDEX idx_mvfatv03l3_pk ON farol.mv_fat_v03_l3 (empresa_id, data_faturamento, cod_gerente, cod_supervisor, cod_rca, cnpj);
CREATE INDEX idx_mvfatv03l3_anomes ON farol.mv_fat_v03_l3 (empresa_id, ano, mes);
REFRESH MATERIALIZED VIEW farol.mv_fat_v03_l3;

-- Marketing (continua como antes — sem mudança na lógica de positivação)
CREATE MATERIALIZED VIEW farol.mv_fat_mkt_produto AS
SELECT
    empresa_id, data_faturamento,
    EXTRACT(YEAR  FROM data_faturamento)::int AS ano,
    EXTRACT(MONTH FROM data_faturamento)::int AS mes,
    COALESCE(cod_fornec, '')  AS cod_fornec, MAX(COALESCE(nome_fornec, '')) AS nome_fornec,
    COALESCE(cod_prod, '')    AS cod_prod,   MAX(COALESCE(nome_prod, ''))   AS nome_prod,
    MAX(COALESCE(ean, ''))    AS ean,
    COUNT(DISTINCT NULLIF(cnpj, ''))                   AS qt_clientes,
    COUNT(DISTINCT CASE WHEN qt > 0 THEN cnpj END)     AS qt_positivados,
    SUM(pvenda) AS pvenda, SUM(plucro) AS plucro, SUM(qt) AS qt
FROM vendas_faturadas WHERE cod_prod <> ''
GROUP BY empresa_id, data_faturamento, cod_fornec, cod_prod WITH NO DATA;
CREATE UNIQUE INDEX idx_mvfatmktp_pk ON farol.mv_fat_mkt_produto (empresa_id, data_faturamento, cod_fornec, cod_prod);
CREATE INDEX idx_mvfatmktp_anomes ON farol.mv_fat_mkt_produto (empresa_id, ano, mes);
REFRESH MATERIALIZED VIEW farol.mv_fat_mkt_produto;

CREATE MATERIALIZED VIEW farol.mv_fat_mkt_prod_pen AS
SELECT
    empresa_id, data_faturamento,
    EXTRACT(YEAR  FROM data_faturamento)::int AS ano,
    EXTRACT(MONTH FROM data_faturamento)::int AS mes,
    COALESCE(cod_prod, '')  AS cod_prod, MAX(COALESCE(nome_prod,  ''))  AS nome_prod,
    MAX(COALESCE(cod_fornec, '')) AS cod_fornec, MAX(COALESCE(nome_fornec,'')) AS nome_fornec,
    MAX(COALESCE(ean, '')) AS ean,
    COUNT(DISTINCT CASE WHEN qt > 0 THEN cnpj END) AS qt_positivados,
    SUM(pvenda) AS pvenda, SUM(plucro) AS plucro, SUM(qt) AS qt
FROM vendas_faturadas WHERE cod_prod <> ''
GROUP BY empresa_id, data_faturamento, cod_prod WITH NO DATA;
CREATE UNIQUE INDEX idx_mvfatmktpp_pk ON farol.mv_fat_mkt_prod_pen (empresa_id, data_faturamento, cod_prod);
CREATE INDEX idx_mvfatmktpp_anomes ON farol.mv_fat_mkt_prod_pen (empresa_id, ano, mes);
REFRESH MATERIALIZED VIEW farol.mv_fat_mkt_prod_pen;

-- ═══════════════════════════════════════════════════════════════════════════════
-- ════════════════════════════ VENDAS TRANSMITIDAS ═════════════════════════════
-- Estrutura idêntica, apenas: vendas_transmitidas + data_transmissao + mv_trans_*
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
    COALESCE(cnpj,           '') AS cnpj,
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
    cod_fornec, cod_gerente, cod_supervisor, cod_rca, cnpj, cod_cli, empresa, uf
WITH NO DATA;

CREATE UNIQUE INDEX idx_mvtranscli_pk ON farol.mv_trans_cli
    (empresa_id, data_transmissao, cod_fornec, cod_gerente, cod_supervisor, cod_rca, cnpj, cod_cli, empresa, uf);
CREATE INDEX idx_mvtranscli_data   ON farol.mv_trans_cli (empresa_id, data_transmissao);
CREATE INDEX idx_mvtranscli_anomes ON farol.mv_trans_cli (empresa_id, ano, mes);
CREATE INDEX idx_mvtranscli_cnpj   ON farol.mv_trans_cli (empresa_id, cnpj);
REFRESH MATERIALIZED VIEW farol.mv_trans_cli;
ANALYZE farol.mv_trans_cli;

CREATE MATERIALIZED VIEW farol.mv_trans_v01_l0 AS
SELECT
    m.empresa_id, m.data_transmissao, m.ano, m.mes,
    m.cod_fornec, MAX(m.nome_fornec) AS nome_fornec,
    (SELECT COALESCE(SUM(c.qtcli_rca), 0)
       FROM farol.mv_trans_carteira_rca c
      WHERE c.empresa_id = m.empresa_id)::int             AS base_cli,
    COUNT(DISTINCT m.cnpj) FILTER (WHERE m.qt > 0)::int   AS positivados,
    COALESCE(AVG(m.mix) FILTER (WHERE m.qt > 0), 0)       AS mix,
    SUM(m.pvenda) AS pvenda, SUM(m.plucro) AS plucro, SUM(m.qt) AS qt
FROM farol.mv_trans_cli m WHERE m.cod_fornec <> ''
GROUP BY m.empresa_id, m.data_transmissao, m.ano, m.mes, m.cod_fornec WITH NO DATA;
CREATE UNIQUE INDEX idx_mvtransv01l0_pk ON farol.mv_trans_v01_l0 (empresa_id, data_transmissao, cod_fornec);
CREATE INDEX idx_mvtransv01l0_anomes ON farol.mv_trans_v01_l0 (empresa_id, ano, mes);
REFRESH MATERIALIZED VIEW farol.mv_trans_v01_l0;

CREATE MATERIALIZED VIEW farol.mv_trans_v01_l1 AS
SELECT
    m.empresa_id, m.data_transmissao, m.ano, m.mes,
    m.cod_fornec, m.cod_gerente, MAX(m.nome_gerente) AS nome_gerente,
    (SELECT COALESCE(SUM(c.qtcli_rca), 0)
       FROM farol.mv_trans_carteira_rca c
      WHERE c.empresa_id = m.empresa_id AND c.cod_gerente = m.cod_gerente)::int AS base_cli,
    COUNT(DISTINCT m.cnpj) FILTER (WHERE m.qt > 0)::int AS positivados,
    COALESCE(AVG(m.mix) FILTER (WHERE m.qt > 0), 0)     AS mix,
    SUM(m.pvenda) AS pvenda, SUM(m.plucro) AS plucro, SUM(m.qt) AS qt
FROM farol.mv_trans_cli m WHERE m.cod_gerente <> ''
GROUP BY m.empresa_id, m.data_transmissao, m.ano, m.mes, m.cod_fornec, m.cod_gerente WITH NO DATA;
CREATE UNIQUE INDEX idx_mvtransv01l1_pk ON farol.mv_trans_v01_l1 (empresa_id, data_transmissao, cod_fornec, cod_gerente);
CREATE INDEX idx_mvtransv01l1_anomes ON farol.mv_trans_v01_l1 (empresa_id, ano, mes);
REFRESH MATERIALIZED VIEW farol.mv_trans_v01_l1;

CREATE MATERIALIZED VIEW farol.mv_trans_v01_l2 AS
SELECT
    m.empresa_id, m.data_transmissao, m.ano, m.mes,
    m.cod_fornec, m.cod_gerente, m.cod_supervisor, MAX(m.nome_supervisor) AS nome_supervisor,
    (SELECT COALESCE(SUM(c.qtcli_rca), 0)
       FROM farol.mv_trans_carteira_rca c
      WHERE c.empresa_id = m.empresa_id AND c.cod_supervisor = m.cod_supervisor)::int AS base_cli,
    COUNT(DISTINCT m.cnpj) FILTER (WHERE m.qt > 0)::int AS positivados,
    COALESCE(AVG(m.mix) FILTER (WHERE m.qt > 0), 0)     AS mix,
    SUM(m.pvenda) AS pvenda, SUM(m.plucro) AS plucro, SUM(m.qt) AS qt
FROM farol.mv_trans_cli m WHERE m.cod_supervisor <> ''
GROUP BY m.empresa_id, m.data_transmissao, m.ano, m.mes, m.cod_fornec, m.cod_gerente, m.cod_supervisor WITH NO DATA;
CREATE UNIQUE INDEX idx_mvtransv01l2_pk ON farol.mv_trans_v01_l2 (empresa_id, data_transmissao, cod_fornec, cod_gerente, cod_supervisor);
CREATE INDEX idx_mvtransv01l2_anomes ON farol.mv_trans_v01_l2 (empresa_id, ano, mes);
REFRESH MATERIALIZED VIEW farol.mv_trans_v01_l2;

CREATE MATERIALIZED VIEW farol.mv_trans_v01_l3 AS
SELECT
    m.empresa_id, m.data_transmissao, m.ano, m.mes,
    m.cod_fornec, m.cod_gerente, m.cod_supervisor, m.cod_rca, MAX(m.nome_rca) AS nome_rca,
    MAX(m.qtcli_rca)::int                               AS base_cli,
    COUNT(DISTINCT m.cnpj) FILTER (WHERE m.qt > 0)::int AS positivados,
    COALESCE(AVG(m.mix) FILTER (WHERE m.qt > 0), 0)     AS mix,
    SUM(m.pvenda) AS pvenda, SUM(m.plucro) AS plucro, SUM(m.qt) AS qt
FROM farol.mv_trans_cli m WHERE m.cod_rca <> ''
GROUP BY m.empresa_id, m.data_transmissao, m.ano, m.mes, m.cod_fornec, m.cod_gerente, m.cod_supervisor, m.cod_rca WITH NO DATA;
CREATE UNIQUE INDEX idx_mvtransv01l3_pk ON farol.mv_trans_v01_l3 (empresa_id, data_transmissao, cod_fornec, cod_gerente, cod_supervisor, cod_rca);
CREATE INDEX idx_mvtransv01l3_anomes ON farol.mv_trans_v01_l3 (empresa_id, ano, mes);
REFRESH MATERIALIZED VIEW farol.mv_trans_v01_l3;

CREATE MATERIALIZED VIEW farol.mv_trans_v02_l0 AS
SELECT
    m.empresa_id, m.data_transmissao, m.ano, m.mes,
    m.cod_supervisor, MAX(m.nome_supervisor) AS nome_supervisor,
    (SELECT COALESCE(SUM(c.qtcli_rca), 0)
       FROM farol.mv_trans_carteira_rca c
      WHERE c.empresa_id = m.empresa_id AND c.cod_supervisor = m.cod_supervisor)::int AS base_cli,
    COUNT(DISTINCT m.cnpj) FILTER (WHERE m.qt > 0)::int AS positivados,
    COALESCE(AVG(m.mix) FILTER (WHERE m.qt > 0), 0)     AS mix,
    SUM(m.pvenda) AS pvenda, SUM(m.plucro) AS plucro, SUM(m.qt) AS qt
FROM farol.mv_trans_cli m WHERE m.cod_supervisor <> ''
GROUP BY m.empresa_id, m.data_transmissao, m.ano, m.mes, m.cod_supervisor WITH NO DATA;
CREATE UNIQUE INDEX idx_mvtransv02l0_pk ON farol.mv_trans_v02_l0 (empresa_id, data_transmissao, cod_supervisor);
CREATE INDEX idx_mvtransv02l0_anomes ON farol.mv_trans_v02_l0 (empresa_id, ano, mes);
REFRESH MATERIALIZED VIEW farol.mv_trans_v02_l0;

CREATE MATERIALIZED VIEW farol.mv_trans_v02_l1 AS
SELECT
    m.empresa_id, m.data_transmissao, m.ano, m.mes,
    m.cod_supervisor, m.cod_rca, MAX(m.nome_rca) AS nome_rca,
    MAX(m.qtcli_rca)::int                               AS base_cli,
    COUNT(DISTINCT m.cnpj) FILTER (WHERE m.qt > 0)::int AS positivados,
    COALESCE(AVG(m.mix) FILTER (WHERE m.qt > 0), 0)     AS mix,
    SUM(m.pvenda) AS pvenda, SUM(m.plucro) AS plucro, SUM(m.qt) AS qt
FROM farol.mv_trans_cli m WHERE m.cod_supervisor <> '' AND m.cod_rca <> ''
GROUP BY m.empresa_id, m.data_transmissao, m.ano, m.mes, m.cod_supervisor, m.cod_rca WITH NO DATA;
CREATE UNIQUE INDEX idx_mvtransv02l1_pk ON farol.mv_trans_v02_l1 (empresa_id, data_transmissao, cod_supervisor, cod_rca);
CREATE INDEX idx_mvtransv02l1_anomes ON farol.mv_trans_v02_l1 (empresa_id, ano, mes);
REFRESH MATERIALIZED VIEW farol.mv_trans_v02_l1;

CREATE MATERIALIZED VIEW farol.mv_trans_v02_l2 AS
SELECT
    m.empresa_id, m.data_transmissao, m.ano, m.mes,
    m.cod_supervisor, m.cod_rca, m.cod_fornec, MAX(m.nome_fornec) AS nome_fornec,
    MAX(m.qtcli_rca)::int                               AS base_cli,
    COUNT(DISTINCT m.cnpj) FILTER (WHERE m.qt > 0)::int AS positivados,
    COALESCE(AVG(m.mix) FILTER (WHERE m.qt > 0), 0)     AS mix,
    SUM(m.pvenda) AS pvenda, SUM(m.plucro) AS plucro, SUM(m.qt) AS qt
FROM farol.mv_trans_cli m WHERE m.cod_supervisor <> '' AND m.cod_rca <> '' AND m.cod_fornec <> ''
GROUP BY m.empresa_id, m.data_transmissao, m.ano, m.mes, m.cod_supervisor, m.cod_rca, m.cod_fornec WITH NO DATA;
CREATE UNIQUE INDEX idx_mvtransv02l2_pk ON farol.mv_trans_v02_l2 (empresa_id, data_transmissao, cod_supervisor, cod_rca, cod_fornec);
CREATE INDEX idx_mvtransv02l2_anomes ON farol.mv_trans_v02_l2 (empresa_id, ano, mes);
REFRESH MATERIALIZED VIEW farol.mv_trans_v02_l2;

CREATE MATERIALIZED VIEW farol.mv_trans_v03_l0 AS
SELECT
    m.empresa_id, m.data_transmissao, m.ano, m.mes,
    m.cod_gerente, MAX(m.nome_gerente) AS nome_gerente,
    (SELECT COALESCE(SUM(c.qtcli_rca), 0)
       FROM farol.mv_trans_carteira_rca c
      WHERE c.empresa_id = m.empresa_id AND c.cod_gerente = m.cod_gerente)::int AS base_cli,
    COUNT(DISTINCT m.cnpj) FILTER (WHERE m.qt > 0)::int AS positivados,
    COALESCE(AVG(m.mix) FILTER (WHERE m.qt > 0), 0)     AS mix,
    SUM(m.pvenda) AS pvenda, SUM(m.plucro) AS plucro, SUM(m.qt) AS qt
FROM farol.mv_trans_cli m WHERE m.cod_gerente <> ''
GROUP BY m.empresa_id, m.data_transmissao, m.ano, m.mes, m.cod_gerente WITH NO DATA;
CREATE UNIQUE INDEX idx_mvtransv03l0_pk ON farol.mv_trans_v03_l0 (empresa_id, data_transmissao, cod_gerente);
CREATE INDEX idx_mvtransv03l0_anomes ON farol.mv_trans_v03_l0 (empresa_id, ano, mes);
REFRESH MATERIALIZED VIEW farol.mv_trans_v03_l0;

CREATE MATERIALIZED VIEW farol.mv_trans_v03_l1 AS
SELECT
    m.empresa_id, m.data_transmissao, m.ano, m.mes,
    m.cod_gerente, m.cod_supervisor, MAX(m.nome_supervisor) AS nome_supervisor,
    (SELECT COALESCE(SUM(c.qtcli_rca), 0)
       FROM farol.mv_trans_carteira_rca c
      WHERE c.empresa_id = m.empresa_id AND c.cod_supervisor = m.cod_supervisor)::int AS base_cli,
    COUNT(DISTINCT m.cnpj) FILTER (WHERE m.qt > 0)::int AS positivados,
    COALESCE(AVG(m.mix) FILTER (WHERE m.qt > 0), 0)     AS mix,
    SUM(m.pvenda) AS pvenda, SUM(m.plucro) AS plucro, SUM(m.qt) AS qt
FROM farol.mv_trans_cli m WHERE m.cod_gerente <> '' AND m.cod_supervisor <> ''
GROUP BY m.empresa_id, m.data_transmissao, m.ano, m.mes, m.cod_gerente, m.cod_supervisor WITH NO DATA;
CREATE UNIQUE INDEX idx_mvtransv03l1_pk ON farol.mv_trans_v03_l1 (empresa_id, data_transmissao, cod_gerente, cod_supervisor);
CREATE INDEX idx_mvtransv03l1_anomes ON farol.mv_trans_v03_l1 (empresa_id, ano, mes);
REFRESH MATERIALIZED VIEW farol.mv_trans_v03_l1;

CREATE MATERIALIZED VIEW farol.mv_trans_v03_l2 AS
SELECT
    m.empresa_id, m.data_transmissao, m.ano, m.mes,
    m.cod_gerente, m.cod_supervisor, m.cod_rca, MAX(m.nome_rca) AS nome_rca,
    MAX(m.qtcli_rca)::int                               AS base_cli,
    COUNT(DISTINCT m.cnpj) FILTER (WHERE m.qt > 0)::int AS positivados,
    COALESCE(AVG(m.mix) FILTER (WHERE m.qt > 0), 0)     AS mix,
    SUM(m.pvenda) AS pvenda, SUM(m.plucro) AS plucro, SUM(m.qt) AS qt
FROM farol.mv_trans_cli m WHERE m.cod_gerente <> '' AND m.cod_supervisor <> '' AND m.cod_rca <> ''
GROUP BY m.empresa_id, m.data_transmissao, m.ano, m.mes, m.cod_gerente, m.cod_supervisor, m.cod_rca WITH NO DATA;
CREATE UNIQUE INDEX idx_mvtransv03l2_pk ON farol.mv_trans_v03_l2 (empresa_id, data_transmissao, cod_gerente, cod_supervisor, cod_rca);
CREATE INDEX idx_mvtransv03l2_anomes ON farol.mv_trans_v03_l2 (empresa_id, ano, mes);
REFRESH MATERIALIZED VIEW farol.mv_trans_v03_l2;

CREATE MATERIALIZED VIEW farol.mv_trans_v03_l3 AS
SELECT
    m.empresa_id, m.data_transmissao, m.ano, m.mes,
    m.cod_gerente, m.cod_supervisor, m.cod_rca, m.cnpj,
    MAX(m.cod_cli)  AS cod_cli,
    MAX(m.nome_cli) AS nome_cli,
    1::int                                            AS base_cli,
    (CASE WHEN SUM(m.qt) > 0 THEN 1 ELSE 0 END)::int  AS positivados,
    SUM(m.mix)                                        AS mix,
    SUM(m.pvenda) AS pvenda, SUM(m.plucro) AS plucro, SUM(m.qt) AS qt
FROM farol.mv_trans_cli m
WHERE m.cod_gerente <> '' AND m.cod_supervisor <> '' AND m.cod_rca <> '' AND m.cnpj <> ''
GROUP BY m.empresa_id, m.data_transmissao, m.ano, m.mes, m.cod_gerente, m.cod_supervisor, m.cod_rca, m.cnpj WITH NO DATA;
CREATE UNIQUE INDEX idx_mvtransv03l3_pk ON farol.mv_trans_v03_l3 (empresa_id, data_transmissao, cod_gerente, cod_supervisor, cod_rca, cnpj);
CREATE INDEX idx_mvtransv03l3_anomes ON farol.mv_trans_v03_l3 (empresa_id, ano, mes);
REFRESH MATERIALIZED VIEW farol.mv_trans_v03_l3;

CREATE MATERIALIZED VIEW farol.mv_trans_mkt_produto AS
SELECT
    empresa_id, data_transmissao,
    EXTRACT(YEAR  FROM data_transmissao)::int AS ano,
    EXTRACT(MONTH FROM data_transmissao)::int AS mes,
    COALESCE(cod_fornec, '')  AS cod_fornec, MAX(COALESCE(nome_fornec, '')) AS nome_fornec,
    COALESCE(cod_prod, '')    AS cod_prod,   MAX(COALESCE(nome_prod, ''))   AS nome_prod,
    MAX(COALESCE(ean, ''))    AS ean,
    COUNT(DISTINCT NULLIF(cnpj, ''))                   AS qt_clientes,
    COUNT(DISTINCT CASE WHEN qt > 0 THEN cnpj END)     AS qt_positivados,
    SUM(pvenda) AS pvenda, SUM(plucro) AS plucro, SUM(qt) AS qt
FROM vendas_transmitidas WHERE cod_prod <> ''
GROUP BY empresa_id, data_transmissao, cod_fornec, cod_prod WITH NO DATA;
CREATE UNIQUE INDEX idx_mvtransmktp_pk ON farol.mv_trans_mkt_produto (empresa_id, data_transmissao, cod_fornec, cod_prod);
CREATE INDEX idx_mvtransmktp_anomes ON farol.mv_trans_mkt_produto (empresa_id, ano, mes);
REFRESH MATERIALIZED VIEW farol.mv_trans_mkt_produto;

CREATE MATERIALIZED VIEW farol.mv_trans_mkt_prod_pen AS
SELECT
    empresa_id, data_transmissao,
    EXTRACT(YEAR  FROM data_transmissao)::int AS ano,
    EXTRACT(MONTH FROM data_transmissao)::int AS mes,
    COALESCE(cod_prod, '')  AS cod_prod, MAX(COALESCE(nome_prod,  ''))  AS nome_prod,
    MAX(COALESCE(cod_fornec, '')) AS cod_fornec, MAX(COALESCE(nome_fornec,'')) AS nome_fornec,
    MAX(COALESCE(ean, '')) AS ean,
    COUNT(DISTINCT CASE WHEN qt > 0 THEN cnpj END) AS qt_positivados,
    SUM(pvenda) AS pvenda, SUM(plucro) AS plucro, SUM(qt) AS qt
FROM vendas_transmitidas WHERE cod_prod <> ''
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
