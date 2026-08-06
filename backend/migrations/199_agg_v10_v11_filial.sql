-- 199_agg_v10_v11_filial.sql
-- ════════════════════════════════════════════════════════════════════════════
-- AGGS COM FILIAL NO GRÃO — V10 e V11 (aceleração do filtro cruzado por filial)
--
-- CONTEXTO: a "filial" é a coluna `empresa` de vendas_* (valores 1, 11, 12, 13,
-- 15, 18, 20, 28, 32, 33 — os mesmos códigos da Rotina 1464 do WinThor). O nome
-- da coluna vem do layout do ION VENDAS e ficou infeliz; mantido por
-- compatibilidade de dados e de query string. Só o RÓTULO da UI vira "Filial".
--
-- PROBLEMA (idêntico ao que a mig 197 resolveu para UF): `empresa` não existe em
-- NENHUMA agg → aggServesFilters devolve false e todo filtro por filial cai no
-- scan de vendas_*. Isso custa dezenas de segundos E, pior, troca o significado
-- de dois números na tela: o valor exibido vira BRUTO (queryAggregatedVendas faz
-- liquido = valor, ~+11%) e o denominador da positivação vira rolling-12M em vez
-- da base normal. Um filtro que deveria só estreitar o recorte muda a métrica.
--
-- SOLUÇÃO: duas hierarquias novas, descobertas automaticamente pelo
-- pickAggForCrossFilter (mesmo mecanismo da 197) — nenhuma view nova na UI:
--
--   V10: FILIAL → Gerente → Supervisor → RCA
--   V11: FILIAL → Fornecedor → Gerente → Supervisor → RCA
--   l0 (só filial) é o MESMO grão nas duas → tabela única agg_*_v10_l0_mes.
--
-- ⚠ DIFERENÇA CRÍTICA EM RELAÇÃO À MIG 197 — LEIA ANTES DE MEXER:
--
-- A 197 pôde usar UF como grão porque "cada cliente pertence a UMA UF → somar
-- positivados entre UFs é exato". **ISSO NÃO VALE PARA FILIAL.** Filial é
-- atributo da TRANSAÇÃO, não do cliente: medido em 06/08/2026, de 37.719
-- clientes, **8.650 (23%) compram de 2+ filiais**. Somar positivados entre
-- filiais contaria o mesmo CNPJ duas vezes.
--
-- Por isso o backend só roteia para estas tabelas quando o filtro seleciona
-- UMA ÚNICA filial (aí o SUM é sobre um valor só → exato). Com 2+ filiais
-- selecionadas segue no scan de vendas_* — mais lento, porém correto. O guard
-- está em pickAggForCrossFilter (farol_v2_api.go); se alguém removê-lo, os
-- positivados passam a ser inflados silenciosamente.
--
-- Mesma razão vale para `mix` e `mix_total`.
--
-- base_cli = CARTEIRA do escopo organizacional (Rotina 302, IGNORANDO filial) —
-- mesmo precedente da 197 e do caminho rápido atual. A carteira não tem vínculo
-- com filial (qtcli_rca é um número por RCA, não há como fatiá-lo), e como 23%
-- dos clientes cruzam filiais, fatiar seria inventar dado. Fórmulas espelham V01:
--   l0/v11_l1 (sem org)  → SUM(qtcli_rca) da empresa (carteira total)
--   + gerente            → SUM(qtcli_rca) WHERE cod_gerente
--   + supervisor         → SUM(qtcli_rca) WHERE cod_supervisor
--   + rca                → MAX(qtcli_rca)
--
-- mix_total: calculado INLINE no upsert (mesmo padrão da 197).
--
-- Composição líquida (liquido/pv_*): via upsert_venda_liquida_cols (mig 190,
-- redefinida na 197), aqui redefinida de novo com `empresa` no temp e os 8
-- níveis fat novos SOMADOS aos 8 da 197 — as duas famílias convivem.
--
-- ⚠ BACKFILL: NÃO roda aqui (precedente mig 175/197 — bloquearia o startup e o
-- healthcheck do Coolify poderia derrubar o container em loop). As tabelas
-- nascem VAZIAS; o backend só roteia filtro de filial para elas depois que o
-- gate aggFilialReady detectar dados (até lá, segue no scan — comportamento
-- atual). Após o deploy, rodar UMA VEZ via psql, FORA da janela de import:
--
--   DO $b$ DECLARE r RECORD; BEGIN
--     FOR r IN SELECT DISTINCT empresa_id, ano, mes FROM farol.agg_fat_v01_l0_mes
--              ORDER BY ano, mes LOOP
--       RAISE NOTICE 'v10/v11 %-%', r.ano, r.mes;
--       PERFORM farol.upsert_aggs_mes_v10_v11(r.empresa_id, r.ano, r.mes);
--       PERFORM farol.upsert_venda_liquida_cols(r.empresa_id, r.ano, r.mes);
--     END LOOP;
--   END $b$;
--
-- Imports futuros populam automaticamente (upsertAggsMesParallel no Go chama
-- upsert_aggs_mes_v10_v11 junto com v06/v07 e v08/v09).
-- ════════════════════════════════════════════════════════════════════════════

-- ────────────────────────────────────────────────────────────────────────────
-- PARTE 1 — DDL das 16 tabelas (8 fat + 8 trans), particionadas por ano.
-- fat tem mix_total + colunas de composição (mig 175/189); trans só mix_total.
-- ────────────────────────────────────────────────────────────────────────────

-- l0 (só filial) — compartilhada por V10 e V11
CREATE TABLE IF NOT EXISTS farol.agg_fat_v10_l0_mes (
    empresa_id  UUID    NOT NULL,
    ano         INT     NOT NULL,
    mes         INT     NOT NULL,
    empresa          TEXT    NOT NULL,
    nome_empresa     TEXT    NOT NULL DEFAULT '',
    base_cli    INT     NOT NULL DEFAULT 0,
    positivados INT     NOT NULL DEFAULT 0,
    mix         NUMERIC NOT NULL DEFAULT 0,
    mix_total   INT     NOT NULL DEFAULT 0,
    pvenda      NUMERIC NOT NULL DEFAULT 0,
    plucro      NUMERIC NOT NULL DEFAULT 0,
    qt          NUMERIC NOT NULL DEFAULT 0,
    liquido     NUMERIC NOT NULL DEFAULT 0,
    pv_bonif    NUMERIC NOT NULL DEFAULT 0,
    pv_transf   NUMERIC NOT NULL DEFAULT 0,
    pv_remessa  NUMERIC NOT NULL DEFAULT 0,
    pv_devol    NUMERIC NOT NULL DEFAULT 0,
    pv_cancel   NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, empresa)
) PARTITION BY RANGE (ano);

