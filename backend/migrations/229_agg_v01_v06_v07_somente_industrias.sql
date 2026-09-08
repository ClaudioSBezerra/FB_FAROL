-- 229_agg_v01_v06_v07_somente_industrias.sql
-- ════════════════════════════════════════════════════════════════════════════
-- Completa o "Somente Indústrias" (mig 228) pras 3 visões que faltavam do
-- painel geral: Por FORN.GERAL (V01), Por Rede (V06) e Por Departamento
-- (V07). Contexto (08/09/2026): o projeto nasceu pra acompanhar só os
-- fornecedores cadastrados como indústria (o mobile já é assim, ver
-- FarolV2PublicCardsHandler/industriaMappedFornecs); "importar todos os
-- fornecedores" veio depois. Este ajuste deixa o painel geral no mesmo
-- caminho do mobile pras 5 visões principais (V01/V02/V03/V06/V07).
--
-- CUSTO BEM DIFERENTE por visão, por causa do que já existia:
--   V01 "Por FORN.GERAL" — GRÁTIS. cod_fornec é a RAIZ da hierarquia
--     (Fornecedor→Gerente→Supervisor→RCA→Cliente→Produto), está em TODO
--     nível já existente — só filtro, nenhuma tabela nova.
--   V06 "Por Rede" — quase grátis. Só o L0 (Rede) usa tabela pronta hoje
--     (agg_fat_v06_l0_mes); os níveis abaixo (Cliente/Fornecedor/Produto)
--     JÁ leem a base ao vivo desde 21/07/2026 (agg_fat_v06_l1/l2_mes viraram
--     "escreve mas não lê", ver mig 226) — um filtro ali é "grátis" porque
--     já é o caminho lento mesmo. Só precisa de 1 tabela nova (L0).
--   V07 "Por Departamento" — caro, igual V03: nunca teve fornecedor no
--     grão em nível nenhum (é taxonomia de produto, atravessa fornecedores)
--     — precisa de tabela nova em TODOS os 3 níveis.
--
-- Nenhuma das duas (V06/V07) tem base_cli/positivados/mix (decisão da Fase 2,
-- migrations 183/184) — só pvenda/plucro/qt + liquido/pv_* (mig 189/190).
-- ════════════════════════════════════════════════════════════════════════════

-- ─── V06 "Por Rede", só indústria (L0 — L1/L2 nem são mais lidos, ver mig 226) ──

CREATE TABLE IF NOT EXISTS farol.agg_fat_v06_ind_l0_mes (
    empresa_id    UUID    NOT NULL,
    ano           INT     NOT NULL,
    mes           INT     NOT NULL,
    cod_cliprinc  TEXT    NOT NULL,
    nome_cliprinc TEXT    NOT NULL DEFAULT '',
    pvenda        NUMERIC NOT NULL DEFAULT 0,
    plucro        NUMERIC NOT NULL DEFAULT 0,
    qt            NUMERIC NOT NULL DEFAULT 0,
    liquido       NUMERIC NOT NULL DEFAULT 0,
    pv_bonif      NUMERIC NOT NULL DEFAULT 0,
    pv_transf     NUMERIC NOT NULL DEFAULT 0,
    pv_remessa    NUMERIC NOT NULL DEFAULT 0,
    pv_devol      NUMERIC NOT NULL DEFAULT 0,
    pv_cancel     NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, cod_cliprinc)
) PARTITION BY RANGE (ano);

CREATE TABLE IF NOT EXISTS farol.agg_trans_v06_ind_l0_mes (LIKE farol.agg_fat_v06_ind_l0_mes INCLUDING ALL) PARTITION BY RANGE (ano);

-- ─── V07 "Por Departamento", só indústria (L0/L1/L2 — nenhum nível tinha fornecedor) ──

CREATE TABLE IF NOT EXISTS farol.agg_fat_v07_ind_l0_mes (
    empresa_id UUID    NOT NULL,
    ano        INT     NOT NULL,
    mes        INT     NOT NULL,
    cod_depto  TEXT    NOT NULL,
    depto      TEXT    NOT NULL DEFAULT '',
    pvenda     NUMERIC NOT NULL DEFAULT 0,
    plucro     NUMERIC NOT NULL DEFAULT 0,
    qt         NUMERIC NOT NULL DEFAULT 0,
    liquido    NUMERIC NOT NULL DEFAULT 0,
    pv_bonif   NUMERIC NOT NULL DEFAULT 0,
    pv_transf  NUMERIC NOT NULL DEFAULT 0,
    pv_remessa NUMERIC NOT NULL DEFAULT 0,
    pv_devol   NUMERIC NOT NULL DEFAULT 0,
    pv_cancel  NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, cod_depto)
) PARTITION BY RANGE (ano);

