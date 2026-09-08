-- 228_agg_v02_v03_somente_industrias.sql
-- ════════════════════════════════════════════════════════════════════════════
-- Suporte a "Somente Indústrias" — botão novo no painel geral (planejado em
-- 08/09/2026, ver memória "somente_industrias_plano") que mostra Por Gerência
-- (V03) e Por Equipe (V02) considerando só a venda dos fornecedores
-- cadastrados como indústria (farol.industria_fornecedores), sem cair no
-- scan ao vivo que um filtro cod_fornec com dezenas de valores forçaria hoje
-- (ver fornecMultiValor em pickAggForCrossFilter, farol_v2_api.go).
--
-- POR QUE TABELA NOVA, NÃO SÓ FILTRO: V03 (Gerência) nunca teve cod_fornec no
-- grão — Gerente→Supervisor→RCA→Cliente soma TODOS os fornecedores desde a
-- origem (migration 165). Filtrar por 21 fornecedores ao mesmo tempo nessas
-- tabelas exigiria somar linhas pré-agregadas de fornecedores diferentes, o
-- que duplica cliente que compra de 2+ indústrias — o mesmo bug de
-- "Clientes Ativos" corrigido em 08/09/2026 (ver [[farol_kpi_totalizador_bugs]]).
-- A saída correta é filtrar NA ORIGEM (direto em vendas_faturadas/
-- vendas_transmitidas), uma vez por dia, igual toda tabela agg_*_mes já faz.
--
-- V02 (Equipe) só precisa de tabela nova pro L0 (Supervisor) e L1 (+RCA) —
-- L2/L3 (+Fornecedor[+Cliente]) JÁ têm cod_fornec no grão hoje
-- (agg_fat_v02_l2/l3_mes), então um filtro `cod_fornec = ANY(...)` ali é só
-- WHERE normal (linha por fornecedor, sem soma entre fornecedores diferentes)
-- — não precisa de tabela nova, o roteamento no Go injeta o filtro nesse
-- nível (ver farol_v2_api.go, hierarquias/aggTables "V02I").
--
-- Mesmas colunas das tabelas originais (agg_fat_v0X_lY_mes) — só o filtro de
-- origem muda. Populadas por farol.upsert_aggs_mes_ind, chamada na mesma
-- carga diária das demais (farol_v2_api.go).
-- ════════════════════════════════════════════════════════════════════════════

-- ─── V03 "Por Gerência", só indústria ──────────────────────────────────────────

-- Colunas liquido/pv_* — OBRIGATÓRIAS em toda tabela `agg_fat_*` (não
-- opcionais como mix_total): queryAggregatedMes (farol_v2_api.go) decide se
-- referencia essas colunas pelo PREFIXO do nome da tabela (agg_fat_* sempre
-- as espera; agg_trans_* nunca), não por uma allowlist — achado rodando o
-- teste de integração desta migration (erro real: "column v.liquido does
-- not exist"). Populadas pela PRÓPRIA upsert_aggs_mes_ind (não pela genérica
-- upsert_venda_liquida_cols — aquela soma TODOS os fornecedores por
-- gerente/supervisor/rca, quebraria o "só indústria" desta tabela).
--
-- ESCOPO REDUZIDO DE PROPÓSITO: pv_devol/pv_cancel ficam sempre 0 aqui — não
-- filtramos vendas_ccd por indústria nesta rodada (fora do pedido original,
-- que era só Faturado/Transmitido). Efeito: os toggles "+Devoluções"/
-- "+Canceladas" não têm efeito nenhum com "Somente Indústrias" ligado.

CREATE TABLE IF NOT EXISTS farol.agg_fat_v03_ind_l0_mes (
    empresa_id   UUID    NOT NULL,
    ano          INT     NOT NULL,
    mes          INT     NOT NULL,
    cod_gerente  TEXT    NOT NULL,
    nome_gerente TEXT    NOT NULL DEFAULT '',
    base_cli     INT     NOT NULL DEFAULT 0,
    positivados  INT     NOT NULL DEFAULT 0,
    mix          NUMERIC NOT NULL DEFAULT 0,
    pvenda       NUMERIC NOT NULL DEFAULT 0,
    plucro       NUMERIC NOT NULL DEFAULT 0,
    qt           NUMERIC NOT NULL DEFAULT 0,
    liquido      NUMERIC NOT NULL DEFAULT 0,
    pv_bonif     NUMERIC NOT NULL DEFAULT 0,
    pv_transf    NUMERIC NOT NULL DEFAULT 0,
    pv_remessa   NUMERIC NOT NULL DEFAULT 0,
    pv_devol     NUMERIC NOT NULL DEFAULT 0,
    pv_cancel    NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, cod_gerente)
) PARTITION BY RANGE (ano);

