-- 167_agg_v05_sup_fornec_rca.sql
-- Nova hierarquia V05 para o painel mobile (toggle "Por Fornecedor"):
--   Supervisor → Fornecedor → RCA → Cliente → Produto
--
-- A hierarquia V02 atual (Sup → RCA → Fornec → Cli) atende "Por RCA".
-- O gerente quer alternar no mesmo painel mobile — daí V05.
--
-- A partir do próximo mês haverá carga DIÁRIA de vendas. As tabelas V05
-- ficam definitivas agora pra não termos que repopular depois.
--
-- Estrutura — todas particionadas por ano, mesmo padrão das demais:
--   agg_fat_v05_l0_mes   PK (ano, empresa_id, mes, cod_supervisor)
--   agg_fat_v05_l1_mes   PK (ano, empresa_id, mes, cod_supervisor, cod_fornec)
--   agg_fat_v05_l2_mes   PK (ano, empresa_id, mes, cod_supervisor, cod_fornec, cod_rca)
--   agg_fat_v05_l3_mes   PK (ano, empresa_id, mes, cod_supervisor, cod_fornec, cod_rca, cnpj)
--   + agg_trans_v05_l0..l3_mes (idêntico)
--
-- base_cli hierárquico:
--   l0 (Sup)               → SUM qtcli_rca dos RCAs do supervisor
--   l1 (Sup + Fornec)      → SUM qtcli_rca dos RCAs do supervisor (denominador fixo no escopo)
--   l2 (Sup + Fornec + RCA)→ qtcli_rca do RCA
--   l3 (+ Cliente)         → 1

-- ────────────────────────────────────────────────────────────────────────────
-- PARTE 1 — DDL das 8 tabelas
-- ────────────────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS farol.agg_fat_v05_l0_mes (
    empresa_id      UUID    NOT NULL,
    ano             INT     NOT NULL,
    mes             INT     NOT NULL,
    cod_supervisor  TEXT    NOT NULL,
    nome_supervisor TEXT    NOT NULL DEFAULT '',
    base_cli        INT     NOT NULL DEFAULT 0,
    positivados     INT     NOT NULL DEFAULT 0,
    mix             NUMERIC NOT NULL DEFAULT 0,
    pvenda          NUMERIC NOT NULL DEFAULT 0,
    plucro          NUMERIC NOT NULL DEFAULT 0,
    qt              NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, cod_supervisor)
) PARTITION BY RANGE (ano);

CREATE TABLE IF NOT EXISTS farol.agg_fat_v05_l1_mes (
    empresa_id      UUID    NOT NULL,
    ano             INT     NOT NULL,
    mes             INT     NOT NULL,
    cod_supervisor  TEXT    NOT NULL,
    cod_fornec      TEXT    NOT NULL,
    nome_fornec     TEXT    NOT NULL DEFAULT '',
    base_cli        INT     NOT NULL DEFAULT 0,
    positivados     INT     NOT NULL DEFAULT 0,
    mix             NUMERIC NOT NULL DEFAULT 0,
    pvenda          NUMERIC NOT NULL DEFAULT 0,
    plucro          NUMERIC NOT NULL DEFAULT 0,
    qt              NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, cod_supervisor, cod_fornec)
) PARTITION BY RANGE (ano);

CREATE TABLE IF NOT EXISTS farol.agg_fat_v05_l2_mes (
    empresa_id      UUID    NOT NULL,
    ano             INT     NOT NULL,
    mes             INT     NOT NULL,
    cod_supervisor  TEXT    NOT NULL,
    cod_fornec      TEXT    NOT NULL,
    cod_rca         TEXT    NOT NULL,
    nome_rca        TEXT    NOT NULL DEFAULT '',
    base_cli        INT     NOT NULL DEFAULT 0,
    positivados     INT     NOT NULL DEFAULT 0,
    mix             NUMERIC NOT NULL DEFAULT 0,
    pvenda          NUMERIC NOT NULL DEFAULT 0,
    plucro          NUMERIC NOT NULL DEFAULT 0,
    qt              NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, cod_supervisor, cod_fornec, cod_rca)
) PARTITION BY RANGE (ano);