CREATE TABLE IF NOT EXISTS farol.agg_fat_v10_l1_mes (
    empresa_id   UUID    NOT NULL,
    ano          INT     NOT NULL,
    mes          INT     NOT NULL,
    empresa      TEXT    NOT NULL,
    cod_gerente  TEXT    NOT NULL,
    nome_gerente TEXT    NOT NULL DEFAULT '',
    base_cli     INT     NOT NULL DEFAULT 0,
    positivados  INT     NOT NULL DEFAULT 0,
    mix          NUMERIC NOT NULL DEFAULT 0,
    mix_total    INT     NOT NULL DEFAULT 0,
    pvenda       NUMERIC NOT NULL DEFAULT 0,
    plucro       NUMERIC NOT NULL DEFAULT 0,
    qt           NUMERIC NOT NULL DEFAULT 0,
    liquido      NUMERIC NOT NULL DEFAULT 0,
    pv_bonif     NUMERIC NOT NULL DEFAULT 0,
    pv_transf    NUMERIC NOT NULL DEFAULT 0,
    pv_remessa   NUMERIC NOT NULL DEFAULT 0,
    pv_devol     NUMERIC NOT NULL DEFAULT 0,
    pv_cancel    NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, empresa, cod_gerente)
) PARTITION BY RANGE (ano);

CREATE TABLE IF NOT EXISTS farol.agg_fat_v10_l2_mes (
    empresa_id      UUID    NOT NULL,
    ano             INT     NOT NULL,
    mes             INT     NOT NULL,
    empresa         TEXT    NOT NULL,
    cod_gerente     TEXT    NOT NULL,
    cod_supervisor  TEXT    NOT NULL,
    nome_supervisor TEXT    NOT NULL DEFAULT '',
    base_cli        INT     NOT NULL DEFAULT 0,
    positivados     INT     NOT NULL DEFAULT 0,
    mix             NUMERIC NOT NULL DEFAULT 0,
    mix_total       INT     NOT NULL DEFAULT 0,
    pvenda          NUMERIC NOT NULL DEFAULT 0,
    plucro          NUMERIC NOT NULL DEFAULT 0,
    qt              NUMERIC NOT NULL DEFAULT 0,
    liquido         NUMERIC NOT NULL DEFAULT 0,
    pv_bonif        NUMERIC NOT NULL DEFAULT 0,
    pv_transf       NUMERIC NOT NULL DEFAULT 0,
    pv_remessa      NUMERIC NOT NULL DEFAULT 0,
    pv_devol        NUMERIC NOT NULL DEFAULT 0,
    pv_cancel       NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, empresa, cod_gerente, cod_supervisor)
) PARTITION BY RANGE (ano);

CREATE TABLE IF NOT EXISTS farol.agg_fat_v10_l3_mes (
    empresa_id     UUID    NOT NULL,
    ano            INT     NOT NULL,
    mes            INT     NOT NULL,
    empresa        TEXT    NOT NULL,
    cod_gerente    TEXT    NOT NULL,
    cod_supervisor TEXT    NOT NULL,
    cod_rca        TEXT    NOT NULL,
    nome_rca       TEXT    NOT NULL DEFAULT '',
    base_cli       INT     NOT NULL DEFAULT 0,
    positivados    INT     NOT NULL DEFAULT 0,
    mix            NUMERIC NOT NULL DEFAULT 0,
    mix_total      INT     NOT NULL DEFAULT 0,
    pvenda         NUMERIC NOT NULL DEFAULT 0,
    plucro         NUMERIC NOT NULL DEFAULT 0,
    qt             NUMERIC NOT NULL DEFAULT 0,
    liquido        NUMERIC NOT NULL DEFAULT 0,
    pv_bonif       NUMERIC NOT NULL DEFAULT 0,
    pv_transf      NUMERIC NOT NULL DEFAULT 0,
    pv_remessa     NUMERIC NOT NULL DEFAULT 0,
    pv_devol       NUMERIC NOT NULL DEFAULT 0,
    pv_cancel      NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, empresa, cod_gerente, cod_supervisor, cod_rca)
) PARTITION BY RANGE (ano);

CREATE TABLE IF NOT EXISTS farol.agg_fat_v11_l1_mes (
    empresa_id  UUID    NOT NULL,
    ano         INT     NOT NULL,
    mes         INT     NOT NULL,
    empresa     TEXT    NOT NULL,
    cod_fornec  TEXT    NOT NULL,
    nome_fornec TEXT    NOT NULL DEFAULT '',
    base_cli    INT     NOT NULL DEFAULT 0,
    positivados INT     NOT NULL DEFAULT 0,
    mix         NUMERIC NOT NULL DEFAULT 0,
    mix_total   INT     NOT NULL DEFAULT 0,
    pvenda      NUMERIC NOT NULL DEFAULT 0,
    plucro      NUMERIC NOT NULL DEFAULT 0,
    qt          NUMERIC NOT NULL DEFAULT 0,
    liquido     NUMERIC NOT NULL DEFAULT 0,
    pv_bonif    NUMERIC NOT NULL DEFAULT 0,
    pv_transf   NUMERIC NOT NULL DEFAULT 0,
    pv_remessa  NUMERIC NOT NULL DEFAULT 0,
    pv_devol    NUMERIC NOT NULL DEFAULT 0,
    pv_cancel   NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, empresa, cod_fornec)
) PARTITION BY RANGE (ano);

CREATE TABLE IF NOT EXISTS farol.agg_fat_v11_l2_mes (
    empresa_id   UUID    NOT NULL,
    ano          INT     NOT NULL,
    mes          INT     NOT NULL,
    empresa      TEXT    NOT NULL,
    cod_fornec   TEXT    NOT NULL,
    cod_gerente  TEXT    NOT NULL,
    nome_gerente TEXT    NOT NULL DEFAULT '',
    base_cli     INT     NOT NULL DEFAULT 0,
    positivados  INT     NOT NULL DEFAULT 0,
    mix          NUMERIC NOT NULL DEFAULT 0,
    mix_total    INT     NOT NULL DEFAULT 0,
    pvenda       NUMERIC NOT NULL DEFAULT 0,
    plucro       NUMERIC NOT NULL DEFAULT 0,
    qt           NUMERIC NOT NULL DEFAULT 0,
    liquido      NUMERIC NOT NULL DEFAULT 0,
    pv_bonif     NUMERIC NOT NULL DEFAULT 0,
    pv_transf    NUMERIC NOT NULL DEFAULT 0,
    pv_remessa   NUMERIC NOT NULL DEFAULT 0,
    pv_devol     NUMERIC NOT NULL DEFAULT 0,
    pv_cancel    NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, empresa, cod_fornec, cod_gerente)
) PARTITION BY RANGE (ano);

CREATE TABLE IF NOT EXISTS farol.agg_fat_v11_l3_mes (
    empresa_id      UUID    NOT NULL,
    ano             INT     NOT NULL,
    mes             INT     NOT NULL,
    empresa         TEXT    NOT NULL,
    cod_fornec      TEXT    NOT NULL,
    cod_gerente     TEXT    NOT NULL,
    cod_supervisor  TEXT    NOT NULL,
    nome_supervisor TEXT    NOT NULL DEFAULT '',
    base_cli        INT     NOT NULL DEFAULT 0,
    positivados     INT     NOT NULL DEFAULT 0,
    mix             NUMERIC NOT NULL DEFAULT 0,
    mix_total       INT     NOT NULL DEFAULT 0,
    pvenda          NUMERIC NOT NULL DEFAULT 0,
    plucro          NUMERIC NOT NULL DEFAULT 0,
    qt              NUMERIC NOT NULL DEFAULT 0,
    liquido         NUMERIC NOT NULL DEFAULT 0,
    pv_bonif        NUMERIC NOT NULL DEFAULT 0,
    pv_transf       NUMERIC NOT NULL DEFAULT 0,
    pv_remessa      NUMERIC NOT NULL DEFAULT 0,
    pv_devol        NUMERIC NOT NULL DEFAULT 0,
    pv_cancel       NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, empresa, cod_fornec, cod_gerente, cod_supervisor)
) PARTITION BY RANGE (ano);

