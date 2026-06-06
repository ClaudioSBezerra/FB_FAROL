-- 162_agg_partitioned_mes.sql
-- ════════════════════════════════════════════════════════════════════════════
-- Agregados particionados por ANO, grão MÊS — substituem as MVs diárias.
-- ════════════════════════════════════════════════════════════════════════════
--
-- DECISÕES (sessão 2026-06-05/06):
--   • 4 visões: V01 (Fornec), V02 (Supv), V03 (Gerente), V04 (RCA — novo)
--   • Grão MES (não DIA) — corrige semântica de positivados/mix
--   • PARTITION BY RANGE (ano) — DROP de ano expirado é O(1)
--   • Carteira RCA continua nas MVs antigas (mv_*_carteira_rca da mig 161)
--   • MVs antigas COEXISTEM com estas tabelas — rollback safe
--   • Retenção (opção C): 1 ano anterior + ano corrente. Sem arquivamento.
--     Em fim de jan/2027 → DROP partition ano 2025.
--
-- MÉTRICAS PRÉ-ESTABELECIDAS (6):
--   pvenda      → SUM por nível
--   plucro      → SUM por nível
--   qt          → SUM por nível
--   positivados → COUNT(DISTINCT cnpj) FILTER (qt>0)  (correto por MÊS)
--   base_cli    → SUM qtcli_rca da carteira (hierárquico por nível)
--   mix         → AVG produtos distintos por cliente positivado, calculado
--                 como COUNT(DISTINCT (cnpj,cod_prod)) / COUNT(DISTINCT cnpj)
--
-- BASE_CLI HIERÁRQUICO (mesma lógica da mig 161):
--   V01_l0 (Fornec)        → SUM qtcli_rca de toda empresa (denominador fixo)
--   V01_l1 (+Gerente)      → SUM qtcli_rca dos RCAs do gerente
--   V01_l2 (+Supervisor)   → SUM qtcli_rca dos RCAs do supervisor
--   V01_l3 (+RCA)          → qtcli_rca próprio do RCA
--   V01_l4 (+Cliente)      → 1
--   V02_l0 (Supv)          → SUM qtcli_rca dos RCAs do supervisor
--   V02_l1 (+RCA)          → qtcli_rca do RCA
--   V02_l2 (+Fornec)       → qtcli_rca do RCA (mesmo escopo)
--   V02_l3 (+Cliente)      → 1
--   V03_l0 (Gerente)       → SUM qtcli_rca dos RCAs do gerente
--   V03_l1 (+Supv)         → SUM qtcli_rca dos RCAs do supervisor
--   V03_l2 (+RCA)          → qtcli_rca do RCA
--   V03_l3 (+Cliente)      → 1
--   V04_l0 (RCA)           → qtcli_rca do RCA
--   V04_l1 (+Fornec)       → qtcli_rca do RCA
--   V04_l2 (+Cliente)      → 1
--
-- ESTRUTURA TOTAL:
--   32 tabelas agregadas (V01:5 + V02:4 + V03:4 + V04:3 = 16 níveis × 2 fluxos)
--   +  2 tabelas dims_mes (uma por fluxo)
--   =  34 tabelas particionadas
--
-- PIPELINE DE USO (não implementado neste arquivo):
--   após cada import → SELECT farol.upsert_aggs_mes(empresa_id, ano, mes)
--   janela de manutenção anual → SELECT farol.drop_agg_year_partitions(ano)
-- ════════════════════════════════════════════════════════════════════════════

CREATE SCHEMA IF NOT EXISTS farol;

-- ════════════════════════════════════════════════════════════════════════════
-- PARTE 1 — DDL das 34 tabelas particionadas
-- ════════════════════════════════════════════════════════════════════════════
-- Convenção: agg_<fluxo>_<visao>_<nivel>_mes
--   fluxo  ∈ {fat, trans}
--   visao  ∈ {v01, v02, v03, v04}
--   nivel  ∈ {l0, l1, l2, l3, l4}
-- Cada uma é PARTITION BY RANGE (ano) — partições reais criadas adiante.

-- ─── V01 (Fornecedor) — faturado ────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS farol.agg_fat_v01_l0_mes (
    empresa_id   UUID    NOT NULL,
    ano          INT     NOT NULL,
    mes          INT     NOT NULL,
    cod_fornec   TEXT    NOT NULL,
    nome_fornec  TEXT    NOT NULL DEFAULT '',
    base_cli     INT     NOT NULL DEFAULT 0,
    positivados  INT     NOT NULL DEFAULT 0,
    mix          NUMERIC NOT NULL DEFAULT 0,
    pvenda       NUMERIC NOT NULL DEFAULT 0,
    plucro       NUMERIC NOT NULL DEFAULT 0,
    qt           NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, cod_fornec)
) PARTITION BY RANGE (ano);

CREATE TABLE IF NOT EXISTS farol.agg_fat_v01_l1_mes (
    empresa_id    UUID    NOT NULL,
    ano           INT     NOT NULL,
    mes           INT     NOT NULL,
    cod_fornec    TEXT    NOT NULL,
    cod_gerente   TEXT    NOT NULL,
    nome_gerente  TEXT    NOT NULL DEFAULT '',
    base_cli      INT     NOT NULL DEFAULT 0,
    positivados   INT     NOT NULL DEFAULT 0,
    mix           NUMERIC NOT NULL DEFAULT 0,
    pvenda        NUMERIC NOT NULL DEFAULT 0,
    plucro        NUMERIC NOT NULL DEFAULT 0,
    qt            NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, cod_fornec, cod_gerente)
) PARTITION BY RANGE (ano);

CREATE TABLE IF NOT EXISTS farol.agg_fat_v01_l2_mes (
    empresa_id       UUID    NOT NULL,
    ano              INT     NOT NULL,
    mes              INT     NOT NULL,
    cod_fornec       TEXT    NOT NULL,
    cod_gerente      TEXT    NOT NULL,
    cod_supervisor   TEXT    NOT NULL,
    nome_supervisor  TEXT    NOT NULL DEFAULT '',
    base_cli         INT     NOT NULL DEFAULT 0,
    positivados      INT     NOT NULL DEFAULT 0,
    mix              NUMERIC NOT NULL DEFAULT 0,
    pvenda           NUMERIC NOT NULL DEFAULT 0,
    plucro           NUMERIC NOT NULL DEFAULT 0,
    qt               NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, cod_fornec, cod_gerente, cod_supervisor)
) PARTITION BY RANGE (ano);

CREATE TABLE IF NOT EXISTS farol.agg_fat_v01_l3_mes (
    empresa_id      UUID    NOT NULL,
    ano             INT     NOT NULL,
    mes             INT     NOT NULL,
    cod_fornec      TEXT    NOT NULL,
    cod_gerente     TEXT    NOT NULL,
    cod_supervisor  TEXT    NOT NULL,
    cod_rca         TEXT    NOT NULL,
    nome_rca        TEXT    NOT NULL DEFAULT '',
    base_cli        INT     NOT NULL DEFAULT 0,
    positivados     INT     NOT NULL DEFAULT 0,
    mix             NUMERIC NOT NULL DEFAULT 0,
    pvenda          NUMERIC NOT NULL DEFAULT 0,
    plucro          NUMERIC NOT NULL DEFAULT 0,
    qt              NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, cod_fornec, cod_gerente, cod_supervisor, cod_rca)
) PARTITION BY RANGE (ano);