CREATE TABLE IF NOT EXISTS farol.agg_fat_v05_l3_mes (
    empresa_id      UUID    NOT NULL,
    ano             INT     NOT NULL,
    mes             INT     NOT NULL,
    cod_supervisor  TEXT    NOT NULL,
    cod_fornec      TEXT    NOT NULL,
    cod_rca         TEXT    NOT NULL,
    cnpj            TEXT    NOT NULL,
    cod_cli         TEXT    NOT NULL DEFAULT '',
    nome_cli        TEXT    NOT NULL DEFAULT '',
    base_cli        INT     NOT NULL DEFAULT 0,
    positivados     INT     NOT NULL DEFAULT 0,
    mix             NUMERIC NOT NULL DEFAULT 0,
    pvenda          NUMERIC NOT NULL DEFAULT 0,
    plucro          NUMERIC NOT NULL DEFAULT 0,
    qt              NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, cod_supervisor, cod_fornec, cod_rca, cnpj)
) PARTITION BY RANGE (ano);

-- Equivalentes trans (LIKE INCLUDING ALL não funciona em particionadas; CREATE explícito)

CREATE TABLE IF NOT EXISTS farol.agg_trans_v05_l0_mes (
    empresa_id      UUID    NOT NULL,
    ano             INT     NOT NULL,
    mes             INT     NOT NULL,
    cod_supervisor  TEXT    NOT NULL,
    nome_supervisor TEXT    NOT NULL DEFAULT '',
    base_cli        INT     NOT NULL DEFAULT 0,
    positivados     INT     NOT NULL DEFAULT 0,
    mix             NUMERIC NOT NULL DEFAULT 0,
    pvenda          NUMERIC NOT NULL DEFAULT 0,
    plucro          NUMERIC NOT NULL DEFAULT 0,
    qt              NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, cod_supervisor)
) PARTITION BY RANGE (ano);

CREATE TABLE IF NOT EXISTS farol.agg_trans_v05_l1_mes (
    empresa_id      UUID    NOT NULL,
    ano             INT     NOT NULL,
    mes             INT     NOT NULL,
    cod_supervisor  TEXT    NOT NULL,
    cod_fornec      TEXT    NOT NULL,
    nome_fornec     TEXT    NOT NULL DEFAULT '',
    base_cli        INT     NOT NULL DEFAULT 0,
    positivados     INT     NOT NULL DEFAULT 0,
    mix             NUMERIC NOT NULL DEFAULT 0,
    pvenda          NUMERIC NOT NULL DEFAULT 0,
    plucro          NUMERIC NOT NULL DEFAULT 0,
    qt              NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, cod_supervisor, cod_fornec)
) PARTITION BY RANGE (ano);

CREATE TABLE IF NOT EXISTS farol.agg_trans_v05_l2_mes (
    empresa_id      UUID    NOT NULL,
    ano             INT     NOT NULL,
    mes             INT     NOT NULL,
    cod_supervisor  TEXT    NOT NULL,
    cod_fornec      TEXT    NOT NULL,
    cod_rca         TEXT    NOT NULL,
    nome_rca        TEXT    NOT NULL DEFAULT '',
    base_cli        INT     NOT NULL DEFAULT 0,
    positivados     INT     NOT NULL DEFAULT 0,
    mix             NUMERIC NOT NULL DEFAULT 0,
    pvenda          NUMERIC NOT NULL DEFAULT 0,
    plucro          NUMERIC NOT NULL DEFAULT 0,
    qt              NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, cod_supervisor, cod_fornec, cod_rca)
) PARTITION BY RANGE (ano);

