-- 172_restore_dims_mkt_in_upsert.sql
-- ════════════════════════════════════════════════════════════════════════════
-- Bugfix definitivo da migration 169 (terceira e última correção).
--
-- A 169 recriou upsert_aggs_mes para o rolling 12M (base_cli V01) tomando como
-- base apenas os INSERTs V01-V04. Comparando com a 166 (versão completa), a
-- 169 perdeu 6 INSERTs + 1 PERFORM:
--   • agg_fat_dims_mes / agg_trans_dims_mes      → filtros do painel vazios
--   • agg_fat_mkt_cli_mes / agg_trans_mkt_cli_mes → painel Marketing zerado
--   • agg_fat_mkt_produto_mes / agg_trans_mkt_produto_mes → produtos Marketing
--   • upsert_aggs_mes_v05                         → corrigido na 170
--
-- Esta migration:
--   1) Recria upsert_aggs_mes DEFINITIVA = corpo da 166 completo
--      + rolling 12M V01 (da 169) + PERFORM v05 (da 170) + dims/mkt (da 166)
--   2) Cria índices de mix p/ cod_gerente e cod_rca (faltavam na 171;
--      queryMixTotal por gerência levava ~6s)
--   3) Re-popula APENAS dims_mes e mkt_*_mes dos meses existentes — usa as
--      agg_* já populadas como fonte quando possível (rápido); evita refazer
--      todas as aggs (que levou 34 min na 170).
--
-- Função abaixo é a composição literal: 170 (linhas 64-386, 387-685, 686-694)
-- com os blocos dims/mkt da 166 (linhas 334-392 e 654-707) enxertados.
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

    -- DIMS fat
    INSERT INTO farol.agg_fat_dims_mes AS t (empresa_id, ano, mes, dim, key, label)
    SELECT empresa_id, p_ano, p_mes, 'fornec', cod_fornec, COALESCE(MAX(nome_fornec),'')
        FROM _v_fat WHERE cod_fornec <> '' GROUP BY empresa_id, cod_fornec
    UNION ALL
    SELECT empresa_id, p_ano, p_mes, 'gerente', cod_gerente, COALESCE(MAX(nome_gerente),'')
        FROM _v_fat WHERE cod_gerente <> '' GROUP BY empresa_id, cod_gerente
    UNION ALL
    SELECT empresa_id, p_ano, p_mes, 'supervisor', cod_supervisor, COALESCE(MAX(nome_supervisor),'')
        FROM _v_fat WHERE cod_supervisor <> '' GROUP BY empresa_id, cod_supervisor
    UNION ALL
    SELECT empresa_id, p_ano, p_mes, 'rca', cod_rca, COALESCE(MAX(nome_rca),'')
        FROM _v_fat WHERE cod_rca <> '' GROUP BY empresa_id, cod_rca
    UNION ALL
    SELECT empresa_id, p_ano, p_mes, 'cli', cod_cli, COALESCE(MAX(nome_cli),'')
        FROM _v_fat WHERE cod_cli <> '' GROUP BY empresa_id, cod_cli
    UNION ALL
    SELECT empresa_id, p_ano, p_mes, 'uf', uf, uf
        FROM _v_fat WHERE uf <> '' GROUP BY empresa_id, uf
    UNION ALL
    SELECT empresa_id, p_ano, p_mes, 'empresa', empresa, empresa
        FROM _v_fat WHERE empresa <> '' GROUP BY empresa_id, empresa
    ON CONFLICT (ano, empresa_id, mes, dim, key) DO UPDATE SET label = EXCLUDED.label;

    -- Marketing Cliente×mês
    INSERT INTO farol.agg_fat_mkt_cli_mes AS t
        (empresa_id, ano, mes, cnpj, cod_cli, nome_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cnpj,
        MAX(v.cod_cli), MAX(v.nome_cli),
        (CASE WHEN SUM(v.qt) > 0 THEN 1 ELSE 0 END)::INT,
        COUNT(DISTINCT v.cod_prod) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC,
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v_fat v
    WHERE v.cnpj <> ''
    GROUP BY v.empresa_id, v.cnpj
    ON CONFLICT (ano, empresa_id, mes, cnpj) DO UPDATE SET
        cod_cli = EXCLUDED.cod_cli, nome_cli = EXCLUDED.nome_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- Marketing Produto×mês
    INSERT INTO farol.agg_fat_mkt_produto_mes AS t
        (empresa_id, ano, mes, cod_prod, nome_prod, cod_fornec, nome_fornec, ean,
         qt_clientes, qt_positivados, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_prod,
        MAX(v.nome_prod), MAX(v.cod_fornec), MAX(v.nome_fornec), MAX(v.ean),
        COUNT(DISTINCT NULLIF(v.cnpj, ''))::INT,
        COUNT(DISTINCT CASE WHEN v.qt > 0 THEN v.cnpj END)::INT,
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v_fat v
    WHERE v.cod_prod <> ''
    GROUP BY v.empresa_id, v.cod_prod
    ON CONFLICT (ano, empresa_id, mes, cod_prod) DO UPDATE SET
        nome_prod = EXCLUDED.nome_prod, cod_fornec = EXCLUDED.cod_fornec,
        nome_fornec = EXCLUDED.nome_fornec, ean = EXCLUDED.ean,
        qt_clientes = EXCLUDED.qt_clientes, qt_positivados = EXCLUDED.qt_positivados,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

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

    -- DIMS trans
    INSERT INTO farol.agg_trans_dims_mes AS t (empresa_id, ano, mes, dim, key, label)
    SELECT empresa_id, p_ano, p_mes, 'fornec', cod_fornec, COALESCE(MAX(nome_fornec),'')
        FROM _v_trans WHERE cod_fornec <> '' GROUP BY empresa_id, cod_fornec
    UNION ALL
    SELECT empresa_id, p_ano, p_mes, 'gerente', cod_gerente, COALESCE(MAX(nome_gerente),'')
        FROM _v_trans WHERE cod_gerente <> '' GROUP BY empresa_id, cod_gerente
    UNION ALL
    SELECT empresa_id, p_ano, p_mes, 'supervisor', cod_supervisor, COALESCE(MAX(nome_supervisor),'')
        FROM _v_trans WHERE cod_supervisor <> '' GROUP BY empresa_id, cod_supervisor
    UNION ALL
    SELECT empresa_id, p_ano, p_mes, 'rca', cod_rca, COALESCE(MAX(nome_rca),'')
        FROM _v_trans WHERE cod_rca <> '' GROUP BY empresa_id, cod_rca
    UNION ALL
    SELECT empresa_id, p_ano, p_mes, 'cli', cod_cli, COALESCE(MAX(nome_cli),'')
        FROM _v_trans WHERE cod_cli <> '' GROUP BY empresa_id, cod_cli
    UNION ALL
    SELECT empresa_id, p_ano, p_mes, 'uf', uf, uf
        FROM _v_trans WHERE uf <> '' GROUP BY empresa_id, uf
    UNION ALL
    SELECT empresa_id, p_ano, p_mes, 'empresa', empresa, empresa
        FROM _v_trans WHERE empresa <> '' GROUP BY empresa_id, empresa
    ON CONFLICT (ano, empresa_id, mes, dim, key) DO UPDATE SET label = EXCLUDED.label;

    -- Marketing trans
    INSERT INTO farol.agg_trans_mkt_cli_mes AS t
        (empresa_id, ano, mes, cnpj, cod_cli, nome_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cnpj,
        MAX(v.cod_cli), MAX(v.nome_cli),
        (CASE WHEN SUM(v.qt) > 0 THEN 1 ELSE 0 END)::INT,
        COUNT(DISTINCT v.cod_prod) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC,
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v_trans v WHERE v.cnpj <> ''
    GROUP BY v.empresa_id, v.cnpj
    ON CONFLICT (ano, empresa_id, mes, cnpj) DO UPDATE SET
        cod_cli = EXCLUDED.cod_cli, nome_cli = EXCLUDED.nome_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_trans_mkt_produto_mes AS t
        (empresa_id, ano, mes, cod_prod, nome_prod, cod_fornec, nome_fornec, ean,
         qt_clientes, qt_positivados, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_prod,
        MAX(v.nome_prod), MAX(v.cod_fornec), MAX(v.nome_fornec), MAX(v.ean),
        COUNT(DISTINCT NULLIF(v.cnpj, ''))::INT,
        COUNT(DISTINCT CASE WHEN v.qt > 0 THEN v.cnpj END)::INT,
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v_trans v WHERE v.cod_prod <> ''
    GROUP BY v.empresa_id, v.cod_prod
    ON CONFLICT (ano, empresa_id, mes, cod_prod) DO UPDATE SET
        nome_prod = EXCLUDED.nome_prod, cod_fornec = EXCLUDED.cod_fornec,
        nome_fornec = EXCLUDED.nome_fornec, ean = EXCLUDED.ean,
        qt_clientes = EXCLUDED.qt_clientes, qt_positivados = EXCLUDED.qt_positivados,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- ════════════════════════════════════════════════════════════════════════
    -- V05 (Por Fornecedor sob supervisor) — função auxiliar separada.
    -- Restaurando chamada perdida na 169 (vide cabeçalho desta migration).
    -- ════════════════════════════════════════════════════════════════════════
    PERFORM farol.upsert_aggs_mes_v05(p_empresa_id, p_ano, p_mes);

END;
$$ LANGUAGE plpgsql;

-- ════════════════════════════════════════════════════════════════════════════
-- PARTE 2 — Índices de mix que faltaram na 171 (gerência/RCA levavam 4-6s)
-- ════════════════════════════════════════════════════════════════════════════

CREATE INDEX IF NOT EXISTS idx_vf_mixtotal_gerente
  ON vendas_faturadas (empresa_id, data_faturamento, cod_gerente, cod_prod)
  WHERE qt > 0 AND cod_prod <> '' AND cod_gerente <> '';

CREATE INDEX IF NOT EXISTS idx_vf_mixtotal_rca
  ON vendas_faturadas (empresa_id, data_faturamento, cod_rca, cod_prod)
  WHERE qt > 0 AND cod_prod <> '' AND cod_rca <> '';

CREATE INDEX IF NOT EXISTS idx_vt_mixtotal_gerente
  ON vendas_transmitidas (empresa_id, data_transmissao, cod_gerente, cod_prod)
  WHERE qt > 0 AND cod_prod <> '' AND cod_gerente <> '';

CREATE INDEX IF NOT EXISTS idx_vt_mixtotal_rca
  ON vendas_transmitidas (empresa_id, data_transmissao, cod_rca, cod_prod)
  WHERE qt > 0 AND cod_prod <> '' AND cod_rca <> '';

-- ════════════════════════════════════════════════════════════════════════════
-- PARTE 3 — Re-popula APENAS as tabelas que a 169 deixou vazias.
-- dims hierárquicas: lidas das agg_* já populadas (instantâneo).
-- uf/empresa + mkt_produto: de vendas_* (scans inevitáveis, ~1-3 min).
-- mkt_cli: derivado de agg_*_v01_l4_mes (SKU pertence a 1 fornec → SUM(mix) ok).
-- ════════════════════════════════════════════════════════════════════════════

-- ── DIMS FAT (hierárquicas, das aggs) ────────────────────────────────────────
INSERT INTO farol.agg_fat_dims_mes (empresa_id, ano, mes, dim, key, label)
SELECT empresa_id, ano, mes, 'fornec', cod_fornec, COALESCE(MAX(nome_fornec),'')
  FROM farol.agg_fat_v01_l0_mes WHERE cod_fornec <> '' GROUP BY empresa_id, ano, mes, cod_fornec
UNION ALL
SELECT empresa_id, ano, mes, 'gerente', cod_gerente, COALESCE(MAX(nome_gerente),'')
  FROM farol.agg_fat_v03_l0_mes WHERE cod_gerente <> '' GROUP BY empresa_id, ano, mes, cod_gerente
UNION ALL
SELECT empresa_id, ano, mes, 'supervisor', cod_supervisor, COALESCE(MAX(nome_supervisor),'')
  FROM farol.agg_fat_v02_l0_mes WHERE cod_supervisor <> '' GROUP BY empresa_id, ano, mes, cod_supervisor
UNION ALL
SELECT empresa_id, ano, mes, 'rca', cod_rca, COALESCE(MAX(nome_rca),'')
  FROM farol.agg_fat_v04_l0_mes WHERE cod_rca <> '' GROUP BY empresa_id, ano, mes, cod_rca
UNION ALL
SELECT empresa_id, ano, mes, 'cli', cod_cli::text, COALESCE(MAX(nome_cli),'')
  FROM farol.agg_fat_v01_l4_mes WHERE cod_cli IS NOT NULL AND cod_cli::text <> ''
  GROUP BY empresa_id, ano, mes, cod_cli
ON CONFLICT (ano, empresa_id, mes, dim, key) DO UPDATE SET label = EXCLUDED.label;

-- ── DIMS FAT (uf/empresa, de vendas) ─────────────────────────────────────────
INSERT INTO farol.agg_fat_dims_mes (empresa_id, ano, mes, dim, key, label)
SELECT empresa_id, EXTRACT(YEAR FROM data_faturamento)::int, EXTRACT(MONTH FROM data_faturamento)::int,
       'uf', uf, uf
  FROM vendas_faturadas WHERE uf IS NOT NULL AND uf <> ''
 GROUP BY empresa_id, 2, 3, uf
UNION ALL
SELECT empresa_id, EXTRACT(YEAR FROM data_faturamento)::int, EXTRACT(MONTH FROM data_faturamento)::int,
       'empresa', empresa, empresa
  FROM vendas_faturadas WHERE empresa IS NOT NULL AND empresa <> ''
 GROUP BY empresa_id, 2, 3, empresa
ON CONFLICT (ano, empresa_id, mes, dim, key) DO UPDATE SET label = EXCLUDED.label;

-- ── DIMS TRANS (hierárquicas, das aggs) ──────────────────────────────────────
INSERT INTO farol.agg_trans_dims_mes (empresa_id, ano, mes, dim, key, label)
SELECT empresa_id, ano, mes, 'fornec', cod_fornec, COALESCE(MAX(nome_fornec),'')
  FROM farol.agg_trans_v01_l0_mes WHERE cod_fornec <> '' GROUP BY empresa_id, ano, mes, cod_fornec
UNION ALL
SELECT empresa_id, ano, mes, 'gerente', cod_gerente, COALESCE(MAX(nome_gerente),'')
  FROM farol.agg_trans_v03_l0_mes WHERE cod_gerente <> '' GROUP BY empresa_id, ano, mes, cod_gerente
UNION ALL
SELECT empresa_id, ano, mes, 'supervisor', cod_supervisor, COALESCE(MAX(nome_supervisor),'')
  FROM farol.agg_trans_v02_l0_mes WHERE cod_supervisor <> '' GROUP BY empresa_id, ano, mes, cod_supervisor
UNION ALL
SELECT empresa_id, ano, mes, 'rca', cod_rca, COALESCE(MAX(nome_rca),'')
  FROM farol.agg_trans_v04_l0_mes WHERE cod_rca <> '' GROUP BY empresa_id, ano, mes, cod_rca
UNION ALL
SELECT empresa_id, ano, mes, 'cli', cod_cli::text, COALESCE(MAX(nome_cli),'')
  FROM farol.agg_trans_v01_l4_mes WHERE cod_cli IS NOT NULL AND cod_cli::text <> ''
  GROUP BY empresa_id, ano, mes, cod_cli
ON CONFLICT (ano, empresa_id, mes, dim, key) DO UPDATE SET label = EXCLUDED.label;

-- ── DIMS TRANS (uf/empresa, de vendas) ───────────────────────────────────────
INSERT INTO farol.agg_trans_dims_mes (empresa_id, ano, mes, dim, key, label)
SELECT empresa_id, EXTRACT(YEAR FROM data_transmissao)::int, EXTRACT(MONTH FROM data_transmissao)::int,
       'uf', uf, uf
  FROM vendas_transmitidas WHERE uf IS NOT NULL AND uf <> ''
 GROUP BY empresa_id, 2, 3, uf
UNION ALL
SELECT empresa_id, EXTRACT(YEAR FROM data_transmissao)::int, EXTRACT(MONTH FROM data_transmissao)::int,
       'empresa', empresa, empresa
  FROM vendas_transmitidas WHERE empresa IS NOT NULL AND empresa <> ''
 GROUP BY empresa_id, 2, 3, empresa
ON CONFLICT (ano, empresa_id, mes, dim, key) DO UPDATE SET label = EXCLUDED.label;

-- ── MKT CLI (derivado das aggs v01_l4: positivado em qualquer fornec = MAX;
--    mix = SUM porque cada SKU pertence a um único fornecedor) ────────────────
INSERT INTO farol.agg_fat_mkt_cli_mes (empresa_id, ano, mes, cnpj, cod_cli, nome_cli, positivados, mix, pvenda, plucro, qt)
SELECT empresa_id, ano, mes, cnpj, MAX(cod_cli), MAX(nome_cli),
       MAX(positivados)::int, SUM(mix), SUM(pvenda), SUM(plucro), SUM(qt)
  FROM farol.agg_fat_v01_l4_mes WHERE cnpj <> ''
 GROUP BY empresa_id, ano, mes, cnpj
ON CONFLICT (ano, empresa_id, mes, cnpj) DO UPDATE SET
    cod_cli = EXCLUDED.cod_cli, nome_cli = EXCLUDED.nome_cli,
    positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
    pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

INSERT INTO farol.agg_trans_mkt_cli_mes (empresa_id, ano, mes, cnpj, cod_cli, nome_cli, positivados, mix, pvenda, plucro, qt)
SELECT empresa_id, ano, mes, cnpj, MAX(cod_cli), MAX(nome_cli),
       MAX(positivados)::int, SUM(mix), SUM(pvenda), SUM(plucro), SUM(qt)
  FROM farol.agg_trans_v01_l4_mes WHERE cnpj <> ''
 GROUP BY empresa_id, ano, mes, cnpj
ON CONFLICT (ano, empresa_id, mes, cnpj) DO UPDATE SET
    cod_cli = EXCLUDED.cod_cli, nome_cli = EXCLUDED.nome_cli,
    positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
    pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

-- ── MKT PRODUTO (de vendas_* — único caminho, aggs não preservam cod_prod) ───
INSERT INTO farol.agg_fat_mkt_produto_mes (empresa_id, ano, mes, cod_prod, nome_prod, cod_fornec, nome_fornec, ean, qt_clientes, qt_positivados, pvenda, plucro, qt)
SELECT empresa_id, EXTRACT(YEAR FROM data_faturamento)::int, EXTRACT(MONTH FROM data_faturamento)::int,
       cod_prod, MAX(nome_prod), MAX(cod_fornec), MAX(nome_fornec), MAX(ean),
       COUNT(DISTINCT NULLIF(cnpj,''))::INT,
       COUNT(DISTINCT CASE WHEN qt > 0 THEN cnpj END)::INT,
       SUM(pvenda), SUM(plucro), SUM(qt)
  FROM vendas_faturadas WHERE cod_prod <> ''
 GROUP BY empresa_id, 2, 3, cod_prod
ON CONFLICT (ano, empresa_id, mes, cod_prod) DO UPDATE SET
    nome_prod = EXCLUDED.nome_prod, cod_fornec = EXCLUDED.cod_fornec,
    nome_fornec = EXCLUDED.nome_fornec, ean = EXCLUDED.ean,
    qt_clientes = EXCLUDED.qt_clientes, qt_positivados = EXCLUDED.qt_positivados,
    pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

INSERT INTO farol.agg_trans_mkt_produto_mes (empresa_id, ano, mes, cod_prod, nome_prod, cod_fornec, nome_fornec, ean, qt_clientes, qt_positivados, pvenda, plucro, qt)
SELECT empresa_id, EXTRACT(YEAR FROM data_transmissao)::int, EXTRACT(MONTH FROM data_transmissao)::int,
       cod_prod, MAX(nome_prod), MAX(cod_fornec), MAX(nome_fornec), MAX(ean),
       COUNT(DISTINCT NULLIF(cnpj,''))::INT,
       COUNT(DISTINCT CASE WHEN qt > 0 THEN cnpj END)::INT,
       SUM(pvenda), SUM(plucro), SUM(qt)
  FROM vendas_transmitidas WHERE cod_prod <> ''
 GROUP BY empresa_id, 2, 3, cod_prod
ON CONFLICT (ano, empresa_id, mes, cod_prod) DO UPDATE SET
    nome_prod = EXCLUDED.nome_prod, cod_fornec = EXCLUDED.cod_fornec,
    nome_fornec = EXCLUDED.nome_fornec, ean = EXCLUDED.ean,
    qt_clientes = EXCLUDED.qt_clientes, qt_positivados = EXCLUDED.qt_positivados,
    pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

ANALYZE farol.agg_fat_dims_mes;
ANALYZE farol.agg_trans_dims_mes;
ANALYZE farol.agg_fat_mkt_cli_mes;
ANALYZE farol.agg_trans_mkt_cli_mes;
ANALYZE farol.agg_fat_mkt_produto_mes;
ANALYZE farol.agg_trans_mkt_produto_mes;
