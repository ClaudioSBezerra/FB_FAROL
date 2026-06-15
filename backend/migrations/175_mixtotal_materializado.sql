-- Migration 175 — P1: materializar mix_total (universo de SKUs) nas agg
--
-- O "Mix X de Y" precisa do Y = COUNT(DISTINCT cod_prod) por grupo. Hoje isso é
-- calculado na LEITURA (queryMixTotal varre vendas_* → 15-21s no YTD). Aqui
-- passamos a calcular na CONSOLIDAÇÃO e guardar na coluna mix_total → leitura
-- vira SELECT da coluna (~150ms).
--
-- ABORDAGEM SEGURA: NÃO mexe na função upsert_aggs_mes (a que mais deu trabalho).
-- Cria uma função isolada upsert_mixtotal_mes que lê de vendas_* (usando os
-- índices idx_v[ft]_mixtotal_*) e atualiza só a coluna mix_total. É chamada:
--   • pelo repopulate ao final desta migration (backfill dos meses existentes)
--   • pelo backend (RefreshViews) após consolidar os meses do import
--
-- Agregação multi-mês no read = MAX(mix_total) ("maior portfólio mensal");
-- mês único continua exato. (decisão do gestor, 2026-06-15)
--
-- Níveis materializados = os exibidos no painel (não-folha) de V01/V02/V03/V05.
-- V04, folhas (cli/cnpj) e cod_prod ficam com mix_total=0 (frontend só mostra
-- "/ Y" quando >0).

-- ════════════════════════════════════════════════════════════════════════════
-- PARTE 1 — Coluna mix_total
-- ════════════════════════════════════════════════════════════════════════════
ALTER TABLE farol.agg_fat_v01_l0_mes   ADD COLUMN IF NOT EXISTS mix_total INT NOT NULL DEFAULT 0;
ALTER TABLE farol.agg_fat_v01_l1_mes   ADD COLUMN IF NOT EXISTS mix_total INT NOT NULL DEFAULT 0;
ALTER TABLE farol.agg_fat_v01_l2_mes   ADD COLUMN IF NOT EXISTS mix_total INT NOT NULL DEFAULT 0;
ALTER TABLE farol.agg_fat_v01_l3_mes   ADD COLUMN IF NOT EXISTS mix_total INT NOT NULL DEFAULT 0;
ALTER TABLE farol.agg_fat_v02_l0_mes   ADD COLUMN IF NOT EXISTS mix_total INT NOT NULL DEFAULT 0;
ALTER TABLE farol.agg_fat_v02_l1_mes   ADD COLUMN IF NOT EXISTS mix_total INT NOT NULL DEFAULT 0;
ALTER TABLE farol.agg_fat_v02_l2_mes   ADD COLUMN IF NOT EXISTS mix_total INT NOT NULL DEFAULT 0;
ALTER TABLE farol.agg_fat_v03_l0_mes   ADD COLUMN IF NOT EXISTS mix_total INT NOT NULL DEFAULT 0;
ALTER TABLE farol.agg_fat_v03_l1_mes   ADD COLUMN IF NOT EXISTS mix_total INT NOT NULL DEFAULT 0;
ALTER TABLE farol.agg_fat_v03_l2_mes   ADD COLUMN IF NOT EXISTS mix_total INT NOT NULL DEFAULT 0;
ALTER TABLE farol.agg_fat_v05_l0_mes   ADD COLUMN IF NOT EXISTS mix_total INT NOT NULL DEFAULT 0;
ALTER TABLE farol.agg_fat_v05_l1_mes   ADD COLUMN IF NOT EXISTS mix_total INT NOT NULL DEFAULT 0;
ALTER TABLE farol.agg_fat_v05_l2_mes   ADD COLUMN IF NOT EXISTS mix_total INT NOT NULL DEFAULT 0;
ALTER TABLE farol.agg_trans_v01_l0_mes ADD COLUMN IF NOT EXISTS mix_total INT NOT NULL DEFAULT 0;
ALTER TABLE farol.agg_trans_v01_l1_mes ADD COLUMN IF NOT EXISTS mix_total INT NOT NULL DEFAULT 0;
ALTER TABLE farol.agg_trans_v01_l2_mes ADD COLUMN IF NOT EXISTS mix_total INT NOT NULL DEFAULT 0;
ALTER TABLE farol.agg_trans_v01_l3_mes ADD COLUMN IF NOT EXISTS mix_total INT NOT NULL DEFAULT 0;
ALTER TABLE farol.agg_trans_v02_l0_mes ADD COLUMN IF NOT EXISTS mix_total INT NOT NULL DEFAULT 0;
ALTER TABLE farol.agg_trans_v02_l1_mes ADD COLUMN IF NOT EXISTS mix_total INT NOT NULL DEFAULT 0;
ALTER TABLE farol.agg_trans_v02_l2_mes ADD COLUMN IF NOT EXISTS mix_total INT NOT NULL DEFAULT 0;
ALTER TABLE farol.agg_trans_v03_l0_mes ADD COLUMN IF NOT EXISTS mix_total INT NOT NULL DEFAULT 0;
ALTER TABLE farol.agg_trans_v03_l1_mes ADD COLUMN IF NOT EXISTS mix_total INT NOT NULL DEFAULT 0;
ALTER TABLE farol.agg_trans_v03_l2_mes ADD COLUMN IF NOT EXISTS mix_total INT NOT NULL DEFAULT 0;
ALTER TABLE farol.agg_trans_v05_l0_mes ADD COLUMN IF NOT EXISTS mix_total INT NOT NULL DEFAULT 0;
ALTER TABLE farol.agg_trans_v05_l1_mes ADD COLUMN IF NOT EXISTS mix_total INT NOT NULL DEFAULT 0;
ALTER TABLE farol.agg_trans_v05_l2_mes ADD COLUMN IF NOT EXISTS mix_total INT NOT NULL DEFAULT 0;

