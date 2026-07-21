-- 189_venda_liquida_cols.sql
-- ════════════════════════════════════════════════════════════════════════════
-- VENDA LÍQUIDA — Fase 1 (schema + captura). Ver spec-venda-liquida-composicao.md
--
-- Conceito (D4 resolvido 2026-07-21): pvenda PERMANECE bruto (preserva
-- objetivos/KPIs). O Líquido entra como COLUNA NOVA, e cada categoria excluída
-- ganha sua própria coluna de valor para os botões "Incluir X" somarem de volta.
--
--   Líquido = Σ pvenda (tipo_venda ∈ {1,4,7,8,9,11,14,20})  − devoluções − canceladas
--   Venda real:  1,4,7,8,9,11,14,20    Exclusões: 5 Bonif, 10 Transf, 13 Remessa
--   Eventos que subtraem: DEVOLVIDO, CANCELADO (de vendas_ccd)
--
--   Total exibido = liquido + (pv_bonif|pv_transf|pv_remessa|pv_devol|pv_cancel
--   se o botão correspondente estiver ligado). Todos ligados → = pvenda (bruto).
--
-- ESTA MIGRATION (só schema — colunas ficam 0 até a Fase 2 popular):
--   1. vendas_ccd ADD tipo_venda (import já grava a partir desta versão).
--   2. As 26 agg_fat_*_mes ganham: liquido, pv_bonif, pv_transf, pv_remessa,
--      pv_devol, pv_cancel. ADD COLUMN na tabela-mãe particionada propaga p/
--      partições. agg_trans NÃO muda (Líquido é conceito do faturado).
--
-- IDEMPOTENTE: ADD COLUMN IF NOT EXISTS.
-- ════════════════════════════════════════════════════════════════════════════

ALTER TABLE vendas_ccd
    ADD COLUMN IF NOT EXISTS tipo_venda TEXT NOT NULL DEFAULT '';

DO $$
DECLARE
    t    TEXT;
    col  TEXT;
    cols TEXT[] := ARRAY['liquido','pv_bonif','pv_transf','pv_remessa','pv_devol','pv_cancel'];
    tbls TEXT[] := ARRAY[
        'agg_fat_v01_l0_mes','agg_fat_v01_l1_mes','agg_fat_v01_l2_mes','agg_fat_v01_l3_mes','agg_fat_v01_l4_mes',
        'agg_fat_v02_l0_mes','agg_fat_v02_l1_mes','agg_fat_v02_l2_mes','agg_fat_v02_l3_mes',
        'agg_fat_v03_l0_mes','agg_fat_v03_l1_mes','agg_fat_v03_l2_mes','agg_fat_v03_l3_mes',
        'agg_fat_v04_l0_mes','agg_fat_v04_l1_mes','agg_fat_v04_l2_mes',
        'agg_fat_v05_l0_mes','agg_fat_v05_l1_mes','agg_fat_v05_l2_mes','agg_fat_v05_l3_mes',
        'agg_fat_v06_l0_mes','agg_fat_v06_l1_mes','agg_fat_v06_l2_mes',
        'agg_fat_v07_l0_mes','agg_fat_v07_l1_mes','agg_fat_v07_l2_mes'
    ];
BEGIN
    FOREACH t IN ARRAY tbls LOOP
        IF to_regclass('farol.' || t) IS NULL THEN
            RAISE NOTICE 'farol.% inexistente — pulando', t;
            CONTINUE;
        END IF;
        FOREACH col IN ARRAY cols LOOP
            EXECUTE format('ALTER TABLE farol.%I ADD COLUMN IF NOT EXISTS %I NUMERIC NOT NULL DEFAULT 0', t, col);
        END LOOP;
    END LOOP;
END $$;