CREATE TABLE IF NOT EXISTS farol.agg_fat_v11_l4_mes (
    empresa_id     UUID    NOT NULL,
    ano            INT     NOT NULL,
    mes            INT     NOT NULL,
    empresa        TEXT    NOT NULL,
    cod_fornec     TEXT    NOT NULL,
    cod_gerente    TEXT    NOT NULL,
    cod_supervisor TEXT    NOT NULL,
    cod_rca        TEXT    NOT NULL,
    nome_rca       TEXT    NOT NULL DEFAULT '',
    base_cli       INT     NOT NULL DEFAULT 0,
    positivados    INT     NOT NULL DEFAULT 0,
    mix            NUMERIC NOT NULL DEFAULT 0,
    mix_total      INT     NOT NULL DEFAULT 0,
    pvenda         NUMERIC NOT NULL DEFAULT 0,
    plucro         NUMERIC NOT NULL DEFAULT 0,
    qt             NUMERIC NOT NULL DEFAULT 0,
    liquido        NUMERIC NOT NULL DEFAULT 0,
    pv_bonif       NUMERIC NOT NULL DEFAULT 0,
    pv_transf      NUMERIC NOT NULL DEFAULT 0,
    pv_remessa     NUMERIC NOT NULL DEFAULT 0,
    pv_devol       NUMERIC NOT NULL DEFAULT 0,
    pv_cancel      NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, empresa, cod_fornec, cod_gerente, cod_supervisor, cod_rca)
) PARTITION BY RANGE (ano);

-- Equivalentes trans (sem colunas de composição — mig 189 é só fat)

CREATE TABLE IF NOT EXISTS farol.agg_trans_v10_l0_mes (
    empresa_id  UUID    NOT NULL,
    ano         INT     NOT NULL,
    mes         INT     NOT NULL,
    empresa          TEXT    NOT NULL,
    nome_empresa     TEXT    NOT NULL DEFAULT '',
    base_cli    INT     NOT NULL DEFAULT 0,
    positivados INT     NOT NULL DEFAULT 0,
    mix         NUMERIC NOT NULL DEFAULT 0,
    mix_total   INT     NOT NULL DEFAULT 0,
    pvenda      NUMERIC NOT NULL DEFAULT 0,
    plucro      NUMERIC NOT NULL DEFAULT 0,
    qt          NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, empresa)
) PARTITION BY RANGE (ano);

CREATE TABLE IF NOT EXISTS farol.agg_trans_v10_l1_mes (
    empresa_id   UUID    NOT NULL,
    ano          INT     NOT NULL,
    mes          INT     NOT NULL,
    empresa      TEXT    NOT NULL,
    cod_gerente  TEXT    NOT NULL,
    nome_gerente TEXT    NOT NULL DEFAULT '',
    base_cli     INT     NOT NULL DEFAULT 0,
    positivados  INT     NOT NULL DEFAULT 0,
    mix          NUMERIC NOT NULL DEFAULT 0,
    mix_total    INT     NOT NULL DEFAULT 0,
    pvenda       NUMERIC NOT NULL DEFAULT 0,
    plucro       NUMERIC NOT NULL DEFAULT 0,
    qt           NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, empresa, cod_gerente)
) PARTITION BY RANGE (ano);

CREATE TABLE IF NOT EXISTS farol.agg_trans_v10_l2_mes (
    empresa_id      UUID    NOT NULL,
    ano             INT     NOT NULL,
    mes             INT     NOT NULL,
    empresa         TEXT    NOT NULL,
    cod_gerente     TEXT    NOT NULL,
    cod_supervisor  TEXT    NOT NULL,
    nome_supervisor TEXT    NOT NULL DEFAULT '',
    base_cli        INT     NOT NULL DEFAULT 0,
    positivados     INT     NOT NULL DEFAULT 0,
    mix             NUMERIC NOT NULL DEFAULT 0,
    mix_total       INT     NOT NULL DEFAULT 0,
    pvenda          NUMERIC NOT NULL DEFAULT 0,
    plucro          NUMERIC NOT NULL DEFAULT 0,
    qt              NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, empresa, cod_gerente, cod_supervisor)
) PARTITION BY RANGE (ano);

CREATE TABLE IF NOT EXISTS farol.agg_trans_v10_l3_mes (
    empresa_id     UUID    NOT NULL,
    ano            INT     NOT NULL,
    mes            INT     NOT NULL,
    empresa        TEXT    NOT NULL,
    cod_gerente    TEXT    NOT NULL,
    cod_supervisor TEXT    NOT NULL,
    cod_rca        TEXT    NOT NULL,
    nome_rca       TEXT    NOT NULL DEFAULT '',
    base_cli       INT     NOT NULL DEFAULT 0,
    positivados    INT     NOT NULL DEFAULT 0,
    mix            NUMERIC NOT NULL DEFAULT 0,
    mix_total      INT     NOT NULL DEFAULT 0,
    pvenda         NUMERIC NOT NULL DEFAULT 0,
    plucro         NUMERIC NOT NULL DEFAULT 0,
    qt             NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, empresa, cod_gerente, cod_supervisor, cod_rca)
) PARTITION BY RANGE (ano);

CREATE TABLE IF NOT EXISTS farol.agg_trans_v11_l1_mes (
    empresa_id  UUID    NOT NULL,
    ano         INT     NOT NULL,
    mes         INT     NOT NULL,
    empresa     TEXT    NOT NULL,
    cod_fornec  TEXT    NOT NULL,
    nome_fornec TEXT    NOT NULL DEFAULT '',
    base_cli    INT     NOT NULL DEFAULT 0,
    positivados INT     NOT NULL DEFAULT 0,
    mix         NUMERIC NOT NULL DEFAULT 0,
    mix_total   INT     NOT NULL DEFAULT 0,
    pvenda      NUMERIC NOT NULL DEFAULT 0,
    plucro      NUMERIC NOT NULL DEFAULT 0,
    qt          NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, empresa, cod_fornec)
) PARTITION BY RANGE (ano);

CREATE TABLE IF NOT EXISTS farol.agg_trans_v11_l2_mes (
    empresa_id   UUID    NOT NULL,
    ano          INT     NOT NULL,
    mes          INT     NOT NULL,
    empresa      TEXT    NOT NULL,
    cod_fornec   TEXT    NOT NULL,
    cod_gerente  TEXT    NOT NULL,
    nome_gerente TEXT    NOT NULL DEFAULT '',
    base_cli     INT     NOT NULL DEFAULT 0,
    positivados  INT     NOT NULL DEFAULT 0,
    mix          NUMERIC NOT NULL DEFAULT 0,
    mix_total    INT     NOT NULL DEFAULT 0,
    pvenda       NUMERIC NOT NULL DEFAULT 0,
    plucro       NUMERIC NOT NULL DEFAULT 0,
    qt           NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, empresa, cod_fornec, cod_gerente)
) PARTITION BY RANGE (ano);