CREATE TABLE IF NOT EXISTS farol.agg_fat_v03_ind_l1_mes (
    empresa_id      UUID    NOT NULL,
    ano             INT     NOT NULL,
    mes             INT     NOT NULL,
    cod_gerente     TEXT    NOT NULL,
    cod_supervisor  TEXT    NOT NULL,
    nome_supervisor TEXT    NOT NULL DEFAULT '',
    base_cli        INT     NOT NULL DEFAULT 0,
    positivados     INT     NOT NULL DEFAULT 0,
    mix             NUMERIC NOT NULL DEFAULT 0,
    pvenda          NUMERIC NOT NULL DEFAULT 0,
    plucro          NUMERIC NOT NULL DEFAULT 0,
    qt              NUMERIC NOT NULL DEFAULT 0,
    liquido         NUMERIC NOT NULL DEFAULT 0,
    pv_bonif        NUMERIC NOT NULL DEFAULT 0,
    pv_transf       NUMERIC NOT NULL DEFAULT 0,
    pv_remessa      NUMERIC NOT NULL DEFAULT 0,
    pv_devol        NUMERIC NOT NULL DEFAULT 0,
    pv_cancel       NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, cod_gerente, cod_supervisor)
) PARTITION BY RANGE (ano);

CREATE TABLE IF NOT EXISTS farol.agg_fat_v03_ind_l2_mes (
    empresa_id     UUID    NOT NULL,
    ano            INT     NOT NULL,
    mes            INT     NOT NULL,
    cod_gerente    TEXT    NOT NULL,
    cod_supervisor TEXT    NOT NULL,
    cod_rca        TEXT    NOT NULL,
    nome_rca       TEXT    NOT NULL DEFAULT '',
    base_cli       INT     NOT NULL DEFAULT 0,
    positivados    INT     NOT NULL DEFAULT 0,
    mix            NUMERIC NOT NULL DEFAULT 0,
    pvenda         NUMERIC NOT NULL DEFAULT 0,
    plucro         NUMERIC NOT NULL DEFAULT 0,
    qt             NUMERIC NOT NULL DEFAULT 0,
    liquido        NUMERIC NOT NULL DEFAULT 0,
    pv_bonif       NUMERIC NOT NULL DEFAULT 0,
    pv_transf      NUMERIC NOT NULL DEFAULT 0,
    pv_remessa     NUMERIC NOT NULL DEFAULT 0,
    pv_devol       NUMERIC NOT NULL DEFAULT 0,
    pv_cancel      NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, cod_gerente, cod_supervisor, cod_rca)
) PARTITION BY RANGE (ano);

CREATE TABLE IF NOT EXISTS farol.agg_fat_v03_ind_l3_mes (
    empresa_id     UUID    NOT NULL,
    ano            INT     NOT NULL,
    mes            INT     NOT NULL,
    cod_gerente    TEXT    NOT NULL,
    cod_supervisor TEXT    NOT NULL,
    cod_rca        TEXT    NOT NULL,
    cnpj           TEXT    NOT NULL,
    cod_cli        TEXT    NOT NULL DEFAULT '',
    nome_cli       TEXT    NOT NULL DEFAULT '',
    base_cli       INT     NOT NULL DEFAULT 0,
    positivados    INT     NOT NULL DEFAULT 0,
    mix            NUMERIC NOT NULL DEFAULT 0,
    pvenda         NUMERIC NOT NULL DEFAULT 0,
    plucro         NUMERIC NOT NULL DEFAULT 0,
    qt             NUMERIC NOT NULL DEFAULT 0,
    liquido        NUMERIC NOT NULL DEFAULT 0,
    pv_bonif       NUMERIC NOT NULL DEFAULT 0,
    pv_transf      NUMERIC NOT NULL DEFAULT 0,
    pv_remessa     NUMERIC NOT NULL DEFAULT 0,
    pv_devol       NUMERIC NOT NULL DEFAULT 0,
    pv_cancel      NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, cod_gerente, cod_supervisor, cod_rca, cnpj)
) PARTITION BY RANGE (ano);