CREATE TABLE IF NOT EXISTS farol.agg_fat_v01_l4_mes (
    empresa_id      UUID    NOT NULL,
    ano             INT     NOT NULL,
    mes             INT     NOT NULL,
    cod_fornec      TEXT    NOT NULL,
    cod_gerente     TEXT    NOT NULL,
    cod_supervisor  TEXT    NOT NULL,
    cod_rca         TEXT    NOT NULL,
    cnpj            TEXT    NOT NULL,
    cod_cli         TEXT    NOT NULL DEFAULT '',
    nome_cli        TEXT    NOT NULL DEFAULT '',
    base_cli        INT     NOT NULL DEFAULT 1,
    positivados     INT     NOT NULL DEFAULT 0,
    mix             NUMERIC NOT NULL DEFAULT 0,
    pvenda          NUMERIC NOT NULL DEFAULT 0,
    plucro          NUMERIC NOT NULL DEFAULT 0,
    qt              NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, cod_fornec, cod_gerente, cod_supervisor, cod_rca, cnpj)
) PARTITION BY RANGE (ano);

-- ─── V02 (Supervisor) — faturado ────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS farol.agg_fat_v02_l0_mes (
    empresa_id       UUID    NOT NULL,
    ano              INT     NOT NULL,
    mes              INT     NOT NULL,
    cod_supervisor   TEXT    NOT NULL,
    nome_supervisor  TEXT    NOT NULL DEFAULT '',
    base_cli         INT     NOT NULL DEFAULT 0,
    positivados      INT     NOT NULL DEFAULT 0,
    mix              NUMERIC NOT NULL DEFAULT 0,
    pvenda           NUMERIC NOT NULL DEFAULT 0,
    plucro           NUMERIC NOT NULL DEFAULT 0,
    qt               NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, cod_supervisor)
) PARTITION BY RANGE (ano);

CREATE TABLE IF NOT EXISTS farol.agg_fat_v02_l1_mes (
    empresa_id      UUID    NOT NULL,
    ano             INT     NOT NULL,
    mes             INT     NOT NULL,
    cod_supervisor  TEXT    NOT NULL,
    cod_rca         TEXT    NOT NULL,
    nome_rca        TEXT    NOT NULL DEFAULT '',
    base_cli        INT     NOT NULL DEFAULT 0,
    positivados     INT     NOT NULL DEFAULT 0,
    mix             NUMERIC NOT NULL DEFAULT 0,
    pvenda          NUMERIC NOT NULL DEFAULT 0,
    plucro          NUMERIC NOT NULL DEFAULT 0,
    qt              NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, cod_supervisor, cod_rca)
) PARTITION BY RANGE (ano);

CREATE TABLE IF NOT EXISTS farol.agg_fat_v02_l2_mes (
    empresa_id      UUID    NOT NULL,
    ano             INT     NOT NULL,
    mes             INT     NOT NULL,
    cod_supervisor  TEXT    NOT NULL,
    cod_rca         TEXT    NOT NULL,
    cod_fornec      TEXT    NOT NULL,
    nome_fornec     TEXT    NOT NULL DEFAULT '',
    base_cli        INT     NOT NULL DEFAULT 0,
    positivados     INT     NOT NULL DEFAULT 0,
    mix             NUMERIC NOT NULL DEFAULT 0,
    pvenda          NUMERIC NOT NULL DEFAULT 0,
    plucro          NUMERIC NOT NULL DEFAULT 0,
    qt              NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, cod_supervisor, cod_rca, cod_fornec)
) PARTITION BY RANGE (ano);

CREATE TABLE IF NOT EXISTS farol.agg_fat_v02_l3_mes (
    empresa_id      UUID    NOT NULL,
    ano             INT     NOT NULL,
    mes             INT     NOT NULL,
    cod_supervisor  TEXT    NOT NULL,
    cod_rca         TEXT    NOT NULL,
    cod_fornec      TEXT    NOT NULL,
    cnpj            TEXT    NOT NULL,
    cod_cli         TEXT    NOT NULL DEFAULT '',
    nome_cli        TEXT    NOT NULL DEFAULT '',
    base_cli        INT     NOT NULL DEFAULT 1,
    positivados     INT     NOT NULL DEFAULT 0,
    mix             NUMERIC NOT NULL DEFAULT 0,
    pvenda          NUMERIC NOT NULL DEFAULT 0,
    plucro          NUMERIC NOT NULL DEFAULT 0,
    qt              NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, cod_supervisor, cod_rca, cod_fornec, cnpj)
) PARTITION BY RANGE (ano);

-- ─── V03 (Gerente) — faturado ───────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS farol.agg_fat_v03_l0_mes (
    empresa_id    UUID    NOT NULL,
    ano           INT     NOT NULL,
    mes           INT     NOT NULL,
    cod_gerente   TEXT    NOT NULL,
    nome_gerente  TEXT    NOT NULL DEFAULT '',
    base_cli      INT     NOT NULL DEFAULT 0,
    positivados   INT     NOT NULL DEFAULT 0,
    mix           NUMERIC NOT NULL DEFAULT 0,
    pvenda        NUMERIC NOT NULL DEFAULT 0,
    plucro        NUMERIC NOT NULL DEFAULT 0,
    qt            NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, cod_gerente)
) PARTITION BY RANGE (ano);

CREATE TABLE IF NOT EXISTS farol.agg_fat_v03_l1_mes (
    empresa_id       UUID    NOT NULL,
    ano              INT     NOT NULL,
    mes              INT     NOT NULL,
    cod_gerente      TEXT    NOT NULL,
    cod_supervisor   TEXT    NOT NULL,
    nome_supervisor  TEXT    NOT NULL DEFAULT '',
    base_cli         INT     NOT NULL DEFAULT 0,
    positivados      INT     NOT NULL DEFAULT 0,
    mix              NUMERIC NOT NULL DEFAULT 0,
    pvenda           NUMERIC NOT NULL DEFAULT 0,
    plucro           NUMERIC NOT NULL DEFAULT 0,
    qt               NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, cod_gerente, cod_supervisor)
) PARTITION BY RANGE (ano);

CREATE TABLE IF NOT EXISTS farol.agg_fat_v03_l2_mes (
    empresa_id      UUID    NOT NULL,
    ano             INT     NOT NULL,
    mes             INT     NOT NULL,
    cod_gerente     TEXT    NOT NULL,
    cod_supervisor  TEXT    NOT NULL,
    cod_rca         TEXT    NOT NULL,
    nome_rca        TEXT    NOT NULL DEFAULT '',
    base_cli        INT     NOT NULL DEFAULT 0,
    positivados     INT     NOT NULL DEFAULT 0,
    mix             NUMERIC NOT NULL DEFAULT 0,
    pvenda          NUMERIC NOT NULL DEFAULT 0,
    plucro          NUMERIC NOT NULL DEFAULT 0,
    qt              NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, cod_gerente, cod_supervisor, cod_rca)
) PARTITION BY RANGE (ano);

CREATE TABLE IF NOT EXISTS farol.agg_fat_v03_l3_mes (
    empresa_id      UUID    NOT NULL,
    ano             INT     NOT NULL,
    mes             INT     NOT NULL,
    cod_gerente     TEXT    NOT NULL,
    cod_supervisor  TEXT    NOT NULL,
    cod_rca         TEXT    NOT NULL,
    cnpj            TEXT    NOT NULL,
    cod_cli         TEXT    NOT NULL DEFAULT '',
    nome_cli        TEXT    NOT NULL DEFAULT '',
    base_cli        INT     NOT NULL DEFAULT 1,
    positivados     INT     NOT NULL DEFAULT 0,
    mix             NUMERIC NOT NULL DEFAULT 0,
    pvenda          NUMERIC NOT NULL DEFAULT 0,
    plucro          NUMERIC NOT NULL DEFAULT 0,
    qt              NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, cod_gerente, cod_supervisor, cod_rca, cnpj)
) PARTITION BY RANGE (ano);

-- ─── V04 (RCA — persona campo) — faturado ───────────────────────────────────