CREATE TABLE IF NOT EXISTS farol.agg_fat_v07_ind_l1_mes (
    empresa_id UUID    NOT NULL,
    ano        INT     NOT NULL,
    mes        INT     NOT NULL,
    cod_depto  TEXT    NOT NULL,
    cod_sec    TEXT    NOT NULL,
    secao      TEXT    NOT NULL DEFAULT '',
    pvenda     NUMERIC NOT NULL DEFAULT 0,
    plucro     NUMERIC NOT NULL DEFAULT 0,
    qt         NUMERIC NOT NULL DEFAULT 0,
    liquido    NUMERIC NOT NULL DEFAULT 0,
    pv_bonif   NUMERIC NOT NULL DEFAULT 0,
    pv_transf  NUMERIC NOT NULL DEFAULT 0,
    pv_remessa NUMERIC NOT NULL DEFAULT 0,
    pv_devol   NUMERIC NOT NULL DEFAULT 0,
    pv_cancel  NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, cod_depto, cod_sec)
) PARTITION BY RANGE (ano);

CREATE TABLE IF NOT EXISTS farol.agg_fat_v07_ind_l2_mes (
    empresa_id    UUID    NOT NULL,
    ano           INT     NOT NULL,
    mes           INT     NOT NULL,
    cod_depto     TEXT    NOT NULL,
    cod_sec       TEXT    NOT NULL,
    cod_categoria TEXT    NOT NULL,
    categoria     TEXT    NOT NULL DEFAULT '',
    pvenda        NUMERIC NOT NULL DEFAULT 0,
    plucro        NUMERIC NOT NULL DEFAULT 0,
    qt            NUMERIC NOT NULL DEFAULT 0,
    liquido       NUMERIC NOT NULL DEFAULT 0,
    pv_bonif      NUMERIC NOT NULL DEFAULT 0,
    pv_transf     NUMERIC NOT NULL DEFAULT 0,
    pv_remessa    NUMERIC NOT NULL DEFAULT 0,
    pv_devol      NUMERIC NOT NULL DEFAULT 0,
    pv_cancel     NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, cod_depto, cod_sec, cod_categoria)
) PARTITION BY RANGE (ano);

CREATE TABLE IF NOT EXISTS farol.agg_trans_v07_ind_l0_mes (LIKE farol.agg_fat_v07_ind_l0_mes INCLUDING ALL) PARTITION BY RANGE (ano);
CREATE TABLE IF NOT EXISTS farol.agg_trans_v07_ind_l1_mes (LIKE farol.agg_fat_v07_ind_l1_mes INCLUDING ALL) PARTITION BY RANGE (ano);
CREATE TABLE IF NOT EXISTS farol.agg_trans_v07_ind_l2_mes (LIKE farol.agg_fat_v07_ind_l2_mes INCLUDING ALL) PARTITION BY RANGE (ano);

COMMENT ON TABLE farol.agg_fat_v06_ind_l0_mes IS 'Somente Indústrias — V06 L0 (Rede). L1/L2 não têm versão _ind: já leem a base ao vivo desde a mig 226, o filtro entra direto em `filters`.';
COMMENT ON TABLE farol.agg_fat_v07_ind_l0_mes IS 'Somente Indústrias — V07 L0 (Departamento).';
COMMENT ON TABLE farol.agg_fat_v07_ind_l1_mes IS 'Somente Indústrias — V07 L1 (+Seção).';
COMMENT ON TABLE farol.agg_fat_v07_ind_l2_mes IS 'Somente Indústrias — V07 L2 (+Categoria).';