-- ─── V02 "Por Equipe", só indústria (L0/L1 — L2/L3 reaproveitam as originais) ──

CREATE TABLE IF NOT EXISTS farol.agg_fat_v02_ind_l0_mes (
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
    liquido         NUMERIC NOT NULL DEFAULT 0,
    pv_bonif        NUMERIC NOT NULL DEFAULT 0,
    pv_transf       NUMERIC NOT NULL DEFAULT 0,
    pv_remessa      NUMERIC NOT NULL DEFAULT 0,
    pv_devol        NUMERIC NOT NULL DEFAULT 0,
    pv_cancel       NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, cod_supervisor)
) PARTITION BY RANGE (ano);

CREATE TABLE IF NOT EXISTS farol.agg_fat_v02_ind_l1_mes (
    empresa_id     UUID    NOT NULL,
    ano            INT     NOT NULL,
    mes            INT     NOT NULL,
    cod_supervisor TEXT    NOT NULL,
    cod_rca        TEXT    NOT NULL,
    nome_rca       TEXT    NOT NULL DEFAULT '',
    base_cli       INT     NOT NULL DEFAULT 0,
    positivados    INT     NOT NULL DEFAULT 0,
    mix            NUMERIC NOT NULL DEFAULT 0,
    pvenda         NUMERIC NOT NULL DEFAULT 0,
    plucro         NUMERIC NOT NULL DEFAULT 0,
    qt             NUMERIC NOT NULL DEFAULT 0,
    liquido        NUMERIC NOT NULL DEFAULT 0,
    pv_bonif       NUMERIC NOT NULL DEFAULT 0,
    pv_transf      NUMERIC NOT NULL DEFAULT 0,
    pv_remessa     NUMERIC NOT NULL DEFAULT 0,
    pv_devol       NUMERIC NOT NULL DEFAULT 0,
    pv_cancel      NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, cod_supervisor, cod_rca)
) PARTITION BY RANGE (ano);

-- ─── Espelho Transmitido ────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS farol.agg_trans_v03_ind_l0_mes (LIKE farol.agg_fat_v03_ind_l0_mes INCLUDING ALL) PARTITION BY RANGE (ano);
CREATE TABLE IF NOT EXISTS farol.agg_trans_v03_ind_l1_mes (LIKE farol.agg_fat_v03_ind_l1_mes INCLUDING ALL) PARTITION BY RANGE (ano);
CREATE TABLE IF NOT EXISTS farol.agg_trans_v03_ind_l2_mes (LIKE farol.agg_fat_v03_ind_l2_mes INCLUDING ALL) PARTITION BY RANGE (ano);
CREATE TABLE IF NOT EXISTS farol.agg_trans_v03_ind_l3_mes (LIKE farol.agg_fat_v03_ind_l3_mes INCLUDING ALL) PARTITION BY RANGE (ano);
CREATE TABLE IF NOT EXISTS farol.agg_trans_v02_ind_l0_mes (LIKE farol.agg_fat_v02_ind_l0_mes INCLUDING ALL) PARTITION BY RANGE (ano);
CREATE TABLE IF NOT EXISTS farol.agg_trans_v02_ind_l1_mes (LIKE farol.agg_fat_v02_ind_l1_mes INCLUDING ALL) PARTITION BY RANGE (ano);

COMMENT ON TABLE farol.agg_fat_v03_ind_l0_mes IS 'Somente Indústrias — V03 L0 (Gerência), só fornecedores em farol.industria_fornecedores.';
COMMENT ON TABLE farol.agg_fat_v03_ind_l1_mes IS 'Somente Indústrias — V03 L1 (Gerência+Supervisor).';
COMMENT ON TABLE farol.agg_fat_v03_ind_l2_mes IS 'Somente Indústrias — V03 L2 (+RCA).';
COMMENT ON TABLE farol.agg_fat_v03_ind_l3_mes IS 'Somente Indústrias — V03 L3 (+Cliente).';
COMMENT ON TABLE farol.agg_fat_v02_ind_l0_mes IS 'Somente Indústrias — V02 L0 (Supervisor). L2/L3 reaproveitam agg_fat_v02_l2/l3_mes (já têm cod_fornec no grão) com filtro no momento da consulta.';
COMMENT ON TABLE farol.agg_fat_v02_ind_l1_mes IS 'Somente Indústrias — V02 L1 (+RCA).';

