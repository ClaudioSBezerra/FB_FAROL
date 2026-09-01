-- 211_agg_v12_produto_filial_mes.sql
-- ════════════════════════════════════════════════════════════════════════════
-- AGG V12 — PRODUTO × FILIAL × MÊS (fat only)
--
-- Primeiro agregado persistido em grão PRODUTO. A V07 (mig 184) deliberadamente
-- não persistiu o nível L3/Produto por ser "pesado demais" — mas aquela decisão
-- foi tomada no contexto de drill-down interativo (usuário navegando ao vivo,
-- qualquer combinação de filtros). Aqui o caso é bem mais restrito: um rollup
-- fixo (produto×filial×mês → ano), não um cubo OLAP navegável, disparado só
-- como job assíncrono (pós-import/refresh), nunca como leitura ao vivo por
-- request. Precedente de que grão-produto mensal não é proibitivo:
-- agg_fat_mkt_produto_mes (mig 165) já roda em produção — só falta o eixo
-- filial, que é exatamente o que esta tabela adiciona.
--
-- MOTIVAÇÃO: base para farol.agg_sazonalidade_produto_ano (mig 212) — "qual o
-- mês de maior giro de cada produto, por filial, no ano" só pode ser respondido
-- olhando o ano inteiro, mas o pipeline de upsert roda por (ano,mes). Sem este
-- nível intermediário, calcular sazonalidade exigiria reagregar o ano inteiro
-- de vendas_faturadas toda vez que qualquer mês daquele ano for tocado — todo
-- dia, para o ano corrente, indefinidamente. Com este nível mensal incremental,
-- o trabalho pesado (varrer vendas_faturadas) fica restrito a UM mês por
-- chamada — mesma classe de custo de upsert_aggs_mes_v10_v11 — e o rollup
-- anual (mig 212) lê só ~12 linhas já pequenas por (filial, produto).
--
-- Sem espelho _trans: sazonalidade deve refletir venda FECHADA/faturada, não
-- trânsito — mesma fonte que farol_api_produtos_faturados.go/
-- SazonalidadeSecaoAPIHandler já usa hoje (só vendas_faturadas).
-- ════════════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS farol.agg_fat_v12_l1_mes (
    empresa_id  UUID    NOT NULL,
    ano         INT     NOT NULL,
    mes         INT     NOT NULL,
    empresa     TEXT    NOT NULL,   -- cod_filial (mesma coluna usada em v10/v11)
    cod_prod    TEXT    NOT NULL,
    nome_prod   TEXT    NOT NULL DEFAULT '',
    cod_depto   TEXT    NOT NULL DEFAULT '',
    cod_sec     TEXT    NOT NULL DEFAULT '',
    pvenda      NUMERIC NOT NULL DEFAULT 0,
    plucro      NUMERIC NOT NULL DEFAULT 0,
    qt          NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, empresa, cod_prod)
) PARTITION BY RANGE (ano);

-- ────────────────────────────────────────────────────────────────────────────
-- agg_table_names() ganha a tabela nova (partições de anos futuros e drop de
-- retenção passam a cobri-la — mig 162/186).
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
        'agg_fat_v11_l1_mes','agg_fat_v11_l2_mes','agg_fat_v11_l3_mes','agg_fat_v11_l4_mes','agg_fat_v11_l5_mes',
        'agg_fat_dims_mes',
        'agg_fat_mkt_cli_mes','agg_fat_mkt_produto_mes',
        -- V12 (mig 211) — Produto × Filial, base pra sazonalidade (mig 212)
        'agg_fat_v12_l1_mes',
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

-- ────────────────────────────────────────────────────────────────────────────
-- Partições 2025-2027 da tabela nova (idempotente; anos existentes das demais
-- tabelas são NO-OP via IF NOT EXISTS).
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
-- farol.upsert_aggs_mes_v12 — mesmo padrão de upsert_aggs_mes_v10_v11 (mig
-- 199), grão adicional Produto. Chamada pelo Go (upsertAggsMesParallel) logo
-- após v10/v11.
-- ────────────────────────────────────────────────────────────────────────────

CREATE OR REPLACE FUNCTION farol.upsert_aggs_mes_v12(
    p_empresa_id UUID,
    p_ano        INT,
    p_mes        INT
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
       AND v.empresa <> '' AND v.cod_prod <> '';
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