CREATE TABLE IF NOT EXISTS farol.agg_fat_v04_l0_mes (
    empresa_id  UUID    NOT NULL,
    ano         INT     NOT NULL,
    mes         INT     NOT NULL,
    cod_rca     TEXT    NOT NULL,
    nome_rca    TEXT    NOT NULL DEFAULT '',
    base_cli    INT     NOT NULL DEFAULT 0,
    positivados INT     NOT NULL DEFAULT 0,
    mix         NUMERIC NOT NULL DEFAULT 0,
    pvenda      NUMERIC NOT NULL DEFAULT 0,
    plucro      NUMERIC NOT NULL DEFAULT 0,
    qt          NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, cod_rca)
) PARTITION BY RANGE (ano);

CREATE TABLE IF NOT EXISTS farol.agg_fat_v04_l1_mes (
    empresa_id  UUID    NOT NULL,
    ano         INT     NOT NULL,
    mes         INT     NOT NULL,
    cod_rca     TEXT    NOT NULL,
    cod_fornec  TEXT    NOT NULL,
    nome_fornec TEXT    NOT NULL DEFAULT '',
    base_cli    INT     NOT NULL DEFAULT 0,
    positivados INT     NOT NULL DEFAULT 0,
    mix         NUMERIC NOT NULL DEFAULT 0,
    pvenda      NUMERIC NOT NULL DEFAULT 0,
    plucro      NUMERIC NOT NULL DEFAULT 0,
    qt          NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, cod_rca, cod_fornec)
) PARTITION BY RANGE (ano);

CREATE TABLE IF NOT EXISTS farol.agg_fat_v04_l2_mes (
    empresa_id  UUID    NOT NULL,
    ano         INT     NOT NULL,
    mes         INT     NOT NULL,
    cod_rca     TEXT    NOT NULL,
    cod_fornec  TEXT    NOT NULL,
    cnpj        TEXT    NOT NULL,
    cod_cli     TEXT    NOT NULL DEFAULT '',
    nome_cli    TEXT    NOT NULL DEFAULT '',
    base_cli    INT     NOT NULL DEFAULT 1,
    positivados INT     NOT NULL DEFAULT 0,
    mix         NUMERIC NOT NULL DEFAULT 0,
    pvenda      NUMERIC NOT NULL DEFAULT 0,
    plucro      NUMERIC NOT NULL DEFAULT 0,
    qt          NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, cod_rca, cod_fornec, cnpj)
) PARTITION BY RANGE (ano);

-- ─── DIMS faturado (substitui os 7 GROUP BYs em mv_fat_cli) ─────────────────

CREATE TABLE IF NOT EXISTS farol.agg_fat_dims_mes (
    empresa_id UUID NOT NULL,
    ano        INT  NOT NULL,
    mes        INT  NOT NULL,
    dim        TEXT NOT NULL,  -- 'fornec','gerente','supervisor','rca','cli','uf','empresa'
    key        TEXT NOT NULL,
    label      TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (ano, empresa_id, mes, dim, key)
) PARTITION BY RANGE (ano);

-- ─── V01 (Fornecedor) — transmitido ─────────────────────────────────────────

CREATE TABLE IF NOT EXISTS farol.agg_trans_v01_l0_mes (LIKE farol.agg_fat_v01_l0_mes INCLUDING ALL) PARTITION BY RANGE (ano);
CREATE TABLE IF NOT EXISTS farol.agg_trans_v01_l1_mes (LIKE farol.agg_fat_v01_l1_mes INCLUDING ALL) PARTITION BY RANGE (ano);
CREATE TABLE IF NOT EXISTS farol.agg_trans_v01_l2_mes (LIKE farol.agg_fat_v01_l2_mes INCLUDING ALL) PARTITION BY RANGE (ano);
CREATE TABLE IF NOT EXISTS farol.agg_trans_v01_l3_mes (LIKE farol.agg_fat_v01_l3_mes INCLUDING ALL) PARTITION BY RANGE (ano);
CREATE TABLE IF NOT EXISTS farol.agg_trans_v01_l4_mes (LIKE farol.agg_fat_v01_l4_mes INCLUDING ALL) PARTITION BY RANGE (ano);

CREATE TABLE IF NOT EXISTS farol.agg_trans_v02_l0_mes (LIKE farol.agg_fat_v02_l0_mes INCLUDING ALL) PARTITION BY RANGE (ano);
CREATE TABLE IF NOT EXISTS farol.agg_trans_v02_l1_mes (LIKE farol.agg_fat_v02_l1_mes INCLUDING ALL) PARTITION BY RANGE (ano);
CREATE TABLE IF NOT EXISTS farol.agg_trans_v02_l2_mes (LIKE farol.agg_fat_v02_l2_mes INCLUDING ALL) PARTITION BY RANGE (ano);
CREATE TABLE IF NOT EXISTS farol.agg_trans_v02_l3_mes (LIKE farol.agg_fat_v02_l3_mes INCLUDING ALL) PARTITION BY RANGE (ano);

CREATE TABLE IF NOT EXISTS farol.agg_trans_v03_l0_mes (LIKE farol.agg_fat_v03_l0_mes INCLUDING ALL) PARTITION BY RANGE (ano);
CREATE TABLE IF NOT EXISTS farol.agg_trans_v03_l1_mes (LIKE farol.agg_fat_v03_l1_mes INCLUDING ALL) PARTITION BY RANGE (ano);
CREATE TABLE IF NOT EXISTS farol.agg_trans_v03_l2_mes (LIKE farol.agg_fat_v03_l2_mes INCLUDING ALL) PARTITION BY RANGE (ano);
CREATE TABLE IF NOT EXISTS farol.agg_trans_v03_l3_mes (LIKE farol.agg_fat_v03_l3_mes INCLUDING ALL) PARTITION BY RANGE (ano);

CREATE TABLE IF NOT EXISTS farol.agg_trans_v04_l0_mes (LIKE farol.agg_fat_v04_l0_mes INCLUDING ALL) PARTITION BY RANGE (ano);
CREATE TABLE IF NOT EXISTS farol.agg_trans_v04_l1_mes (LIKE farol.agg_fat_v04_l1_mes INCLUDING ALL) PARTITION BY RANGE (ano);
CREATE TABLE IF NOT EXISTS farol.agg_trans_v04_l2_mes (LIKE farol.agg_fat_v04_l2_mes INCLUDING ALL) PARTITION BY RANGE (ano);

CREATE TABLE IF NOT EXISTS farol.agg_trans_dims_mes (LIKE farol.agg_fat_dims_mes INCLUDING ALL) PARTITION BY RANGE (ano);

-- ════════════════════════════════════════════════════════════════════════════
-- PARTE 2 — Funções de manutenção (criar / dropar partições anuais)
-- ════════════════════════════════════════════════════════════════════════════
-- Lista canônica das 34 tabelas para iterar uniformemente.

CREATE OR REPLACE FUNCTION farol.agg_table_names() RETURNS TEXT[] AS $$
SELECT ARRAY[
    'agg_fat_v01_l0_mes','agg_fat_v01_l1_mes','agg_fat_v01_l2_mes','agg_fat_v01_l3_mes','agg_fat_v01_l4_mes',
    'agg_fat_v02_l0_mes','agg_fat_v02_l1_mes','agg_fat_v02_l2_mes','agg_fat_v02_l3_mes',
    'agg_fat_v03_l0_mes','agg_fat_v03_l1_mes','agg_fat_v03_l2_mes','agg_fat_v03_l3_mes',
    'agg_fat_v04_l0_mes','agg_fat_v04_l1_mes','agg_fat_v04_l2_mes',
    'agg_fat_dims_mes',
    'agg_trans_v01_l0_mes','agg_trans_v01_l1_mes','agg_trans_v01_l2_mes','agg_trans_v01_l3_mes','agg_trans_v01_l4_mes',
    'agg_trans_v02_l0_mes','agg_trans_v02_l1_mes','agg_trans_v02_l2_mes','agg_trans_v02_l3_mes',
    'agg_trans_v03_l0_mes','agg_trans_v03_l1_mes','agg_trans_v03_l2_mes','agg_trans_v03_l3_mes',
    'agg_trans_v04_l0_mes','agg_trans_v04_l1_mes','agg_trans_v04_l2_mes',
    'agg_trans_dims_mes'
];
$$ LANGUAGE sql IMMUTABLE;