-- ════════════════════════════════════════════════════════════════════════════
-- PARTE 2 — Função isolada upsert_mixtotal_mes
-- ════════════════════════════════════════════════════════════════════════════
CREATE OR REPLACE FUNCTION farol.upsert_mixtotal_mes(
    p_empresa_id UUID, p_ano INT, p_mes INT
) RETURNS VOID AS $$
DECLARE
    p_ini DATE := make_date(p_ano, p_mes, 1);
    p_fim DATE := (make_date(p_ano, p_mes, 1) + INTERVAL '1 month' - INTERVAL '1 day')::date;
BEGIN
    -- ─────────── FATURADO (vendas_faturadas / data_faturamento) ───────────
    UPDATE farol.agg_fat_v01_l0_mes t SET mix_total = s.mt FROM (
        SELECT cod_fornec, COUNT(DISTINCT cod_prod)::int mt FROM vendas_faturadas
        WHERE empresa_id=p_empresa_id AND data_faturamento BETWEEN p_ini AND p_fim
          AND qt>0 AND cod_prod<>'' AND cod_fornec<>'' GROUP BY cod_fornec
    ) s WHERE t.empresa_id=p_empresa_id AND t.ano=p_ano AND t.mes=p_mes AND t.cod_fornec=s.cod_fornec;

    UPDATE farol.agg_fat_v01_l1_mes t SET mix_total = s.mt FROM (
        SELECT cod_fornec, cod_gerente, COUNT(DISTINCT cod_prod)::int mt FROM vendas_faturadas
        WHERE empresa_id=p_empresa_id AND data_faturamento BETWEEN p_ini AND p_fim
          AND qt>0 AND cod_prod<>'' AND cod_fornec<>'' AND cod_gerente<>'' GROUP BY cod_fornec, cod_gerente
    ) s WHERE t.empresa_id=p_empresa_id AND t.ano=p_ano AND t.mes=p_mes AND t.cod_fornec=s.cod_fornec AND t.cod_gerente=s.cod_gerente;

    UPDATE farol.agg_fat_v01_l2_mes t SET mix_total = s.mt FROM (
        SELECT cod_fornec, cod_gerente, cod_supervisor, COUNT(DISTINCT cod_prod)::int mt FROM vendas_faturadas
        WHERE empresa_id=p_empresa_id AND data_faturamento BETWEEN p_ini AND p_fim
          AND qt>0 AND cod_prod<>'' AND cod_fornec<>'' AND cod_gerente<>'' AND cod_supervisor<>'' GROUP BY cod_fornec, cod_gerente, cod_supervisor
    ) s WHERE t.empresa_id=p_empresa_id AND t.ano=p_ano AND t.mes=p_mes AND t.cod_fornec=s.cod_fornec AND t.cod_gerente=s.cod_gerente AND t.cod_supervisor=s.cod_supervisor;

    UPDATE farol.agg_fat_v01_l3_mes t SET mix_total = s.mt FROM (
        SELECT cod_fornec, cod_gerente, cod_supervisor, cod_rca, COUNT(DISTINCT cod_prod)::int mt FROM vendas_faturadas
        WHERE empresa_id=p_empresa_id AND data_faturamento BETWEEN p_ini AND p_fim
          AND qt>0 AND cod_prod<>'' AND cod_fornec<>'' AND cod_gerente<>'' AND cod_supervisor<>'' AND cod_rca<>'' GROUP BY cod_fornec, cod_gerente, cod_supervisor, cod_rca
    ) s WHERE t.empresa_id=p_empresa_id AND t.ano=p_ano AND t.mes=p_mes AND t.cod_fornec=s.cod_fornec AND t.cod_gerente=s.cod_gerente AND t.cod_supervisor=s.cod_supervisor AND t.cod_rca=s.cod_rca;

    UPDATE farol.agg_fat_v02_l0_mes t SET mix_total = s.mt FROM (
        SELECT cod_supervisor, COUNT(DISTINCT cod_prod)::int mt FROM vendas_faturadas
        WHERE empresa_id=p_empresa_id AND data_faturamento BETWEEN p_ini AND p_fim
          AND qt>0 AND cod_prod<>'' AND cod_supervisor<>'' GROUP BY cod_supervisor
    ) s WHERE t.empresa_id=p_empresa_id AND t.ano=p_ano AND t.mes=p_mes AND t.cod_supervisor=s.cod_supervisor;

    UPDATE farol.agg_fat_v02_l1_mes t SET mix_total = s.mt FROM (
        SELECT cod_supervisor, cod_rca, COUNT(DISTINCT cod_prod)::int mt FROM vendas_faturadas
        WHERE empresa_id=p_empresa_id AND data_faturamento BETWEEN p_ini AND p_fim
          AND qt>0 AND cod_prod<>'' AND cod_supervisor<>'' AND cod_rca<>'' GROUP BY cod_supervisor, cod_rca
    ) s WHERE t.empresa_id=p_empresa_id AND t.ano=p_ano AND t.mes=p_mes AND t.cod_supervisor=s.cod_supervisor AND t.cod_rca=s.cod_rca;

    UPDATE farol.agg_fat_v02_l2_mes t SET mix_total = s.mt FROM (
        SELECT cod_supervisor, cod_rca, cod_fornec, COUNT(DISTINCT cod_prod)::int mt FROM vendas_faturadas
        WHERE empresa_id=p_empresa_id AND data_faturamento BETWEEN p_ini AND p_fim
          AND qt>0 AND cod_prod<>'' AND cod_supervisor<>'' AND cod_rca<>'' AND cod_fornec<>'' GROUP BY cod_supervisor, cod_rca, cod_fornec
    ) s WHERE t.empresa_id=p_empresa_id AND t.ano=p_ano AND t.mes=p_mes AND t.cod_supervisor=s.cod_supervisor AND t.cod_rca=s.cod_rca AND t.cod_fornec=s.cod_fornec;

    UPDATE farol.agg_fat_v03_l0_mes t SET mix_total = s.mt FROM (
        SELECT cod_gerente, COUNT(DISTINCT cod_prod)::int mt FROM vendas_faturadas
        WHERE empresa_id=p_empresa_id AND data_faturamento BETWEEN p_ini AND p_fim
          AND qt>0 AND cod_prod<>'' AND cod_gerente<>'' GROUP BY cod_gerente
    ) s WHERE t.empresa_id=p_empresa_id AND t.ano=p_ano AND t.mes=p_mes AND t.cod_gerente=s.cod_gerente;

    UPDATE farol.agg_fat_v03_l1_mes t SET mix_total = s.mt FROM (
        SELECT cod_gerente, cod_supervisor, COUNT(DISTINCT cod_prod)::int mt FROM vendas_faturadas
        WHERE empresa_id=p_empresa_id AND data_faturamento BETWEEN p_ini AND p_fim
          AND qt>0 AND cod_prod<>'' AND cod_gerente<>'' AND cod_supervisor<>'' GROUP BY cod_gerente, cod_supervisor
    ) s WHERE t.empresa_id=p_empresa_id AND t.ano=p_ano AND t.mes=p_mes AND t.cod_gerente=s.cod_gerente AND t.cod_supervisor=s.cod_supervisor;

    UPDATE farol.agg_fat_v03_l2_mes t SET mix_total = s.mt FROM (
        SELECT cod_gerente, cod_supervisor, cod_rca, COUNT(DISTINCT cod_prod)::int mt FROM vendas_faturadas
        WHERE empresa_id=p_empresa_id AND data_faturamento BETWEEN p_ini AND p_fim
          AND qt>0 AND cod_prod<>'' AND cod_gerente<>'' AND cod_supervisor<>'' AND cod_rca<>'' GROUP BY cod_gerente, cod_supervisor, cod_rca
    ) s WHERE t.empresa_id=p_empresa_id AND t.ano=p_ano AND t.mes=p_mes AND t.cod_gerente=s.cod_gerente AND t.cod_supervisor=s.cod_supervisor AND t.cod_rca=s.cod_rca;

    UPDATE farol.agg_fat_v05_l0_mes t SET mix_total = s.mt FROM (
        SELECT cod_supervisor, COUNT(DISTINCT cod_prod)::int mt FROM vendas_faturadas
        WHERE empresa_id=p_empresa_id AND data_faturamento BETWEEN p_ini AND p_fim
          AND qt>0 AND cod_prod<>'' AND cod_supervisor<>'' GROUP BY cod_supervisor
    ) s WHERE t.empresa_id=p_empresa_id AND t.ano=p_ano AND t.mes=p_mes AND t.cod_supervisor=s.cod_supervisor;

    UPDATE farol.agg_fat_v05_l1_mes t SET mix_total = s.mt FROM (
        SELECT cod_supervisor, cod_fornec, COUNT(DISTINCT cod_prod)::int mt FROM vendas_faturadas
        WHERE empresa_id=p_empresa_id AND data_faturamento BETWEEN p_ini AND p_fim
          AND qt>0 AND cod_prod<>'' AND cod_supervisor<>'' AND cod_fornec<>'' GROUP BY cod_supervisor, cod_fornec
    ) s WHERE t.empresa_id=p_empresa_id AND t.ano=p_ano AND t.mes=p_mes AND t.cod_supervisor=s.cod_supervisor AND t.cod_fornec=s.cod_fornec;

    UPDATE farol.agg_fat_v05_l2_mes t SET mix_total = s.mt FROM (
        SELECT cod_supervisor, cod_fornec, cod_rca, COUNT(DISTINCT cod_prod)::int mt FROM vendas_faturadas
        WHERE empresa_id=p_empresa_id AND data_faturamento BETWEEN p_ini AND p_fim
          AND qt>0 AND cod_prod<>'' AND cod_supervisor<>'' AND cod_fornec<>'' AND cod_rca<>'' GROUP BY cod_supervisor, cod_fornec, cod_rca
    ) s WHERE t.empresa_id=p_empresa_id AND t.ano=p_ano AND t.mes=p_mes AND t.cod_supervisor=s.cod_supervisor AND t.cod_fornec=s.cod_fornec AND t.cod_rca=s.cod_rca;

    -- ─────────── TRANSMITIDO (vendas_transmitidas / data_transmissao) ───────────
    UPDATE farol.agg_trans_v01_l0_mes t SET mix_total = s.mt FROM (
        SELECT cod_fornec, COUNT(DISTINCT cod_prod)::int mt FROM vendas_transmitidas
        WHERE empresa_id=p_empresa_id AND data_transmissao BETWEEN p_ini AND p_fim
          AND qt>0 AND cod_prod<>'' AND cod_fornec<>'' GROUP BY cod_fornec
    ) s WHERE t.empresa_id=p_empresa_id AND t.ano=p_ano AND t.mes=p_mes AND t.cod_fornec=s.cod_fornec;

    UPDATE farol.agg_trans_v01_l1_mes t SET mix_total = s.mt FROM (
        SELECT cod_fornec, cod_gerente, COUNT(DISTINCT cod_prod)::int mt FROM vendas_transmitidas
        WHERE empresa_id=p_empresa_id AND data_transmissao BETWEEN p_ini AND p_fim
          AND qt>0 AND cod_prod<>'' AND cod_fornec<>'' AND cod_gerente<>'' GROUP BY cod_fornec, cod_gerente
    ) s WHERE t.empresa_id=p_empresa_id AND t.ano=p_ano AND t.mes=p_mes AND t.cod_fornec=s.cod_fornec AND t.cod_gerente=s.cod_gerente;

    UPDATE farol.agg_trans_v01_l2_mes t SET mix_total = s.mt FROM (
        SELECT cod_fornec, cod_gerente, cod_supervisor, COUNT(DISTINCT cod_prod)::int mt FROM vendas_transmitidas
        WHERE empresa_id=p_empresa_id AND data_transmissao BETWEEN p_ini AND p_fim
          AND qt>0 AND cod_prod<>'' AND cod_fornec<>'' AND cod_gerente<>'' AND cod_supervisor<>'' GROUP BY cod_fornec, cod_gerente, cod_supervisor
    ) s WHERE t.empresa_id=p_empresa_id AND t.ano=p_ano AND t.mes=p_mes AND t.cod_fornec=s.cod_fornec AND t.cod_gerente=s.cod_gerente AND t.cod_supervisor=s.cod_supervisor;

    UPDATE farol.agg_trans_v01_l3_mes t SET mix_total = s.mt FROM (
        SELECT cod_fornec, cod_gerente, cod_supervisor, cod_rca, COUNT(DISTINCT cod_prod)::int mt FROM vendas_transmitidas
        WHERE empresa_id=p_empresa_id AND data_transmissao BETWEEN p_ini AND p_fim
          AND qt>0 AND cod_prod<>'' AND cod_fornec<>'' AND cod_gerente<>'' AND cod_supervisor<>'' AND cod_rca<>'' GROUP BY cod_fornec, cod_gerente, cod_supervisor, cod_rca
    ) s WHERE t.empresa_id=p_empresa_id AND t.ano=p_ano AND t.mes=p_mes AND t.cod_fornec=s.cod_fornec AND t.cod_gerente=s.cod_gerente AND t.cod_supervisor=s.cod_supervisor AND t.cod_rca=s.cod_rca;

    UPDATE farol.agg_trans_v02_l0_mes t SET mix_total = s.mt FROM (
        SELECT cod_supervisor, COUNT(DISTINCT cod_prod)::int mt FROM vendas_transmitidas
        WHERE empresa_id=p_empresa_id AND data_transmissao BETWEEN p_ini AND p_fim
          AND qt>0 AND cod_prod<>'' AND cod_supervisor<>'' GROUP BY cod_supervisor
    ) s WHERE t.empresa_id=p_empresa_id AND t.ano=p_ano AND t.mes=p_mes AND t.cod_supervisor=s.cod_supervisor;

    UPDATE farol.agg_trans_v02_l1_mes t SET mix_total = s.mt FROM (
        SELECT cod_supervisor, cod_rca, COUNT(DISTINCT cod_prod)::int mt FROM vendas_transmitidas
        WHERE empresa_id=p_empresa_id AND data_transmissao BETWEEN p_ini AND p_fim
          AND qt>0 AND cod_prod<>'' AND cod_supervisor<>'' AND cod_rca<>'' GROUP BY cod_supervisor, cod_rca
    ) s WHERE t.empresa_id=p_empresa_id AND t.ano=p_ano AND t.mes=p_mes AND t.cod_supervisor=s.cod_supervisor AND t.cod_rca=s.cod_rca;

    UPDATE farol.agg_trans_v02_l2_mes t SET mix_total = s.mt FROM (
        SELECT cod_supervisor, cod_rca, cod_fornec, COUNT(DISTINCT cod_prod)::int mt FROM vendas_transmitidas
        WHERE empresa_id=p_empresa_id AND data_transmissao BETWEEN p_ini AND p_fim
          AND qt>0 AND cod_prod<>'' AND cod_supervisor<>'' AND cod_rca<>'' AND cod_fornec<>'' GROUP BY cod_supervisor, cod_rca, cod_fornec
    ) s WHERE t.empresa_id=p_empresa_id AND t.ano=p_ano AND t.mes=p_mes AND t.cod_supervisor=s.cod_supervisor AND t.cod_rca=s.cod_rca AND t.cod_fornec=s.cod_fornec;

    UPDATE farol.agg_trans_v03_l0_mes t SET mix_total = s.mt FROM (
        SELECT cod_gerente, COUNT(DISTINCT cod_prod)::int mt FROM vendas_transmitidas
        WHERE empresa_id=p_empresa_id AND data_transmissao BETWEEN p_ini AND p_fim
          AND qt>0 AND cod_prod<>'' AND cod_gerente<>'' GROUP BY cod_gerente
    ) s WHERE t.empresa_id=p_empresa_id AND t.ano=p_ano AND t.mes=p_mes AND t.cod_gerente=s.cod_gerente;

    UPDATE farol.agg_trans_v03_l1_mes t SET mix_total = s.mt FROM (
        SELECT cod_gerente, cod_supervisor, COUNT(DISTINCT cod_prod)::int mt FROM vendas_transmitidas
        WHERE empresa_id=p_empresa_id AND data_transmissao BETWEEN p_ini AND p_fim
          AND qt>0 AND cod_prod<>'' AND cod_gerente<>'' AND cod_supervisor<>'' GROUP BY cod_gerente, cod_supervisor
    ) s WHERE t.empresa_id=p_empresa_id AND t.ano=p_ano AND t.mes=p_mes AND t.cod_gerente=s.cod_gerente AND t.cod_supervisor=s.cod_supervisor;

    UPDATE farol.agg_trans_v03_l2_mes t SET mix_total = s.mt FROM (
        SELECT cod_gerente, cod_supervisor, cod_rca, COUNT(DISTINCT cod_prod)::int mt FROM vendas_transmitidas
        WHERE empresa_id=p_empresa_id AND data_transmissao BETWEEN p_ini AND p_fim
          AND qt>0 AND cod_prod<>'' AND cod_gerente<>'' AND cod_supervisor<>'' AND cod_rca<>'' GROUP BY cod_gerente, cod_supervisor, cod_rca
    ) s WHERE t.empresa_id=p_empresa_id AND t.ano=p_ano AND t.mes=p_mes AND t.cod_gerente=s.cod_gerente AND t.cod_supervisor=s.cod_supervisor AND t.cod_rca=s.cod_rca;

    UPDATE farol.agg_trans_v05_l0_mes t SET mix_total = s.mt FROM (
        SELECT cod_supervisor, COUNT(DISTINCT cod_prod)::int mt FROM vendas_transmitidas
        WHERE empresa_id=p_empresa_id AND data_transmissao BETWEEN p_ini AND p_fim
          AND qt>0 AND cod_prod<>'' AND cod_supervisor<>'' GROUP BY cod_supervisor
    ) s WHERE t.empresa_id=p_empresa_id AND t.ano=p_ano AND t.mes=p_mes AND t.cod_supervisor=s.cod_supervisor;

    UPDATE farol.agg_trans_v05_l1_mes t SET mix_total = s.mt FROM (
        SELECT cod_supervisor, cod_fornec, COUNT(DISTINCT cod_prod)::int mt FROM vendas_transmitidas
        WHERE empresa_id=p_empresa_id AND data_transmissao BETWEEN p_ini AND p_fim
          AND qt>0 AND cod_prod<>'' AND cod_supervisor<>'' AND cod_fornec<>'' GROUP BY cod_supervisor, cod_fornec
    ) s WHERE t.empresa_id=p_empresa_id AND t.ano=p_ano AND t.mes=p_mes AND t.cod_supervisor=s.cod_supervisor AND t.cod_fornec=s.cod_fornec;

    UPDATE farol.agg_trans_v05_l2_mes t SET mix_total = s.mt FROM (
        SELECT cod_supervisor, cod_fornec, cod_rca, COUNT(DISTINCT cod_prod)::int mt FROM vendas_transmitidas
        WHERE empresa_id=p_empresa_id AND data_transmissao BETWEEN p_ini AND p_fim
          AND qt>0 AND cod_prod<>'' AND cod_supervisor<>'' AND cod_fornec<>'' AND cod_rca<>'' GROUP BY cod_supervisor, cod_fornec, cod_rca
    ) s WHERE t.empresa_id=p_empresa_id AND t.ano=p_ano AND t.mes=p_mes AND t.cod_supervisor=s.cod_supervisor AND t.cod_fornec=s.cod_fornec AND t.cod_rca=s.cod_rca;
END;
$$ LANGUAGE plpgsql;

-- ════════════════════════════════════════════════════════════════════════════
-- PARTE 3 — Backfill: NÃO roda aqui de propósito.
--
-- O backfill (popular mix_total dos meses já consolidados) leva ~15-30 min e, se
-- rodasse dentro da migration, bloquearia o startup do backend (risco de o Coolify
-- reiniciar o container em loop por healthcheck). Após o deploy desta migration,
-- rodar manualmente via psql (uma vez):
--
--   DO $b$ DECLARE r RECORD; BEGIN
--     FOR r IN SELECT DISTINCT empresa_id, ano, mes FROM farol.agg_fat_v01_l0_mes ORDER BY ano,mes
--     LOOP RAISE NOTICE 'mixtotal %/%', r.ano, r.mes;
--          PERFORM farol.upsert_mixtotal_mes(r.empresa_id, r.ano, r.mes); END LOOP;
--   END; $b$;
--
-- Até o backfill rodar, mix_total fica 0 (painel mostra "X" sem "/ Y") — sem erro.
-- Imports futuros já populam mix_total automaticamente (RefreshViews chama
-- upsert_mixtotal_mes nos meses consolidados).
-- ════════════════════════════════════════════════════════════════════════════