CREATE TABLE IF NOT EXISTS farol.agg_trans_v05_l3_mes (
    empresa_id      UUID    NOT NULL,
    ano             INT     NOT NULL,
    mes             INT     NOT NULL,
    cod_supervisor  TEXT    NOT NULL,
    cod_fornec      TEXT    NOT NULL,
    cod_rca         TEXT    NOT NULL,
    cnpj            TEXT    NOT NULL,
    cod_cli         TEXT    NOT NULL DEFAULT '',
    nome_cli        TEXT    NOT NULL DEFAULT '',
    base_cli        INT     NOT NULL DEFAULT 0,
    positivados     INT     NOT NULL DEFAULT 0,
    mix             NUMERIC NOT NULL DEFAULT 0,
    pvenda          NUMERIC NOT NULL DEFAULT 0,
    plucro          NUMERIC NOT NULL DEFAULT 0,
    qt              NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, cod_supervisor, cod_fornec, cod_rca, cnpj)
) PARTITION BY RANGE (ano);

-- ────────────────────────────────────────────────────────────────────────────
-- PARTE 2 — Atualiza agg_table_names() para incluir V05
-- ────────────────────────────────────────────────────────────────────────────

CREATE OR REPLACE FUNCTION farol.agg_table_names() RETURNS TEXT[] AS $$
BEGIN
    RETURN ARRAY[
        'agg_fat_v01_l0_mes','agg_fat_v01_l1_mes','agg_fat_v01_l2_mes','agg_fat_v01_l3_mes','agg_fat_v01_l4_mes',
        'agg_fat_v02_l0_mes','agg_fat_v02_l1_mes','agg_fat_v02_l2_mes','agg_fat_v02_l3_mes',
        'agg_fat_v03_l0_mes','agg_fat_v03_l1_mes','agg_fat_v03_l2_mes','agg_fat_v03_l3_mes',
        'agg_fat_v04_l0_mes','agg_fat_v04_l1_mes','agg_fat_v04_l2_mes',
        'agg_fat_v05_l0_mes','agg_fat_v05_l1_mes','agg_fat_v05_l2_mes','agg_fat_v05_l3_mes',
        'agg_fat_dims_mes',
        'agg_fat_mkt_cli_mes','agg_fat_mkt_produto_mes',
        'agg_trans_v01_l0_mes','agg_trans_v01_l1_mes','agg_trans_v01_l2_mes','agg_trans_v01_l3_mes','agg_trans_v01_l4_mes',
        'agg_trans_v02_l0_mes','agg_trans_v02_l1_mes','agg_trans_v02_l2_mes','agg_trans_v02_l3_mes',
        'agg_trans_v03_l0_mes','agg_trans_v03_l1_mes','agg_trans_v03_l2_mes','agg_trans_v03_l3_mes',
        'agg_trans_v04_l0_mes','agg_trans_v04_l1_mes','agg_trans_v04_l2_mes',
        'agg_trans_v05_l0_mes','agg_trans_v05_l1_mes','agg_trans_v05_l2_mes','agg_trans_v05_l3_mes',
        'agg_trans_dims_mes',
        'agg_trans_mkt_cli_mes','agg_trans_mkt_produto_mes'
    ];
END;
$$ LANGUAGE plpgsql IMMUTABLE;

-- ────────────────────────────────────────────────────────────────────────────
-- PARTE 3 — Cria partições 2025, 2026, 2027 para as 8 novas tabelas
-- ────────────────────────────────────────────────────────────────────────────

DO $$
DECLARE
    a INT;
    tabs TEXT[] := ARRAY[
        'agg_fat_v05_l0_mes','agg_fat_v05_l1_mes','agg_fat_v05_l2_mes','agg_fat_v05_l3_mes',
        'agg_trans_v05_l0_mes','agg_trans_v05_l1_mes','agg_trans_v05_l2_mes','agg_trans_v05_l3_mes'
    ];
    t TEXT;
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