CREATE OR REPLACE FUNCTION farol.create_agg_year_partitions(p_ano INT) RETURNS VOID AS $$
DECLARE
    tbl TEXT;
BEGIN
    FOREACH tbl IN ARRAY farol.agg_table_names() LOOP
        EXECUTE format(
            'CREATE TABLE IF NOT EXISTS farol.%I PARTITION OF farol.%I FOR VALUES FROM (%s) TO (%s)',
            tbl || '_' || p_ano, tbl, p_ano, p_ano + 1
        );
    END LOOP;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION farol.drop_agg_year_partitions(p_ano INT) RETURNS VOID AS $$
DECLARE
    tbl TEXT;
BEGIN
    FOREACH tbl IN ARRAY farol.agg_table_names() LOOP
        EXECUTE format('DROP TABLE IF EXISTS farol.%I', tbl || '_' || p_ano);
    END LOOP;
END;
$$ LANGUAGE plpgsql;

-- Provisiona partições para 2025, 2026 e 2027.
SELECT farol.create_agg_year_partitions(2025);
SELECT farol.create_agg_year_partitions(2026);
SELECT farol.create_agg_year_partitions(2027);

-- ════════════════════════════════════════════════════════════════════════════
-- PARTE 3 — Função orquestradora de UPSERT mensal
-- ════════════════════════════════════════════════════════════════════════════
-- farol.upsert_aggs_mes(empresa_id, ano, mes)
--   Lê do range (mês inteiro) das tabelas base vendas_faturadas/transmitidas
--   e faz INSERT … ON CONFLICT DO UPDATE em todas as 34 agregadas.
--
-- Convenção de fórmulas (reusadas via macros conceituais — código repete por
-- clareza de leitura/auditoria):
--
--   pvenda, plucro, qt → SUM
--   positivados        → COUNT(DISTINCT cnpj) FILTER (qt>0)
--   mix                → COUNT(DISTINCT (cnpj,cod_prod)) FILTER (qt>0 AND cod_prod<>'')
--                         / NULLIF(COUNT(DISTINCT cnpj) FILTER (qt>0), 0)
--   base_cli           → sub-SELECT em farol.mv_*_carteira_rca conforme o nível
--                        (regras na seção de cabeçalho)

CREATE OR REPLACE FUNCTION farol.upsert_aggs_mes(
    p_empresa_id UUID,
    p_ano        INT,
    p_mes        INT
) RETURNS VOID AS $$
DECLARE
    p_ini DATE := make_date(p_ano, p_mes, 1);
    p_fim DATE := (p_ini + INTERVAL '1 month' - INTERVAL '1 day')::date;
