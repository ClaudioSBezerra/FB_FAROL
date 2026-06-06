-- 165_drop_mvs_use_agg_mes.sql
-- ════════════════════════════════════════════════════════════════════════════
-- REFACTOR: substituir todas as MVs diárias (mv_fat_cli, mv_*_v0X_lY,
-- mv_*_mkt_cli_dia, mv_*_mkt_produto, mv_*_mkt_prod_pen, mv_*_dim*) por
-- tabelas agregadas mensais (agg_*_mes) já criadas pela migration 162,
-- mais 2 novas particionadas:
--   farol.agg_fat_mkt_cli_mes     — para fetchMktKPI/fetchMktCliente/fetchClientesInativos
--   farol.agg_fat_mkt_produto_mes — para fetchMktProduto
--   (+ equivalentes trans)
--
-- MOTIVAÇÃO:
--   • REFRESH MATERIALIZED VIEW CONCURRENTLY de mv_fat_cli (6M+ linhas)
--     levava 13+ minutos em uma única transação gigante. Cascateava para
--     22 MVs derivadas. Carga diária inviável em produção.
--   • upsert_aggs_mes(empresa, ano, mes) já existe e faz UPSERT mensal
--     particionado, em segundos. Carga diária: chama por mês afetado.
--   • Marketing era o único bloqueador (ainda lia mv_fat_v01_l0 etc.).
--
-- TRADEOFF:
--   • Grão mensal: Marketing perde range "01-15 de jun" e usa "jun inteiro
--     até onde tem dado". Aceitável porque vendas só existe até ontem.
--
-- MANTÉM:
--   • mv_fat_carteira_rca / mv_trans_carteira_rca — pequenas (centenas de
--     linhas), usadas pelo sub-SELECT de base_cli hierárquico em
--     upsert_aggs_mes. Refresh é rápido (< 1s).
--
-- DROP (em ordem: derivadas primeiro, base depois — CASCADE garante):
--   mv_fat_cli + 11 derivadas + 4 mkt/dim
--   mv_trans_cli + 11 derivadas + 4 mkt/dim
-- ════════════════════════════════════════════════════════════════════════════

-- ─── DROP MVs antigas ───────────────────────────────────────────────────────

DROP MATERIALIZED VIEW IF EXISTS farol.mv_fat_cli              CASCADE;
DROP MATERIALIZED VIEW IF EXISTS farol.mv_fat_mkt_cli_dia      CASCADE;
DROP MATERIALIZED VIEW IF EXISTS farol.mv_fat_mkt_produto      CASCADE;
DROP MATERIALIZED VIEW IF EXISTS farol.mv_fat_mkt_prod_pen     CASCADE;
DROP MATERIALIZED VIEW IF EXISTS farol.mv_fat_dim              CASCADE;
DROP MATERIALIZED VIEW IF EXISTS farol.mv_fat_dim_cli          CASCADE;

DROP MATERIALIZED VIEW IF EXISTS farol.mv_trans_cli            CASCADE;
DROP MATERIALIZED VIEW IF EXISTS farol.mv_trans_mkt_cli_dia    CASCADE;
DROP MATERIALIZED VIEW IF EXISTS farol.mv_trans_mkt_produto    CASCADE;
DROP MATERIALIZED VIEW IF EXISTS farol.mv_trans_mkt_prod_pen   CASCADE;
DROP MATERIALIZED VIEW IF EXISTS farol.mv_trans_dim            CASCADE;
DROP MATERIALIZED VIEW IF EXISTS farol.mv_trans_dim_cli        CASCADE;

-- ─── Novas tabelas particionadas para Marketing ────────────────────────────

-- Cliente×mês: usado por fetchMktKPI, fetchMktCliente, fetchClientesInativos
CREATE TABLE IF NOT EXISTS farol.agg_fat_mkt_cli_mes (
    empresa_id   UUID    NOT NULL,
    ano          INT     NOT NULL,
    mes          INT     NOT NULL,
    cnpj         TEXT    NOT NULL,
    cod_cli      TEXT    NOT NULL DEFAULT '',
    nome_cli     TEXT    NOT NULL DEFAULT '',
    positivados  INT     NOT NULL DEFAULT 0,  -- 1 se SUM(qt)>0 no mês
    mix          NUMERIC NOT NULL DEFAULT 0,  -- COUNT DISTINCT cod_prod no mês
    pvenda       NUMERIC NOT NULL DEFAULT 0,
    plucro       NUMERIC NOT NULL DEFAULT 0,
    qt           NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, cnpj)
) PARTITION BY RANGE (ano);

CREATE TABLE IF NOT EXISTS farol.agg_trans_mkt_cli_mes (LIKE farol.agg_fat_mkt_cli_mes INCLUDING ALL) PARTITION BY RANGE (ano);

-- Produto×mês: usado por fetchMktProduto
CREATE TABLE IF NOT EXISTS farol.agg_fat_mkt_produto_mes (
    empresa_id    UUID    NOT NULL,
    ano           INT     NOT NULL,
    mes           INT     NOT NULL,
    cod_prod      TEXT    NOT NULL,
    nome_prod     TEXT    NOT NULL DEFAULT '',
    cod_fornec    TEXT    NOT NULL DEFAULT '',
    nome_fornec   TEXT    NOT NULL DEFAULT '',
    ean           TEXT    NOT NULL DEFAULT '',
    qt_clientes   INT     NOT NULL DEFAULT 0,  -- COUNT DISTINCT cnpj que tem o produto
    qt_positivados INT    NOT NULL DEFAULT 0,  -- COUNT DISTINCT cnpj com qt>0
    pvenda        NUMERIC NOT NULL DEFAULT 0,
    plucro        NUMERIC NOT NULL DEFAULT 0,
    qt            NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, cod_prod)
) PARTITION BY RANGE (ano);

CREATE TABLE IF NOT EXISTS farol.agg_trans_mkt_produto_mes (LIKE farol.agg_fat_mkt_produto_mes INCLUDING ALL) PARTITION BY RANGE (ano);

-- ─── Atualiza catálogo de tabelas (agg_table_names) ─────────────────────────

