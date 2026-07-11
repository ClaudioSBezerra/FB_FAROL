-- 184_agg_v07_por_departamento.sql
-- ════════════════════════════════════════════════════════════════════════════
-- Cria as tabelas agregadas mensais da V07 "Por Departamento".
--
-- Hierarquia:  L0 Depto → L1 Seção → L2 Categoria → (L3 Produto lê vendas_*)
--
-- Sem métricas de positivação (mesma decisão da V06 — Fase 2 simplificada).
-- ════════════════════════════════════════════════════════════════════════════

-- ─── L0: por departamento ────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS farol.agg_fat_v07_l0_mes (
    empresa_id  UUID    NOT NULL,
    ano         INT     NOT NULL,
    mes         INT     NOT NULL,
    cod_depto   TEXT    NOT NULL,
    depto       TEXT    NOT NULL DEFAULT '',
    pvenda      NUMERIC NOT NULL DEFAULT 0,
    plucro      NUMERIC NOT NULL DEFAULT 0,
    qt          NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, cod_depto)
) PARTITION BY RANGE (ano);

-- ─── L1: por departamento + seção ────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS farol.agg_fat_v07_l1_mes (
    empresa_id  UUID    NOT NULL,
    ano         INT     NOT NULL,
    mes         INT     NOT NULL,
    cod_depto   TEXT    NOT NULL,
    cod_sec     TEXT    NOT NULL,
    secao       TEXT    NOT NULL DEFAULT '',
    pvenda      NUMERIC NOT NULL DEFAULT 0,
    plucro      NUMERIC NOT NULL DEFAULT 0,
    qt          NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, cod_depto, cod_sec)
) PARTITION BY RANGE (ano);

-- ─── L2: por departamento + seção + categoria ────────────────────────────────
CREATE TABLE IF NOT EXISTS farol.agg_fat_v07_l2_mes (
    empresa_id     UUID    NOT NULL,
    ano            INT     NOT NULL,
    mes            INT     NOT NULL,
    cod_depto      TEXT    NOT NULL,
    cod_sec        TEXT    NOT NULL,
    cod_categoria  TEXT    NOT NULL,
    categoria      TEXT    NOT NULL DEFAULT '',
    pvenda         NUMERIC NOT NULL DEFAULT 0,
    plucro         NUMERIC NOT NULL DEFAULT 0,
    qt             NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, cod_depto, cod_sec, cod_categoria)
) PARTITION BY RANGE (ano);

-- ─── Espelho para o fluxo Transmitido ────────────────────────────────────────
CREATE TABLE IF NOT EXISTS farol.agg_trans_v07_l0_mes (LIKE farol.agg_fat_v07_l0_mes INCLUDING ALL) PARTITION BY RANGE (ano);
CREATE TABLE IF NOT EXISTS farol.agg_trans_v07_l1_mes (LIKE farol.agg_fat_v07_l1_mes INCLUDING ALL) PARTITION BY RANGE (ano);
CREATE TABLE IF NOT EXISTS farol.agg_trans_v07_l2_mes (LIKE farol.agg_fat_v07_l2_mes INCLUDING ALL) PARTITION BY RANGE (ano);

COMMENT ON TABLE farol.agg_fat_v07_l0_mes IS 'Agregado mensal fat V07 L0 — total por departamento.';
COMMENT ON TABLE farol.agg_fat_v07_l1_mes IS 'Agregado mensal fat V07 L1 — departamento × seção.';
COMMENT ON TABLE farol.agg_fat_v07_l2_mes IS 'Agregado mensal fat V07 L2 — departamento × seção × categoria.';

-- ─── Partições para os anos existentes ───────────────────────────────────────
DO $$
DECLARE
    r RECORD;
BEGIN
    FOR r IN
        SELECT DISTINCT EXTRACT(YEAR FROM data_faturamento)::int AS ano
        FROM vendas_faturadas
        UNION
        SELECT DISTINCT EXTRACT(YEAR FROM data_transmissao)::int AS ano
        FROM vendas_transmitidas
    LOOP
        PERFORM farol.create_agg_year_partitions(r.ano);
    END LOOP;
END $$;
