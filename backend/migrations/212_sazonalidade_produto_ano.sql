-- 212_sazonalidade_produto_ano.sql
-- ════════════════════════════════════════════════════════════════════════════
-- SAZONALIDADE PRODUTO × FILIAL × ANO — persistida, consumida pelo próprio
-- Farol e pelo SmartPick (via /api/farol/sazonalidade-produto, mig futura no
-- handler Go). Substitui, do lado SmartPick, o uso da sazonalidade por Seção
-- (SazonalidadeSecaoAPIHandler) — grão mais fino, mais preciso (produtos de
-- perfil sazonal oposto dentro da mesma seção se cancelam na média hoje).
--
-- FONTE: farol.agg_fat_v12_l1_mes (mig 211), NÃO vendas_faturadas diretamente
-- — por isso upsert_sazonalidade_produto_ano é barata mesmo rodando todo dia
-- pro ano corrente (lê ~12 linhas já pré-agregadas por produto, não escaneia
-- a tabela de vendas inteira).
--
-- ÍNDICE POR QUANTIDADE, NÃO RECEITA: o consumo final é dimensionamento de
-- picking (espaço físico) — usar pvenda introduziria ruído de reajuste de
-- preço como falso pico. pvenda fica armazenado como contexto (relatórios
-- financeiros que preferirem esse eixo), mas o índice/pico é sobre qt.
--
-- THRESHOLD "É SAZONAL" MAIS RÍGIDO QUE O DE SEÇÃO: reusar cegamente o 1.5×
-- já usado no grão Seção seria ruído aqui — produtos individuais têm
-- variância mês a mês muito maior que uma seção inteira (que soma centenas de
-- produtos e cancela ruído). Um produto que vendeu 1 unidade em julho e 0 no
-- resto do ano teria índice=12 sem significado nenhum. Critério (constantes
-- nomeadas abaixo, fáceis de recalibrar depois de ver dado real):
--   indice_pico >= 2.0   AND   qt_total_ano >= 12   AND   meses_com_dado >= 3
-- O Farol já entrega o booleano `sazonal` pronto — os consumidores não
-- reimplementam a regra de corte.
--
-- ANO SEM HARDCODE: diferente de SazonalidadeSecaoAPIHandler (2025 fixo no
-- código), esta tabela persiste TODO ano incrementalmente (inclusive o
-- corrente). A resolução de "qual ano usar quando não informado" fica no
-- handler Go do endpoint (mig futura): MAX(ano) WHERE meses_com_dado=12 —
-- pula sozinho pro ano seguinte assim que ele fechar, sem deploy.
-- ════════════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS farol.agg_sazonalidade_produto_ano (
    empresa_id       UUID    NOT NULL,
    ano              INT     NOT NULL,
    empresa          TEXT    NOT NULL,   -- cod_filial
    cod_prod         TEXT    NOT NULL,
    nome_prod        TEXT    NOT NULL DEFAULT '',
    cod_depto        TEXT    NOT NULL DEFAULT '',
    cod_sec          TEXT    NOT NULL DEFAULT '',
    qt_total_ano     NUMERIC NOT NULL DEFAULT 0,
    pvenda_total_ano NUMERIC NOT NULL DEFAULT 0,
    meses_com_dado   INT     NOT NULL DEFAULT 0,  -- nº de meses distintos com qt>0 (cobertura)
    mes_pico         INT,                          -- 1-12; NULL se sem dado suficiente
    qt_mes_pico      NUMERIC NOT NULL DEFAULT 0,
    pvenda_mes_pico  NUMERIC NOT NULL DEFAULT 0,
    indice_pico      NUMERIC,                       -- qt_mes_pico / média mensal do ano
    sazonal          BOOLEAN NOT NULL DEFAULT FALSE, -- ver critério acima
    atualizado_em    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (ano, empresa_id, empresa, cod_prod)
) PARTITION BY RANGE (ano);

-- ────────────────────────────────────────────────────────────────────────────
-- agg_table_names() ganha a tabela nova.
-- ────────────────────────────────────────────────────────────────────────────

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
        -- Sazonalidade Produto×Filial×Ano (mig 212)
        'agg_sazonalidade_produto_ano',
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
        'agg_trans_mkt_cli_mes','agg_trans_mkt_produto_mes'
    ];
END;
$$ LANGUAGE plpgsql IMMUTABLE;

DO $$
DECLARE
    a INT;
BEGIN
    FOR a IN 2025..2027 LOOP
        PERFORM farol.create_agg_year_partitions(a);
    END LOOP;
END $$;

-- ────────────────────────────────────────────────────────────────────────────
-- farol.upsert_sazonalidade_produto_ano — grão ANO (não mês). Não entra no
-- loop upsertAggsMesParallel (que é por mês); chamada uma vez por ano
-- efetivamente tocado, reusando os maps anosTocados/anosVistos que o Go já
-- computa em cada um dos 3 pontos de disparo (import, refresh manual, carga
-- histórica) — ver farol_v2_import.go / farol_v2_api.go / jc_carga.go.
-- ────────────────────────────────────────────────────────────────────────────

CREATE OR REPLACE FUNCTION farol.upsert_sazonalidade_produto_ano(
    p_empresa_id UUID,
    p_ano        INT
) RETURNS VOID AS $$
DECLARE
    -- Parâmetros de calibragem do critério "é sazonal" — ver comentário no
    -- topo do arquivo. Primeira estimativa de engenharia, ajustável.
    c_indice_min NUMERIC := 2.0;
    c_qt_min_ano NUMERIC := 12;
    c_meses_min  INT     := 3;
BEGIN
    SET LOCAL work_mem = '128MB';

    -- Rollup por produto×filial sobre os 12 meses do ano (lê o agregado
    -- mensal, não vendas_faturadas — barato).
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
    GROUP BY empresa, cod_prod;

    -- Mês de pico (maior qt no ano) por produto×filial — só entre meses com
    -- venda de fato (qt>0), pra não "eleger" pico um mês vazio em produto sem
    -- dado nenhum. DISTINCT ON desempata por menor mês (mais estável).
    DROP TABLE IF EXISTS _saz_prod_pico;
    CREATE TEMP TABLE _saz_prod_pico ON COMMIT DROP AS
    SELECT DISTINCT ON (empresa, cod_prod)
        empresa, cod_prod,
        mes    AS mes_pico,
        qt     AS qt_mes_pico,
        pvenda AS pvenda_mes_pico
    FROM farol.agg_fat_v12_l1_mes
    WHERE empresa_id = p_empresa_id AND ano = p_ano AND qt > 0
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