CREATE OR REPLACE FUNCTION farol.agg_table_names() RETURNS TEXT[] AS $$
BEGIN
    RETURN ARRAY[
        'agg_fat_v01_l0_mes','agg_fat_v01_l1_mes','agg_fat_v01_l2_mes','agg_fat_v01_l3_mes','agg_fat_v01_l4_mes',
        'agg_fat_v02_l0_mes','agg_fat_v02_l1_mes','agg_fat_v02_l2_mes','agg_fat_v02_l3_mes',
        'agg_fat_v03_l0_mes','agg_fat_v03_l1_mes','agg_fat_v03_l2_mes','agg_fat_v03_l3_mes',
        'agg_fat_v04_l0_mes','agg_fat_v04_l1_mes','agg_fat_v04_l2_mes',
        'agg_fat_dims_mes',
        'agg_fat_mkt_cli_mes','agg_fat_mkt_produto_mes',
        'agg_trans_v01_l0_mes','agg_trans_v01_l1_mes','agg_trans_v01_l2_mes','agg_trans_v01_l3_mes','agg_trans_v01_l4_mes',
        'agg_trans_v02_l0_mes','agg_trans_v02_l1_mes','agg_trans_v02_l2_mes','agg_trans_v02_l3_mes',
        'agg_trans_v03_l0_mes','agg_trans_v03_l1_mes','agg_trans_v03_l2_mes','agg_trans_v03_l3_mes',
        'agg_trans_v04_l0_mes','agg_trans_v04_l1_mes','agg_trans_v04_l2_mes',
        'agg_trans_dims_mes',
        'agg_trans_mkt_cli_mes','agg_trans_mkt_produto_mes'
    ];
END;
$$ LANGUAGE plpgsql IMMUTABLE;

-- ─── Cria partições 2025, 2026, 2027 para as novas tabelas ──────────────────

DO $$
DECLARE
    a INT;
    tabs TEXT[] := ARRAY['agg_fat_mkt_cli_mes','agg_trans_mkt_cli_mes','agg_fat_mkt_produto_mes','agg_trans_mkt_produto_mes'];
    t   TEXT;
BEGIN
    FOREACH a IN ARRAY ARRAY[2025, 2026, 2027] LOOP
        FOREACH t IN ARRAY tabs LOOP
            EXECUTE format(
                'CREATE TABLE IF NOT EXISTS farol.%I PARTITION OF farol.%I FOR VALUES FROM (%L) TO (%L)',
                t || '_' || a, t, a, a + 1
            );
        END LOOP;
    END LOOP;
END $$;

-- ─── Atualiza create_agg_year_partitions para incluir as novas ──────────────

CREATE OR REPLACE FUNCTION farol.create_agg_year_partitions(p_ano INT) RETURNS VOID AS $$
DECLARE t TEXT;
BEGIN
    FOREACH t IN ARRAY farol.agg_table_names() LOOP
        EXECUTE format(
            'CREATE TABLE IF NOT EXISTS farol.%I PARTITION OF farol.%I FOR VALUES FROM (%L) TO (%L)',
            t || '_' || p_ano, t, p_ano, p_ano + 1
        );
    END LOOP;
END;
$$ LANGUAGE plpgsql;

-- ─── Estende upsert_aggs_mes para popular os 4 novos agg_mkt ────────────────
-- Estratégia: chama a função original (não vou repetir 1300 linhas) via
-- CREATE OR REPLACE — mas como o código original está em outra migration,
-- não dá pra apenas "estender". Vou recriar a função inteira aqui.
-- O que muda: adiciona 4 blocos INSERT … ON CONFLICT no final.

CREATE OR REPLACE FUNCTION farol.upsert_aggs_mes(
    p_empresa_id UUID,
    p_ano        INT,
    p_mes        INT
) RETURNS VOID AS $$
DECLARE
    p_ini DATE := make_date(p_ano, p_mes, 1);
    p_fim DATE := (p_ini + INTERVAL '1 month' - INTERVAL '1 day')::date;
