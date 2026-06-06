-- 163_mkt_cli_dim_mvs.sql
-- Performance fix: MVs especializadas para o painel Marketing.
--
-- PROBLEMA: mv_fat_cli / mv_trans_cli agrupam por 8 dimensões
--   (fornec, gerente, supervisor, rca, cnpj, cod_cli, empresa, uf)
--   → potencialmente centenas de milhões de linhas para YTD.
--   fetchMktKPI varria essa base em 7–21s.
--   FarolV2DimsHandler executava 7 queries paralelas na mesma MV → 7–19s cada.
--
-- SOLUÇÃO:
--   1. mv_fat_mkt_cli_dia / mv_trans_mkt_cli_dia
--      1 linha por (dia, cnpj, cod_cli) — sem fanout de fornec/supervisor/rca.
--      Reduz ~100× o volume varrido por fetchMktKPI e fetchMktCliente.
--
--   2. mv_fat_dim / mv_trans_dim
--      Lookup de hierarquia sem coluna de data (poucas linhas por empresa).
--      Dims carregam em <1 ms em vez de 7–19 s.
--
--   3. mv_fat_dim_cli / mv_trans_dim_cli
--      Lookup de clientes sem data (28 K linhas).

-- ═══════════════════════════════════════════════════════════════════════════════
-- VENDAS FATURADAS
-- ═══════════════════════════════════════════════════════════════════════════════

CREATE MATERIALIZED VIEW farol.mv_fat_mkt_cli_dia AS
SELECT
    empresa_id, data_faturamento,
    EXTRACT(YEAR  FROM data_faturamento)::int AS ano,
    EXTRACT(MONTH FROM data_faturamento)::int AS mes,
    COALESCE(cnpj,    '') AS cnpj,
    COALESCE(cod_cli, '') AS cod_cli,
    MAX(COALESCE(nome_cli, '')) AS nome_cli,
    (CASE WHEN SUM(qt) > 0 THEN 1 ELSE 0 END)::int AS positivados,
    COUNT(DISTINCT NULLIF(cod_prod, ''))::float      AS mix,
    SUM(pvenda) AS pvenda, SUM(plucro) AS plucro, SUM(qt) AS qt
FROM vendas_faturadas
WHERE cnpj != '' AND cod_cli != ''
GROUP BY empresa_id, data_faturamento, ano, mes, cnpj, cod_cli
WITH NO DATA;

CREATE UNIQUE INDEX idx_mvfatmktclidia_pk    ON farol.mv_fat_mkt_cli_dia (empresa_id, data_faturamento, cnpj, cod_cli);
CREATE INDEX        idx_mvfatmktclidia_data   ON farol.mv_fat_mkt_cli_dia (empresa_id, data_faturamento);
CREATE INDEX        idx_mvfatmktclidia_anomes ON farol.mv_fat_mkt_cli_dia (empresa_id, ano, mes);
REFRESH MATERIALIZED VIEW farol.mv_fat_mkt_cli_dia;
ANALYZE farol.mv_fat_mkt_cli_dia;

-- Lookup de hierarquia sem coluna de data
CREATE MATERIALIZED VIEW farol.mv_fat_dim AS
SELECT
    empresa_id,
    COALESCE(cod_fornec,     '') AS cod_fornec,
    COALESCE(cod_gerente,    '') AS cod_gerente,
    COALESCE(cod_supervisor, '') AS cod_supervisor,
    COALESCE(cod_rca,        '') AS cod_rca,
    MAX(COALESCE(nome_fornec,     '')) AS nome_fornec,
    MAX(COALESCE(nome_gerente,    '')) AS nome_gerente,
    MAX(COALESCE(nome_supervisor, '')) AS nome_supervisor,
    MAX(COALESCE(nome_rca,        '')) AS nome_rca,
    COALESCE(empresa, '') AS empresa,
    COALESCE(uf,      '') AS uf
FROM vendas_faturadas
WHERE cod_rca != ''
GROUP BY empresa_id, cod_fornec, cod_gerente, cod_supervisor, cod_rca, empresa, uf
WITH NO DATA;

CREATE INDEX idx_mvfatdim_empresa ON farol.mv_fat_dim (empresa_id);
REFRESH MATERIALIZED VIEW farol.mv_fat_dim;
ANALYZE farol.mv_fat_dim;

