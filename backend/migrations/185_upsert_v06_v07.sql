-- 185_upsert_v06_v07.sql
-- ════════════════════════════════════════════════════════════════════════════
-- Cria duas funções auxiliares que populam as agg tables da V06 e V07
-- (introduzidas nas migrations 183 e 184).
--
-- Padrão: espelha farol.upsert_aggs_mes_v05 (função auxiliar chamada pela
-- upsert_aggs_mes principal, ver 173). Isso evita ter que reescrever a
-- função-mãe de 800 linhas — o worker Go chama estas duas em sequência
-- após a upsert_aggs_mes principal (ajustado em farol_v2_api.go).
--
-- Cada função:
--   1. Cria uma temp table com as colunas necessárias para o cálculo
--   2. Faz INSERT ... ON CONFLICT DO UPDATE para cada nível (L0, L1, L2)
--   3. Sem métricas de positivação (só pvenda, plucro, qt)
--
-- Regra para o nome da rede (nome_cliprinc):
--   COALESCE(NULLIF(MAX(v.fantasia), ''), MAX(v.nome_cli))
--   → prefere o nome fantasia; se não houver, usa razão social do cliente.
-- ════════════════════════════════════════════════════════════════════════════


-- ═══════════════════════════════════════════════════════════════════════════
-- V06 "Por Rede" — cod_cliprinc → cod_fornec → cod_cli → cod_prod
-- ═══════════════════════════════════════════════════════════════════════════
CREATE OR REPLACE FUNCTION farol.upsert_aggs_mes_v06(
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
    DROP TABLE IF EXISTS _v06_fat;
    CREATE TEMP TABLE _v06_fat ON COMMIT DROP AS
    SELECT
        v.empresa_id,
        v.cod_cliprinc,
        v.cod_fornec, v.nome_fornec,
        v.cnpj, v.cod_cli, v.nome_cli, v.fantasia,
        v.pvenda, v.plucro, v.qt
    FROM vendas_faturadas v
    WHERE v.empresa_id = p_empresa_id
      AND v.data_faturamento BETWEEN p_ini AND p_fim
      AND v.cod_cliprinc <> '';

    -- L0: por rede
    INSERT INTO farol.agg_fat_v06_l0_mes AS t
        (empresa_id, ano, mes, cod_cliprinc, nome_cliprinc, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_cliprinc,
        COALESCE(NULLIF(MAX(v.fantasia), ''), MAX(v.nome_cli)),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v06_fat v
    GROUP BY v.empresa_id, v.cod_cliprinc
    ON CONFLICT (ano, empresa_id, mes, cod_cliprinc) DO UPDATE SET
        nome_cliprinc = EXCLUDED.nome_cliprinc,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- L1: por rede + fornecedor
    INSERT INTO farol.agg_fat_v06_l1_mes AS t
        (empresa_id, ano, mes, cod_cliprinc, cod_fornec, nome_fornec, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_cliprinc, v.cod_fornec, MAX(v.nome_fornec),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v06_fat v
    WHERE v.cod_fornec <> ''
    GROUP BY v.empresa_id, v.cod_cliprinc, v.cod_fornec
    ON CONFLICT (ano, empresa_id, mes, cod_cliprinc, cod_fornec) DO UPDATE SET
        nome_fornec = EXCLUDED.nome_fornec,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- L2: por rede + fornecedor + cliente (chave cnpj)
    INSERT INTO farol.agg_fat_v06_l2_mes AS t
        (empresa_id, ano, mes, cod_cliprinc, cod_fornec, cnpj, cod_cli, nome_cli, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_cliprinc, v.cod_fornec,
        v.cnpj, MAX(v.cod_cli), MAX(v.nome_cli),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v06_fat v
    WHERE v.cod_fornec <> '' AND v.cnpj <> ''
    GROUP BY v.empresa_id, v.cod_cliprinc, v.cod_fornec, v.cnpj
    ON CONFLICT (ano, empresa_id, mes, cod_cliprinc, cod_fornec, cnpj) DO UPDATE SET
        cod_cli = EXCLUDED.cod_cli, nome_cli = EXCLUDED.nome_cli,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- ─── TRANSMITIDO ─────────────────────────────────────────────────────────
    DROP TABLE IF EXISTS _v06_trans;
    CREATE TEMP TABLE _v06_trans ON COMMIT DROP AS
    SELECT
        v.empresa_id,
        v.cod_cliprinc,
        v.cod_fornec, v.nome_fornec,
        v.cnpj, v.cod_cli, v.nome_cli, v.fantasia,
        v.pvenda, v.plucro, v.qt
    FROM vendas_transmitidas v
    WHERE v.empresa_id = p_empresa_id
      AND v.data_transmissao BETWEEN p_ini AND p_fim
      AND v.cod_cliprinc <> '';

    INSERT INTO farol.agg_trans_v06_l0_mes AS t
        (empresa_id, ano, mes, cod_cliprinc, nome_cliprinc, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_cliprinc,
        COALESCE(NULLIF(MAX(v.fantasia), ''), MAX(v.nome_cli)),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v06_trans v
    GROUP BY v.empresa_id, v.cod_cliprinc
    ON CONFLICT (ano, empresa_id, mes, cod_cliprinc) DO UPDATE SET
        nome_cliprinc = EXCLUDED.nome_cliprinc,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_trans_v06_l1_mes AS t
        (empresa_id, ano, mes, cod_cliprinc, cod_fornec, nome_fornec, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_cliprinc, v.cod_fornec, MAX(v.nome_fornec),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v06_trans v
    WHERE v.cod_fornec <> ''
    GROUP BY v.empresa_id, v.cod_cliprinc, v.cod_fornec
    ON CONFLICT (ano, empresa_id, mes, cod_cliprinc, cod_fornec) DO UPDATE SET
        nome_fornec = EXCLUDED.nome_fornec,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_trans_v06_l2_mes AS t
        (empresa_id, ano, mes, cod_cliprinc, cod_fornec, cnpj, cod_cli, nome_cli, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_cliprinc, v.cod_fornec,
        v.cnpj, MAX(v.cod_cli), MAX(v.nome_cli),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v06_trans v
    WHERE v.cod_fornec <> '' AND v.cnpj <> ''
    GROUP BY v.empresa_id, v.cod_cliprinc, v.cod_fornec, v.cnpj
    ON CONFLICT (ano, empresa_id, mes, cod_cliprinc, cod_fornec, cnpj) DO UPDATE SET
        cod_cli = EXCLUDED.cod_cli, nome_cli = EXCLUDED.nome_cli,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

END;
$$ LANGUAGE plpgsql;


-- ═══════════════════════════════════════════════════════════════════════════
-- V07 "Por Departamento" — cod_depto → cod_sec → cod_categoria → cod_prod
-- ═══════════════════════════════════════════════════════════════════════════
CREATE OR REPLACE FUNCTION farol.upsert_aggs_mes_v07(
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
    DROP TABLE IF EXISTS _v07_fat;
    CREATE TEMP TABLE _v07_fat ON COMMIT DROP AS
    SELECT
        v.empresa_id,
        v.cod_depto, v.depto,
        v.cod_sec, v.secao,
        v.cod_categoria, v.categoria,
        v.pvenda, v.plucro, v.qt
    FROM vendas_faturadas v
    WHERE v.empresa_id = p_empresa_id
      AND v.data_faturamento BETWEEN p_ini AND p_fim
      AND v.cod_depto <> '';

    -- L0: por departamento
    INSERT INTO farol.agg_fat_v07_l0_mes AS t
        (empresa_id, ano, mes, cod_depto, depto, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_depto, MAX(v.depto),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v07_fat v
    GROUP BY v.empresa_id, v.cod_depto
    ON CONFLICT (ano, empresa_id, mes, cod_depto) DO UPDATE SET
        depto = EXCLUDED.depto,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- L1: por departamento + seção
    INSERT INTO farol.agg_fat_v07_l1_mes AS t
        (empresa_id, ano, mes, cod_depto, cod_sec, secao, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_depto, v.cod_sec, MAX(v.secao),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v07_fat v
    WHERE v.cod_sec <> ''
    GROUP BY v.empresa_id, v.cod_depto, v.cod_sec
    ON CONFLICT (ano, empresa_id, mes, cod_depto, cod_sec) DO UPDATE SET
        secao = EXCLUDED.secao,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- L2: por departamento + seção + categoria
    INSERT INTO farol.agg_fat_v07_l2_mes AS t
        (empresa_id, ano, mes, cod_depto, cod_sec, cod_categoria, categoria, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_depto, v.cod_sec, v.cod_categoria, MAX(v.categoria),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v07_fat v
    WHERE v.cod_sec <> '' AND v.cod_categoria <> ''
    GROUP BY v.empresa_id, v.cod_depto, v.cod_sec, v.cod_categoria
    ON CONFLICT (ano, empresa_id, mes, cod_depto, cod_sec, cod_categoria) DO UPDATE SET
        categoria = EXCLUDED.categoria,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- ─── TRANSMITIDO ─────────────────────────────────────────────────────────
    DROP TABLE IF EXISTS _v07_trans;
    CREATE TEMP TABLE _v07_trans ON COMMIT DROP AS
    SELECT
        v.empresa_id,
        v.cod_depto, v.depto,
        v.cod_sec, v.secao,
        v.cod_categoria, v.categoria,
        v.pvenda, v.plucro, v.qt
    FROM vendas_transmitidas v
    WHERE v.empresa_id = p_empresa_id
      AND v.data_transmissao BETWEEN p_ini AND p_fim
      AND v.cod_depto <> '';

    INSERT INTO farol.agg_trans_v07_l0_mes AS t
        (empresa_id, ano, mes, cod_depto, depto, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_depto, MAX(v.depto),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v07_trans v
    GROUP BY v.empresa_id, v.cod_depto
    ON CONFLICT (ano, empresa_id, mes, cod_depto) DO UPDATE SET
        depto = EXCLUDED.depto,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_trans_v07_l1_mes AS t
        (empresa_id, ano, mes, cod_depto, cod_sec, secao, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_depto, v.cod_sec, MAX(v.secao),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v07_trans v
    WHERE v.cod_sec <> ''
    GROUP BY v.empresa_id, v.cod_depto, v.cod_sec
    ON CONFLICT (ano, empresa_id, mes, cod_depto, cod_sec) DO UPDATE SET
        secao = EXCLUDED.secao,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_trans_v07_l2_mes AS t
        (empresa_id, ano, mes, cod_depto, cod_sec, cod_categoria, categoria, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_depto, v.cod_sec, v.cod_categoria, MAX(v.categoria),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v07_trans v
    WHERE v.cod_sec <> '' AND v.cod_categoria <> ''
    GROUP BY v.empresa_id, v.cod_depto, v.cod_sec, v.cod_categoria
    ON CONFLICT (ano, empresa_id, mes, cod_depto, cod_sec, cod_categoria) DO UPDATE SET
        categoria = EXCLUDED.categoria,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION farol.upsert_aggs_mes_v06(UUID, INT, INT) IS 'Popula agg_fat/trans_v06_l0/l1/l2_mes para um empresa/ano/mes. Chamada pelo backend Go após farol.upsert_aggs_mes principal.';
COMMENT ON FUNCTION farol.upsert_aggs_mes_v07(UUID, INT, INT) IS 'Popula agg_fat/trans_v07_l0/l1/l2_mes. Chamada pelo backend Go após farol.upsert_aggs_mes principal.';