CREATE TABLE IF NOT EXISTS farol.agg_trans_v11_l3_mes (
    empresa_id      UUID    NOT NULL,
    ano             INT     NOT NULL,
    mes             INT     NOT NULL,
    empresa         TEXT    NOT NULL,
    cod_fornec      TEXT    NOT NULL,
    cod_gerente     TEXT    NOT NULL,
    cod_supervisor  TEXT    NOT NULL,
    nome_supervisor TEXT    NOT NULL DEFAULT '',
    base_cli        INT     NOT NULL DEFAULT 0,
    positivados     INT     NOT NULL DEFAULT 0,
    mix             NUMERIC NOT NULL DEFAULT 0,
    mix_total       INT     NOT NULL DEFAULT 0,
    pvenda          NUMERIC NOT NULL DEFAULT 0,
    plucro          NUMERIC NOT NULL DEFAULT 0,
    qt              NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, empresa, cod_fornec, cod_gerente, cod_supervisor)
) PARTITION BY RANGE (ano);

CREATE TABLE IF NOT EXISTS farol.agg_trans_v11_l4_mes (
    empresa_id     UUID    NOT NULL,
    ano            INT     NOT NULL,
    mes            INT     NOT NULL,
    empresa        TEXT    NOT NULL,
    cod_fornec     TEXT    NOT NULL,
    cod_gerente    TEXT    NOT NULL,
    cod_supervisor TEXT    NOT NULL,
    cod_rca        TEXT    NOT NULL,
    nome_rca       TEXT    NOT NULL DEFAULT '',
    base_cli       INT     NOT NULL DEFAULT 0,
    positivados    INT     NOT NULL DEFAULT 0,
    mix            NUMERIC NOT NULL DEFAULT 0,
    mix_total      INT     NOT NULL DEFAULT 0,
    pvenda         NUMERIC NOT NULL DEFAULT 0,
    plucro         NUMERIC NOT NULL DEFAULT 0,
    qt             NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, empresa, cod_fornec, cod_gerente, cod_supervisor, cod_rca)
) PARTITION BY RANGE (ano);

-- ────────────────────────────────────────────────────────────────────────────
-- PARTE 2 — agg_table_names() ganha as 16 novas (partições de anos futuros e
-- drop de retenção passam a cobri-las — mig 162/186).
-- ────────────────────────────────────────────────────────────────────────────

CREATE OR REPLACE FUNCTION farol.agg_table_names() RETURNS TEXT[] AS $$
BEGIN
    RETURN ARRAY[
        'agg_fat_v01_l0_mes','agg_fat_v01_l1_mes','agg_fat_v01_l2_mes','agg_fat_v01_l3_mes','agg_fat_v01_l4_mes',
        'agg_fat_v02_l0_mes','agg_fat_v02_l1_mes','agg_fat_v02_l2_mes','agg_fat_v02_l3_mes',
        'agg_fat_v03_l0_mes','agg_fat_v03_l1_mes','agg_fat_v03_l2_mes','agg_fat_v03_l3_mes',
        'agg_fat_v04_l0_mes','agg_fat_v04_l1_mes','agg_fat_v04_l2_mes',
        'agg_fat_v05_l0_mes','agg_fat_v05_l1_mes','agg_fat_v05_l2_mes','agg_fat_v05_l3_mes',
        -- V06/V07 (mig 183/184) — hierarquias Por Rede e Por Departamento
        'agg_fat_v06_l0_mes','agg_fat_v06_l1_mes','agg_fat_v06_l2_mes',
        'agg_fat_v07_l0_mes','agg_fat_v07_l1_mes','agg_fat_v07_l2_mes',
        -- V08/V09 (mig 197) — grão com UF
        'agg_fat_v08_l0_mes','agg_fat_v08_l1_mes','agg_fat_v08_l2_mes','agg_fat_v08_l3_mes',
        'agg_fat_v09_l1_mes','agg_fat_v09_l2_mes','agg_fat_v09_l3_mes','agg_fat_v09_l4_mes',
        -- V10/V11 (mig 199) — grão com FILIAL (coluna `empresa`)
        'agg_fat_v10_l0_mes','agg_fat_v10_l1_mes','agg_fat_v10_l2_mes','agg_fat_v10_l3_mes',
        'agg_fat_v11_l1_mes','agg_fat_v11_l2_mes','agg_fat_v11_l3_mes','agg_fat_v11_l4_mes',
        'agg_fat_dims_mes',
        'agg_fat_mkt_cli_mes','agg_fat_mkt_produto_mes',
        'agg_trans_v01_l0_mes','agg_trans_v01_l1_mes','agg_trans_v01_l2_mes','agg_trans_v01_l3_mes','agg_trans_v01_l4_mes',
        'agg_trans_v02_l0_mes','agg_trans_v02_l1_mes','agg_trans_v02_l2_mes','agg_trans_v02_l3_mes',
        'agg_trans_v03_l0_mes','agg_trans_v03_l1_mes','agg_trans_v03_l2_mes','agg_trans_v03_l3_mes',
        'agg_trans_v04_l0_mes','agg_trans_v04_l1_mes','agg_trans_v04_l2_mes',
        'agg_trans_v05_l0_mes','agg_trans_v05_l1_mes','agg_trans_v05_l2_mes','agg_trans_v05_l3_mes',
        'agg_trans_v06_l0_mes','agg_trans_v06_l1_mes','agg_trans_v06_l2_mes',
        'agg_trans_v07_l0_mes','agg_trans_v07_l1_mes','agg_trans_v07_l2_mes',
        'agg_trans_v08_l0_mes','agg_trans_v08_l1_mes','agg_trans_v08_l2_mes','agg_trans_v08_l3_mes',
        'agg_trans_v09_l1_mes','agg_trans_v09_l2_mes','agg_trans_v09_l3_mes','agg_trans_v09_l4_mes',
        'agg_trans_v10_l0_mes','agg_trans_v10_l1_mes','agg_trans_v10_l2_mes','agg_trans_v10_l3_mes',
        'agg_trans_v11_l1_mes','agg_trans_v11_l2_mes','agg_trans_v11_l3_mes','agg_trans_v11_l4_mes',
        'agg_trans_dims_mes',
        'agg_trans_mkt_cli_mes','agg_trans_mkt_produto_mes'
    ];
END;
$$ LANGUAGE plpgsql IMMUTABLE;

-- ────────────────────────────────────────────────────────────────────────────
-- PARTE 3 — Partições 2025-2027 das 16 novas (idempotente; anos existentes
-- das demais tabelas são NO-OP via IF NOT EXISTS).
-- ────────────────────────────────────────────────────────────────────────────

DO $$
DECLARE
    a INT;
BEGIN
    FOR a IN 2025..2027 LOOP
        PERFORM farol.create_agg_year_partitions(a);
    END LOOP;
END $$;