BEGIN
    -- ════════════════════ FATURADO — base_cli hierárquico ════════════════════
    -- (todos os blocos abaixo replicam exatamente a função da migration 162;
    --  acrescento mkt_cli_mes e mkt_produto_mes no final)

    -- V01_l0 (Fornec)
    INSERT INTO farol.agg_fat_v01_l0_mes AS t
        (empresa_id, ano, mes, cod_fornec, nome_fornec, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_fornec, MAX(v.nome_fornec),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_fat_carteira_rca c WHERE c.empresa_id = v.empresa_id),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_faturadas v
    WHERE v.empresa_id = p_empresa_id AND v.data_faturamento BETWEEN p_ini AND p_fim AND v.cod_fornec <> ''
    GROUP BY v.empresa_id, v.cod_fornec
    ON CONFLICT (ano, empresa_id, mes, cod_fornec) DO UPDATE SET
        nome_fornec = EXCLUDED.nome_fornec, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V01_l1 (+Gerente)
    INSERT INTO farol.agg_fat_v01_l1_mes AS t
        (empresa_id, ano, mes, cod_fornec, cod_gerente, nome_gerente, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_fornec, v.cod_gerente, MAX(v.nome_gerente),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_fat_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_gerente = v.cod_gerente),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_faturadas v
    WHERE v.empresa_id = p_empresa_id AND v.data_faturamento BETWEEN p_ini AND p_fim
      AND v.cod_fornec <> '' AND v.cod_gerente <> ''
    GROUP BY v.empresa_id, v.cod_fornec, v.cod_gerente
    ON CONFLICT (ano, empresa_id, mes, cod_fornec, cod_gerente) DO UPDATE SET
        nome_gerente = EXCLUDED.nome_gerente, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V01_l2 (+Supervisor)
    INSERT INTO farol.agg_fat_v01_l2_mes AS t
        (empresa_id, ano, mes, cod_fornec, cod_gerente, cod_supervisor, nome_supervisor, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_fornec, v.cod_gerente, v.cod_supervisor, MAX(v.nome_supervisor),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_fat_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_supervisor = v.cod_supervisor),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_faturadas v
    WHERE v.empresa_id = p_empresa_id AND v.data_faturamento BETWEEN p_ini AND p_fim
      AND v.cod_fornec <> '' AND v.cod_gerente <> '' AND v.cod_supervisor <> ''
    GROUP BY v.empresa_id, v.cod_fornec, v.cod_gerente, v.cod_supervisor
    ON CONFLICT (ano, empresa_id, mes, cod_fornec, cod_gerente, cod_supervisor) DO UPDATE SET
        nome_supervisor = EXCLUDED.nome_supervisor, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V01_l3 (+RCA)
    INSERT INTO farol.agg_fat_v01_l3_mes AS t
        (empresa_id, ano, mes, cod_fornec, cod_gerente, cod_supervisor, cod_rca, nome_rca,
         base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_fornec, v.cod_gerente, v.cod_supervisor, v.cod_rca, MAX(v.nome_rca),
        MAX(v.qtcli_rca)::INT,
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_faturadas v
    WHERE v.empresa_id = p_empresa_id AND v.data_faturamento BETWEEN p_ini AND p_fim
      AND v.cod_fornec <> '' AND v.cod_gerente <> '' AND v.cod_supervisor <> '' AND v.cod_rca <> ''
    GROUP BY v.empresa_id, v.cod_fornec, v.cod_gerente, v.cod_supervisor, v.cod_rca
    ON CONFLICT (ano, empresa_id, mes, cod_fornec, cod_gerente, cod_supervisor, cod_rca) DO UPDATE SET
        nome_rca = EXCLUDED.nome_rca, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V01_l4 (+Cliente) — base_cli = 1
    INSERT INTO farol.agg_fat_v01_l4_mes AS t
        (empresa_id, ano, mes, cod_fornec, cod_gerente, cod_supervisor, cod_rca, cnpj, cod_cli, nome_cli,
         base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_fornec, v.cod_gerente, v.cod_supervisor, v.cod_rca,
        v.cnpj, MAX(v.cod_cli), MAX(v.nome_cli),
        1,
        (CASE WHEN SUM(v.qt) > 0 THEN 1 ELSE 0 END)::INT,
        COUNT(DISTINCT v.cod_prod) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC,
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_faturadas v
    WHERE v.empresa_id = p_empresa_id AND v.data_faturamento BETWEEN p_ini AND p_fim
      AND v.cod_fornec <> '' AND v.cod_gerente <> '' AND v.cod_supervisor <> '' AND v.cod_rca <> '' AND v.cnpj <> ''
    GROUP BY v.empresa_id, v.cod_fornec, v.cod_gerente, v.cod_supervisor, v.cod_rca, v.cnpj
    ON CONFLICT (ano, empresa_id, mes, cod_fornec, cod_gerente, cod_supervisor, cod_rca, cnpj) DO UPDATE SET
        cod_cli = EXCLUDED.cod_cli, nome_cli = EXCLUDED.nome_cli,
        base_cli = EXCLUDED.base_cli, positivados = EXCLUDED.positivados,
        mix = EXCLUDED.mix, pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V02_l0 (Supv) - base_cli = sum carteira do supervisor
    INSERT INTO farol.agg_fat_v02_l0_mes AS t
        (empresa_id, ano, mes, cod_supervisor, nome_supervisor, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_supervisor, MAX(v.nome_supervisor),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_fat_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_supervisor = v.cod_supervisor),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_faturadas v
    WHERE v.empresa_id = p_empresa_id AND v.data_faturamento BETWEEN p_ini AND p_fim AND v.cod_supervisor <> ''
    GROUP BY v.empresa_id, v.cod_supervisor
    ON CONFLICT (ano, empresa_id, mes, cod_supervisor) DO UPDATE SET
        nome_supervisor = EXCLUDED.nome_supervisor, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V02_l1 (+RCA)
    INSERT INTO farol.agg_fat_v02_l1_mes AS t
        (empresa_id, ano, mes, cod_supervisor, cod_rca, nome_rca, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_supervisor, v.cod_rca, MAX(v.nome_rca),
        MAX(v.qtcli_rca)::INT,
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_faturadas v
    WHERE v.empresa_id = p_empresa_id AND v.data_faturamento BETWEEN p_ini AND p_fim
      AND v.cod_supervisor <> '' AND v.cod_rca <> ''
    GROUP BY v.empresa_id, v.cod_supervisor, v.cod_rca
    ON CONFLICT (ano, empresa_id, mes, cod_supervisor, cod_rca) DO UPDATE SET
        nome_rca = EXCLUDED.nome_rca, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V02_l2 (+Fornec)
    INSERT INTO farol.agg_fat_v02_l2_mes AS t
        (empresa_id, ano, mes, cod_supervisor, cod_rca, cod_fornec, nome_fornec, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_supervisor, v.cod_rca, v.cod_fornec, MAX(v.nome_fornec),
        MAX(v.qtcli_rca)::INT,
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_faturadas v
    WHERE v.empresa_id = p_empresa_id AND v.data_faturamento BETWEEN p_ini AND p_fim
      AND v.cod_supervisor <> '' AND v.cod_rca <> '' AND v.cod_fornec <> ''
    GROUP BY v.empresa_id, v.cod_supervisor, v.cod_rca, v.cod_fornec
    ON CONFLICT (ano, empresa_id, mes, cod_supervisor, cod_rca, cod_fornec) DO UPDATE SET
        nome_fornec = EXCLUDED.nome_fornec, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V02_l3 (+Cliente)
    INSERT INTO farol.agg_fat_v02_l3_mes AS t
        (empresa_id, ano, mes, cod_supervisor, cod_rca, cod_fornec, cnpj, cod_cli, nome_cli,
         base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_supervisor, v.cod_rca, v.cod_fornec,
        v.cnpj, MAX(v.cod_cli), MAX(v.nome_cli),
        1, (CASE WHEN SUM(v.qt) > 0 THEN 1 ELSE 0 END)::INT,
        COUNT(DISTINCT v.cod_prod) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC,
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_faturadas v
    WHERE v.empresa_id = p_empresa_id AND v.data_faturamento BETWEEN p_ini AND p_fim
      AND v.cod_supervisor <> '' AND v.cod_rca <> '' AND v.cod_fornec <> '' AND v.cnpj <> ''
    GROUP BY v.empresa_id, v.cod_supervisor, v.cod_rca, v.cod_fornec, v.cnpj
    ON CONFLICT (ano, empresa_id, mes, cod_supervisor, cod_rca, cod_fornec, cnpj) DO UPDATE SET
        cod_cli = EXCLUDED.cod_cli, nome_cli = EXCLUDED.nome_cli,
        base_cli = EXCLUDED.base_cli, positivados = EXCLUDED.positivados,
        mix = EXCLUDED.mix, pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V03_l0 (Gerente)
    INSERT INTO farol.agg_fat_v03_l0_mes AS t
        (empresa_id, ano, mes, cod_gerente, nome_gerente, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_gerente, MAX(v.nome_gerente),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_fat_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_gerente = v.cod_gerente),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_faturadas v
    WHERE v.empresa_id = p_empresa_id AND v.data_faturamento BETWEEN p_ini AND p_fim AND v.cod_gerente <> ''
    GROUP BY v.empresa_id, v.cod_gerente
    ON CONFLICT (ano, empresa_id, mes, cod_gerente) DO UPDATE SET
        nome_gerente = EXCLUDED.nome_gerente, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V03_l1 (+Supv)
    INSERT INTO farol.agg_fat_v03_l1_mes AS t
        (empresa_id, ano, mes, cod_gerente, cod_supervisor, nome_supervisor, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_gerente, v.cod_supervisor, MAX(v.nome_supervisor),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_fat_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_supervisor = v.cod_supervisor),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_faturadas v
    WHERE v.empresa_id = p_empresa_id AND v.data_faturamento BETWEEN p_ini AND p_fim
      AND v.cod_gerente <> '' AND v.cod_supervisor <> ''
    GROUP BY v.empresa_id, v.cod_gerente, v.cod_supervisor
    ON CONFLICT (ano, empresa_id, mes, cod_gerente, cod_supervisor) DO UPDATE SET
        nome_supervisor = EXCLUDED.nome_supervisor, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V03_l2 (+RCA)
    INSERT INTO farol.agg_fat_v03_l2_mes AS t
        (empresa_id, ano, mes, cod_gerente, cod_supervisor, cod_rca, nome_rca, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_gerente, v.cod_supervisor, v.cod_rca, MAX(v.nome_rca),
        MAX(v.qtcli_rca)::INT,
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_faturadas v
    WHERE v.empresa_id = p_empresa_id AND v.data_faturamento BETWEEN p_ini AND p_fim
      AND v.cod_gerente <> '' AND v.cod_supervisor <> '' AND v.cod_rca <> ''
    GROUP BY v.empresa_id, v.cod_gerente, v.cod_supervisor, v.cod_rca
    ON CONFLICT (ano, empresa_id, mes, cod_gerente, cod_supervisor, cod_rca) DO UPDATE SET
        nome_rca = EXCLUDED.nome_rca, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V03_l3 (+Cliente)
    INSERT INTO farol.agg_fat_v03_l3_mes AS t
        (empresa_id, ano, mes, cod_gerente, cod_supervisor, cod_rca, cnpj, cod_cli, nome_cli,
         base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_gerente, v.cod_supervisor, v.cod_rca,
        v.cnpj, MAX(v.cod_cli), MAX(v.nome_cli),
        1, (CASE WHEN SUM(v.qt) > 0 THEN 1 ELSE 0 END)::INT,
        COUNT(DISTINCT v.cod_prod) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC,
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_faturadas v
    WHERE v.empresa_id = p_empresa_id AND v.data_faturamento BETWEEN p_ini AND p_fim
      AND v.cod_gerente <> '' AND v.cod_supervisor <> '' AND v.cod_rca <> '' AND v.cnpj <> ''
    GROUP BY v.empresa_id, v.cod_gerente, v.cod_supervisor, v.cod_rca, v.cnpj
    ON CONFLICT (ano, empresa_id, mes, cod_gerente, cod_supervisor, cod_rca, cnpj) DO UPDATE SET
        cod_cli = EXCLUDED.cod_cli, nome_cli = EXCLUDED.nome_cli,
        base_cli = EXCLUDED.base_cli, positivados = EXCLUDED.positivados,
        mix = EXCLUDED.mix, pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V04_l0 (RCA)
    INSERT INTO farol.agg_fat_v04_l0_mes AS t
        (empresa_id, ano, mes, cod_rca, nome_rca, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_rca, MAX(v.nome_rca),
        MAX(v.qtcli_rca)::INT,
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_faturadas v
    WHERE v.empresa_id = p_empresa_id AND v.data_faturamento BETWEEN p_ini AND p_fim AND v.cod_rca <> ''
    GROUP BY v.empresa_id, v.cod_rca
    ON CONFLICT (ano, empresa_id, mes, cod_rca) DO UPDATE SET
        nome_rca = EXCLUDED.nome_rca, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V04_l1 (+Fornec)
    INSERT INTO farol.agg_fat_v04_l1_mes AS t
        (empresa_id, ano, mes, cod_rca, cod_fornec, nome_fornec, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_rca, v.cod_fornec, MAX(v.nome_fornec),
        MAX(v.qtcli_rca)::INT,
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_faturadas v
    WHERE v.empresa_id = p_empresa_id AND v.data_faturamento BETWEEN p_ini AND p_fim
      AND v.cod_rca <> '' AND v.cod_fornec <> ''
    GROUP BY v.empresa_id, v.cod_rca, v.cod_fornec
    ON CONFLICT (ano, empresa_id, mes, cod_rca, cod_fornec) DO UPDATE SET
        nome_fornec = EXCLUDED.nome_fornec, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V04_l2 (+Cliente)
    INSERT INTO farol.agg_fat_v04_l2_mes AS t
        (empresa_id, ano, mes, cod_rca, cod_fornec, cnpj, cod_cli, nome_cli,
         base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_rca, v.cod_fornec,
        v.cnpj, MAX(v.cod_cli), MAX(v.nome_cli),
        1, (CASE WHEN SUM(v.qt) > 0 THEN 1 ELSE 0 END)::INT,
        COUNT(DISTINCT v.cod_prod) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC,
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_faturadas v
    WHERE v.empresa_id = p_empresa_id AND v.data_faturamento BETWEEN p_ini AND p_fim
      AND v.cod_rca <> '' AND v.cod_fornec <> '' AND v.cnpj <> ''
    GROUP BY v.empresa_id, v.cod_rca, v.cod_fornec, v.cnpj
    ON CONFLICT (ano, empresa_id, mes, cod_rca, cod_fornec, cnpj) DO UPDATE SET
        cod_cli = EXCLUDED.cod_cli, nome_cli = EXCLUDED.nome_cli,
        base_cli = EXCLUDED.base_cli, positivados = EXCLUDED.positivados,
        mix = EXCLUDED.mix, pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- DIMS fat
    INSERT INTO farol.agg_fat_dims_mes AS t (empresa_id, ano, mes, dim, key, label)
    SELECT empresa_id, p_ano, p_mes, 'fornec', cod_fornec, COALESCE(MAX(nome_fornec),'')
        FROM vendas_faturadas WHERE empresa_id = p_empresa_id AND data_faturamento BETWEEN p_ini AND p_fim AND cod_fornec <> ''
       GROUP BY empresa_id, cod_fornec
    UNION ALL
    SELECT empresa_id, p_ano, p_mes, 'gerente', cod_gerente, COALESCE(MAX(nome_gerente),'')
        FROM vendas_faturadas WHERE empresa_id = p_empresa_id AND data_faturamento BETWEEN p_ini AND p_fim AND cod_gerente <> ''
       GROUP BY empresa_id, cod_gerente
    UNION ALL
    SELECT empresa_id, p_ano, p_mes, 'supervisor', cod_supervisor, COALESCE(MAX(nome_supervisor),'')
        FROM vendas_faturadas WHERE empresa_id = p_empresa_id AND data_faturamento BETWEEN p_ini AND p_fim AND cod_supervisor <> ''
       GROUP BY empresa_id, cod_supervisor
    UNION ALL
    SELECT empresa_id, p_ano, p_mes, 'rca', cod_rca, COALESCE(MAX(nome_rca),'')
        FROM vendas_faturadas WHERE empresa_id = p_empresa_id AND data_faturamento BETWEEN p_ini AND p_fim AND cod_rca <> ''
       GROUP BY empresa_id, cod_rca
    UNION ALL
    SELECT empresa_id, p_ano, p_mes, 'cli', cod_cli, COALESCE(MAX(nome_cli),'')
        FROM vendas_faturadas WHERE empresa_id = p_empresa_id AND data_faturamento BETWEEN p_ini AND p_fim AND cod_cli <> ''
       GROUP BY empresa_id, cod_cli
    UNION ALL
    SELECT empresa_id, p_ano, p_mes, 'uf', uf, uf
        FROM vendas_faturadas WHERE empresa_id = p_empresa_id AND data_faturamento BETWEEN p_ini AND p_fim AND uf <> ''
       GROUP BY empresa_id, uf
    UNION ALL
    SELECT empresa_id, p_ano, p_mes, 'empresa', empresa, empresa
        FROM vendas_faturadas WHERE empresa_id = p_empresa_id AND data_faturamento BETWEEN p_ini AND p_fim AND empresa <> ''
       GROUP BY empresa_id, empresa
    ON CONFLICT (ano, empresa_id, mes, dim, key) DO UPDATE SET label = EXCLUDED.label;

    -- ════════════════════ FATURADO — NOVOS agg_mkt ════════════════════════════

    -- Marketing Cliente×mês (1 linha por cnpj no mês)
    INSERT INTO farol.agg_fat_mkt_cli_mes AS t
        (empresa_id, ano, mes, cnpj, cod_cli, nome_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cnpj,
        MAX(v.cod_cli), MAX(v.nome_cli),
        (CASE WHEN SUM(v.qt) > 0 THEN 1 ELSE 0 END)::INT,
        COUNT(DISTINCT v.cod_prod) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC,
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_faturadas v
    WHERE v.empresa_id = p_empresa_id AND v.data_faturamento BETWEEN p_ini AND p_fim AND v.cnpj <> ''
    GROUP BY v.empresa_id, v.cnpj
    ON CONFLICT (ano, empresa_id, mes, cnpj) DO UPDATE SET
        cod_cli = EXCLUDED.cod_cli, nome_cli = EXCLUDED.nome_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- Marketing Produto×mês (1 linha por cod_prod no mês)
    INSERT INTO farol.agg_fat_mkt_produto_mes AS t
        (empresa_id, ano, mes, cod_prod, nome_prod, cod_fornec, nome_fornec, ean,
         qt_clientes, qt_positivados, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_prod,
        MAX(v.nome_prod), MAX(v.cod_fornec), MAX(v.nome_fornec), MAX(v.ean),
        COUNT(DISTINCT NULLIF(v.cnpj, ''))::INT,
        COUNT(DISTINCT CASE WHEN v.qt > 0 THEN v.cnpj END)::INT,
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_faturadas v
    WHERE v.empresa_id = p_empresa_id AND v.data_faturamento BETWEEN p_ini AND p_fim AND v.cod_prod <> ''
    GROUP BY v.empresa_id, v.cod_prod
    ON CONFLICT (ano, empresa_id, mes, cod_prod) DO UPDATE SET
        nome_prod = EXCLUDED.nome_prod, cod_fornec = EXCLUDED.cod_fornec,
        nome_fornec = EXCLUDED.nome_fornec, ean = EXCLUDED.ean,
        qt_clientes = EXCLUDED.qt_clientes, qt_positivados = EXCLUDED.qt_positivados,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- ════════════════════ TRANSMITIDO — equivalentes ═══════════════════════════
    -- (Mesma estrutura. Dropei comentários pra economizar tokens.)

    INSERT INTO farol.agg_trans_v01_l0_mes AS t
        (empresa_id, ano, mes, cod_fornec, nome_fornec, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_fornec, MAX(v.nome_fornec),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_trans_carteira_rca c WHERE c.empresa_id = v.empresa_id),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_transmitidas v
    WHERE v.empresa_id = p_empresa_id AND v.data_transmissao BETWEEN p_ini AND p_fim AND v.cod_fornec <> ''
    GROUP BY v.empresa_id, v.cod_fornec
    ON CONFLICT (ano, empresa_id, mes, cod_fornec) DO UPDATE SET
        nome_fornec = EXCLUDED.nome_fornec, base_cli = EXCLUDED.base_cli, positivados = EXCLUDED.positivados,
        mix = EXCLUDED.mix, pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_trans_v01_l1_mes AS t
        (empresa_id, ano, mes, cod_fornec, cod_gerente, nome_gerente, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_fornec, v.cod_gerente, MAX(v.nome_gerente),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_trans_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_gerente = v.cod_gerente),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_transmitidas v
    WHERE v.empresa_id = p_empresa_id AND v.data_transmissao BETWEEN p_ini AND p_fim
      AND v.cod_fornec <> '' AND v.cod_gerente <> ''
    GROUP BY v.empresa_id, v.cod_fornec, v.cod_gerente
    ON CONFLICT (ano, empresa_id, mes, cod_fornec, cod_gerente) DO UPDATE SET
        nome_gerente = EXCLUDED.nome_gerente, base_cli = EXCLUDED.base_cli, positivados = EXCLUDED.positivados,
        mix = EXCLUDED.mix, pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_trans_v01_l2_mes AS t
        (empresa_id, ano, mes, cod_fornec, cod_gerente, cod_supervisor, nome_supervisor, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_fornec, v.cod_gerente, v.cod_supervisor, MAX(v.nome_supervisor),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_trans_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_supervisor = v.cod_supervisor),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_transmitidas v
    WHERE v.empresa_id = p_empresa_id AND v.data_transmissao BETWEEN p_ini AND p_fim
      AND v.cod_fornec <> '' AND v.cod_gerente <> '' AND v.cod_supervisor <> ''
    GROUP BY v.empresa_id, v.cod_fornec, v.cod_gerente, v.cod_supervisor
    ON CONFLICT (ano, empresa_id, mes, cod_fornec, cod_gerente, cod_supervisor) DO UPDATE SET
        nome_supervisor = EXCLUDED.nome_supervisor, base_cli = EXCLUDED.base_cli, positivados = EXCLUDED.positivados,
        mix = EXCLUDED.mix, pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_trans_v01_l3_mes AS t
        (empresa_id, ano, mes, cod_fornec, cod_gerente, cod_supervisor, cod_rca, nome_rca, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_fornec, v.cod_gerente, v.cod_supervisor, v.cod_rca, MAX(v.nome_rca),
        MAX(v.qtcli_rca)::INT,
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_transmitidas v
    WHERE v.empresa_id = p_empresa_id AND v.data_transmissao BETWEEN p_ini AND p_fim
      AND v.cod_fornec <> '' AND v.cod_gerente <> '' AND v.cod_supervisor <> '' AND v.cod_rca <> ''
    GROUP BY v.empresa_id, v.cod_fornec, v.cod_gerente, v.cod_supervisor, v.cod_rca
    ON CONFLICT (ano, empresa_id, mes, cod_fornec, cod_gerente, cod_supervisor, cod_rca) DO UPDATE SET
        nome_rca = EXCLUDED.nome_rca, base_cli = EXCLUDED.base_cli, positivados = EXCLUDED.positivados,
        mix = EXCLUDED.mix, pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_trans_v01_l4_mes AS t
        (empresa_id, ano, mes, cod_fornec, cod_gerente, cod_supervisor, cod_rca, cnpj, cod_cli, nome_cli,
         base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_fornec, v.cod_gerente, v.cod_supervisor, v.cod_rca,
        v.cnpj, MAX(v.cod_cli), MAX(v.nome_cli),
        1, (CASE WHEN SUM(v.qt) > 0 THEN 1 ELSE 0 END)::INT,
        COUNT(DISTINCT v.cod_prod) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC,
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_transmitidas v
    WHERE v.empresa_id = p_empresa_id AND v.data_transmissao BETWEEN p_ini AND p_fim
      AND v.cod_fornec <> '' AND v.cod_gerente <> '' AND v.cod_supervisor <> '' AND v.cod_rca <> '' AND v.cnpj <> ''
    GROUP BY v.empresa_id, v.cod_fornec, v.cod_gerente, v.cod_supervisor, v.cod_rca, v.cnpj
    ON CONFLICT (ano, empresa_id, mes, cod_fornec, cod_gerente, cod_supervisor, cod_rca, cnpj) DO UPDATE SET
        cod_cli = EXCLUDED.cod_cli, nome_cli = EXCLUDED.nome_cli, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_trans_v02_l0_mes AS t
        (empresa_id, ano, mes, cod_supervisor, nome_supervisor, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_supervisor, MAX(v.nome_supervisor),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_trans_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_supervisor = v.cod_supervisor),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_transmitidas v
    WHERE v.empresa_id = p_empresa_id AND v.data_transmissao BETWEEN p_ini AND p_fim AND v.cod_supervisor <> ''
    GROUP BY v.empresa_id, v.cod_supervisor
    ON CONFLICT (ano, empresa_id, mes, cod_supervisor) DO UPDATE SET
        nome_supervisor = EXCLUDED.nome_supervisor, base_cli = EXCLUDED.base_cli, positivados = EXCLUDED.positivados,
        mix = EXCLUDED.mix, pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_trans_v02_l1_mes AS t
        (empresa_id, ano, mes, cod_supervisor, cod_rca, nome_rca, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_supervisor, v.cod_rca, MAX(v.nome_rca),
        MAX(v.qtcli_rca)::INT,
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_transmitidas v
    WHERE v.empresa_id = p_empresa_id AND v.data_transmissao BETWEEN p_ini AND p_fim
      AND v.cod_supervisor <> '' AND v.cod_rca <> ''
    GROUP BY v.empresa_id, v.cod_supervisor, v.cod_rca
    ON CONFLICT (ano, empresa_id, mes, cod_supervisor, cod_rca) DO UPDATE SET
        nome_rca = EXCLUDED.nome_rca, base_cli = EXCLUDED.base_cli, positivados = EXCLUDED.positivados,
        mix = EXCLUDED.mix, pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_trans_v02_l2_mes AS t
        (empresa_id, ano, mes, cod_supervisor, cod_rca, cod_fornec, nome_fornec, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_supervisor, v.cod_rca, v.cod_fornec, MAX(v.nome_fornec),
        MAX(v.qtcli_rca)::INT,
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_transmitidas v
    WHERE v.empresa_id = p_empresa_id AND v.data_transmissao BETWEEN p_ini AND p_fim
      AND v.cod_supervisor <> '' AND v.cod_rca <> '' AND v.cod_fornec <> ''
    GROUP BY v.empresa_id, v.cod_supervisor, v.cod_rca, v.cod_fornec
    ON CONFLICT (ano, empresa_id, mes, cod_supervisor, cod_rca, cod_fornec) DO UPDATE SET
        nome_fornec = EXCLUDED.nome_fornec, base_cli = EXCLUDED.base_cli, positivados = EXCLUDED.positivados,
        mix = EXCLUDED.mix, pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_trans_v02_l3_mes AS t
        (empresa_id, ano, mes, cod_supervisor, cod_rca, cod_fornec, cnpj, cod_cli, nome_cli,
         base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_supervisor, v.cod_rca, v.cod_fornec,
        v.cnpj, MAX(v.cod_cli), MAX(v.nome_cli),
        1, (CASE WHEN SUM(v.qt) > 0 THEN 1 ELSE 0 END)::INT,
        COUNT(DISTINCT v.cod_prod) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC,
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_transmitidas v
    WHERE v.empresa_id = p_empresa_id AND v.data_transmissao BETWEEN p_ini AND p_fim
      AND v.cod_supervisor <> '' AND v.cod_rca <> '' AND v.cod_fornec <> '' AND v.cnpj <> ''
    GROUP BY v.empresa_id, v.cod_supervisor, v.cod_rca, v.cod_fornec, v.cnpj
    ON CONFLICT (ano, empresa_id, mes, cod_supervisor, cod_rca, cod_fornec, cnpj) DO UPDATE SET
        cod_cli = EXCLUDED.cod_cli, nome_cli = EXCLUDED.nome_cli, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_trans_v03_l0_mes AS t
        (empresa_id, ano, mes, cod_gerente, nome_gerente, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_gerente, MAX(v.nome_gerente),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_trans_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_gerente = v.cod_gerente),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_transmitidas v
    WHERE v.empresa_id = p_empresa_id AND v.data_transmissao BETWEEN p_ini AND p_fim AND v.cod_gerente <> ''
    GROUP BY v.empresa_id, v.cod_gerente
    ON CONFLICT (ano, empresa_id, mes, cod_gerente) DO UPDATE SET
        nome_gerente = EXCLUDED.nome_gerente, base_cli = EXCLUDED.base_cli, positivados = EXCLUDED.positivados,
        mix = EXCLUDED.mix, pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_trans_v03_l1_mes AS t
        (empresa_id, ano, mes, cod_gerente, cod_supervisor, nome_supervisor, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_gerente, v.cod_supervisor, MAX(v.nome_supervisor),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_trans_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_supervisor = v.cod_supervisor),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_transmitidas v
    WHERE v.empresa_id = p_empresa_id AND v.data_transmissao BETWEEN p_ini AND p_fim
      AND v.cod_gerente <> '' AND v.cod_supervisor <> ''
    GROUP BY v.empresa_id, v.cod_gerente, v.cod_supervisor
    ON CONFLICT (ano, empresa_id, mes, cod_gerente, cod_supervisor) DO UPDATE SET
        nome_supervisor = EXCLUDED.nome_supervisor, base_cli = EXCLUDED.base_cli, positivados = EXCLUDED.positivados,
        mix = EXCLUDED.mix, pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_trans_v03_l2_mes AS t
        (empresa_id, ano, mes, cod_gerente, cod_supervisor, cod_rca, nome_rca, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_gerente, v.cod_supervisor, v.cod_rca, MAX(v.nome_rca),
        MAX(v.qtcli_rca)::INT,
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_transmitidas v
    WHERE v.empresa_id = p_empresa_id AND v.data_transmissao BETWEEN p_ini AND p_fim
      AND v.cod_gerente <> '' AND v.cod_supervisor <> '' AND v.cod_rca <> ''
    GROUP BY v.empresa_id, v.cod_gerente, v.cod_supervisor, v.cod_rca
    ON CONFLICT (ano, empresa_id, mes, cod_gerente, cod_supervisor, cod_rca) DO UPDATE SET
        nome_rca = EXCLUDED.nome_rca, base_cli = EXCLUDED.base_cli, positivados = EXCLUDED.positivados,
        mix = EXCLUDED.mix, pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_trans_v03_l3_mes AS t
        (empresa_id, ano, mes, cod_gerente, cod_supervisor, cod_rca, cnpj, cod_cli, nome_cli,
         base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_gerente, v.cod_supervisor, v.cod_rca,
        v.cnpj, MAX(v.cod_cli), MAX(v.nome_cli),
        1, (CASE WHEN SUM(v.qt) > 0 THEN 1 ELSE 0 END)::INT,
        COUNT(DISTINCT v.cod_prod) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC,
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_transmitidas v
    WHERE v.empresa_id = p_empresa_id AND v.data_transmissao BETWEEN p_ini AND p_fim
      AND v.cod_gerente <> '' AND v.cod_supervisor <> '' AND v.cod_rca <> '' AND v.cnpj <> ''
    GROUP BY v.empresa_id, v.cod_gerente, v.cod_supervisor, v.cod_rca, v.cnpj
    ON CONFLICT (ano, empresa_id, mes, cod_gerente, cod_supervisor, cod_rca, cnpj) DO UPDATE SET
        cod_cli = EXCLUDED.cod_cli, nome_cli = EXCLUDED.nome_cli, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_trans_v04_l0_mes AS t
        (empresa_id, ano, mes, cod_rca, nome_rca, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_rca, MAX(v.nome_rca),
        MAX(v.qtcli_rca)::INT,
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_transmitidas v
    WHERE v.empresa_id = p_empresa_id AND v.data_transmissao BETWEEN p_ini AND p_fim AND v.cod_rca <> ''
    GROUP BY v.empresa_id, v.cod_rca
    ON CONFLICT (ano, empresa_id, mes, cod_rca) DO UPDATE SET
        nome_rca = EXCLUDED.nome_rca, base_cli = EXCLUDED.base_cli, positivados = EXCLUDED.positivados,
        mix = EXCLUDED.mix, pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_trans_v04_l1_mes AS t
        (empresa_id, ano, mes, cod_rca, cod_fornec, nome_fornec, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_rca, v.cod_fornec, MAX(v.nome_fornec),
        MAX(v.qtcli_rca)::INT,
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_transmitidas v
    WHERE v.empresa_id = p_empresa_id AND v.data_transmissao BETWEEN p_ini AND p_fim
      AND v.cod_rca <> '' AND v.cod_fornec <> ''
    GROUP BY v.empresa_id, v.cod_rca, v.cod_fornec
    ON CONFLICT (ano, empresa_id, mes, cod_rca, cod_fornec) DO UPDATE SET
        nome_fornec = EXCLUDED.nome_fornec, base_cli = EXCLUDED.base_cli, positivados = EXCLUDED.positivados,
        mix = EXCLUDED.mix, pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_trans_v04_l2_mes AS t
        (empresa_id, ano, mes, cod_rca, cod_fornec, cnpj, cod_cli, nome_cli,
         base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_rca, v.cod_fornec,
        v.cnpj, MAX(v.cod_cli), MAX(v.nome_cli),
        1, (CASE WHEN SUM(v.qt) > 0 THEN 1 ELSE 0 END)::INT,
        COUNT(DISTINCT v.cod_prod) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC,
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_transmitidas v
    WHERE v.empresa_id = p_empresa_id AND v.data_transmissao BETWEEN p_ini AND p_fim
      AND v.cod_rca <> '' AND v.cod_fornec <> '' AND v.cnpj <> ''
    GROUP BY v.empresa_id, v.cod_rca, v.cod_fornec, v.cnpj
    ON CONFLICT (ano, empresa_id, mes, cod_rca, cod_fornec, cnpj) DO UPDATE SET
        cod_cli = EXCLUDED.cod_cli, nome_cli = EXCLUDED.nome_cli, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- DIMS trans
    INSERT INTO farol.agg_trans_dims_mes AS t (empresa_id, ano, mes, dim, key, label)
    SELECT empresa_id, p_ano, p_mes, 'fornec', cod_fornec, COALESCE(MAX(nome_fornec),'')
        FROM vendas_transmitidas WHERE empresa_id = p_empresa_id AND data_transmissao BETWEEN p_ini AND p_fim AND cod_fornec <> ''
       GROUP BY empresa_id, cod_fornec
    UNION ALL
    SELECT empresa_id, p_ano, p_mes, 'gerente', cod_gerente, COALESCE(MAX(nome_gerente),'')
        FROM vendas_transmitidas WHERE empresa_id = p_empresa_id AND data_transmissao BETWEEN p_ini AND p_fim AND cod_gerente <> ''
       GROUP BY empresa_id, cod_gerente
    UNION ALL
    SELECT empresa_id, p_ano, p_mes, 'supervisor', cod_supervisor, COALESCE(MAX(nome_supervisor),'')
        FROM vendas_transmitidas WHERE empresa_id = p_empresa_id AND data_transmissao BETWEEN p_ini AND p_fim AND cod_supervisor <> ''
       GROUP BY empresa_id, cod_supervisor
    UNION ALL
    SELECT empresa_id, p_ano, p_mes, 'rca', cod_rca, COALESCE(MAX(nome_rca),'')
        FROM vendas_transmitidas WHERE empresa_id = p_empresa_id AND data_transmissao BETWEEN p_ini AND p_fim AND cod_rca <> ''
       GROUP BY empresa_id, cod_rca
    UNION ALL
    SELECT empresa_id, p_ano, p_mes, 'cli', cod_cli, COALESCE(MAX(nome_cli),'')
        FROM vendas_transmitidas WHERE empresa_id = p_empresa_id AND data_transmissao BETWEEN p_ini AND p_fim AND cod_cli <> ''
       GROUP BY empresa_id, cod_cli
    UNION ALL
    SELECT empresa_id, p_ano, p_mes, 'uf', uf, uf
        FROM vendas_transmitidas WHERE empresa_id = p_empresa_id AND data_transmissao BETWEEN p_ini AND p_fim AND uf <> ''
       GROUP BY empresa_id, uf
    UNION ALL
    SELECT empresa_id, p_ano, p_mes, 'empresa', empresa, empresa
        FROM vendas_transmitidas WHERE empresa_id = p_empresa_id AND data_transmissao BETWEEN p_ini AND p_fim AND empresa <> ''
       GROUP BY empresa_id, empresa
    ON CONFLICT (ano, empresa_id, mes, dim, key) DO UPDATE SET label = EXCLUDED.label;

    -- ════════════════════ TRANSMITIDO — NOVOS agg_mkt ═════════════════════════

    INSERT INTO farol.agg_trans_mkt_cli_mes AS t
        (empresa_id, ano, mes, cnpj, cod_cli, nome_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cnpj,
        MAX(v.cod_cli), MAX(v.nome_cli),
        (CASE WHEN SUM(v.qt) > 0 THEN 1 ELSE 0 END)::INT,
        COUNT(DISTINCT v.cod_prod) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC,
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_transmitidas v
    WHERE v.empresa_id = p_empresa_id AND v.data_transmissao BETWEEN p_ini AND p_fim AND v.cnpj <> ''
    GROUP BY v.empresa_id, v.cnpj
    ON CONFLICT (ano, empresa_id, mes, cnpj) DO UPDATE SET
        cod_cli = EXCLUDED.cod_cli, nome_cli = EXCLUDED.nome_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_trans_mkt_produto_mes AS t
        (empresa_id, ano, mes, cod_prod, nome_prod, cod_fornec, nome_fornec, ean,
         qt_clientes, qt_positivados, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_prod,
        MAX(v.nome_prod), MAX(v.cod_fornec), MAX(v.nome_fornec), MAX(v.ean),
        COUNT(DISTINCT NULLIF(v.cnpj, ''))::INT,
        COUNT(DISTINCT CASE WHEN v.qt > 0 THEN v.cnpj END)::INT,
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_transmitidas v
    WHERE v.empresa_id = p_empresa_id AND v.data_transmissao BETWEEN p_ini AND p_fim AND v.cod_prod <> ''
    GROUP BY v.empresa_id, v.cod_prod
    ON CONFLICT (ano, empresa_id, mes, cod_prod) DO UPDATE SET
        nome_prod = EXCLUDED.nome_prod, cod_fornec = EXCLUDED.cod_fornec,
        nome_fornec = EXCLUDED.nome_fornec, ean = EXCLUDED.ean,
        qt_clientes = EXCLUDED.qt_clientes, qt_positivados = EXCLUDED.qt_positivados,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;
END;
$$ LANGUAGE plpgsql;