-- ────────────────────────────────────────────────────────────────────────────
-- PARTE 4 — Função auxiliar farol.upsert_aggs_mes_v05
-- Roda DEPOIS de farol.upsert_aggs_mes (que ainda cria as TEMP _v_fat e _v_trans
-- no escopo da sessão). Aqui criamos NOVAS TEMP TABLES locais — necessário
-- porque ON COMMIT DROP da função principal já as dropou ao final dela.
-- ────────────────────────────────────────────────────────────────────────────

CREATE OR REPLACE FUNCTION farol.upsert_aggs_mes_v05(
    p_empresa_id UUID,
    p_ano        INT,
    p_mes        INT
) RETURNS VOID AS $$
DECLARE
    p_ini DATE := make_date(p_ano, p_mes, 1);
    p_fim DATE := (p_ini + INTERVAL '1 month' - INTERVAL '1 day')::date;
BEGIN
    SET LOCAL work_mem = '256MB';

    -- ════════════════════ TEMP FAT ════════════════════════════════════════════
    DROP TABLE IF EXISTS _v05_fat;
    CREATE TEMP TABLE _v05_fat ON COMMIT DROP AS
    SELECT
        v.empresa_id, v.cod_supervisor, v.cod_fornec, v.nome_fornec,
        v.cod_rca, v.nome_rca, v.qtcli_rca,
        v.cnpj, v.cod_cli, v.nome_cli,
        v.cod_prod, v.pvenda, v.plucro, v.qt
    FROM vendas_faturadas v
    WHERE v.empresa_id = p_empresa_id
      AND v.data_faturamento BETWEEN p_ini AND p_fim
      AND v.cod_supervisor <> '';
    CREATE INDEX ON _v05_fat (cod_supervisor);
    CREATE INDEX ON _v05_fat (cod_fornec);
    CREATE INDEX ON _v05_fat (cod_rca);
    CREATE INDEX ON _v05_fat (cnpj);
    ANALYZE _v05_fat;

    -- V05_l0 fat (sup) — base = SUM qtcli_rca dos RCAs do supervisor.
    -- O nome_supervisor vem de vendas_faturadas direto, MAX(...) por consistência
    -- (esse campo NÃO está na TEMP _v05_fat — busca via lookup adicional logo abaixo)
    INSERT INTO farol.agg_fat_v05_l0_mes AS t
        (empresa_id, ano, mes, cod_supervisor, nome_supervisor, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_supervisor,
        COALESCE((SELECT MAX(d.label) FROM farol.agg_fat_dims_mes d
                   WHERE d.empresa_id=p_empresa_id AND d.dim='supervisor' AND d.key=v.cod_supervisor), ''),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_fat_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_supervisor = v.cod_supervisor),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v05_fat v
    GROUP BY v.empresa_id, v.cod_supervisor
    ON CONFLICT (ano, empresa_id, mes, cod_supervisor) DO UPDATE SET
        nome_supervisor = EXCLUDED.nome_supervisor, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V05_l1 fat (sup + fornec) — base = SUM qtcli_rca dos RCAs do supervisor (denominador fixo)
    INSERT INTO farol.agg_fat_v05_l1_mes AS t
        (empresa_id, ano, mes, cod_supervisor, cod_fornec, nome_fornec, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_supervisor, v.cod_fornec, MAX(v.nome_fornec),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_fat_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_supervisor = v.cod_supervisor),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v05_fat v
    WHERE v.cod_fornec <> ''
    GROUP BY v.empresa_id, v.cod_supervisor, v.cod_fornec
    ON CONFLICT (ano, empresa_id, mes, cod_supervisor, cod_fornec) DO UPDATE SET
        nome_fornec = EXCLUDED.nome_fornec, base_cli = EXCLUDED.base_cli, positivados = EXCLUDED.positivados,
        mix = EXCLUDED.mix, pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V05_l2 fat (sup + fornec + rca) — base = qtcli_rca do RCA
    INSERT INTO farol.agg_fat_v05_l2_mes AS t
        (empresa_id, ano, mes, cod_supervisor, cod_fornec, cod_rca, nome_rca, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_supervisor, v.cod_fornec, v.cod_rca, MAX(v.nome_rca),
        MAX(v.qtcli_rca)::INT,
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v05_fat v
    WHERE v.cod_fornec <> '' AND v.cod_rca <> ''
    GROUP BY v.empresa_id, v.cod_supervisor, v.cod_fornec, v.cod_rca
    ON CONFLICT (ano, empresa_id, mes, cod_supervisor, cod_fornec, cod_rca) DO UPDATE SET
        nome_rca = EXCLUDED.nome_rca, base_cli = EXCLUDED.base_cli, positivados = EXCLUDED.positivados,
        mix = EXCLUDED.mix, pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V05_l3 fat (sup + fornec + rca + cnpj) — base = 1
    INSERT INTO farol.agg_fat_v05_l3_mes AS t
        (empresa_id, ano, mes, cod_supervisor, cod_fornec, cod_rca, cnpj, cod_cli, nome_cli,
         base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_supervisor, v.cod_fornec, v.cod_rca,
        v.cnpj, MAX(v.cod_cli), MAX(v.nome_cli),
        1, (CASE WHEN SUM(v.qt) > 0 THEN 1 ELSE 0 END)::INT,
        COUNT(DISTINCT v.cod_prod) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC,
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v05_fat v
    WHERE v.cod_fornec <> '' AND v.cod_rca <> '' AND v.cnpj <> ''
    GROUP BY v.empresa_id, v.cod_supervisor, v.cod_fornec, v.cod_rca, v.cnpj
    ON CONFLICT (ano, empresa_id, mes, cod_supervisor, cod_fornec, cod_rca, cnpj) DO UPDATE SET
        cod_cli = EXCLUDED.cod_cli, nome_cli = EXCLUDED.nome_cli, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    DROP TABLE _v05_fat;

    -- ════════════════════ TEMP TRANS ══════════════════════════════════════════
    DROP TABLE IF EXISTS _v05_trans;
    CREATE TEMP TABLE _v05_trans ON COMMIT DROP AS
    SELECT
        v.empresa_id, v.cod_supervisor, v.cod_fornec, v.nome_fornec,
        v.cod_rca, v.nome_rca, v.qtcli_rca,
        v.cnpj, v.cod_cli, v.nome_cli,
        v.cod_prod, v.pvenda, v.plucro, v.qt
    FROM vendas_transmitidas v
    WHERE v.empresa_id = p_empresa_id
      AND v.data_transmissao BETWEEN p_ini AND p_fim
      AND v.cod_supervisor <> '';
    CREATE INDEX ON _v05_trans (cod_supervisor);
    CREATE INDEX ON _v05_trans (cod_fornec);
    CREATE INDEX ON _v05_trans (cod_rca);
    CREATE INDEX ON _v05_trans (cnpj);
    ANALYZE _v05_trans;

    INSERT INTO farol.agg_trans_v05_l0_mes AS t
        (empresa_id, ano, mes, cod_supervisor, nome_supervisor, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_supervisor,
        COALESCE((SELECT MAX(d.label) FROM farol.agg_trans_dims_mes d
                   WHERE d.empresa_id=p_empresa_id AND d.dim='supervisor' AND d.key=v.cod_supervisor), ''),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_trans_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_supervisor = v.cod_supervisor),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v05_trans v
    GROUP BY v.empresa_id, v.cod_supervisor
    ON CONFLICT (ano, empresa_id, mes, cod_supervisor) DO UPDATE SET
        nome_supervisor = EXCLUDED.nome_supervisor, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_trans_v05_l1_mes AS t
        (empresa_id, ano, mes, cod_supervisor, cod_fornec, nome_fornec, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_supervisor, v.cod_fornec, MAX(v.nome_fornec),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_trans_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_supervisor = v.cod_supervisor),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v05_trans v
    WHERE v.cod_fornec <> ''
    GROUP BY v.empresa_id, v.cod_supervisor, v.cod_fornec
    ON CONFLICT (ano, empresa_id, mes, cod_supervisor, cod_fornec) DO UPDATE SET
        nome_fornec = EXCLUDED.nome_fornec, base_cli = EXCLUDED.base_cli, positivados = EXCLUDED.positivados,
        mix = EXCLUDED.mix, pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_trans_v05_l2_mes AS t
        (empresa_id, ano, mes, cod_supervisor, cod_fornec, cod_rca, nome_rca, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_supervisor, v.cod_fornec, v.cod_rca, MAX(v.nome_rca),
        MAX(v.qtcli_rca)::INT,
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v05_trans v
    WHERE v.cod_fornec <> '' AND v.cod_rca <> ''
    GROUP BY v.empresa_id, v.cod_supervisor, v.cod_fornec, v.cod_rca
    ON CONFLICT (ano, empresa_id, mes, cod_supervisor, cod_fornec, cod_rca) DO UPDATE SET
        nome_rca = EXCLUDED.nome_rca, base_cli = EXCLUDED.base_cli, positivados = EXCLUDED.positivados,
        mix = EXCLUDED.mix, pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_trans_v05_l3_mes AS t
        (empresa_id, ano, mes, cod_supervisor, cod_fornec, cod_rca, cnpj, cod_cli, nome_cli,
         base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_supervisor, v.cod_fornec, v.cod_rca,
        v.cnpj, MAX(v.cod_cli), MAX(v.nome_cli),
        1, (CASE WHEN SUM(v.qt) > 0 THEN 1 ELSE 0 END)::INT,
        COUNT(DISTINCT v.cod_prod) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC,
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v05_trans v
    WHERE v.cod_fornec <> '' AND v.cod_rca <> '' AND v.cnpj <> ''
    GROUP BY v.empresa_id, v.cod_supervisor, v.cod_fornec, v.cod_rca, v.cnpj
    ON CONFLICT (ano, empresa_id, mes, cod_supervisor, cod_fornec, cod_rca, cnpj) DO UPDATE SET
        cod_cli = EXCLUDED.cod_cli, nome_cli = EXCLUDED.nome_cli, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    DROP TABLE _v05_trans;
END;
$$ LANGUAGE plpgsql;

-- ────────────────────────────────────────────────────────────────────────────
-- PARTE 5 — Faz upsert_aggs_mes chamar a v05 ao final.
-- Mais simples que reescrever a função inteira (mig 166 tem ~600 linhas):
-- criamos um wrapper que chama a original e depois a v05.
-- ────────────────────────────────────────────────────────────────────────────

-- Renomeia a função atual e cria um wrapper. Como CREATE OR REPLACE permite
-- mudar o corpo mas mantém a assinatura, o wrapper fica como upsert_aggs_mes
-- (mesma assinatura) e chama uma cópia interna upsert_aggs_mes_core.
-- Idempotente: só renomeia se ainda não existe upsert_aggs_mes_core.

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace
         WHERE n.nspname='farol' AND p.proname='upsert_aggs_mes_core'
    ) THEN
        ALTER FUNCTION farol.upsert_aggs_mes(UUID, INT, INT) RENAME TO upsert_aggs_mes_core;
    END IF;
END $$;

CREATE OR REPLACE FUNCTION farol.upsert_aggs_mes(
    p_empresa_id UUID,
    p_ano        INT,
    p_mes        INT
) RETURNS VOID AS $$
BEGIN
    PERFORM farol.upsert_aggs_mes_core(p_empresa_id, p_ano, p_mes);
    PERFORM farol.upsert_aggs_mes_v05(p_empresa_id, p_ano, p_mes);
END;
$$ LANGUAGE plpgsql;
