-- 213_sazonalidade_produto_filial_param.sql
-- ════════════════════════════════════════════════════════════════════════════
-- Parâmetro opcional p_empresa (cod_filial) em upsert_aggs_mes_v12 e
-- upsert_sazonalidade_produto_ano — DEFAULT NULL preserva 100% o
-- comportamento atual (processa todas as filiais da empresa) usado pelo
-- pipeline automático (upsertAggsMesParallel/upsertSazonalidadeProdutoAnos,
-- disparado após import/refresh/carga histórica — nunca passa esse argumento).
--
-- Motivação: tela admin nova "Configuração → Sazonalidade" (gestor_geral) pra
-- gerar sazonalidade sob demanda, escopada a UMA filial (a importação roda
-- automática de madrugada e não pode ser alterada — usuário pediu um jeito de
-- forçar/reprocessar uma filial específica sem esperar o próximo import).
-- CREATE OR REPLACE com parâmetro adicional é compatível com as chamadas
-- existentes de 3/2 argumentos (Postgres usa o DEFAULT quando omitido).
-- ════════════════════════════════════════════════════════════════════════════

CREATE OR REPLACE FUNCTION farol.upsert_aggs_mes_v12(
    p_empresa_id UUID,
    p_ano        INT,
    p_mes        INT,
    p_empresa    TEXT DEFAULT NULL
) RETURNS VOID AS $$
DECLARE
    p_ini DATE := make_date(p_ano, p_mes, 1);
    p_fim DATE := (p_ini + INTERVAL '1 month' - INTERVAL '1 day')::date;
BEGIN
    SET LOCAL work_mem = '256MB';

    DROP TABLE IF EXISTS _v12_fat;
    CREATE TEMP TABLE _v12_fat ON COMMIT DROP AS
    SELECT v.empresa_id, v.empresa, v.cod_prod, v.nome_prod, v.cod_depto, v.cod_sec,
           v.pvenda, v.plucro, v.qt
      FROM vendas_faturadas v
     WHERE v.empresa_id = p_empresa_id
       AND v.data_faturamento BETWEEN p_ini AND p_fim
       AND v.empresa <> '' AND v.cod_prod <> ''
       AND (p_empresa IS NULL OR v.empresa = p_empresa);
    CREATE INDEX ON _v12_fat (empresa, cod_prod);
    ANALYZE _v12_fat;

    INSERT INTO farol.agg_fat_v12_l1_mes AS t
        (empresa_id, ano, mes, empresa, cod_prod, nome_prod, cod_depto, cod_sec, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.empresa, v.cod_prod,
           MAX(v.nome_prod), MAX(v.cod_depto), MAX(v.cod_sec),
           SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
      FROM _v12_fat v
     GROUP BY v.empresa_id, v.empresa, v.cod_prod
    ON CONFLICT (ano, empresa_id, mes, empresa, cod_prod) DO UPDATE SET
        nome_prod = EXCLUDED.nome_prod, cod_depto = EXCLUDED.cod_depto, cod_sec = EXCLUDED.cod_sec,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    DROP TABLE _v12_fat;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION farol.upsert_sazonalidade_produto_ano(
    p_empresa_id UUID,
    p_ano        INT,
    p_empresa    TEXT DEFAULT NULL
) RETURNS VOID AS $$
DECLARE
    c_indice_min NUMERIC := 2.0;
    c_qt_min_ano NUMERIC := 12;
    c_meses_min  INT     := 3;
BEGIN
    SET LOCAL work_mem = '128MB';

    DROP TABLE IF EXISTS _saz_prod_ano;
    CREATE TEMP TABLE _saz_prod_ano ON COMMIT DROP AS
    SELECT
        empresa, cod_prod,
        MAX(nome_prod)                 AS nome_prod,
        MAX(cod_depto)                 AS cod_depto,
        MAX(cod_sec)                   AS cod_sec,
        SUM(qt)                        AS qt_total_ano,
        SUM(pvenda)                    AS pvenda_total_ano,
        COUNT(*) FILTER (WHERE qt > 0) AS meses_com_dado,
        AVG(qt) FILTER (WHERE qt > 0)  AS media_mensal
    FROM farol.agg_fat_v12_l1_mes
    WHERE empresa_id = p_empresa_id AND ano = p_ano
      AND (p_empresa IS NULL OR empresa = p_empresa)
    GROUP BY empresa, cod_prod;

    DROP TABLE IF EXISTS _saz_prod_pico;
    CREATE TEMP TABLE _saz_prod_pico ON COMMIT DROP AS
    SELECT DISTINCT ON (empresa, cod_prod)
        empresa, cod_prod,
        mes    AS mes_pico,
        qt     AS qt_mes_pico,
        pvenda AS pvenda_mes_pico
    FROM farol.agg_fat_v12_l1_mes
    WHERE empresa_id = p_empresa_id AND ano = p_ano AND qt > 0
      AND (p_empresa IS NULL OR empresa = p_empresa)
    ORDER BY empresa, cod_prod, qt DESC, mes ASC;

    CREATE INDEX ON _saz_prod_ano (empresa, cod_prod);
    CREATE INDEX ON _saz_prod_pico (empresa, cod_prod);
    ANALYZE _saz_prod_ano;
    ANALYZE _saz_prod_pico;

    INSERT INTO farol.agg_sazonalidade_produto_ano AS t
        (empresa_id, ano, empresa, cod_prod, nome_prod, cod_depto, cod_sec,
         qt_total_ano, pvenda_total_ano, meses_com_dado,
         mes_pico, qt_mes_pico, pvenda_mes_pico, indice_pico, sazonal, atualizado_em)
    SELECT
        p_empresa_id, p_ano, a.empresa, a.cod_prod, a.nome_prod, a.cod_depto, a.cod_sec,
        a.qt_total_ano, a.pvenda_total_ano, a.meses_com_dado,
        p.mes_pico,
        COALESCE(p.qt_mes_pico, 0),
        COALESCE(p.pvenda_mes_pico, 0),
        ROUND((p.qt_mes_pico / NULLIF(a.media_mensal, 0))::numeric, 3),
        COALESCE(
            (p.qt_mes_pico / NULLIF(a.media_mensal, 0)) >= c_indice_min
            AND a.qt_total_ano >= c_qt_min_ano
            AND a.meses_com_dado >= c_meses_min,
            FALSE
        ),
        now()
    FROM _saz_prod_ano a
    LEFT JOIN _saz_prod_pico p USING (empresa, cod_prod)
    ON CONFLICT (ano, empresa_id, empresa, cod_prod) DO UPDATE SET
        nome_prod = EXCLUDED.nome_prod, cod_depto = EXCLUDED.cod_depto, cod_sec = EXCLUDED.cod_sec,
        qt_total_ano = EXCLUDED.qt_total_ano, pvenda_total_ano = EXCLUDED.pvenda_total_ano,
        meses_com_dado = EXCLUDED.meses_com_dado,
        mes_pico = EXCLUDED.mes_pico, qt_mes_pico = EXCLUDED.qt_mes_pico, pvenda_mes_pico = EXCLUDED.pvenda_mes_pico,
        indice_pico = EXCLUDED.indice_pico, sazonal = EXCLUDED.sazonal,
        atualizado_em = EXCLUDED.atualizado_em;

    DROP TABLE _saz_prod_ano;
    DROP TABLE _saz_prod_pico;
END;
$$ LANGUAGE plpgsql;