-- ─── Rotina de carga — estende upsert_aggs_mes_ind (mig 228) ───────────────────
-- Reaproveita os MESMOS _vind_fat/_vind_trans da mig 228, só que agora
-- também trazendo as colunas de Rede (cod_cliprinc/fantasia) e Departamento
-- (cod_depto/cod_sec/cod_categoria) — mesma filtragem por indústria, uma
-- única leitura de vendas_faturadas/transmitidas serve V02/V03/V06/V07.
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
        v.cod_cliprinc, v.fantasia,
        v.cod_depto, v.depto, v.cod_sec, v.secao, v.cod_categoria, v.categoria,
        v.tipo_venda, v.pvenda, v.plucro, v.qt
    FROM vendas_faturadas v
    WHERE v.empresa_id = p_empresa_id
      AND v.data_faturamento BETWEEN p_ini AND p_fim
      AND v.cod_fornec IN (SELECT fi.cod_fornec FROM farol.industria_fornecedores fi WHERE fi.empresa_id = p_empresa_id);

    CREATE INDEX ON _vind_fat (cod_gerente);
    CREATE INDEX ON _vind_fat (cod_supervisor);
    CREATE INDEX ON _vind_fat (cod_rca);
    CREATE INDEX ON _vind_fat (cod_cliprinc);
    CREATE INDEX ON _vind_fat (cod_depto);
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

    -- V06_ind_l0 (Rede)
    INSERT INTO farol.agg_fat_v06_ind_l0_mes AS t
        (empresa_id, ano, mes, cod_cliprinc, nome_cliprinc, pvenda, plucro, qt,
         liquido, pv_bonif, pv_transf, pv_remessa)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_cliprinc,
        COALESCE(NULLIF(MAX(v.fantasia), ''), MAX(v.nome_cli)),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt),
        COALESCE(SUM(v.pvenda) FILTER (WHERE v.tipo_venda IN ('1','4','7','8','9','11','14','20')), 0),
        COALESCE(SUM(v.pvenda) FILTER (WHERE v.tipo_venda = '5'), 0),
        COALESCE(SUM(v.pvenda) FILTER (WHERE v.tipo_venda = '10'), 0),
        COALESCE(SUM(v.pvenda) FILTER (WHERE v.tipo_venda = '13'), 0)
    FROM _vind_fat v
    WHERE v.cod_cliprinc <> ''
    GROUP BY v.empresa_id, v.cod_cliprinc
    ON CONFLICT (ano, empresa_id, mes, cod_cliprinc) DO UPDATE SET
        nome_cliprinc = EXCLUDED.nome_cliprinc,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt,
        liquido = EXCLUDED.liquido, pv_bonif = EXCLUDED.pv_bonif,
        pv_transf = EXCLUDED.pv_transf, pv_remessa = EXCLUDED.pv_remessa;

    -- V07_ind_l0 (Departamento)
    INSERT INTO farol.agg_fat_v07_ind_l0_mes AS t
        (empresa_id, ano, mes, cod_depto, depto, pvenda, plucro, qt,
         liquido, pv_bonif, pv_transf, pv_remessa)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_depto, MAX(v.depto),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt),
        COALESCE(SUM(v.pvenda) FILTER (WHERE v.tipo_venda IN ('1','4','7','8','9','11','14','20')), 0),
        COALESCE(SUM(v.pvenda) FILTER (WHERE v.tipo_venda = '5'), 0),
        COALESCE(SUM(v.pvenda) FILTER (WHERE v.tipo_venda = '10'), 0),
        COALESCE(SUM(v.pvenda) FILTER (WHERE v.tipo_venda = '13'), 0)
    FROM _vind_fat v
    WHERE v.cod_depto <> ''
    GROUP BY v.empresa_id, v.cod_depto
    ON CONFLICT (ano, empresa_id, mes, cod_depto) DO UPDATE SET
        depto = EXCLUDED.depto,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt,
        liquido = EXCLUDED.liquido, pv_bonif = EXCLUDED.pv_bonif,
        pv_transf = EXCLUDED.pv_transf, pv_remessa = EXCLUDED.pv_remessa;

    -- V07_ind_l1 (+Seção)
    INSERT INTO farol.agg_fat_v07_ind_l1_mes AS t
        (empresa_id, ano, mes, cod_depto, cod_sec, secao, pvenda, plucro, qt,
         liquido, pv_bonif, pv_transf, pv_remessa)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_depto, v.cod_sec, MAX(v.secao),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt),
        COALESCE(SUM(v.pvenda) FILTER (WHERE v.tipo_venda IN ('1','4','7','8','9','11','14','20')), 0),
        COALESCE(SUM(v.pvenda) FILTER (WHERE v.tipo_venda = '5'), 0),
        COALESCE(SUM(v.pvenda) FILTER (WHERE v.tipo_venda = '10'), 0),
        COALESCE(SUM(v.pvenda) FILTER (WHERE v.tipo_venda = '13'), 0)
    FROM _vind_fat v
    WHERE v.cod_depto <> '' AND v.cod_sec <> ''
    GROUP BY v.empresa_id, v.cod_depto, v.cod_sec
    ON CONFLICT (ano, empresa_id, mes, cod_depto, cod_sec) DO UPDATE SET
        secao = EXCLUDED.secao,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt,
        liquido = EXCLUDED.liquido, pv_bonif = EXCLUDED.pv_bonif,
        pv_transf = EXCLUDED.pv_transf, pv_remessa = EXCLUDED.pv_remessa;

    -- V07_ind_l2 (+Categoria)
    INSERT INTO farol.agg_fat_v07_ind_l2_mes AS t
        (empresa_id, ano, mes, cod_depto, cod_sec, cod_categoria, categoria, pvenda, plucro, qt,
         liquido, pv_bonif, pv_transf, pv_remessa)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_depto, v.cod_sec, v.cod_categoria, MAX(v.categoria),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt),
        COALESCE(SUM(v.pvenda) FILTER (WHERE v.tipo_venda IN ('1','4','7','8','9','11','14','20')), 0),
        COALESCE(SUM(v.pvenda) FILTER (WHERE v.tipo_venda = '5'), 0),
        COALESCE(SUM(v.pvenda) FILTER (WHERE v.tipo_venda = '10'), 0),
        COALESCE(SUM(v.pvenda) FILTER (WHERE v.tipo_venda = '13'), 0)
    FROM _vind_fat v
    WHERE v.cod_depto <> '' AND v.cod_sec <> '' AND v.cod_categoria <> ''
    GROUP BY v.empresa_id, v.cod_depto, v.cod_sec, v.cod_categoria
    ON CONFLICT (ano, empresa_id, mes, cod_depto, cod_sec, cod_categoria) DO UPDATE SET
        categoria = EXCLUDED.categoria,
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
        v.cod_cliprinc, v.fantasia,
        v.cod_depto, v.depto, v.cod_sec, v.secao, v.cod_categoria, v.categoria,
        v.pvenda, v.plucro, v.qt
    FROM vendas_transmitidas v
    WHERE v.empresa_id = p_empresa_id
      AND v.data_transmissao BETWEEN p_ini AND p_fim
      AND v.cod_fornec IN (SELECT fi.cod_fornec FROM farol.industria_fornecedores fi WHERE fi.empresa_id = p_empresa_id);

    CREATE INDEX ON _vind_trans (cod_gerente);
    CREATE INDEX ON _vind_trans (cod_supervisor);
    CREATE INDEX ON _vind_trans (cod_rca);
    CREATE INDEX ON _vind_trans (cod_cliprinc);
    CREATE INDEX ON _vind_trans (cod_depto);
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

    INSERT INTO farol.agg_trans_v06_ind_l0_mes AS t
        (empresa_id, ano, mes, cod_cliprinc, nome_cliprinc, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_cliprinc,
        COALESCE(NULLIF(MAX(v.fantasia), ''), MAX(v.nome_cli)),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _vind_trans v
    WHERE v.cod_cliprinc <> ''
    GROUP BY v.empresa_id, v.cod_cliprinc
    ON CONFLICT (ano, empresa_id, mes, cod_cliprinc) DO UPDATE SET
        nome_cliprinc = EXCLUDED.nome_cliprinc,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_trans_v07_ind_l0_mes AS t
        (empresa_id, ano, mes, cod_depto, depto, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_depto, MAX(v.depto),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _vind_trans v
    WHERE v.cod_depto <> ''
    GROUP BY v.empresa_id, v.cod_depto
    ON CONFLICT (ano, empresa_id, mes, cod_depto) DO UPDATE SET
        depto = EXCLUDED.depto,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_trans_v07_ind_l1_mes AS t
        (empresa_id, ano, mes, cod_depto, cod_sec, secao, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_depto, v.cod_sec, MAX(v.secao),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _vind_trans v
    WHERE v.cod_depto <> '' AND v.cod_sec <> ''
    GROUP BY v.empresa_id, v.cod_depto, v.cod_sec
    ON CONFLICT (ano, empresa_id, mes, cod_depto, cod_sec) DO UPDATE SET
        secao = EXCLUDED.secao,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_trans_v07_ind_l2_mes AS t
        (empresa_id, ano, mes, cod_depto, cod_sec, cod_categoria, categoria, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_depto, v.cod_sec, v.cod_categoria, MAX(v.categoria),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _vind_trans v
    WHERE v.cod_depto <> '' AND v.cod_sec <> '' AND v.cod_categoria <> ''
    GROUP BY v.empresa_id, v.cod_depto, v.cod_sec, v.cod_categoria
    ON CONFLICT (ano, empresa_id, mes, cod_depto, cod_sec, cod_categoria) DO UPDATE SET
        categoria = EXCLUDED.categoria,
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
        'agg_fat_v03_ind_l0_mes','agg_fat_v03_ind_l1_mes','agg_fat_v03_ind_l2_mes','agg_fat_v03_ind_l3_mes',
        'agg_fat_v02_ind_l0_mes','agg_fat_v02_ind_l1_mes',
        -- Somente Indústrias — V06/V07 (mig 229)
        'agg_fat_v06_ind_l0_mes',
        'agg_fat_v07_ind_l0_mes','agg_fat_v07_ind_l1_mes','agg_fat_v07_ind_l2_mes',
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
        'agg_trans_v02_ind_l0_mes','agg_trans_v02_ind_l1_mes',
        'agg_trans_v06_ind_l0_mes',
        'agg_trans_v07_ind_l0_mes','agg_trans_v07_ind_l1_mes','agg_trans_v07_ind_l2_mes'
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