BEGIN
    -- ════════════════════ FATURADO ════════════════════════════════════════════

    -- V01_l0 (Fornec)
    INSERT INTO farol.agg_fat_v01_l0_mes AS t
        (empresa_id, ano, mes, cod_fornec, nome_fornec,
         base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_fornec,
        MAX(v.nome_fornec),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT
           FROM farol.mv_fat_carteira_rca c WHERE c.empresa_id = v.empresa_id),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(
            COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_faturadas v
    WHERE v.empresa_id = p_empresa_id
      AND v.data_faturamento BETWEEN p_ini AND p_fim
      AND v.cod_fornec <> ''
    GROUP BY v.empresa_id, v.cod_fornec
    ON CONFLICT (ano, empresa_id, mes, cod_fornec) DO UPDATE SET
        nome_fornec = EXCLUDED.nome_fornec, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V01_l1 (+Gerente)
    INSERT INTO farol.agg_fat_v01_l1_mes AS t
        (empresa_id, ano, mes, cod_fornec, cod_gerente, nome_gerente,
         base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_fornec, v.cod_gerente,
        MAX(v.nome_gerente),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT
           FROM farol.mv_fat_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_gerente = v.cod_gerente),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(
            COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_faturadas v
    WHERE v.empresa_id = p_empresa_id
      AND v.data_faturamento BETWEEN p_ini AND p_fim
      AND v.cod_fornec <> '' AND v.cod_gerente <> ''
    GROUP BY v.empresa_id, v.cod_fornec, v.cod_gerente
    ON CONFLICT (ano, empresa_id, mes, cod_fornec, cod_gerente) DO UPDATE SET
        nome_gerente = EXCLUDED.nome_gerente, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V01_l2 (+Supervisor)
    INSERT INTO farol.agg_fat_v01_l2_mes AS t
        (empresa_id, ano, mes, cod_fornec, cod_gerente, cod_supervisor, nome_supervisor,
         base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_fornec, v.cod_gerente, v.cod_supervisor,
        MAX(v.nome_supervisor),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT
           FROM farol.mv_fat_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_supervisor = v.cod_supervisor),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(
            COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_faturadas v
    WHERE v.empresa_id = p_empresa_id
      AND v.data_faturamento BETWEEN p_ini AND p_fim
      AND v.cod_fornec <> '' AND v.cod_gerente <> '' AND v.cod_supervisor <> ''
    GROUP BY v.empresa_id, v.cod_fornec, v.cod_gerente, v.cod_supervisor
    ON CONFLICT (ano, empresa_id, mes, cod_fornec, cod_gerente, cod_supervisor) DO UPDATE SET
        nome_supervisor = EXCLUDED.nome_supervisor, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V01_l3 (+RCA) — base_cli = qtcli_rca do RCA
    INSERT INTO farol.agg_fat_v01_l3_mes AS t
        (empresa_id, ano, mes, cod_fornec, cod_gerente, cod_supervisor, cod_rca, nome_rca,
         base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_fornec, v.cod_gerente, v.cod_supervisor, v.cod_rca,
        MAX(v.nome_rca),
        MAX(v.qtcli_rca)::INT,
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(
            COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_faturadas v
    WHERE v.empresa_id = p_empresa_id
      AND v.data_faturamento BETWEEN p_ini AND p_fim
      AND v.cod_fornec <> '' AND v.cod_gerente <> '' AND v.cod_supervisor <> '' AND v.cod_rca <> ''
    GROUP BY v.empresa_id, v.cod_fornec, v.cod_gerente, v.cod_supervisor, v.cod_rca
    ON CONFLICT (ano, empresa_id, mes, cod_fornec, cod_gerente, cod_supervisor, cod_rca) DO UPDATE SET
        nome_rca = EXCLUDED.nome_rca, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V01_l4 (+Cliente) — base_cli=1, positivados=0/1, mix = produtos distintos do cliente
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
    WHERE v.empresa_id = p_empresa_id
      AND v.data_faturamento BETWEEN p_ini AND p_fim
      AND v.cod_fornec <> '' AND v.cod_gerente <> '' AND v.cod_supervisor <> ''
      AND v.cod_rca <> '' AND v.cnpj <> ''
    GROUP BY v.empresa_id, v.cod_fornec, v.cod_gerente, v.cod_supervisor, v.cod_rca, v.cnpj
    ON CONFLICT (ano, empresa_id, mes, cod_fornec, cod_gerente, cod_supervisor, cod_rca, cnpj) DO UPDATE SET
        cod_cli = EXCLUDED.cod_cli, nome_cli = EXCLUDED.nome_cli,
        base_cli = EXCLUDED.base_cli, positivados = EXCLUDED.positivados,
        mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V02_l0 (Supv)
    INSERT INTO farol.agg_fat_v02_l0_mes AS t
        (empresa_id, ano, mes, cod_supervisor, nome_supervisor,
         base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_supervisor,
        MAX(v.nome_supervisor),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT
           FROM farol.mv_fat_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_supervisor = v.cod_supervisor),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(
            COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_faturadas v
    WHERE v.empresa_id = p_empresa_id
      AND v.data_faturamento BETWEEN p_ini AND p_fim
      AND v.cod_supervisor <> ''
    GROUP BY v.empresa_id, v.cod_supervisor
    ON CONFLICT (ano, empresa_id, mes, cod_supervisor) DO UPDATE SET
        nome_supervisor = EXCLUDED.nome_supervisor, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V02_l1 (+RCA) — base = qtcli_rca do RCA
    INSERT INTO farol.agg_fat_v02_l1_mes AS t
        (empresa_id, ano, mes, cod_supervisor, cod_rca, nome_rca,
         base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_supervisor, v.cod_rca,
        MAX(v.nome_rca),
        MAX(v.qtcli_rca)::INT,
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(
            COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_faturadas v
    WHERE v.empresa_id = p_empresa_id
      AND v.data_faturamento BETWEEN p_ini AND p_fim
      AND v.cod_supervisor <> '' AND v.cod_rca <> ''
    GROUP BY v.empresa_id, v.cod_supervisor, v.cod_rca
    ON CONFLICT (ano, empresa_id, mes, cod_supervisor, cod_rca) DO UPDATE SET
        nome_rca = EXCLUDED.nome_rca, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V02_l2 (+Fornec) — base = qtcli_rca do RCA (mesmo escopo)
    INSERT INTO farol.agg_fat_v02_l2_mes AS t
        (empresa_id, ano, mes, cod_supervisor, cod_rca, cod_fornec, nome_fornec,
         base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_supervisor, v.cod_rca, v.cod_fornec,
        MAX(v.nome_fornec),
        MAX(v.qtcli_rca)::INT,
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(
            COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_faturadas v
    WHERE v.empresa_id = p_empresa_id
      AND v.data_faturamento BETWEEN p_ini AND p_fim
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
        1,
        (CASE WHEN SUM(v.qt) > 0 THEN 1 ELSE 0 END)::INT,
        COUNT(DISTINCT v.cod_prod) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC,
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_faturadas v
    WHERE v.empresa_id = p_empresa_id
      AND v.data_faturamento BETWEEN p_ini AND p_fim
      AND v.cod_supervisor <> '' AND v.cod_rca <> '' AND v.cod_fornec <> '' AND v.cnpj <> ''
    GROUP BY v.empresa_id, v.cod_supervisor, v.cod_rca, v.cod_fornec, v.cnpj
    ON CONFLICT (ano, empresa_id, mes, cod_supervisor, cod_rca, cod_fornec, cnpj) DO UPDATE SET
        cod_cli = EXCLUDED.cod_cli, nome_cli = EXCLUDED.nome_cli,
        base_cli = EXCLUDED.base_cli, positivados = EXCLUDED.positivados,
        mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V03_l0 (Gerente)
    INSERT INTO farol.agg_fat_v03_l0_mes AS t
        (empresa_id, ano, mes, cod_gerente, nome_gerente,
         base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_gerente,
        MAX(v.nome_gerente),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT
           FROM farol.mv_fat_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_gerente = v.cod_gerente),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(
            COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_faturadas v
    WHERE v.empresa_id = p_empresa_id
      AND v.data_faturamento BETWEEN p_ini AND p_fim
      AND v.cod_gerente <> ''
    GROUP BY v.empresa_id, v.cod_gerente
    ON CONFLICT (ano, empresa_id, mes, cod_gerente) DO UPDATE SET
        nome_gerente = EXCLUDED.nome_gerente, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V03_l1 (+Supv)
    INSERT INTO farol.agg_fat_v03_l1_mes AS t
        (empresa_id, ano, mes, cod_gerente, cod_supervisor, nome_supervisor,
         base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_gerente, v.cod_supervisor,
        MAX(v.nome_supervisor),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT
           FROM farol.mv_fat_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_supervisor = v.cod_supervisor),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(
            COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_faturadas v
    WHERE v.empresa_id = p_empresa_id
      AND v.data_faturamento BETWEEN p_ini AND p_fim
      AND v.cod_gerente <> '' AND v.cod_supervisor <> ''
    GROUP BY v.empresa_id, v.cod_gerente, v.cod_supervisor
    ON CONFLICT (ano, empresa_id, mes, cod_gerente, cod_supervisor) DO UPDATE SET
        nome_supervisor = EXCLUDED.nome_supervisor, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V03_l2 (+RCA)
    INSERT INTO farol.agg_fat_v03_l2_mes AS t
        (empresa_id, ano, mes, cod_gerente, cod_supervisor, cod_rca, nome_rca,
         base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_gerente, v.cod_supervisor, v.cod_rca,
        MAX(v.nome_rca),
        MAX(v.qtcli_rca)::INT,
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(
            COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_faturadas v
    WHERE v.empresa_id = p_empresa_id
      AND v.data_faturamento BETWEEN p_ini AND p_fim
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
        1,
        (CASE WHEN SUM(v.qt) > 0 THEN 1 ELSE 0 END)::INT,
        COUNT(DISTINCT v.cod_prod) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC,
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_faturadas v
    WHERE v.empresa_id = p_empresa_id
      AND v.data_faturamento BETWEEN p_ini AND p_fim
      AND v.cod_gerente <> '' AND v.cod_supervisor <> '' AND v.cod_rca <> '' AND v.cnpj <> ''
    GROUP BY v.empresa_id, v.cod_gerente, v.cod_supervisor, v.cod_rca, v.cnpj
    ON CONFLICT (ano, empresa_id, mes, cod_gerente, cod_supervisor, cod_rca, cnpj) DO UPDATE SET
        cod_cli = EXCLUDED.cod_cli, nome_cli = EXCLUDED.nome_cli,
        base_cli = EXCLUDED.base_cli, positivados = EXCLUDED.positivados,
        mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V04_l0 (RCA root)
    INSERT INTO farol.agg_fat_v04_l0_mes AS t
        (empresa_id, ano, mes, cod_rca, nome_rca,
         base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_rca,
        MAX(v.nome_rca),
        MAX(v.qtcli_rca)::INT,
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(
            COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_faturadas v
    WHERE v.empresa_id = p_empresa_id
      AND v.data_faturamento BETWEEN p_ini AND p_fim
      AND v.cod_rca <> ''
    GROUP BY v.empresa_id, v.cod_rca
    ON CONFLICT (ano, empresa_id, mes, cod_rca) DO UPDATE SET
        nome_rca = EXCLUDED.nome_rca, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V04_l1 (+Fornec)
    INSERT INTO farol.agg_fat_v04_l1_mes AS t
        (empresa_id, ano, mes, cod_rca, cod_fornec, nome_fornec,
         base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_rca, v.cod_fornec,
        MAX(v.nome_fornec),
        MAX(v.qtcli_rca)::INT,
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(
            COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_faturadas v
    WHERE v.empresa_id = p_empresa_id
      AND v.data_faturamento BETWEEN p_ini AND p_fim
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
        1,
        (CASE WHEN SUM(v.qt) > 0 THEN 1 ELSE 0 END)::INT,
        COUNT(DISTINCT v.cod_prod) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC,
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_faturadas v
    WHERE v.empresa_id = p_empresa_id
      AND v.data_faturamento BETWEEN p_ini AND p_fim
      AND v.cod_rca <> '' AND v.cod_fornec <> '' AND v.cnpj <> ''
    GROUP BY v.empresa_id, v.cod_rca, v.cod_fornec, v.cnpj
    ON CONFLICT (ano, empresa_id, mes, cod_rca, cod_fornec, cnpj) DO UPDATE SET
        cod_cli = EXCLUDED.cod_cli, nome_cli = EXCLUDED.nome_cli,
        base_cli = EXCLUDED.base_cli, positivados = EXCLUDED.positivados,
        mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- DIMS faturado — 1 SELECT por dimensão via UNION ALL
    INSERT INTO farol.agg_fat_dims_mes AS t
        (empresa_id, ano, mes, dim, key, label)
    SELECT empresa_id, p_ano, p_mes, 'fornec', cod_fornec, COALESCE(MAX(nome_fornec),'')
        FROM vendas_faturadas
       WHERE empresa_id = p_empresa_id AND data_faturamento BETWEEN p_ini AND p_fim AND cod_fornec <> ''
       GROUP BY empresa_id, cod_fornec
    UNION ALL
    SELECT empresa_id, p_ano, p_mes, 'gerente', cod_gerente, COALESCE(MAX(nome_gerente),'')
        FROM vendas_faturadas
       WHERE empresa_id = p_empresa_id AND data_faturamento BETWEEN p_ini AND p_fim AND cod_gerente <> ''
       GROUP BY empresa_id, cod_gerente
    UNION ALL
    SELECT empresa_id, p_ano, p_mes, 'supervisor', cod_supervisor, COALESCE(MAX(nome_supervisor),'')
        FROM vendas_faturadas
       WHERE empresa_id = p_empresa_id AND data_faturamento BETWEEN p_ini AND p_fim AND cod_supervisor <> ''
       GROUP BY empresa_id, cod_supervisor
    UNION ALL
    SELECT empresa_id, p_ano, p_mes, 'rca', cod_rca, COALESCE(MAX(nome_rca),'')
        FROM vendas_faturadas
       WHERE empresa_id = p_empresa_id AND data_faturamento BETWEEN p_ini AND p_fim AND cod_rca <> ''
       GROUP BY empresa_id, cod_rca
    UNION ALL
    SELECT empresa_id, p_ano, p_mes, 'cli', cod_cli, COALESCE(MAX(nome_cli),'')
        FROM vendas_faturadas
       WHERE empresa_id = p_empresa_id AND data_faturamento BETWEEN p_ini AND p_fim AND cod_cli <> ''
       GROUP BY empresa_id, cod_cli
    UNION ALL
    SELECT empresa_id, p_ano, p_mes, 'uf', uf, uf
        FROM vendas_faturadas
       WHERE empresa_id = p_empresa_id AND data_faturamento BETWEEN p_ini AND p_fim AND uf <> ''
       GROUP BY empresa_id, uf
    UNION ALL
    SELECT empresa_id, p_ano, p_mes, 'empresa', empresa, empresa
        FROM vendas_faturadas
       WHERE empresa_id = p_empresa_id AND data_faturamento BETWEEN p_ini AND p_fim AND empresa <> ''
       GROUP BY empresa_id, empresa
    ON CONFLICT (ano, empresa_id, mes, dim, key) DO UPDATE SET label = EXCLUDED.label;

    -- ════════════════════ TRANSMITIDO ═════════════════════════════════════════
    -- Mesma estrutura, swap: vendas_transmitidas + data_transmissao + mv_trans_carteira_rca

    -- V01_l0 trans
    INSERT INTO farol.agg_trans_v01_l0_mes AS t
        (empresa_id, ano, mes, cod_fornec, nome_fornec,
         base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_fornec,
        MAX(v.nome_fornec),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT
           FROM farol.mv_trans_carteira_rca c WHERE c.empresa_id = v.empresa_id),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(
            COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_transmitidas v
    WHERE v.empresa_id = p_empresa_id
      AND v.data_transmissao BETWEEN p_ini AND p_fim
      AND v.cod_fornec <> ''
    GROUP BY v.empresa_id, v.cod_fornec
    ON CONFLICT (ano, empresa_id, mes, cod_fornec) DO UPDATE SET
        nome_fornec = EXCLUDED.nome_fornec, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V01_l1 trans
    INSERT INTO farol.agg_trans_v01_l1_mes AS t
        (empresa_id, ano, mes, cod_fornec, cod_gerente, nome_gerente,
         base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_fornec, v.cod_gerente,
        MAX(v.nome_gerente),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT
           FROM farol.mv_trans_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_gerente = v.cod_gerente),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(
            COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_transmitidas v
    WHERE v.empresa_id = p_empresa_id
      AND v.data_transmissao BETWEEN p_ini AND p_fim
      AND v.cod_fornec <> '' AND v.cod_gerente <> ''
    GROUP BY v.empresa_id, v.cod_fornec, v.cod_gerente
    ON CONFLICT (ano, empresa_id, mes, cod_fornec, cod_gerente) DO UPDATE SET
        nome_gerente = EXCLUDED.nome_gerente, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V01_l2 trans
    INSERT INTO farol.agg_trans_v01_l2_mes AS t
        (empresa_id, ano, mes, cod_fornec, cod_gerente, cod_supervisor, nome_supervisor,
         base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_fornec, v.cod_gerente, v.cod_supervisor,
        MAX(v.nome_supervisor),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT
           FROM farol.mv_trans_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_supervisor = v.cod_supervisor),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(
            COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_transmitidas v
    WHERE v.empresa_id = p_empresa_id
      AND v.data_transmissao BETWEEN p_ini AND p_fim
      AND v.cod_fornec <> '' AND v.cod_gerente <> '' AND v.cod_supervisor <> ''
    GROUP BY v.empresa_id, v.cod_fornec, v.cod_gerente, v.cod_supervisor
    ON CONFLICT (ano, empresa_id, mes, cod_fornec, cod_gerente, cod_supervisor) DO UPDATE SET
        nome_supervisor = EXCLUDED.nome_supervisor, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V01_l3 trans
    INSERT INTO farol.agg_trans_v01_l3_mes AS t
        (empresa_id, ano, mes, cod_fornec, cod_gerente, cod_supervisor, cod_rca, nome_rca,
         base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_fornec, v.cod_gerente, v.cod_supervisor, v.cod_rca,
        MAX(v.nome_rca),
        MAX(v.qtcli_rca)::INT,
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(
            COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_transmitidas v
    WHERE v.empresa_id = p_empresa_id
      AND v.data_transmissao BETWEEN p_ini AND p_fim
      AND v.cod_fornec <> '' AND v.cod_gerente <> '' AND v.cod_supervisor <> '' AND v.cod_rca <> ''
    GROUP BY v.empresa_id, v.cod_fornec, v.cod_gerente, v.cod_supervisor, v.cod_rca
    ON CONFLICT (ano, empresa_id, mes, cod_fornec, cod_gerente, cod_supervisor, cod_rca) DO UPDATE SET
        nome_rca = EXCLUDED.nome_rca, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V01_l4 trans
    INSERT INTO farol.agg_trans_v01_l4_mes AS t
        (empresa_id, ano, mes, cod_fornec, cod_gerente, cod_supervisor, cod_rca, cnpj, cod_cli, nome_cli,
         base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_fornec, v.cod_gerente, v.cod_supervisor, v.cod_rca,
        v.cnpj, MAX(v.cod_cli), MAX(v.nome_cli),
        1,
        (CASE WHEN SUM(v.qt) > 0 THEN 1 ELSE 0 END)::INT,
        COUNT(DISTINCT v.cod_prod) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC,
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_transmitidas v
    WHERE v.empresa_id = p_empresa_id
      AND v.data_transmissao BETWEEN p_ini AND p_fim
      AND v.cod_fornec <> '' AND v.cod_gerente <> '' AND v.cod_supervisor <> ''
      AND v.cod_rca <> '' AND v.cnpj <> ''
    GROUP BY v.empresa_id, v.cod_fornec, v.cod_gerente, v.cod_supervisor, v.cod_rca, v.cnpj
    ON CONFLICT (ano, empresa_id, mes, cod_fornec, cod_gerente, cod_supervisor, cod_rca, cnpj) DO UPDATE SET
        cod_cli = EXCLUDED.cod_cli, nome_cli = EXCLUDED.nome_cli,
        base_cli = EXCLUDED.base_cli, positivados = EXCLUDED.positivados,
        mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V02_l0 trans
    INSERT INTO farol.agg_trans_v02_l0_mes AS t
        (empresa_id, ano, mes, cod_supervisor, nome_supervisor,
         base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_supervisor,
        MAX(v.nome_supervisor),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT
           FROM farol.mv_trans_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_supervisor = v.cod_supervisor),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(
            COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_transmitidas v
    WHERE v.empresa_id = p_empresa_id
      AND v.data_transmissao BETWEEN p_ini AND p_fim
      AND v.cod_supervisor <> ''
    GROUP BY v.empresa_id, v.cod_supervisor
    ON CONFLICT (ano, empresa_id, mes, cod_supervisor) DO UPDATE SET
        nome_supervisor = EXCLUDED.nome_supervisor, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V02_l1 trans
    INSERT INTO farol.agg_trans_v02_l1_mes AS t
        (empresa_id, ano, mes, cod_supervisor, cod_rca, nome_rca,
         base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_supervisor, v.cod_rca,
        MAX(v.nome_rca),
        MAX(v.qtcli_rca)::INT,
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(
            COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_transmitidas v
    WHERE v.empresa_id = p_empresa_id
      AND v.data_transmissao BETWEEN p_ini AND p_fim
      AND v.cod_supervisor <> '' AND v.cod_rca <> ''
    GROUP BY v.empresa_id, v.cod_supervisor, v.cod_rca
    ON CONFLICT (ano, empresa_id, mes, cod_supervisor, cod_rca) DO UPDATE SET
        nome_rca = EXCLUDED.nome_rca, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V02_l2 trans
    INSERT INTO farol.agg_trans_v02_l2_mes AS t
        (empresa_id, ano, mes, cod_supervisor, cod_rca, cod_fornec, nome_fornec,
         base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_supervisor, v.cod_rca, v.cod_fornec,
        MAX(v.nome_fornec),
        MAX(v.qtcli_rca)::INT,
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(
            COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_transmitidas v
    WHERE v.empresa_id = p_empresa_id
      AND v.data_transmissao BETWEEN p_ini AND p_fim
      AND v.cod_supervisor <> '' AND v.cod_rca <> '' AND v.cod_fornec <> ''
    GROUP BY v.empresa_id, v.cod_supervisor, v.cod_rca, v.cod_fornec
    ON CONFLICT (ano, empresa_id, mes, cod_supervisor, cod_rca, cod_fornec) DO UPDATE SET
        nome_fornec = EXCLUDED.nome_fornec, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V02_l3 trans
    INSERT INTO farol.agg_trans_v02_l3_mes AS t
        (empresa_id, ano, mes, cod_supervisor, cod_rca, cod_fornec, cnpj, cod_cli, nome_cli,
         base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_supervisor, v.cod_rca, v.cod_fornec,
        v.cnpj, MAX(v.cod_cli), MAX(v.nome_cli),
        1,
        (CASE WHEN SUM(v.qt) > 0 THEN 1 ELSE 0 END)::INT,
        COUNT(DISTINCT v.cod_prod) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC,
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_transmitidas v
    WHERE v.empresa_id = p_empresa_id
      AND v.data_transmissao BETWEEN p_ini AND p_fim
      AND v.cod_supervisor <> '' AND v.cod_rca <> '' AND v.cod_fornec <> '' AND v.cnpj <> ''
    GROUP BY v.empresa_id, v.cod_supervisor, v.cod_rca, v.cod_fornec, v.cnpj
    ON CONFLICT (ano, empresa_id, mes, cod_supervisor, cod_rca, cod_fornec, cnpj) DO UPDATE SET
        cod_cli = EXCLUDED.cod_cli, nome_cli = EXCLUDED.nome_cli,
        base_cli = EXCLUDED.base_cli, positivados = EXCLUDED.positivados,
        mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V03_l0 trans
    INSERT INTO farol.agg_trans_v03_l0_mes AS t
        (empresa_id, ano, mes, cod_gerente, nome_gerente,
         base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_gerente,
        MAX(v.nome_gerente),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT
           FROM farol.mv_trans_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_gerente = v.cod_gerente),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(
            COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_transmitidas v
    WHERE v.empresa_id = p_empresa_id
      AND v.data_transmissao BETWEEN p_ini AND p_fim
      AND v.cod_gerente <> ''
    GROUP BY v.empresa_id, v.cod_gerente
    ON CONFLICT (ano, empresa_id, mes, cod_gerente) DO UPDATE SET
        nome_gerente = EXCLUDED.nome_gerente, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V03_l1 trans
    INSERT INTO farol.agg_trans_v03_l1_mes AS t
        (empresa_id, ano, mes, cod_gerente, cod_supervisor, nome_supervisor,
         base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_gerente, v.cod_supervisor,
        MAX(v.nome_supervisor),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT
           FROM farol.mv_trans_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_supervisor = v.cod_supervisor),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(
            COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_transmitidas v
    WHERE v.empresa_id = p_empresa_id
      AND v.data_transmissao BETWEEN p_ini AND p_fim
      AND v.cod_gerente <> '' AND v.cod_supervisor <> ''
    GROUP BY v.empresa_id, v.cod_gerente, v.cod_supervisor
    ON CONFLICT (ano, empresa_id, mes, cod_gerente, cod_supervisor) DO UPDATE SET
        nome_supervisor = EXCLUDED.nome_supervisor, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V03_l2 trans
    INSERT INTO farol.agg_trans_v03_l2_mes AS t
        (empresa_id, ano, mes, cod_gerente, cod_supervisor, cod_rca, nome_rca,
         base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_gerente, v.cod_supervisor, v.cod_rca,
        MAX(v.nome_rca),
        MAX(v.qtcli_rca)::INT,
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(
            COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_transmitidas v
    WHERE v.empresa_id = p_empresa_id
      AND v.data_transmissao BETWEEN p_ini AND p_fim
      AND v.cod_gerente <> '' AND v.cod_supervisor <> '' AND v.cod_rca <> ''
    GROUP BY v.empresa_id, v.cod_gerente, v.cod_supervisor, v.cod_rca
    ON CONFLICT (ano, empresa_id, mes, cod_gerente, cod_supervisor, cod_rca) DO UPDATE SET
        nome_rca = EXCLUDED.nome_rca, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V03_l3 trans
    INSERT INTO farol.agg_trans_v03_l3_mes AS t
        (empresa_id, ano, mes, cod_gerente, cod_supervisor, cod_rca, cnpj, cod_cli, nome_cli,
         base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_gerente, v.cod_supervisor, v.cod_rca,
        v.cnpj, MAX(v.cod_cli), MAX(v.nome_cli),
        1,
        (CASE WHEN SUM(v.qt) > 0 THEN 1 ELSE 0 END)::INT,
        COUNT(DISTINCT v.cod_prod) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC,
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_transmitidas v
    WHERE v.empresa_id = p_empresa_id
      AND v.data_transmissao BETWEEN p_ini AND p_fim
      AND v.cod_gerente <> '' AND v.cod_supervisor <> '' AND v.cod_rca <> '' AND v.cnpj <> ''
    GROUP BY v.empresa_id, v.cod_gerente, v.cod_supervisor, v.cod_rca, v.cnpj
    ON CONFLICT (ano, empresa_id, mes, cod_gerente, cod_supervisor, cod_rca, cnpj) DO UPDATE SET
        cod_cli = EXCLUDED.cod_cli, nome_cli = EXCLUDED.nome_cli,
        base_cli = EXCLUDED.base_cli, positivados = EXCLUDED.positivados,
        mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V04_l0 trans
    INSERT INTO farol.agg_trans_v04_l0_mes AS t
        (empresa_id, ano, mes, cod_rca, nome_rca,
         base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_rca,
        MAX(v.nome_rca),
        MAX(v.qtcli_rca)::INT,
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(
            COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_transmitidas v
    WHERE v.empresa_id = p_empresa_id
      AND v.data_transmissao BETWEEN p_ini AND p_fim
      AND v.cod_rca <> ''
    GROUP BY v.empresa_id, v.cod_rca
    ON CONFLICT (ano, empresa_id, mes, cod_rca) DO UPDATE SET
        nome_rca = EXCLUDED.nome_rca, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V04_l1 trans
    INSERT INTO farol.agg_trans_v04_l1_mes AS t
        (empresa_id, ano, mes, cod_rca, cod_fornec, nome_fornec,
         base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_rca, v.cod_fornec,
        MAX(v.nome_fornec),
        MAX(v.qtcli_rca)::INT,
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(
            COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_transmitidas v
    WHERE v.empresa_id = p_empresa_id
      AND v.data_transmissao BETWEEN p_ini AND p_fim
      AND v.cod_rca <> '' AND v.cod_fornec <> ''
    GROUP BY v.empresa_id, v.cod_rca, v.cod_fornec
    ON CONFLICT (ano, empresa_id, mes, cod_rca, cod_fornec) DO UPDATE SET
        nome_fornec = EXCLUDED.nome_fornec, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V04_l2 trans
    INSERT INTO farol.agg_trans_v04_l2_mes AS t
        (empresa_id, ano, mes, cod_rca, cod_fornec, cnpj, cod_cli, nome_cli,
         base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_rca, v.cod_fornec,
        v.cnpj, MAX(v.cod_cli), MAX(v.nome_cli),
        1,
        (CASE WHEN SUM(v.qt) > 0 THEN 1 ELSE 0 END)::INT,
        COUNT(DISTINCT v.cod_prod) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC,
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM vendas_transmitidas v
    WHERE v.empresa_id = p_empresa_id
      AND v.data_transmissao BETWEEN p_ini AND p_fim
      AND v.cod_rca <> '' AND v.cod_fornec <> '' AND v.cnpj <> ''
    GROUP BY v.empresa_id, v.cod_rca, v.cod_fornec, v.cnpj
    ON CONFLICT (ano, empresa_id, mes, cod_rca, cod_fornec, cnpj) DO UPDATE SET
        cod_cli = EXCLUDED.cod_cli, nome_cli = EXCLUDED.nome_cli,
        base_cli = EXCLUDED.base_cli, positivados = EXCLUDED.positivados,
        mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- DIMS transmitido
    INSERT INTO farol.agg_trans_dims_mes AS t
        (empresa_id, ano, mes, dim, key, label)
    SELECT empresa_id, p_ano, p_mes, 'fornec', cod_fornec, COALESCE(MAX(nome_fornec),'')
        FROM vendas_transmitidas
       WHERE empresa_id = p_empresa_id AND data_transmissao BETWEEN p_ini AND p_fim AND cod_fornec <> ''
       GROUP BY empresa_id, cod_fornec
    UNION ALL
    SELECT empresa_id, p_ano, p_mes, 'gerente', cod_gerente, COALESCE(MAX(nome_gerente),'')
        FROM vendas_transmitidas
       WHERE empresa_id = p_empresa_id AND data_transmissao BETWEEN p_ini AND p_fim AND cod_gerente <> ''
       GROUP BY empresa_id, cod_gerente
    UNION ALL
    SELECT empresa_id, p_ano, p_mes, 'supervisor', cod_supervisor, COALESCE(MAX(nome_supervisor),'')
        FROM vendas_transmitidas
       WHERE empresa_id = p_empresa_id AND data_transmissao BETWEEN p_ini AND p_fim AND cod_supervisor <> ''
       GROUP BY empresa_id, cod_supervisor
    UNION ALL
    SELECT empresa_id, p_ano, p_mes, 'rca', cod_rca, COALESCE(MAX(nome_rca),'')
        FROM vendas_transmitidas
       WHERE empresa_id = p_empresa_id AND data_transmissao BETWEEN p_ini AND p_fim AND cod_rca <> ''
       GROUP BY empresa_id, cod_rca
    UNION ALL
    SELECT empresa_id, p_ano, p_mes, 'cli', cod_cli, COALESCE(MAX(nome_cli),'')
        FROM vendas_transmitidas
       WHERE empresa_id = p_empresa_id AND data_transmissao BETWEEN p_ini AND p_fim AND cod_cli <> ''
       GROUP BY empresa_id, cod_cli
    UNION ALL
    SELECT empresa_id, p_ano, p_mes, 'uf', uf, uf
        FROM vendas_transmitidas
       WHERE empresa_id = p_empresa_id AND data_transmissao BETWEEN p_ini AND p_fim AND uf <> ''
       GROUP BY empresa_id, uf
    UNION ALL
    SELECT empresa_id, p_ano, p_mes, 'empresa', empresa, empresa
        FROM vendas_transmitidas
       WHERE empresa_id = p_empresa_id AND data_transmissao BETWEEN p_ini AND p_fim AND empresa <> ''
       GROUP BY empresa_id, empresa
    ON CONFLICT (ano, empresa_id, mes, dim, key) DO UPDATE SET label = EXCLUDED.label;

END;
$$ LANGUAGE plpgsql;

-- ════════════════════════════════════════════════════════════════════════════
-- PARTE 4 — ANALYZE inicial
-- ════════════════════════════════════════════════════════════════════════════
-- Estatísticas começam vazias; o planner precisa delas após qualquer carga real.
-- Após o primeiro upsert_aggs_mes, recomenda-se ANALYZE no schema farol.

ANALYZE farol.agg_fat_v01_l0_mes;
ANALYZE farol.agg_fat_v01_l1_mes;
ANALYZE farol.agg_fat_v01_l2_mes;
ANALYZE farol.agg_fat_v01_l3_mes;
ANALYZE farol.agg_fat_v01_l4_mes;
ANALYZE farol.agg_fat_v02_l0_mes;
ANALYZE farol.agg_fat_v02_l1_mes;
ANALYZE farol.agg_fat_v02_l2_mes;
ANALYZE farol.agg_fat_v02_l3_mes;
ANALYZE farol.agg_fat_v03_l0_mes;
ANALYZE farol.agg_fat_v03_l1_mes;
ANALYZE farol.agg_fat_v03_l2_mes;
ANALYZE farol.agg_fat_v03_l3_mes;
ANALYZE farol.agg_fat_v04_l0_mes;
ANALYZE farol.agg_fat_v04_l1_mes;
ANALYZE farol.agg_fat_v04_l2_mes;
ANALYZE farol.agg_fat_dims_mes;
ANALYZE farol.agg_trans_v01_l0_mes;
ANALYZE farol.agg_trans_v01_l1_mes;
ANALYZE farol.agg_trans_v01_l2_mes;
ANALYZE farol.agg_trans_v01_l3_mes;
ANALYZE farol.agg_trans_v01_l4_mes;
ANALYZE farol.agg_trans_v02_l0_mes;
ANALYZE farol.agg_trans_v02_l1_mes;
ANALYZE farol.agg_trans_v02_l2_mes;
ANALYZE farol.agg_trans_v02_l3_mes;
ANALYZE farol.agg_trans_v03_l0_mes;
ANALYZE farol.agg_trans_v03_l1_mes;
ANALYZE farol.agg_trans_v03_l2_mes;
ANALYZE farol.agg_trans_v03_l3_mes;
ANALYZE farol.agg_trans_v04_l0_mes;
ANALYZE farol.agg_trans_v04_l1_mes;
ANALYZE farol.agg_trans_v04_l2_mes;
ANALYZE farol.agg_trans_dims_mes;