-- ────────────────────────────────────────────────────────────────────────────
-- PARTE 4 — farol.upsert_aggs_mes_v10_v11 (grão FILIAL)
-- Mesmo padrão da upsert_aggs_mes_v05 (mig 167): temp do mês + 8 INSERTs por
-- fluxo. mix_total inline (COUNT DISTINCT cod_prod). Chamada pelo Go
-- (upsertAggsMesParallel) após v06/v07 e antes de upsert_venda_liquida_cols.
-- ────────────────────────────────────────────────────────────────────────────

CREATE OR REPLACE FUNCTION farol.upsert_aggs_mes_v10_v11(
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
    DROP TABLE IF EXISTS _v1011_fat;
    CREATE TEMP TABLE _v1011_fat ON COMMIT DROP AS
    SELECT
        v.empresa_id, v.empresa,
        v.cod_gerente, v.nome_gerente,
        v.cod_supervisor, v.nome_supervisor,
        v.cod_rca, v.nome_rca, v.qtcli_rca,
        v.cod_fornec, v.nome_fornec,
        v.cnpj, v.cod_prod, v.pvenda, v.plucro, v.qt
    FROM vendas_faturadas v
    WHERE v.empresa_id = p_empresa_id
      AND v.data_faturamento BETWEEN p_ini AND p_fim
      AND v.empresa <> '';
    CREATE INDEX ON _v1011_fat (empresa);
    CREATE INDEX ON _v1011_fat (cod_gerente);
    CREATE INDEX ON _v1011_fat (cod_supervisor);
    CREATE INDEX ON _v1011_fat (cod_fornec);
    ANALYZE _v1011_fat;

    -- V10_l0 fat (empresa) — base = carteira total da empresa
    INSERT INTO farol.agg_fat_v10_l0_mes AS t
        (empresa_id, ano, mes, empresa, nome_empresa, base_cli, positivados, mix, mix_total, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.empresa, v.empresa,
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_fat_carteira_rca c
          WHERE c.empresa_id = v.empresa_id),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        COUNT(DISTINCT v.cod_prod) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::INT,
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v1011_fat v
    GROUP BY v.empresa_id, v.empresa
    ON CONFLICT (ano, empresa_id, mes, empresa) DO UPDATE SET
        nome_empresa = EXCLUDED.nome_empresa, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix, mix_total = EXCLUDED.mix_total,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V10_l1 fat (empresa + gerente) — base = carteira da gerência
    INSERT INTO farol.agg_fat_v10_l1_mes AS t
        (empresa_id, ano, mes, empresa, cod_gerente, nome_gerente, base_cli, positivados, mix, mix_total, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.empresa, v.cod_gerente, MAX(v.nome_gerente),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_fat_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_gerente = v.cod_gerente),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        COUNT(DISTINCT v.cod_prod) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::INT,
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v1011_fat v
    WHERE v.cod_gerente <> ''
    GROUP BY v.empresa_id, v.empresa, v.cod_gerente
    ON CONFLICT (ano, empresa_id, mes, empresa, cod_gerente) DO UPDATE SET
        nome_gerente = EXCLUDED.nome_gerente, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix, mix_total = EXCLUDED.mix_total,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V10_l2 fat (empresa + gerente + supervisor) — base = carteira do supervisor
    INSERT INTO farol.agg_fat_v10_l2_mes AS t
        (empresa_id, ano, mes, empresa, cod_gerente, cod_supervisor, nome_supervisor, base_cli, positivados, mix, mix_total, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.empresa, v.cod_gerente, v.cod_supervisor, MAX(v.nome_supervisor),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_fat_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_supervisor = v.cod_supervisor),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        COUNT(DISTINCT v.cod_prod) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::INT,
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v1011_fat v
    WHERE v.cod_gerente <> '' AND v.cod_supervisor <> ''
    GROUP BY v.empresa_id, v.empresa, v.cod_gerente, v.cod_supervisor
    ON CONFLICT (ano, empresa_id, mes, empresa, cod_gerente, cod_supervisor) DO UPDATE SET
        nome_supervisor = EXCLUDED.nome_supervisor, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix, mix_total = EXCLUDED.mix_total,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V10_l3 fat (empresa + gerente + supervisor + rca) — base = qtcli_rca do RCA
    INSERT INTO farol.agg_fat_v10_l3_mes AS t
        (empresa_id, ano, mes, empresa, cod_gerente, cod_supervisor, cod_rca, nome_rca, base_cli, positivados, mix, mix_total, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.empresa, v.cod_gerente, v.cod_supervisor, v.cod_rca, MAX(v.nome_rca),
        MAX(v.qtcli_rca)::INT,
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        COUNT(DISTINCT v.cod_prod) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::INT,
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v1011_fat v
    WHERE v.cod_gerente <> '' AND v.cod_supervisor <> '' AND v.cod_rca <> ''
    GROUP BY v.empresa_id, v.empresa, v.cod_gerente, v.cod_supervisor, v.cod_rca
    ON CONFLICT (ano, empresa_id, mes, empresa, cod_gerente, cod_supervisor, cod_rca) DO UPDATE SET
        nome_rca = EXCLUDED.nome_rca, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix, mix_total = EXCLUDED.mix_total,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V11_l1 fat (empresa + fornec) — base = carteira total (fornec não restringe carteira)
    INSERT INTO farol.agg_fat_v11_l1_mes AS t
        (empresa_id, ano, mes, empresa, cod_fornec, nome_fornec, base_cli, positivados, mix, mix_total, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.empresa, v.cod_fornec, MAX(v.nome_fornec),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_fat_carteira_rca c
          WHERE c.empresa_id = v.empresa_id),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        COUNT(DISTINCT v.cod_prod) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::INT,
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v1011_fat v
    WHERE v.cod_fornec <> ''
    GROUP BY v.empresa_id, v.empresa, v.cod_fornec
    ON CONFLICT (ano, empresa_id, mes, empresa, cod_fornec) DO UPDATE SET
        nome_fornec = EXCLUDED.nome_fornec, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix, mix_total = EXCLUDED.mix_total,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V11_l2 fat (empresa + fornec + gerente) — base = carteira da gerência
    INSERT INTO farol.agg_fat_v11_l2_mes AS t
        (empresa_id, ano, mes, empresa, cod_fornec, cod_gerente, nome_gerente, base_cli, positivados, mix, mix_total, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.empresa, v.cod_fornec, v.cod_gerente, MAX(v.nome_gerente),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_fat_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_gerente = v.cod_gerente),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        COUNT(DISTINCT v.cod_prod) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::INT,
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v1011_fat v
    WHERE v.cod_fornec <> '' AND v.cod_gerente <> ''
    GROUP BY v.empresa_id, v.empresa, v.cod_fornec, v.cod_gerente
    ON CONFLICT (ano, empresa_id, mes, empresa, cod_fornec, cod_gerente) DO UPDATE SET
        nome_gerente = EXCLUDED.nome_gerente, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix, mix_total = EXCLUDED.mix_total,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V11_l3 fat (empresa + fornec + gerente + supervisor) — base = carteira do supervisor
    INSERT INTO farol.agg_fat_v11_l3_mes AS t
        (empresa_id, ano, mes, empresa, cod_fornec, cod_gerente, cod_supervisor, nome_supervisor, base_cli, positivados, mix, mix_total, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.empresa, v.cod_fornec, v.cod_gerente, v.cod_supervisor, MAX(v.nome_supervisor),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_fat_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_supervisor = v.cod_supervisor),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        COUNT(DISTINCT v.cod_prod) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::INT,
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v1011_fat v
    WHERE v.cod_fornec <> '' AND v.cod_gerente <> '' AND v.cod_supervisor <> ''
    GROUP BY v.empresa_id, v.empresa, v.cod_fornec, v.cod_gerente, v.cod_supervisor
    ON CONFLICT (ano, empresa_id, mes, empresa, cod_fornec, cod_gerente, cod_supervisor) DO UPDATE SET
        nome_supervisor = EXCLUDED.nome_supervisor, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix, mix_total = EXCLUDED.mix_total,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V11_l4 fat (empresa + fornec + gerente + supervisor + rca) — base = qtcli_rca do RCA
    INSERT INTO farol.agg_fat_v11_l4_mes AS t
        (empresa_id, ano, mes, empresa, cod_fornec, cod_gerente, cod_supervisor, cod_rca, nome_rca, base_cli, positivados, mix, mix_total, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.empresa, v.cod_fornec, v.cod_gerente, v.cod_supervisor, v.cod_rca, MAX(v.nome_rca),
        MAX(v.qtcli_rca)::INT,
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        COUNT(DISTINCT v.cod_prod) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::INT,
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v1011_fat v
    WHERE v.cod_fornec <> '' AND v.cod_gerente <> '' AND v.cod_supervisor <> '' AND v.cod_rca <> ''
    GROUP BY v.empresa_id, v.empresa, v.cod_fornec, v.cod_gerente, v.cod_supervisor, v.cod_rca
    ON CONFLICT (ano, empresa_id, mes, empresa, cod_fornec, cod_gerente, cod_supervisor, cod_rca) DO UPDATE SET
        nome_rca = EXCLUDED.nome_rca, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix, mix_total = EXCLUDED.mix_total,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    DROP TABLE _v1011_fat;

    -- ════════════════════ TEMP TRANS ══════════════════════════════════════════
    DROP TABLE IF EXISTS _v1011_trans;
    CREATE TEMP TABLE _v1011_trans ON COMMIT DROP AS
    SELECT
        v.empresa_id, v.empresa,
        v.cod_gerente, v.nome_gerente,
        v.cod_supervisor, v.nome_supervisor,
        v.cod_rca, v.nome_rca, v.qtcli_rca,
        v.cod_fornec, v.nome_fornec,
        v.cnpj, v.cod_prod, v.pvenda, v.plucro, v.qt
    FROM vendas_transmitidas v
    WHERE v.empresa_id = p_empresa_id
      AND v.data_transmissao BETWEEN p_ini AND p_fim
      AND v.empresa <> '';
    CREATE INDEX ON _v1011_trans (empresa);
    CREATE INDEX ON _v1011_trans (cod_gerente);
    CREATE INDEX ON _v1011_trans (cod_supervisor);
    CREATE INDEX ON _v1011_trans (cod_fornec);
    ANALYZE _v1011_trans;

    INSERT INTO farol.agg_trans_v10_l0_mes AS t
        (empresa_id, ano, mes, empresa, nome_empresa, base_cli, positivados, mix, mix_total, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.empresa, v.empresa,
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_trans_carteira_rca c
          WHERE c.empresa_id = v.empresa_id),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        COUNT(DISTINCT v.cod_prod) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::INT,
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v1011_trans v
    GROUP BY v.empresa_id, v.empresa
    ON CONFLICT (ano, empresa_id, mes, empresa) DO UPDATE SET
        nome_empresa = EXCLUDED.nome_empresa, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix, mix_total = EXCLUDED.mix_total,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_trans_v10_l1_mes AS t
        (empresa_id, ano, mes, empresa, cod_gerente, nome_gerente, base_cli, positivados, mix, mix_total, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.empresa, v.cod_gerente, MAX(v.nome_gerente),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_trans_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_gerente = v.cod_gerente),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        COUNT(DISTINCT v.cod_prod) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::INT,
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v1011_trans v
    WHERE v.cod_gerente <> ''
    GROUP BY v.empresa_id, v.empresa, v.cod_gerente
    ON CONFLICT (ano, empresa_id, mes, empresa, cod_gerente) DO UPDATE SET
        nome_gerente = EXCLUDED.nome_gerente, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix, mix_total = EXCLUDED.mix_total,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_trans_v10_l2_mes AS t
        (empresa_id, ano, mes, empresa, cod_gerente, cod_supervisor, nome_supervisor, base_cli, positivados, mix, mix_total, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.empresa, v.cod_gerente, v.cod_supervisor, MAX(v.nome_supervisor),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_trans_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_supervisor = v.cod_supervisor),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        COUNT(DISTINCT v.cod_prod) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::INT,
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v1011_trans v
    WHERE v.cod_gerente <> '' AND v.cod_supervisor <> ''
    GROUP BY v.empresa_id, v.empresa, v.cod_gerente, v.cod_supervisor
    ON CONFLICT (ano, empresa_id, mes, empresa, cod_gerente, cod_supervisor) DO UPDATE SET
        nome_supervisor = EXCLUDED.nome_supervisor, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix, mix_total = EXCLUDED.mix_total,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_trans_v10_l3_mes AS t
        (empresa_id, ano, mes, empresa, cod_gerente, cod_supervisor, cod_rca, nome_rca, base_cli, positivados, mix, mix_total, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.empresa, v.cod_gerente, v.cod_supervisor, v.cod_rca, MAX(v.nome_rca),
        MAX(v.qtcli_rca)::INT,
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        COUNT(DISTINCT v.cod_prod) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::INT,
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v1011_trans v
    WHERE v.cod_gerente <> '' AND v.cod_supervisor <> '' AND v.cod_rca <> ''
    GROUP BY v.empresa_id, v.empresa, v.cod_gerente, v.cod_supervisor, v.cod_rca
    ON CONFLICT (ano, empresa_id, mes, empresa, cod_gerente, cod_supervisor, cod_rca) DO UPDATE SET
        nome_rca = EXCLUDED.nome_rca, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix, mix_total = EXCLUDED.mix_total,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_trans_v11_l1_mes AS t
        (empresa_id, ano, mes, empresa, cod_fornec, nome_fornec, base_cli, positivados, mix, mix_total, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.empresa, v.cod_fornec, MAX(v.nome_fornec),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_trans_carteira_rca c
          WHERE c.empresa_id = v.empresa_id),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        COUNT(DISTINCT v.cod_prod) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::INT,
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v1011_trans v
    WHERE v.cod_fornec <> ''
    GROUP BY v.empresa_id, v.empresa, v.cod_fornec
    ON CONFLICT (ano, empresa_id, mes, empresa, cod_fornec) DO UPDATE SET
        nome_fornec = EXCLUDED.nome_fornec, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix, mix_total = EXCLUDED.mix_total,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_trans_v11_l2_mes AS t
        (empresa_id, ano, mes, empresa, cod_fornec, cod_gerente, nome_gerente, base_cli, positivados, mix, mix_total, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.empresa, v.cod_fornec, v.cod_gerente, MAX(v.nome_gerente),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_trans_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_gerente = v.cod_gerente),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        COUNT(DISTINCT v.cod_prod) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::INT,
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v1011_trans v
    WHERE v.cod_fornec <> '' AND v.cod_gerente <> ''
    GROUP BY v.empresa_id, v.empresa, v.cod_fornec, v.cod_gerente
    ON CONFLICT (ano, empresa_id, mes, empresa, cod_fornec, cod_gerente) DO UPDATE SET
        nome_gerente = EXCLUDED.nome_gerente, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix, mix_total = EXCLUDED.mix_total,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_trans_v11_l3_mes AS t
        (empresa_id, ano, mes, empresa, cod_fornec, cod_gerente, cod_supervisor, nome_supervisor, base_cli, positivados, mix, mix_total, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.empresa, v.cod_fornec, v.cod_gerente, v.cod_supervisor, MAX(v.nome_supervisor),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_trans_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_supervisor = v.cod_supervisor),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        COUNT(DISTINCT v.cod_prod) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::INT,
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v1011_trans v
    WHERE v.cod_fornec <> '' AND v.cod_gerente <> '' AND v.cod_supervisor <> ''
    GROUP BY v.empresa_id, v.empresa, v.cod_fornec, v.cod_gerente, v.cod_supervisor
    ON CONFLICT (ano, empresa_id, mes, empresa, cod_fornec, cod_gerente, cod_supervisor) DO UPDATE SET
        nome_supervisor = EXCLUDED.nome_supervisor, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix, mix_total = EXCLUDED.mix_total,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_trans_v11_l4_mes AS t
        (empresa_id, ano, mes, empresa, cod_fornec, cod_gerente, cod_supervisor, cod_rca, nome_rca, base_cli, positivados, mix, mix_total, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.empresa, v.cod_fornec, v.cod_gerente, v.cod_supervisor, v.cod_rca, MAX(v.nome_rca),
        MAX(v.qtcli_rca)::INT,
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        COUNT(DISTINCT v.cod_prod) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::INT,
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v1011_trans v
    WHERE v.cod_fornec <> '' AND v.cod_gerente <> '' AND v.cod_supervisor <> '' AND v.cod_rca <> ''
    GROUP BY v.empresa_id, v.empresa, v.cod_fornec, v.cod_gerente, v.cod_supervisor, v.cod_rca
    ON CONFLICT (ano, empresa_id, mes, empresa, cod_fornec, cod_gerente, cod_supervisor, cod_rca) DO UPDATE SET
        nome_rca = EXCLUDED.nome_rca, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix, mix_total = EXCLUDED.mix_total,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    DROP TABLE _v1011_trans;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION farol.upsert_aggs_mes_v10_v11(UUID, INT, INT) IS 'Popula agg_fat/trans_v10_* e _v11_* (grão com FILIAL) para um empresa/ano/mes. Chamada pelo backend Go após v06/v07, antes de upsert_venda_liquida_cols.';

-- ────────────────────────────────────────────────────────────────────────────
-- PARTE 5 — upsert_venda_liquida_cols redefinida com `empresa` no temp e os 8
-- níveis fat de V10/V11 SOMADOS aos 8 de V08/V09 (mig 197). Corpo idêntico.
-- ────────────────────────────────────────────────────────────────────────────

CREATE OR REPLACE FUNCTION farol.upsert_venda_liquida_cols(
    p_empresa_id UUID,
    p_ano        INT,
    p_mes        INT
) RETURNS VOID AS $$
DECLARE
    p_ini     DATE := make_date(p_ano, p_mes, 1);
    p_fim     DATE := (p_ini + INTERVAL '1 month' - INTERVAL '1 day')::date;
    lvl       RECORD;
    k         TEXT;
    keycols   TEXT;
    wherecond TEXT;
    joincond  TEXT;
BEGIN
    SET LOCAL work_mem = '256MB';

    -- Temp unificado: faturado (evento='') + CCD (devol/cancel). tipo_venda só
    -- é relevante nas linhas faturadas; nas de CCD o discriminador é `evento`.
    -- empresa incluída (mig 197) para os níveis V10/V11 — vendas_ccd também tem empresa.
    DROP TABLE IF EXISTS _liq_all;
    CREATE TEMP TABLE _liq_all ON COMMIT DROP AS
    SELECT empresa_id, cod_fornec, cod_gerente, cod_supervisor, cod_rca, cnpj,
           cod_cliprinc, cod_depto, cod_sec, cod_categoria, uf, empresa,
           tipo_venda, ''::text AS evento, pvenda
      FROM vendas_faturadas
     WHERE empresa_id = p_empresa_id AND data_faturamento BETWEEN p_ini AND p_fim
    UNION ALL
    SELECT empresa_id, cod_fornec, cod_gerente, cod_supervisor, cod_rca, cnpj,
           cod_cliprinc, cod_depto, cod_sec, cod_categoria, uf, empresa,
           ''::text AS tipo_venda, evento, pvenda
      FROM vendas_ccd
     WHERE empresa_id = p_empresa_id AND data_evento BETWEEN p_ini AND p_fim
       AND evento IN ('DEVOLVIDO','CANCELADO');

    CREATE INDEX ON _liq_all (cod_fornec);
    CREATE INDEX ON _liq_all (cod_supervisor);
    CREATE INDEX ON _liq_all (cod_gerente);
    CREATE INDEX ON _liq_all (cod_rca);
    CREATE INDEX ON _liq_all (cod_cliprinc);
    CREATE INDEX ON _liq_all (cod_depto);
    CREATE INDEX ON _liq_all (uf);
    CREATE INDEX ON _liq_all (empresa);
    ANALYZE _liq_all;

    -- (tabela, chaves) de cada um dos níveis faturados.
    FOR lvl IN
        SELECT tbl, keys FROM (VALUES
            ('agg_fat_v01_l0_mes', ARRAY['cod_fornec']),
            ('agg_fat_v01_l1_mes', ARRAY['cod_fornec','cod_gerente']),
            ('agg_fat_v01_l2_mes', ARRAY['cod_fornec','cod_gerente','cod_supervisor']),
            ('agg_fat_v01_l3_mes', ARRAY['cod_fornec','cod_gerente','cod_supervisor','cod_rca']),
            ('agg_fat_v01_l4_mes', ARRAY['cod_fornec','cod_gerente','cod_supervisor','cod_rca','cnpj']),
            ('agg_fat_v02_l0_mes', ARRAY['cod_supervisor']),
            ('agg_fat_v02_l1_mes', ARRAY['cod_supervisor','cod_rca']),
            ('agg_fat_v02_l2_mes', ARRAY['cod_supervisor','cod_rca','cod_fornec']),
            ('agg_fat_v02_l3_mes', ARRAY['cod_supervisor','cod_rca','cod_fornec','cnpj']),
            ('agg_fat_v03_l0_mes', ARRAY['cod_gerente']),
            ('agg_fat_v03_l1_mes', ARRAY['cod_gerente','cod_supervisor']),
            ('agg_fat_v03_l2_mes', ARRAY['cod_gerente','cod_supervisor','cod_rca']),
            ('agg_fat_v03_l3_mes', ARRAY['cod_gerente','cod_supervisor','cod_rca','cnpj']),
            ('agg_fat_v04_l0_mes', ARRAY['cod_rca']),
            ('agg_fat_v04_l1_mes', ARRAY['cod_rca','cod_fornec']),
            ('agg_fat_v04_l2_mes', ARRAY['cod_rca','cod_fornec','cnpj']),
            ('agg_fat_v05_l0_mes', ARRAY['cod_supervisor']),
            ('agg_fat_v05_l1_mes', ARRAY['cod_supervisor','cod_fornec']),
            ('agg_fat_v05_l2_mes', ARRAY['cod_supervisor','cod_fornec','cod_rca']),
            ('agg_fat_v05_l3_mes', ARRAY['cod_supervisor','cod_fornec','cod_rca','cnpj']),
            ('agg_fat_v06_l0_mes', ARRAY['cod_cliprinc']),
            ('agg_fat_v06_l1_mes', ARRAY['cod_cliprinc','cod_fornec']),
            ('agg_fat_v06_l2_mes', ARRAY['cod_cliprinc','cod_fornec','cnpj']),
            ('agg_fat_v07_l0_mes', ARRAY['cod_depto']),
            ('agg_fat_v07_l1_mes', ARRAY['cod_depto','cod_sec']),
            ('agg_fat_v07_l2_mes', ARRAY['cod_depto','cod_sec','cod_categoria']),
            -- V08/V09 (mig 197) — grão com UF
            ('agg_fat_v08_l0_mes', ARRAY['uf']),
            ('agg_fat_v08_l1_mes', ARRAY['uf','cod_gerente']),
            ('agg_fat_v08_l2_mes', ARRAY['uf','cod_gerente','cod_supervisor']),
            ('agg_fat_v08_l3_mes', ARRAY['uf','cod_gerente','cod_supervisor','cod_rca']),
            ('agg_fat_v09_l1_mes', ARRAY['uf','cod_fornec']),
            ('agg_fat_v09_l2_mes', ARRAY['uf','cod_fornec','cod_gerente']),
            ('agg_fat_v09_l3_mes', ARRAY['uf','cod_fornec','cod_gerente','cod_supervisor']),
            ('agg_fat_v09_l4_mes', ARRAY['uf','cod_fornec','cod_gerente','cod_supervisor','cod_rca']),
            -- V10/V11 (mig 199) — grão com FILIAL (coluna `empresa`)
            ('agg_fat_v10_l0_mes', ARRAY['empresa']),
            ('agg_fat_v10_l1_mes', ARRAY['empresa','cod_gerente']),
            ('agg_fat_v10_l2_mes', ARRAY['empresa','cod_gerente','cod_supervisor']),
            ('agg_fat_v10_l3_mes', ARRAY['empresa','cod_gerente','cod_supervisor','cod_rca']),
            ('agg_fat_v11_l1_mes', ARRAY['empresa','cod_fornec']),
            ('agg_fat_v11_l2_mes', ARRAY['empresa','cod_fornec','cod_gerente']),
            ('agg_fat_v11_l3_mes', ARRAY['empresa','cod_fornec','cod_gerente','cod_supervisor']),
            ('agg_fat_v11_l4_mes', ARRAY['empresa','cod_fornec','cod_gerente','cod_supervisor','cod_rca'])
        ) AS t(tbl, keys)
    LOOP
        -- tabela ainda não existe nesse ambiente? pula.
        IF to_regclass('farol.' || lvl.tbl) IS NULL THEN
            CONTINUE;
        END IF;

        keycols := ''; wherecond := ''; joincond := '';
        FOREACH k IN ARRAY lvl.keys LOOP
            keycols   := keycols   || CASE WHEN keycols=''   THEN '' ELSE ', '   END || quote_ident(k);
            wherecond := wherecond || CASE WHEN wherecond='' THEN '' ELSE ' AND ' END || quote_ident(k) || ' <> ' || quote_literal('');
            joincond  := joincond  || CASE WHEN joincond=''  THEN '' ELSE ' AND ' END || 'a.' || quote_ident(k) || ' = s.' || quote_ident(k);
        END LOOP;

        EXECUTE format($f$
            UPDATE farol.%I a SET
                pv_bonif   = s.bonif,
                pv_transf  = s.transf,
                pv_remessa = s.remessa,
                pv_devol   = s.devol,
                pv_cancel  = s.cancel,
                liquido    = s.venda_real - s.devol - s.cancel
            FROM (
                SELECT %s,
                    COALESCE(SUM(pvenda) FILTER (WHERE evento = '' AND tipo_venda IN ('1','4','7','8','9','11','14','20')), 0) AS venda_real,
                    COALESCE(SUM(pvenda) FILTER (WHERE evento = '' AND tipo_venda = '5'),  0) AS bonif,
                    COALESCE(SUM(pvenda) FILTER (WHERE evento = '' AND tipo_venda = '10'), 0) AS transf,
                    COALESCE(SUM(pvenda) FILTER (WHERE evento = '' AND tipo_venda = '13'), 0) AS remessa,
                    COALESCE(SUM(pvenda) FILTER (WHERE evento = 'DEVOLVIDO'), 0) AS devol,
                    COALESCE(SUM(pvenda) FILTER (WHERE evento = 'CANCELADO'), 0) AS cancel
                FROM _liq_all
                WHERE %s
                GROUP BY %s
            ) s
            WHERE a.empresa_id = %L AND a.ano = %s AND a.mes = %s AND %s
        $f$, lvl.tbl, keycols, wherecond, keycols, p_empresa_id, p_ano, p_mes, joincond);
    END LOOP;
END;
$$ LANGUAGE plpgsql;

-- ────────────────────────────────────────────────────────────────────────────
-- PARTE 6 — ÍNDICES DE APOIO AO FALLBACK: **NÃO** são criados aqui.
--
-- A mig 196 dropou idx_vf_filial/idx_vt_filial quando o filtro saiu da UI. Com
-- o filtro de volta eles fazem falta de novo — mas só no caminho de FALLBACK
-- (2+ filiais selecionadas, que segue no scan de vendas_*); com uma filial o
-- V10/V11 resolve e o índice é irrelevante.
--
-- Não entram na migration porque migrations rodam no STARTUP (main.go:166) e
-- CREATE INDEX em ~22M linhas levaria minutos, travando a subida — o
-- healthcheck do Coolify pode derrubar o container em laço. Mesmo motivo pelo
-- qual o backfill também é manual.
--
-- Rodar UMA VEZ após o deploy, em sessão psql própria (CONCURRENTLY não pode
-- estar dentro de transação, e não bloqueia escrita):
--
--   CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_vf_filial
--       ON vendas_faturadas (empresa_id, empresa, data_faturamento)
--       WHERE empresa <> '';
--   CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_vt_filial
--       ON vendas_transmitidas (empresa_id, empresa, data_transmissao)
--       WHERE empresa <> '';
--
-- Ordem (empresa_id, empresa, data): igualdades primeiro, range por último —
-- a lição que a mig 196 aprendeu corrigindo a 195.
-- ────────────────────────────────────────────────────────────────────────────
