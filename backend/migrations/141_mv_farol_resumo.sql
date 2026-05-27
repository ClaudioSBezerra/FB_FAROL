-- 141_mv_farol_resumo.sql
-- View materializada pré-agregada para o Farol V2.
--
-- Agrega vendas_importadas por (empresa, período, hierarquia completa, estado)
-- reduzindo ~2.3M linhas brutas para ~200-500K linhas agregadas.
-- A API lê desta view com GROUP BY dinâmico — Postgres retorna apenas as N
-- colunas do nível atual em vez de carregar toda a tabela no Go.
--
-- REFRESH: executado ao final de cada importação pelo handler Go.
-- Primeiro refresh: síncrono (aqui). Posteriores: CONCURRENTLY (sem bloquear leituras).

CREATE MATERIALIZED VIEW IF NOT EXISTS farol.mv_farol_resumo AS
SELECT
    empresa_id,
    tipo_base,
    ano,
    mes,
    COALESCE(cod_fornec,     '') AS cod_fornec,
    COALESCE(cod_gerente,    '') AS cod_gerente,
    COALESCE(cod_supervisor, '') AS cod_supervisor,
    COALESCE(cod_rca,        '') AS cod_rca,
    COALESCE(cod_cli,        '') AS cod_cli,
    COALESCE(empresa,        '') AS empresa,
    COALESCE(uf,             '') AS uf,
    COALESCE(cod_prod,       '') AS cod_prod,
    COALESCE(estado,         '') AS estado,
    -- Nomes: MAX para resolver variações de capitalização por código
    MAX(COALESCE(nome_fornec,     '')) AS nome_fornec,
    MAX(COALESCE(nome_gerente,    '')) AS nome_gerente,
    MAX(COALESCE(nome_supervisor, '')) AS nome_supervisor,
    MAX(COALESCE(nome_rca,        '')) AS nome_rca,
    MAX(COALESCE(nome_cli,        '')) AS nome_cli,
    MAX(COALESCE(nome_prod,       '')) AS nome_prod,
    SUM(pvenda)     AS pvenda,
    SUM(qt)         AS qt,
    MAX(qtcli_rca)  AS qtcli_rca
FROM vendas_importadas
GROUP BY
    empresa_id, tipo_base, ano, mes,
    cod_fornec, cod_gerente, cod_supervisor, cod_rca,
    cod_cli, empresa, uf, cod_prod, estado;

-- Popula a view na primeira vez (sem CONCURRENTLY — view ainda vazia)
REFRESH MATERIALIZED VIEW farol.mv_farol_resumo;

-- Índice único obrigatório para REFRESH CONCURRENTLY nas próximas importações
CREATE UNIQUE INDEX IF NOT EXISTS idx_mvfr_pk ON farol.mv_farol_resumo (
    empresa_id, tipo_base, ano, mes,
    cod_fornec, cod_gerente, cod_supervisor, cod_rca,
    cod_cli, empresa, uf, cod_prod, estado
);

-- Índice de filtro principal — cobre 99% dos acessos da API
CREATE INDEX IF NOT EXISTS idx_mvfr_filter ON farol.mv_farol_resumo (
    empresa_id, tipo_base, ano, mes
);