-- Lookup de clientes sem data
CREATE MATERIALIZED VIEW farol.mv_fat_dim_cli AS
SELECT
    empresa_id,
    COALESCE(cod_cli, '') AS cod_cli,
    MAX(COALESCE(nome_cli, '')) AS nome_cli
FROM vendas_faturadas
WHERE cod_cli != ''
GROUP BY empresa_id, cod_cli
WITH NO DATA;

CREATE UNIQUE INDEX idx_mvfatdimcli ON farol.mv_fat_dim_cli (empresa_id, cod_cli);
REFRESH MATERIALIZED VIEW farol.mv_fat_dim_cli;
ANALYZE farol.mv_fat_dim_cli;

-- ═══════════════════════════════════════════════════════════════════════════════
-- VENDAS TRANSMITIDAS (estrutura idêntica)
-- ═══════════════════════════════════════════════════════════════════════════════

CREATE MATERIALIZED VIEW farol.mv_trans_mkt_cli_dia AS
SELECT
    empresa_id, data_transmissao,
    EXTRACT(YEAR  FROM data_transmissao)::int AS ano,
    EXTRACT(MONTH FROM data_transmissao)::int AS mes,
    COALESCE(cnpj,    '') AS cnpj,
    COALESCE(cod_cli, '') AS cod_cli,
    MAX(COALESCE(nome_cli, '')) AS nome_cli,
    (CASE WHEN SUM(qt) > 0 THEN 1 ELSE 0 END)::int AS positivados,
    COUNT(DISTINCT NULLIF(cod_prod, ''))::float      AS mix,
    SUM(pvenda) AS pvenda, SUM(plucro) AS plucro, SUM(qt) AS qt
FROM vendas_transmitidas
WHERE cnpj != '' AND cod_cli != ''
GROUP BY empresa_id, data_transmissao, ano, mes, cnpj, cod_cli
WITH NO DATA;

CREATE UNIQUE INDEX idx_mvtransmktclidia_pk    ON farol.mv_trans_mkt_cli_dia (empresa_id, data_transmissao, cnpj, cod_cli);
CREATE INDEX        idx_mvtransmktclidia_data   ON farol.mv_trans_mkt_cli_dia (empresa_id, data_transmissao);
CREATE INDEX        idx_mvtransmktclidia_anomes ON farol.mv_trans_mkt_cli_dia (empresa_id, ano, mes);
REFRESH MATERIALIZED VIEW farol.mv_trans_mkt_cli_dia;
ANALYZE farol.mv_trans_mkt_cli_dia;

CREATE MATERIALIZED VIEW farol.mv_trans_dim AS
SELECT
    empresa_id,
    COALESCE(cod_fornec,     '') AS cod_fornec,
    COALESCE(cod_gerente,    '') AS cod_gerente,
    COALESCE(cod_supervisor, '') AS cod_supervisor,
    COALESCE(cod_rca,        '') AS cod_rca,
    MAX(COALESCE(nome_fornec,     '')) AS nome_fornec,
    MAX(COALESCE(nome_gerente,    '')) AS nome_gerente,
    MAX(COALESCE(nome_supervisor, '')) AS nome_supervisor,
    MAX(COALESCE(nome_rca,        '')) AS nome_rca,
    COALESCE(empresa, '') AS empresa,
    COALESCE(uf,      '') AS uf
FROM vendas_transmitidas
WHERE cod_rca != ''
GROUP BY empresa_id, cod_fornec, cod_gerente, cod_supervisor, cod_rca, empresa, uf
WITH NO DATA;

CREATE INDEX idx_mvtransdim_empresa ON farol.mv_trans_dim (empresa_id);
REFRESH MATERIALIZED VIEW farol.mv_trans_dim;
ANALYZE farol.mv_trans_dim;

CREATE MATERIALIZED VIEW farol.mv_trans_dim_cli AS
SELECT
    empresa_id,
    COALESCE(cod_cli, '') AS cod_cli,
    MAX(COALESCE(nome_cli, '')) AS nome_cli
FROM vendas_transmitidas
WHERE cod_cli != ''
GROUP BY empresa_id, cod_cli
WITH NO DATA;

CREATE UNIQUE INDEX idx_mvtransdimcli ON farol.mv_trans_dim_cli (empresa_id, cod_cli);
REFRESH MATERIALIZED VIEW farol.mv_trans_dim_cli;
ANALYZE farol.mv_trans_dim_cli;
