-- 190_upsert_venda_liquida.sql
-- ════════════════════════════════════════════════════════════════════════════
-- VENDA LÍQUIDA — Fase 2 (população). Ver spec-venda-liquida-composicao.md
--
-- Preenche as colunas adicionadas na mig 189 (liquido, pv_bonif, pv_transf,
-- pv_remessa, pv_devol, pv_cancel) em TODAS as 26 agg_fat_*_mes, para um mês.
--
-- ABORDAGEM (D2 = função separada, aprovado 2026-07-21): NÃO reescreve o
-- upsert principal de 920 linhas. Monta um temp unificado _liq_all =
-- vendas_faturadas (evento='') + vendas_ccd (DEVOLVIDO/CANCELADO) e faz um
-- UPDATE por nível, gerado dinamicamente a partir das chaves de cada tabela.
-- Custo: uma passada extra sobre o mês (mitigada por índices no temp). O Go
-- chama esta função logo após upsert_aggs_mes/_v06/_v07 (upsertAggsMesParallel).
--
--   liquido    = Σ pvenda (tipo_venda ∈ {1,4,7,8,9,11,14,20}) − devol − cancel
--   pv_bonif   = Σ pvenda (tipo_venda = '5')
--   pv_transf  = Σ pvenda (tipo_venda = '10')
--   pv_remessa = Σ pvenda (tipo_venda = '13')
--   pv_devol   = Σ pvenda (vendas_ccd evento='DEVOLVIDO')
--   pv_cancel  = Σ pvenda (vendas_ccd evento='CANCELADO')
--
-- pvenda (bruto) NÃO é tocado. Toda agg_fat_* tem um row por chave com dados
-- faturados no mês → o UPDATE casa (venda_real pode ser 0 se o nó só teve
-- bonificação; a chave existe). Idempotente (SET, recomputado do temp).
-- ════════════════════════════════════════════════════════════════════════════

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
    DROP TABLE IF EXISTS _liq_all;
    CREATE TEMP TABLE _liq_all ON COMMIT DROP AS
    SELECT empresa_id, cod_fornec, cod_gerente, cod_supervisor, cod_rca, cnpj,
           cod_cliprinc, cod_depto, cod_sec, cod_categoria,
           tipo_venda, ''::text AS evento, pvenda
      FROM vendas_faturadas
     WHERE empresa_id = p_empresa_id AND data_faturamento BETWEEN p_ini AND p_fim
    UNION ALL
    SELECT empresa_id, cod_fornec, cod_gerente, cod_supervisor, cod_rca, cnpj,
           cod_cliprinc, cod_depto, cod_sec, cod_categoria,
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
    ANALYZE _liq_all;

    -- (tabela, chaves) de cada um dos 26 níveis faturados.
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
            ('agg_fat_v07_l2_mes', ARRAY['cod_depto','cod_sec','cod_categoria'])
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