-- ─── Rotina de carga — mesmo padrão de upsert_aggs_mes_v06 (mig 185) ───────────
CREATE OR REPLACE FUNCTION farol.upsert_aggs_mes_ind(
    p_empresa_id UUID,
    p_ano        INT,
    p_mes        INT
) RETURNS VOID AS $$
DECLARE
    p_ini DATE := make_date(p_ano, p_mes, 1);
    p_fim DATE := (p_ini + INTERVAL '1 month' - INTERVAL '1 day')::date;
BEGIN
    SET LOCAL work_mem = '256MB';

    -- ─── FATURADO ────────────────────────────────────────────────────────────
    DROP TABLE IF EXISTS _vind_fat;
    CREATE TEMP TABLE _vind_fat ON COMMIT DROP AS
    SELECT
        v.empresa_id, v.cod_gerente, v.nome_gerente,
        v.cod_supervisor, v.nome_supervisor,
        v.cod_rca, v.nome_rca, v.qtcli_rca,
        v.cnpj, v.cod_cli, v.nome_cli, v.cod_prod,
        v.tipo_venda, v.pvenda, v.plucro, v.qt
    FROM vendas_faturadas v
    WHERE v.empresa_id = p_empresa_id
      AND v.data_faturamento BETWEEN p_ini AND p_fim
      AND v.cod_fornec IN (SELECT fi.cod_fornec FROM farol.industria_fornecedores fi WHERE fi.empresa_id = p_empresa_id);

    CREATE INDEX ON _vind_fat (cod_gerente);
    CREATE INDEX ON _vind_fat (cod_supervisor);
    CREATE INDEX ON _vind_fat (cod_rca);
    ANALYZE _vind_fat;

    -- V03_ind_l0 (Gerente)
    INSERT INTO farol.agg_fat_v03_ind_l0_mes AS t
        (empresa_id, ano, mes, cod_gerente, nome_gerente, base_cli, positivados, mix, pvenda, plucro, qt,
         liquido, pv_bonif, pv_transf, pv_remessa)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_gerente, MAX(v.nome_gerente),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_fat_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_gerente = v.cod_gerente),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt),
        COALESCE(SUM(v.pvenda) FILTER (WHERE v.tipo_venda IN ('1','4','7','8','9','11','14','20')), 0),
        COALESCE(SUM(v.pvenda) FILTER (WHERE v.tipo_venda = '5'), 0),
        COALESCE(SUM(v.pvenda) FILTER (WHERE v.tipo_venda = '10'), 0),
        COALESCE(SUM(v.pvenda) FILTER (WHERE v.tipo_venda = '13'), 0)
    FROM _vind_fat v
    WHERE v.cod_gerente <> ''
    GROUP BY v.empresa_id, v.cod_gerente
    ON CONFLICT (ano, empresa_id, mes, cod_gerente) DO UPDATE SET
        nome_gerente = EXCLUDED.nome_gerente, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt,
        liquido = EXCLUDED.liquido, pv_bonif = EXCLUDED.pv_bonif,
        pv_transf = EXCLUDED.pv_transf, pv_remessa = EXCLUDED.pv_remessa;

    -- V03_ind_l1 (+Supervisor)
    INSERT INTO farol.agg_fat_v03_ind_l1_mes AS t
        (empresa_id, ano, mes, cod_gerente, cod_supervisor, nome_supervisor, base_cli, positivados, mix, pvenda, plucro, qt,
         liquido, pv_bonif, pv_transf, pv_remessa)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_gerente, v.cod_supervisor, MAX(v.nome_supervisor),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_fat_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_supervisor = v.cod_supervisor),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt),
        COALESCE(SUM(v.pvenda) FILTER (WHERE v.tipo_venda IN ('1','4','7','8','9','11','14','20')), 0),
        COALESCE(SUM(v.pvenda) FILTER (WHERE v.tipo_venda = '5'), 0),
        COALESCE(SUM(v.pvenda) FILTER (WHERE v.tipo_venda = '10'), 0),
        COALESCE(SUM(v.pvenda) FILTER (WHERE v.tipo_venda = '13'), 0)
    FROM _vind_fat v
    WHERE v.cod_gerente <> '' AND v.cod_supervisor <> ''
    GROUP BY v.empresa_id, v.cod_gerente, v.cod_supervisor
    ON CONFLICT (ano, empresa_id, mes, cod_gerente, cod_supervisor) DO UPDATE SET
        nome_supervisor = EXCLUDED.nome_supervisor, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt,
        liquido = EXCLUDED.liquido, pv_bonif = EXCLUDED.pv_bonif,
        pv_transf = EXCLUDED.pv_transf, pv_remessa = EXCLUDED.pv_remessa;

    -- V03_ind_l2 (+RCA)
    INSERT INTO farol.agg_fat_v03_ind_l2_mes AS t
        (empresa_id, ano, mes, cod_gerente, cod_supervisor, cod_rca, nome_rca, base_cli, positivados, mix, pvenda, plucro, qt,
         liquido, pv_bonif, pv_transf, pv_remessa)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_gerente, v.cod_supervisor, v.cod_rca, MAX(v.nome_rca),
        MAX(v.qtcli_rca)::INT,
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt),
        COALESCE(SUM(v.pvenda) FILTER (WHERE v.tipo_venda IN ('1','4','7','8','9','11','14','20')), 0),
        COALESCE(SUM(v.pvenda) FILTER (WHERE v.tipo_venda = '5'), 0),
        COALESCE(SUM(v.pvenda) FILTER (WHERE v.tipo_venda = '10'), 0),
        COALESCE(SUM(v.pvenda) FILTER (WHERE v.tipo_venda = '13'), 0)
    FROM _vind_fat v
    WHERE v.cod_gerente <> '' AND v.cod_supervisor <> '' AND v.cod_rca <> ''
    GROUP BY v.empresa_id, v.cod_gerente, v.cod_supervisor, v.cod_rca
    ON CONFLICT (ano, empresa_id, mes, cod_gerente, cod_supervisor, cod_rca) DO UPDATE SET
        nome_rca = EXCLUDED.nome_rca, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt,
        liquido = EXCLUDED.liquido, pv_bonif = EXCLUDED.pv_bonif,
        pv_transf = EXCLUDED.pv_transf, pv_remessa = EXCLUDED.pv_remessa;

    -- V03_ind_l3 (+Cliente)
    INSERT INTO farol.agg_fat_v03_ind_l3_mes AS t
        (empresa_id, ano, mes, cod_gerente, cod_supervisor, cod_rca, cnpj, cod_cli, nome_cli,
         base_cli, positivados, mix, pvenda, plucro, qt,
         liquido, pv_bonif, pv_transf, pv_remessa)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_gerente, v.cod_supervisor, v.cod_rca,
        v.cnpj, MAX(v.cod_cli), MAX(v.nome_cli),
        1,
        (CASE WHEN SUM(v.qt) > 0 THEN 1 ELSE 0 END)::INT,
        COUNT(DISTINCT v.cod_prod) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC,
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt),
        COALESCE(SUM(v.pvenda) FILTER (WHERE v.tipo_venda IN ('1','4','7','8','9','11','14','20')), 0),
        COALESCE(SUM(v.pvenda) FILTER (WHERE v.tipo_venda = '5'), 0),
        COALESCE(SUM(v.pvenda) FILTER (WHERE v.tipo_venda = '10'), 0),
        COALESCE(SUM(v.pvenda) FILTER (WHERE v.tipo_venda = '13'), 0)
    FROM _vind_fat v
    WHERE v.cod_gerente <> '' AND v.cod_supervisor <> '' AND v.cod_rca <> '' AND v.cnpj <> ''
    GROUP BY v.empresa_id, v.cod_gerente, v.cod_supervisor, v.cod_rca, v.cnpj
    ON CONFLICT (ano, empresa_id, mes, cod_gerente, cod_supervisor, cod_rca, cnpj) DO UPDATE SET
        cod_cli = EXCLUDED.cod_cli, nome_cli = EXCLUDED.nome_cli,
        base_cli = EXCLUDED.base_cli, positivados = EXCLUDED.positivados,
        mix = EXCLUDED.mix, pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt,
        liquido = EXCLUDED.liquido, pv_bonif = EXCLUDED.pv_bonif,
        pv_transf = EXCLUDED.pv_transf, pv_remessa = EXCLUDED.pv_remessa;

    -- V02_ind_l0 (Supervisor)
    INSERT INTO farol.agg_fat_v02_ind_l0_mes AS t
        (empresa_id, ano, mes, cod_supervisor, nome_supervisor, base_cli, positivados, mix, pvenda, plucro, qt,
         liquido, pv_bonif, pv_transf, pv_remessa)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_supervisor, MAX(v.nome_supervisor),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_fat_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_supervisor = v.cod_supervisor),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt),
        COALESCE(SUM(v.pvenda) FILTER (WHERE v.tipo_venda IN ('1','4','7','8','9','11','14','20')), 0),
        COALESCE(SUM(v.pvenda) FILTER (WHERE v.tipo_venda = '5'), 0),
        COALESCE(SUM(v.pvenda) FILTER (WHERE v.tipo_venda = '10'), 0),
        COALESCE(SUM(v.pvenda) FILTER (WHERE v.tipo_venda = '13'), 0)
    FROM _vind_fat v
    WHERE v.cod_supervisor <> ''
    GROUP BY v.empresa_id, v.cod_supervisor
    ON CONFLICT (ano, empresa_id, mes, cod_supervisor) DO UPDATE SET
        nome_supervisor = EXCLUDED.nome_supervisor, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt,
        liquido = EXCLUDED.liquido, pv_bonif = EXCLUDED.pv_bonif,
        pv_transf = EXCLUDED.pv_transf, pv_remessa = EXCLUDED.pv_remessa;

    -- V02_ind_l1 (+RCA)
    INSERT INTO farol.agg_fat_v02_ind_l1_mes AS t
        (empresa_id, ano, mes, cod_supervisor, cod_rca, nome_rca, base_cli, positivados, mix, pvenda, plucro, qt,
         liquido, pv_bonif, pv_transf, pv_remessa)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_supervisor, v.cod_rca, MAX(v.nome_rca),
        MAX(v.qtcli_rca)::INT,
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt),
        COALESCE(SUM(v.pvenda) FILTER (WHERE v.tipo_venda IN ('1','4','7','8','9','11','14','20')), 0),
        COALESCE(SUM(v.pvenda) FILTER (WHERE v.tipo_venda = '5'), 0),
        COALESCE(SUM(v.pvenda) FILTER (WHERE v.tipo_venda = '10'), 0),
        COALESCE(SUM(v.pvenda) FILTER (WHERE v.tipo_venda = '13'), 0)
    FROM _vind_fat v
    WHERE v.cod_supervisor <> '' AND v.cod_rca <> ''
    GROUP BY v.empresa_id, v.cod_supervisor, v.cod_rca
    ON CONFLICT (ano, empresa_id, mes, cod_supervisor, cod_rca) DO UPDATE SET
        nome_rca = EXCLUDED.nome_rca, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt,
        liquido = EXCLUDED.liquido, pv_bonif = EXCLUDED.pv_bonif,
        pv_transf = EXCLUDED.pv_transf, pv_remessa = EXCLUDED.pv_remessa;

    -- ─── TRANSMITIDO ─────────────────────────────────────────────────────────
    DROP TABLE IF EXISTS _vind_trans;
    CREATE TEMP TABLE _vind_trans ON COMMIT DROP AS
    SELECT
        v.empresa_id, v.cod_gerente, v.nome_gerente,
        v.cod_supervisor, v.nome_supervisor,
        v.cod_rca, v.nome_rca, v.qtcli_rca,
        v.cnpj, v.cod_cli, v.nome_cli, v.cod_prod,
        v.pvenda, v.plucro, v.qt
    FROM vendas_transmitidas v
    WHERE v.empresa_id = p_empresa_id
      AND v.data_transmissao BETWEEN p_ini AND p_fim
      AND v.cod_fornec IN (SELECT fi.cod_fornec FROM farol.industria_fornecedores fi WHERE fi.empresa_id = p_empresa_id);

    CREATE INDEX ON _vind_trans (cod_gerente);
    CREATE INDEX ON _vind_trans (cod_supervisor);
    CREATE INDEX ON _vind_trans (cod_rca);
    ANALYZE _vind_trans;

    INSERT INTO farol.agg_trans_v03_ind_l0_mes AS t
        (empresa_id, ano, mes, cod_gerente, nome_gerente, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_gerente, MAX(v.nome_gerente),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_trans_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_gerente = v.cod_gerente),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _vind_trans v
    WHERE v.cod_gerente <> ''
    GROUP BY v.empresa_id, v.cod_gerente
    ON CONFLICT (ano, empresa_id, mes, cod_gerente) DO UPDATE SET
        nome_gerente = EXCLUDED.nome_gerente, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_trans_v03_ind_l1_mes AS t
        (empresa_id, ano, mes, cod_gerente, cod_supervisor, nome_supervisor, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_gerente, v.cod_supervisor, MAX(v.nome_supervisor),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_trans_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_supervisor = v.cod_supervisor),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _vind_trans v
    WHERE v.cod_gerente <> '' AND v.cod_supervisor <> ''
    GROUP BY v.empresa_id, v.cod_gerente, v.cod_supervisor
    ON CONFLICT (ano, empresa_id, mes, cod_gerente, cod_supervisor) DO UPDATE SET
        nome_supervisor = EXCLUDED.nome_supervisor, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_trans_v03_ind_l2_mes AS t
        (empresa_id, ano, mes, cod_gerente, cod_supervisor, cod_rca, nome_rca, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_gerente, v.cod_supervisor, v.cod_rca, MAX(v.nome_rca),
        MAX(v.qtcli_rca)::INT,
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _vind_trans v
    WHERE v.cod_gerente <> '' AND v.cod_supervisor <> '' AND v.cod_rca <> ''
    GROUP BY v.empresa_id, v.cod_gerente, v.cod_supervisor, v.cod_rca
    ON CONFLICT (ano, empresa_id, mes, cod_gerente, cod_supervisor, cod_rca) DO UPDATE SET
        nome_rca = EXCLUDED.nome_rca, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_trans_v03_ind_l3_mes AS t
        (empresa_id, ano, mes, cod_gerente, cod_supervisor, cod_rca, cnpj, cod_cli, nome_cli,
         base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_gerente, v.cod_supervisor, v.cod_rca,
        v.cnpj, MAX(v.cod_cli), MAX(v.nome_cli),
        1,
        (CASE WHEN SUM(v.qt) > 0 THEN 1 ELSE 0 END)::INT,
        COUNT(DISTINCT v.cod_prod) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC,
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _vind_trans v
    WHERE v.cod_gerente <> '' AND v.cod_supervisor <> '' AND v.cod_rca <> '' AND v.cnpj <> ''
    GROUP BY v.empresa_id, v.cod_gerente, v.cod_supervisor, v.cod_rca, v.cnpj
    ON CONFLICT (ano, empresa_id, mes, cod_gerente, cod_supervisor, cod_rca, cnpj) DO UPDATE SET
        cod_cli = EXCLUDED.cod_cli, nome_cli = EXCLUDED.nome_cli,
        base_cli = EXCLUDED.base_cli, positivados = EXCLUDED.positivados,
        mix = EXCLUDED.mix, pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_trans_v02_ind_l0_mes AS t
        (empresa_id, ano, mes, cod_supervisor, nome_supervisor, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_supervisor, MAX(v.nome_supervisor),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_trans_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_supervisor = v.cod_supervisor),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _vind_trans v
    WHERE v.cod_supervisor <> ''
    GROUP BY v.empresa_id, v.cod_supervisor
    ON CONFLICT (ano, empresa_id, mes, cod_supervisor) DO UPDATE SET
        nome_supervisor = EXCLUDED.nome_supervisor, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_trans_v02_ind_l1_mes AS t
        (empresa_id, ano, mes, cod_supervisor, cod_rca, nome_rca, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_supervisor, v.cod_rca, MAX(v.nome_rca),
        MAX(v.qtcli_rca)::INT,
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _vind_trans v
    WHERE v.cod_supervisor <> '' AND v.cod_rca <> ''
    GROUP BY v.empresa_id, v.cod_supervisor, v.cod_rca
    ON CONFLICT (ano, empresa_id, mes, cod_supervisor, cod_rca) DO UPDATE SET
        nome_rca = EXCLUDED.nome_rca, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

END;
$$ LANGUAGE plpgsql;

-- ─── Catálogo de tabelas (partição anual automática) ───────────────────────────
CREATE OR REPLACE FUNCTION farol.agg_table_names() RETURNS TEXT[] AS $$
BEGIN
    RETURN ARRAY[
        'agg_fat_v01_l0_mes','agg_fat_v01_l1_mes','agg_fat_v01_l2_mes','agg_fat_v01_l3_mes','agg_fat_v01_l4_mes',
        'agg_fat_v02_l0_mes','agg_fat_v02_l1_mes','agg_fat_v02_l2_mes','agg_fat_v02_l3_mes',
        'agg_fat_v03_l0_mes','agg_fat_v03_l1_mes','agg_fat_v03_l2_mes','agg_fat_v03_l3_mes',
        'agg_fat_v04_l0_mes','agg_fat_v04_l1_mes','agg_fat_v04_l2_mes',
        'agg_fat_v05_l0_mes','agg_fat_v05_l1_mes','agg_fat_v05_l2_mes','agg_fat_v05_l3_mes',
        'agg_fat_v06_l0_mes','agg_fat_v06_l1_mes','agg_fat_v06_l2_mes',
        'agg_fat_v07_l0_mes','agg_fat_v07_l1_mes','agg_fat_v07_l2_mes',
        'agg_fat_v08_l0_mes','agg_fat_v08_l1_mes','agg_fat_v08_l2_mes','agg_fat_v08_l3_mes',
        'agg_fat_v09_l1_mes','agg_fat_v09_l2_mes','agg_fat_v09_l3_mes','agg_fat_v09_l4_mes',
        'agg_fat_v10_l0_mes','agg_fat_v10_l1_mes','agg_fat_v10_l2_mes','agg_fat_v10_l3_mes',
        'agg_fat_v11_l1_mes','agg_fat_v11_l2_mes','agg_fat_v11_l3_mes','agg_fat_v11_l4_mes','agg_fat_v11_l5_mes',
        'agg_fat_dims_mes',
        'agg_fat_mkt_cli_mes','agg_fat_mkt_produto_mes',
        'agg_fat_v12_l1_mes',
        'agg_sazonalidade_produto_ano',
        -- Somente Indústrias (mig 228)
        'agg_fat_v03_ind_l0_mes','agg_fat_v03_ind_l1_mes','agg_fat_v03_ind_l2_mes','agg_fat_v03_ind_l3_mes',
        'agg_fat_v02_ind_l0_mes','agg_fat_v02_ind_l1_mes',
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
        'agg_trans_v11_l1_mes','agg_trans_v11_l2_mes','agg_trans_v11_l3_mes','agg_trans_v11_l4_mes','agg_trans_v11_l5_mes',
        'agg_trans_dims_mes',
        'agg_trans_mkt_cli_mes','agg_trans_mkt_produto_mes',
        'agg_trans_v03_ind_l0_mes','agg_trans_v03_ind_l1_mes','agg_trans_v03_ind_l2_mes','agg_trans_v03_ind_l3_mes',
        'agg_trans_v02_ind_l0_mes','agg_trans_v02_ind_l1_mes'
    ];
END;
$$ LANGUAGE plpgsql IMMUTABLE;

-- ─── Partições pros anos já existentes ─────────────────────────────────────────
DO $$
DECLARE r RECORD;
BEGIN
    FOR r IN
        SELECT DISTINCT EXTRACT(YEAR FROM data_faturamento)::int AS ano FROM vendas_faturadas
        UNION
        SELECT DISTINCT EXTRACT(YEAR FROM data_transmissao)::int AS ano FROM vendas_transmitidas
    LOOP
        PERFORM farol.create_agg_year_partitions(r.ano);
    END LOOP;
END $$;
