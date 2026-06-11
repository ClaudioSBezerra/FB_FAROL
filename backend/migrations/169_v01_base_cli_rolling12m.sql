-- 169_v01_base_cli_rolling12m.sql
-- ════════════════════════════════════════════════════════════════════════════
-- Muda a semântica de base_cli nos níveis V01 (Por Indústria) de
-- "total de clientes da empresa (denominador fixo)" para
-- "base histórica de compradores do fornecedor nos últimos 12 meses".
--
-- MOTIVAÇÃO:
--   Por Gerência/Equipe: base_cli = carteira do gerente/supervisor → taxa de
--   COBERTURA ("dos meus clientes atribuídos, quantos comprei?") — números
--   diferentes por card, semanticamente corretos.
--
--   Por Indústria (V01): base_cli era SUM(qtcli_rca empresa toda) = 107.116
--   fixo para TODOS os fornecedores → métrica de PENETRAÇÃO ("dos 107k clientes,
--   quantos compram desta indústria?"). O mesmo número em toda linha é
--   informacionalmente inútil por card.
--
-- NOVA SEMÂNTICA V01:
--   base_cli = COUNT(DISTINCT cnpj) WHERE qt > 0 nos últimos 12 meses
--   para aquele fornecedor (e filtros de drill pertinentes).
--   → taxa de RETENÇÃO ("dos clientes que já compraram desta indústria no
--   último ano, quantos compraram neste período?").
--   Números diferentes por fornecedor, comparáveis entre si.
--
-- ESTRATÉGIA DE PERFORMANCE:
--   Cria temp tables _v_fat_12m / _v_trans_12m com dados rolling 12M uma vez
--   por invocação de upsert_aggs_mes, evitando sub-selects repetidos em
--   vendas_faturadas/transmitidas por grupo.
--
-- IMPACTO:
--   • V01 L0-L3 (fat + trans): base_cli recalculado.
--   • V01 L4, V02-V05 e todas as outras views: sem alteração.
--   • KPI totalizador: backend já foi corrigido (fix overlappingBaseKPI).
--
-- RE-POPULATE:
--   Roda upsert_aggs_mes para todos os meses existentes ao final.
-- ════════════════════════════════════════════════════════════════════════════

CREATE OR REPLACE FUNCTION farol.upsert_aggs_mes(
    p_empresa_id UUID,
    p_ano        INT,
    p_mes        INT
) RETURNS VOID AS $$
DECLARE
    p_ini    DATE := make_date(p_ano, p_mes, 1);
    p_fim    DATE := (p_ini + INTERVAL '1 month' - INTERVAL '1 day')::date;
    p_12m    DATE := (p_ini - INTERVAL '11 months')::date;  -- janela rolling 12M
BEGIN
    SET LOCAL work_mem = '256MB';

    -- ════════════════════ TEMP TABLE FATURADO (mês corrente) ═════════════════
    DROP TABLE IF EXISTS _v_fat;
    CREATE TEMP TABLE _v_fat ON COMMIT DROP AS
    SELECT
        v.empresa_id, v.cod_fornec, v.nome_fornec,
        v.cod_gerente, v.nome_gerente,
        v.cod_supervisor, v.nome_supervisor,
        v.cod_rca, v.nome_rca, v.qtcli_rca,
        v.cnpj, v.cod_cli, v.nome_cli, v.uf, v.empresa,
        v.cod_prod, v.nome_prod, v.ean,
        v.pvenda, v.plucro, v.qt
    FROM vendas_faturadas v
    WHERE v.empresa_id = p_empresa_id
      AND v.data_faturamento BETWEEN p_ini AND p_fim;

    CREATE INDEX ON _v_fat (cod_fornec);
    CREATE INDEX ON _v_fat (cod_gerente);
    CREATE INDEX ON _v_fat (cod_supervisor);
    CREATE INDEX ON _v_fat (cod_rca);
    CREATE INDEX ON _v_fat (cod_cli);
    CREATE INDEX ON _v_fat (cnpj);
    ANALYZE _v_fat;

    -- ════════════════════ TEMP TABLE FATURADO rolling 12M ════════════════════
    -- Usada para calcular base_cli nos níveis V01 (base histórica por fornecedor)
    DROP TABLE IF EXISTS _v_fat_12m;
    CREATE TEMP TABLE _v_fat_12m ON COMMIT DROP AS
    SELECT cod_fornec, cod_gerente, cod_supervisor, cod_rca, cnpj
    FROM vendas_faturadas
    WHERE empresa_id = p_empresa_id
      AND data_faturamento BETWEEN p_12m AND p_fim
      AND qt > 0
      AND cod_fornec <> '';

    CREATE INDEX ON _v_fat_12m (cod_fornec, cnpj);
    CREATE INDEX ON _v_fat_12m (cod_fornec, cod_gerente, cnpj);
    CREATE INDEX ON _v_fat_12m (cod_fornec, cod_supervisor, cnpj);
    CREATE INDEX ON _v_fat_12m (cod_fornec, cod_rca, cnpj);
    ANALYZE _v_fat_12m;

    -- ════════════════════ FATURADO INSERTs ═══════════════════════════════════

    -- V01_l0 (Fornec) — base_cli = compradores rolling 12M deste fornecedor
    INSERT INTO farol.agg_fat_v01_l0_mes AS t
        (empresa_id, ano, mes, cod_fornec, nome_fornec, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_fornec, MAX(v.nome_fornec),
        (SELECT COUNT(DISTINCT h.cnpj) FROM _v_fat_12m h WHERE h.cod_fornec = v.cod_fornec)::INT,
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v_fat v
    WHERE v.cod_fornec <> ''
    GROUP BY v.empresa_id, v.cod_fornec
    ON CONFLICT (ano, empresa_id, mes, cod_fornec) DO UPDATE SET
        nome_fornec = EXCLUDED.nome_fornec, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V01_l1 (+Gerente) — base_cli = compradores rolling 12M deste fornecedor nesta gerência
    INSERT INTO farol.agg_fat_v01_l1_mes AS t
        (empresa_id, ano, mes, cod_fornec, cod_gerente, nome_gerente, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_fornec, v.cod_gerente, MAX(v.nome_gerente),
        (SELECT COUNT(DISTINCT h.cnpj) FROM _v_fat_12m h
          WHERE h.cod_fornec = v.cod_fornec AND h.cod_gerente = v.cod_gerente)::INT,
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v_fat v
    WHERE v.cod_fornec <> '' AND v.cod_gerente <> ''
    GROUP BY v.empresa_id, v.cod_fornec, v.cod_gerente
    ON CONFLICT (ano, empresa_id, mes, cod_fornec, cod_gerente) DO UPDATE SET
        nome_gerente = EXCLUDED.nome_gerente, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V01_l2 (+Supervisor) — base_cli = compradores rolling 12M deste fornecedor neste supervisor
    INSERT INTO farol.agg_fat_v01_l2_mes AS t
        (empresa_id, ano, mes, cod_fornec, cod_gerente, cod_supervisor, nome_supervisor, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_fornec, v.cod_gerente, v.cod_supervisor, MAX(v.nome_supervisor),
        (SELECT COUNT(DISTINCT h.cnpj) FROM _v_fat_12m h
          WHERE h.cod_fornec = v.cod_fornec AND h.cod_supervisor = v.cod_supervisor)::INT,
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v_fat v
    WHERE v.cod_fornec <> '' AND v.cod_gerente <> '' AND v.cod_supervisor <> ''
    GROUP BY v.empresa_id, v.cod_fornec, v.cod_gerente, v.cod_supervisor
    ON CONFLICT (ano, empresa_id, mes, cod_fornec, cod_gerente, cod_supervisor) DO UPDATE SET
        nome_supervisor = EXCLUDED.nome_supervisor, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V01_l3 (+RCA) — base_cli = compradores rolling 12M deste fornecedor neste RCA
    INSERT INTO farol.agg_fat_v01_l3_mes AS t
        (empresa_id, ano, mes, cod_fornec, cod_gerente, cod_supervisor, cod_rca, nome_rca,
         base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_fornec, v.cod_gerente, v.cod_supervisor, v.cod_rca, MAX(v.nome_rca),
        (SELECT COUNT(DISTINCT h.cnpj) FROM _v_fat_12m h
          WHERE h.cod_fornec = v.cod_fornec AND h.cod_rca = v.cod_rca)::INT,
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v_fat v
    WHERE v.cod_fornec <> '' AND v.cod_gerente <> '' AND v.cod_supervisor <> '' AND v.cod_rca <> ''
    GROUP BY v.empresa_id, v.cod_fornec, v.cod_gerente, v.cod_supervisor, v.cod_rca
    ON CONFLICT (ano, empresa_id, mes, cod_fornec, cod_gerente, cod_supervisor, cod_rca) DO UPDATE SET
        nome_rca = EXCLUDED.nome_rca, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V01_l4 (+Cliente) — sem alteração: base_cli=1 por linha de cliente
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
    FROM _v_fat v
    WHERE v.cod_fornec <> '' AND v.cod_gerente <> '' AND v.cod_supervisor <> ''
      AND v.cod_rca <> '' AND v.cnpj <> ''
    GROUP BY v.empresa_id, v.cod_fornec, v.cod_gerente, v.cod_supervisor, v.cod_rca, v.cnpj
    ON CONFLICT (ano, empresa_id, mes, cod_fornec, cod_gerente, cod_supervisor, cod_rca, cnpj) DO UPDATE SET
        cod_cli = EXCLUDED.cod_cli, nome_cli = EXCLUDED.nome_cli,
        base_cli = EXCLUDED.base_cli, positivados = EXCLUDED.positivados,
        mix = EXCLUDED.mix, pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V02-V05 e restante: copiados de migration 166 sem alteração
    INSERT INTO farol.agg_fat_v02_l0_mes AS t
        (empresa_id, ano, mes, cod_supervisor, nome_supervisor, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_supervisor, MAX(v.nome_supervisor),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_fat_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_supervisor = v.cod_supervisor),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v_fat v WHERE v.cod_supervisor <> ''
    GROUP BY v.empresa_id, v.cod_supervisor
    ON CONFLICT (ano, empresa_id, mes, cod_supervisor) DO UPDATE SET
        nome_supervisor = EXCLUDED.nome_supervisor, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_fat_v02_l1_mes AS t
        (empresa_id, ano, mes, cod_supervisor, cod_rca, nome_rca, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_supervisor, v.cod_rca, MAX(v.nome_rca),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_fat_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_rca = v.cod_rca),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v_fat v WHERE v.cod_supervisor <> '' AND v.cod_rca <> ''
    GROUP BY v.empresa_id, v.cod_supervisor, v.cod_rca
    ON CONFLICT (ano, empresa_id, mes, cod_supervisor, cod_rca) DO UPDATE SET
        nome_rca = EXCLUDED.nome_rca, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_fat_v02_l2_mes AS t
        (empresa_id, ano, mes, cod_supervisor, cod_rca, cod_fornec, nome_fornec, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_supervisor, v.cod_rca, v.cod_fornec, MAX(v.nome_fornec),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_fat_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_rca = v.cod_rca),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v_fat v WHERE v.cod_supervisor <> '' AND v.cod_rca <> '' AND v.cod_fornec <> ''
    GROUP BY v.empresa_id, v.cod_supervisor, v.cod_rca, v.cod_fornec
    ON CONFLICT (ano, empresa_id, mes, cod_supervisor, cod_rca, cod_fornec) DO UPDATE SET
        nome_fornec = EXCLUDED.nome_fornec, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_fat_v02_l3_mes AS t
        (empresa_id, ano, mes, cod_supervisor, cod_rca, cod_fornec, cnpj, cod_cli, nome_cli,
         base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_supervisor, v.cod_rca, v.cod_fornec,
        v.cnpj, MAX(v.cod_cli), MAX(v.nome_cli),
        1, (CASE WHEN SUM(v.qt) > 0 THEN 1 ELSE 0 END)::INT,
        COUNT(DISTINCT v.cod_prod) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC,
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v_fat v WHERE v.cod_supervisor <> '' AND v.cod_rca <> '' AND v.cod_fornec <> '' AND v.cnpj <> ''
    GROUP BY v.empresa_id, v.cod_supervisor, v.cod_rca, v.cod_fornec, v.cnpj
    ON CONFLICT (ano, empresa_id, mes, cod_supervisor, cod_rca, cod_fornec, cnpj) DO UPDATE SET
        cod_cli = EXCLUDED.cod_cli, nome_cli = EXCLUDED.nome_cli,
        base_cli = EXCLUDED.base_cli, positivados = EXCLUDED.positivados,
        mix = EXCLUDED.mix, pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_fat_v03_l0_mes AS t
        (empresa_id, ano, mes, cod_gerente, nome_gerente, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_gerente, MAX(v.nome_gerente),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_fat_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_gerente = v.cod_gerente),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v_fat v WHERE v.cod_gerente <> ''
    GROUP BY v.empresa_id, v.cod_gerente
    ON CONFLICT (ano, empresa_id, mes, cod_gerente) DO UPDATE SET
        nome_gerente = EXCLUDED.nome_gerente, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_fat_v03_l1_mes AS t
        (empresa_id, ano, mes, cod_gerente, cod_supervisor, nome_supervisor, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_gerente, v.cod_supervisor, MAX(v.nome_supervisor),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_fat_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_supervisor = v.cod_supervisor),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v_fat v WHERE v.cod_gerente <> '' AND v.cod_supervisor <> ''
    GROUP BY v.empresa_id, v.cod_gerente, v.cod_supervisor
    ON CONFLICT (ano, empresa_id, mes, cod_gerente, cod_supervisor) DO UPDATE SET
        nome_supervisor = EXCLUDED.nome_supervisor, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_fat_v03_l2_mes AS t
        (empresa_id, ano, mes, cod_gerente, cod_supervisor, cod_rca, nome_rca, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_gerente, v.cod_supervisor, v.cod_rca, MAX(v.nome_rca),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_fat_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_rca = v.cod_rca),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v_fat v WHERE v.cod_gerente <> '' AND v.cod_supervisor <> '' AND v.cod_rca <> ''
    GROUP BY v.empresa_id, v.cod_gerente, v.cod_supervisor, v.cod_rca
    ON CONFLICT (ano, empresa_id, mes, cod_gerente, cod_supervisor, cod_rca) DO UPDATE SET
        nome_rca = EXCLUDED.nome_rca, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_fat_v03_l3_mes AS t
        (empresa_id, ano, mes, cod_gerente, cod_supervisor, cod_rca, cnpj, cod_cli, nome_cli,
         base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_gerente, v.cod_supervisor, v.cod_rca,
        v.cnpj, MAX(v.cod_cli), MAX(v.nome_cli),
        1, (CASE WHEN SUM(v.qt) > 0 THEN 1 ELSE 0 END)::INT,
        COUNT(DISTINCT v.cod_prod) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC,
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v_fat v WHERE v.cod_gerente <> '' AND v.cod_supervisor <> '' AND v.cod_rca <> '' AND v.cnpj <> ''
    GROUP BY v.empresa_id, v.cod_gerente, v.cod_supervisor, v.cod_rca, v.cnpj
    ON CONFLICT (ano, empresa_id, mes, cod_gerente, cod_supervisor, cod_rca, cnpj) DO UPDATE SET
        cod_cli = EXCLUDED.cod_cli, nome_cli = EXCLUDED.nome_cli,
        base_cli = EXCLUDED.base_cli, positivados = EXCLUDED.positivados,
        mix = EXCLUDED.mix, pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_fat_v04_l0_mes AS t
        (empresa_id, ano, mes, cod_rca, nome_rca, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_rca, MAX(v.nome_rca),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_fat_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_rca = v.cod_rca),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v_fat v WHERE v.cod_rca <> ''
    GROUP BY v.empresa_id, v.cod_rca
    ON CONFLICT (ano, empresa_id, mes, cod_rca) DO UPDATE SET
        nome_rca = EXCLUDED.nome_rca, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_fat_v04_l1_mes AS t
        (empresa_id, ano, mes, cod_rca, cod_fornec, nome_fornec, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_rca, v.cod_fornec, MAX(v.nome_fornec),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_fat_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_rca = v.cod_rca),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v_fat v WHERE v.cod_rca <> '' AND v.cod_fornec <> ''
    GROUP BY v.empresa_id, v.cod_rca, v.cod_fornec
    ON CONFLICT (ano, empresa_id, mes, cod_rca, cod_fornec) DO UPDATE SET
        nome_fornec = EXCLUDED.nome_fornec, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_fat_v04_l2_mes AS t
        (empresa_id, ano, mes, cod_rca, cod_fornec, cnpj, cod_cli, nome_cli,
         base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_rca, v.cod_fornec,
        v.cnpj, MAX(v.cod_cli), MAX(v.nome_cli),
        1, (CASE WHEN SUM(v.qt) > 0 THEN 1 ELSE 0 END)::INT,
        COUNT(DISTINCT v.cod_prod) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC,
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v_fat v WHERE v.cod_rca <> '' AND v.cod_fornec <> '' AND v.cnpj <> ''
    GROUP BY v.empresa_id, v.cod_rca, v.cod_fornec, v.cnpj
    ON CONFLICT (ano, empresa_id, mes, cod_rca, cod_fornec, cnpj) DO UPDATE SET
        cod_cli = EXCLUDED.cod_cli, nome_cli = EXCLUDED.nome_cli,
        base_cli = EXCLUDED.base_cli, positivados = EXCLUDED.positivados,
        mix = EXCLUDED.mix, pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- ════════════════════ TEMP TABLE TRANSMITIDO (mês corrente) ══════════════
    DROP TABLE IF EXISTS _v_trans;
    CREATE TEMP TABLE _v_trans ON COMMIT DROP AS
    SELECT
        v.empresa_id, v.cod_fornec, v.nome_fornec,
        v.cod_gerente, v.nome_gerente,
        v.cod_supervisor, v.nome_supervisor,
        v.cod_rca, v.nome_rca, v.qtcli_rca,
        v.cnpj, v.cod_cli, v.nome_cli, v.uf, v.empresa,
        v.cod_prod, v.nome_prod, v.ean,
        v.pvenda, v.plucro, v.qt
    FROM vendas_transmitidas v
    WHERE v.empresa_id = p_empresa_id
      AND v.data_transmissao BETWEEN p_ini AND p_fim;

    CREATE INDEX ON _v_trans (cod_fornec);
    CREATE INDEX ON _v_trans (cod_gerente);
    CREATE INDEX ON _v_trans (cod_supervisor);
    CREATE INDEX ON _v_trans (cod_rca);
    CREATE INDEX ON _v_trans (cod_cli);
    CREATE INDEX ON _v_trans (cnpj);
    ANALYZE _v_trans;

    -- ════════════════════ TEMP TABLE TRANSMITIDO rolling 12M ═════════════════
    DROP TABLE IF EXISTS _v_trans_12m;
    CREATE TEMP TABLE _v_trans_12m ON COMMIT DROP AS
    SELECT cod_fornec, cod_gerente, cod_supervisor, cod_rca, cnpj
    FROM vendas_transmitidas
    WHERE empresa_id = p_empresa_id
      AND data_transmissao BETWEEN p_12m AND p_fim
      AND qt > 0
      AND cod_fornec <> '';

    CREATE INDEX ON _v_trans_12m (cod_fornec, cnpj);
    CREATE INDEX ON _v_trans_12m (cod_fornec, cod_gerente, cnpj);
    CREATE INDEX ON _v_trans_12m (cod_fornec, cod_supervisor, cnpj);
    CREATE INDEX ON _v_trans_12m (cod_fornec, cod_rca, cnpj);
    ANALYZE _v_trans_12m;

    -- ════════════════════ TRANSMITIDO INSERTs ════════════════════════════════

    -- V01_l0 trans
    INSERT INTO farol.agg_trans_v01_l0_mes AS t
        (empresa_id, ano, mes, cod_fornec, nome_fornec, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_fornec, MAX(v.nome_fornec),
        (SELECT COUNT(DISTINCT h.cnpj) FROM _v_trans_12m h WHERE h.cod_fornec = v.cod_fornec)::INT,
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v_trans v WHERE v.cod_fornec <> ''
    GROUP BY v.empresa_id, v.cod_fornec
    ON CONFLICT (ano, empresa_id, mes, cod_fornec) DO UPDATE SET
        nome_fornec = EXCLUDED.nome_fornec, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V01_l1 trans
    INSERT INTO farol.agg_trans_v01_l1_mes AS t
        (empresa_id, ano, mes, cod_fornec, cod_gerente, nome_gerente, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_fornec, v.cod_gerente, MAX(v.nome_gerente),
        (SELECT COUNT(DISTINCT h.cnpj) FROM _v_trans_12m h
          WHERE h.cod_fornec = v.cod_fornec AND h.cod_gerente = v.cod_gerente)::INT,
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v_trans v WHERE v.cod_fornec <> '' AND v.cod_gerente <> ''
    GROUP BY v.empresa_id, v.cod_fornec, v.cod_gerente
    ON CONFLICT (ano, empresa_id, mes, cod_fornec, cod_gerente) DO UPDATE SET
        nome_gerente = EXCLUDED.nome_gerente, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V01_l2 trans
    INSERT INTO farol.agg_trans_v01_l2_mes AS t
        (empresa_id, ano, mes, cod_fornec, cod_gerente, cod_supervisor, nome_supervisor, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_fornec, v.cod_gerente, v.cod_supervisor, MAX(v.nome_supervisor),
        (SELECT COUNT(DISTINCT h.cnpj) FROM _v_trans_12m h
          WHERE h.cod_fornec = v.cod_fornec AND h.cod_supervisor = v.cod_supervisor)::INT,
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v_trans v WHERE v.cod_fornec <> '' AND v.cod_gerente <> '' AND v.cod_supervisor <> ''
    GROUP BY v.empresa_id, v.cod_fornec, v.cod_gerente, v.cod_supervisor
    ON CONFLICT (ano, empresa_id, mes, cod_fornec, cod_gerente, cod_supervisor) DO UPDATE SET
        nome_supervisor = EXCLUDED.nome_supervisor, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V01_l3 trans
    INSERT INTO farol.agg_trans_v01_l3_mes AS t
        (empresa_id, ano, mes, cod_fornec, cod_gerente, cod_supervisor, cod_rca, nome_rca,
         base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_fornec, v.cod_gerente, v.cod_supervisor, v.cod_rca, MAX(v.nome_rca),
        (SELECT COUNT(DISTINCT h.cnpj) FROM _v_trans_12m h
          WHERE h.cod_fornec = v.cod_fornec AND h.cod_rca = v.cod_rca)::INT,
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v_trans v WHERE v.cod_fornec <> '' AND v.cod_gerente <> '' AND v.cod_supervisor <> '' AND v.cod_rca <> ''
    GROUP BY v.empresa_id, v.cod_fornec, v.cod_gerente, v.cod_supervisor, v.cod_rca
    ON CONFLICT (ano, empresa_id, mes, cod_fornec, cod_gerente, cod_supervisor, cod_rca) DO UPDATE SET
        nome_rca = EXCLUDED.nome_rca, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V01_l4 trans
    INSERT INTO farol.agg_trans_v01_l4_mes AS t
        (empresa_id, ano, mes, cod_fornec, cod_gerente, cod_supervisor, cod_rca, cnpj, cod_cli, nome_cli,
         base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_fornec, v.cod_gerente, v.cod_supervisor, v.cod_rca,
        v.cnpj, MAX(v.cod_cli), MAX(v.nome_cli),
        1, (CASE WHEN SUM(v.qt) > 0 THEN 1 ELSE 0 END)::INT,
        COUNT(DISTINCT v.cod_prod) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC,
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v_trans v WHERE v.cod_fornec <> '' AND v.cod_gerente <> '' AND v.cod_supervisor <> ''
      AND v.cod_rca <> '' AND v.cnpj <> ''
    GROUP BY v.empresa_id, v.cod_fornec, v.cod_gerente, v.cod_supervisor, v.cod_rca, v.cnpj
    ON CONFLICT (ano, empresa_id, mes, cod_fornec, cod_gerente, cod_supervisor, cod_rca, cnpj) DO UPDATE SET
        cod_cli = EXCLUDED.cod_cli, nome_cli = EXCLUDED.nome_cli,
        base_cli = EXCLUDED.base_cli, positivados = EXCLUDED.positivados,
        mix = EXCLUDED.mix, pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_trans_v02_l0_mes AS t
        (empresa_id, ano, mes, cod_supervisor, nome_supervisor, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_supervisor, MAX(v.nome_supervisor),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_trans_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_supervisor = v.cod_supervisor),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v_trans v WHERE v.cod_supervisor <> ''
    GROUP BY v.empresa_id, v.cod_supervisor
    ON CONFLICT (ano, empresa_id, mes, cod_supervisor) DO UPDATE SET
        nome_supervisor = EXCLUDED.nome_supervisor, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_trans_v02_l1_mes AS t
        (empresa_id, ano, mes, cod_supervisor, cod_rca, nome_rca, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_supervisor, v.cod_rca, MAX(v.nome_rca),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_trans_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_rca = v.cod_rca),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v_trans v WHERE v.cod_supervisor <> '' AND v.cod_rca <> ''
    GROUP BY v.empresa_id, v.cod_supervisor, v.cod_rca
    ON CONFLICT (ano, empresa_id, mes, cod_supervisor, cod_rca) DO UPDATE SET
        nome_rca = EXCLUDED.nome_rca, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_trans_v02_l2_mes AS t
        (empresa_id, ano, mes, cod_supervisor, cod_rca, cod_fornec, nome_fornec, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_supervisor, v.cod_rca, v.cod_fornec, MAX(v.nome_fornec),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_trans_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_rca = v.cod_rca),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v_trans v WHERE v.cod_supervisor <> '' AND v.cod_rca <> '' AND v.cod_fornec <> ''
    GROUP BY v.empresa_id, v.cod_supervisor, v.cod_rca, v.cod_fornec
    ON CONFLICT (ano, empresa_id, mes, cod_supervisor, cod_rca, cod_fornec) DO UPDATE SET
        nome_fornec = EXCLUDED.nome_fornec, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_trans_v02_l3_mes AS t
        (empresa_id, ano, mes, cod_supervisor, cod_rca, cod_fornec, cnpj, cod_cli, nome_cli,
         base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_supervisor, v.cod_rca, v.cod_fornec,
        v.cnpj, MAX(v.cod_cli), MAX(v.nome_cli),
        1, (CASE WHEN SUM(v.qt) > 0 THEN 1 ELSE 0 END)::INT,
        COUNT(DISTINCT v.cod_prod) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC,
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v_trans v WHERE v.cod_supervisor <> '' AND v.cod_rca <> '' AND v.cod_fornec <> '' AND v.cnpj <> ''
    GROUP BY v.empresa_id, v.cod_supervisor, v.cod_rca, v.cod_fornec, v.cnpj
    ON CONFLICT (ano, empresa_id, mes, cod_supervisor, cod_rca, cod_fornec, cnpj) DO UPDATE SET
        cod_cli = EXCLUDED.cod_cli, nome_cli = EXCLUDED.nome_cli,
        base_cli = EXCLUDED.base_cli, positivados = EXCLUDED.positivados,
        mix = EXCLUDED.mix, pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_trans_v03_l0_mes AS t
        (empresa_id, ano, mes, cod_gerente, nome_gerente, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_gerente, MAX(v.nome_gerente),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_trans_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_gerente = v.cod_gerente),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v_trans v WHERE v.cod_gerente <> ''
    GROUP BY v.empresa_id, v.cod_gerente
    ON CONFLICT (ano, empresa_id, mes, cod_gerente) DO UPDATE SET
        nome_gerente = EXCLUDED.nome_gerente, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_trans_v03_l1_mes AS t
        (empresa_id, ano, mes, cod_gerente, cod_supervisor, nome_supervisor, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_gerente, v.cod_supervisor, MAX(v.nome_supervisor),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_trans_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_supervisor = v.cod_supervisor),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v_trans v WHERE v.cod_gerente <> '' AND v.cod_supervisor <> ''
    GROUP BY v.empresa_id, v.cod_gerente, v.cod_supervisor
    ON CONFLICT (ano, empresa_id, mes, cod_gerente, cod_supervisor) DO UPDATE SET
        nome_supervisor = EXCLUDED.nome_supervisor, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_trans_v03_l2_mes AS t
        (empresa_id, ano, mes, cod_gerente, cod_supervisor, cod_rca, nome_rca, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_gerente, v.cod_supervisor, v.cod_rca, MAX(v.nome_rca),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_trans_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_rca = v.cod_rca),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v_trans v WHERE v.cod_gerente <> '' AND v.cod_supervisor <> '' AND v.cod_rca <> ''
    GROUP BY v.empresa_id, v.cod_gerente, v.cod_supervisor, v.cod_rca
    ON CONFLICT (ano, empresa_id, mes, cod_gerente, cod_supervisor, cod_rca) DO UPDATE SET
        nome_rca = EXCLUDED.nome_rca, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_trans_v03_l3_mes AS t
        (empresa_id, ano, mes, cod_gerente, cod_supervisor, cod_rca, cnpj, cod_cli, nome_cli,
         base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_gerente, v.cod_supervisor, v.cod_rca,
        v.cnpj, MAX(v.cod_cli), MAX(v.nome_cli),
        1, (CASE WHEN SUM(v.qt) > 0 THEN 1 ELSE 0 END)::INT,
        COUNT(DISTINCT v.cod_prod) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC,
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v_trans v WHERE v.cod_gerente <> '' AND v.cod_supervisor <> '' AND v.cod_rca <> '' AND v.cnpj <> ''
    GROUP BY v.empresa_id, v.cod_gerente, v.cod_supervisor, v.cod_rca, v.cnpj
    ON CONFLICT (ano, empresa_id, mes, cod_gerente, cod_supervisor, cod_rca, cnpj) DO UPDATE SET
        cod_cli = EXCLUDED.cod_cli, nome_cli = EXCLUDED.nome_cli,
        base_cli = EXCLUDED.base_cli, positivados = EXCLUDED.positivados,
        mix = EXCLUDED.mix, pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_trans_v04_l0_mes AS t
        (empresa_id, ano, mes, cod_rca, nome_rca, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_rca, MAX(v.nome_rca),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_trans_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_rca = v.cod_rca),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v_trans v WHERE v.cod_rca <> ''
    GROUP BY v.empresa_id, v.cod_rca
    ON CONFLICT (ano, empresa_id, mes, cod_rca) DO UPDATE SET
        nome_rca = EXCLUDED.nome_rca, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_trans_v04_l1_mes AS t
        (empresa_id, ano, mes, cod_rca, cod_fornec, nome_fornec, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_rca, v.cod_fornec, MAX(v.nome_fornec),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_trans_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_rca = v.cod_rca),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v_trans v WHERE v.cod_rca <> '' AND v.cod_fornec <> ''
    GROUP BY v.empresa_id, v.cod_rca, v.cod_fornec
    ON CONFLICT (ano, empresa_id, mes, cod_rca, cod_fornec) DO UPDATE SET
        nome_fornec = EXCLUDED.nome_fornec, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_trans_v04_l2_mes AS t
        (empresa_id, ano, mes, cod_rca, cod_fornec, cnpj, cod_cli, nome_cli,
         base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_rca, v.cod_fornec,
        v.cnpj, MAX(v.cod_cli), MAX(v.nome_cli),
        1, (CASE WHEN SUM(v.qt) > 0 THEN 1 ELSE 0 END)::INT,
        COUNT(DISTINCT v.cod_prod) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC,
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v_trans v WHERE v.cod_rca <> '' AND v.cod_fornec <> '' AND v.cnpj <> ''
    GROUP BY v.empresa_id, v.cod_rca, v.cod_fornec, v.cnpj
    ON CONFLICT (ano, empresa_id, mes, cod_rca, cod_fornec, cnpj) DO UPDATE SET
        cod_cli = EXCLUDED.cod_cli, nome_cli = EXCLUDED.nome_cli,
        base_cli = EXCLUDED.base_cli, positivados = EXCLUDED.positivados,
        mix = EXCLUDED.mix, pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

END;
$$ LANGUAGE plpgsql;

-- ════════════════════════════════════════════════════════════════════════════
-- RE-POPULATE todos os meses existentes com a nova semântica de base_cli
-- ════════════════════════════════════════════════════════════════════════════
DO $$
DECLARE
    r RECORD;
BEGIN
    FOR r IN
        SELECT DISTINCT empresa_id, ano, mes
        FROM farol.agg_fat_v01_l0_mes
        ORDER BY ano, mes
    LOOP
        PERFORM farol.upsert_aggs_mes(r.empresa_id, r.ano, r.mes);
    END LOOP;
END;
$$;
