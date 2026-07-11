-- 186_fix_v06_v07_partitions.sql
-- ════════════════════════════════════════════════════════════════════════════
-- Fix descoberto em produção (11/07/2026) após import do novo layout falhar
-- em UPSERT V06/V07 com erro:
--   pq: no partition of relation "agg_fat_v06_l0_mes" found for row (23514)
--
-- CAUSA: a função farol.agg_table_names() (mig 167) não conhecia as tabelas
-- V06/V07 introduzidas nas migrations 183/184. Como as chamadas de
-- farol.create_agg_year_partitions() iteram sobre essa função, as partições
-- anuais das novas tabelas nunca foram criadas.
--
-- FIX: atualiza a função para incluir as 12 novas tabelas e recria partições
-- 2024-2027 para elas.
-- ════════════════════════════════════════════════════════════════════════════

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
        'agg_fat_dims_mes',
        'agg_fat_mkt_cli_mes','agg_fat_mkt_produto_mes',
        'agg_trans_v01_l0_mes','agg_trans_v01_l1_mes','agg_trans_v01_l2_mes','agg_trans_v01_l3_mes','agg_trans_v01_l4_mes',
        'agg_trans_v02_l0_mes','agg_trans_v02_l1_mes','agg_trans_v02_l2_mes','agg_trans_v02_l3_mes',
        'agg_trans_v03_l0_mes','agg_trans_v03_l1_mes','agg_trans_v03_l2_mes','agg_trans_v03_l3_mes',
        'agg_trans_v04_l0_mes','agg_trans_v04_l1_mes','agg_trans_v04_l2_mes',
        'agg_trans_v05_l0_mes','agg_trans_v05_l1_mes','agg_trans_v05_l2_mes','agg_trans_v05_l3_mes',
        'agg_trans_v06_l0_mes','agg_trans_v06_l1_mes','agg_trans_v06_l2_mes',
        'agg_trans_v07_l0_mes','agg_trans_v07_l1_mes','agg_trans_v07_l2_mes',
        'agg_trans_dims_mes',
        'agg_trans_mkt_cli_mes','agg_trans_mkt_produto_mes'
    ];
END;
$$ LANGUAGE plpgsql IMMUTABLE;

-- ─── Provisiona partições 2024-2027 para as novas tabelas ────────────────────
-- create_agg_year_partitions usa CREATE TABLE IF NOT EXISTS, então rodar de
-- novo pros anos existentes é NO-OP (só as partições das tabelas V06/V07 são
-- efetivamente criadas).
DO $$
DECLARE
    a INT;
BEGIN
    FOR a IN 2024..2027 LOOP
        PERFORM farol.create_agg_year_partitions(a);
    END LOOP;
END $$;
