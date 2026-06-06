-- 164_populate_agg_mes_existing.sql
-- Popula as tabelas farol.agg_*_mes criadas em 162 com os dados que já
-- estavam importados em vendas_faturadas / vendas_transmitidas antes da
-- 162 entrar. Sem isso o BI mostra 0 cards em qualquer ambiente que
-- importou vendas ANTES da 162 (ex.: produção que migrou nesta release).
--
-- IDEMPOTENTE: upsert_aggs_mes usa INSERT ... ON CONFLICT DO UPDATE,
-- então rodar mais de uma vez é seguro. A migration só roda uma vez de
-- qualquer forma (schema_migrations), mas o populate em si é safe.

DO $mig164$
DECLARE
    r RECORD;
    anos INT[] := ARRAY[]::INT[];
    a INT;
BEGIN
    -- 1. Garantir partições para cada ano com dados
    FOR a IN
        SELECT DISTINCT EXTRACT(YEAR FROM data_faturamento)::int AS ano FROM vendas_faturadas
        UNION
        SELECT DISTINCT EXTRACT(YEAR FROM data_transmissao)::int AS ano FROM vendas_transmitidas
    LOOP
        PERFORM farol.create_agg_year_partitions(a);
    END LOOP;

    -- 2. Upsert por (empresa, ano, mes) — cobre tudo que está importado
    FOR r IN
        SELECT DISTINCT empresa_id,
               EXTRACT(YEAR  FROM data_faturamento)::int AS ano,
               EXTRACT(MONTH FROM data_faturamento)::int AS mes
        FROM vendas_faturadas
        UNION
        SELECT DISTINCT empresa_id,
               EXTRACT(YEAR  FROM data_transmissao)::int AS ano,
               EXTRACT(MONTH FROM data_transmissao)::int AS mes
        FROM vendas_transmitidas
    LOOP
        PERFORM farol.upsert_aggs_mes(r.empresa_id, r.ano, r.mes);
        RAISE NOTICE 'agg_mes populado: empresa=% %-%', r.empresa_id, r.ano, r.mes;
    END LOOP;
END $mig164$;

-- ANALYZE para o planner saber que as tabelas têm dados
ANALYZE farol.agg_fat_v01_l0_mes;
ANALYZE farol.agg_fat_v01_l1_mes;
ANALYZE farol.agg_fat_v01_l2_mes;
ANALYZE farol.agg_fat_v01_l3_mes;
ANALYZE farol.agg_fat_v01_l4_mes;
ANALYZE farol.agg_fat_v02_l0_mes;
ANALYZE farol.agg_fat_v02_l1_mes;
ANALYZE farol.agg_fat_v02_l2_mes;
ANALYZE farol.agg_fat_v02_l3_mes;
ANALYZE farol.agg_fat_v03_l0_mes;
ANALYZE farol.agg_fat_v03_l1_mes;
ANALYZE farol.agg_fat_v03_l2_mes;
ANALYZE farol.agg_fat_v03_l3_mes;
ANALYZE farol.agg_fat_v04_l0_mes;
ANALYZE farol.agg_fat_v04_l1_mes;
ANALYZE farol.agg_fat_v04_l2_mes;
ANALYZE farol.agg_fat_dims_mes;
ANALYZE farol.agg_trans_v01_l0_mes;
ANALYZE farol.agg_trans_v01_l1_mes;
ANALYZE farol.agg_trans_v01_l2_mes;
ANALYZE farol.agg_trans_v01_l3_mes;
ANALYZE farol.agg_trans_v01_l4_mes;
ANALYZE farol.agg_trans_v02_l0_mes;
ANALYZE farol.agg_trans_v02_l1_mes;
ANALYZE farol.agg_trans_v02_l2_mes;
ANALYZE farol.agg_trans_v02_l3_mes;
ANALYZE farol.agg_trans_v03_l0_mes;
ANALYZE farol.agg_trans_v03_l1_mes;
ANALYZE farol.agg_trans_v03_l2_mes;
ANALYZE farol.agg_trans_v03_l3_mes;
ANALYZE farol.agg_trans_v04_l0_mes;
ANALYZE farol.agg_trans_v04_l1_mes;
ANALYZE farol.agg_trans_v04_l2_mes;
ANALYZE farol.agg_trans_dims_mes;
