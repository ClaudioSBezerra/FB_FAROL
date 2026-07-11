-- 183_agg_v06_por_rede.sql
-- ════════════════════════════════════════════════════════════════════════════
-- Cria as tabelas agregadas mensais da V06 "Por Rede".
--
-- Hierarquia:  L0 Rede → L1 Fornecedor → L2 Cliente → (L3 Produto lê vendas_*)
--
-- Métricas: só pvenda, plucro, qt.
-- Sem base_cli/positivados/mix (decisão da Fase 2 — simplificação).
--
-- Padrão estrutural: espelha agg_fat_v04_*_mes (particionadas por ano).
-- Populadas pela função farol.upsert_aggs_mes (estendida na mig 185).
-- ════════════════════════════════════════════════════════════════════════════

-- ─── L0: por rede ────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS farol.agg_fat_v06_l0_mes (
    empresa_id     UUID    NOT NULL,
    ano            INT     NOT NULL,
    mes            INT     NOT NULL,
    cod_cliprinc   TEXT    NOT NULL,
    nome_cliprinc  TEXT    NOT NULL DEFAULT '',
    pvenda         NUMERIC NOT NULL DEFAULT 0,
    plucro         NUMERIC NOT NULL DEFAULT 0,
    qt             NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, cod_cliprinc)
) PARTITION BY RANGE (ano);

-- ─── L1: por rede + fornecedor ───────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS farol.agg_fat_v06_l1_mes (
    empresa_id     UUID    NOT NULL,
    ano            INT     NOT NULL,
    mes            INT     NOT NULL,
    cod_cliprinc   TEXT    NOT NULL,
    cod_fornec     TEXT    NOT NULL,
    nome_fornec    TEXT    NOT NULL DEFAULT '',
    pvenda         NUMERIC NOT NULL DEFAULT 0,
    plucro         NUMERIC NOT NULL DEFAULT 0,
    qt             NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, cod_cliprinc, cod_fornec)
) PARTITION BY RANGE (ano);

-- ─── L2: por rede + fornecedor + cliente (chave cnpj) ────────────────────────
CREATE TABLE IF NOT EXISTS farol.agg_fat_v06_l2_mes (
    empresa_id     UUID    NOT NULL,
    ano            INT     NOT NULL,
    mes            INT     NOT NULL,
    cod_cliprinc   TEXT    NOT NULL,
    cod_fornec     TEXT    NOT NULL,
    cnpj           TEXT    NOT NULL,
    cod_cli        TEXT    NOT NULL DEFAULT '',
    nome_cli       TEXT    NOT NULL DEFAULT '',
    pvenda         NUMERIC NOT NULL DEFAULT 0,
    plucro         NUMERIC NOT NULL DEFAULT 0,
    qt             NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, cod_cliprinc, cod_fornec, cnpj)
) PARTITION BY RANGE (ano);

-- ─── Espelho para o fluxo Transmitido ────────────────────────────────────────
CREATE TABLE IF NOT EXISTS farol.agg_trans_v06_l0_mes (LIKE farol.agg_fat_v06_l0_mes INCLUDING ALL) PARTITION BY RANGE (ano);
CREATE TABLE IF NOT EXISTS farol.agg_trans_v06_l1_mes (LIKE farol.agg_fat_v06_l1_mes INCLUDING ALL) PARTITION BY RANGE (ano);
CREATE TABLE IF NOT EXISTS farol.agg_trans_v06_l2_mes (LIKE farol.agg_fat_v06_l2_mes INCLUDING ALL) PARTITION BY RANGE (ano);

COMMENT ON TABLE farol.agg_fat_v06_l0_mes IS 'Agregado mensal fat V06 L0 — total por rede (cod_cliprinc). Nome da rede = fantasia se houver, senão razão social.';
COMMENT ON TABLE farol.agg_fat_v06_l1_mes IS 'Agregado mensal fat V06 L1 — rede × fornecedor.';
COMMENT ON TABLE farol.agg_fat_v06_l2_mes IS 'Agregado mensal fat V06 L2 — rede × fornecedor × cliente (chave por CNPJ).';

-- ─── Partições para os anos existentes ───────────────────────────────────────
-- Padrão do projeto: as partições anuais são gerenciadas por
-- farol.create_agg_year_partitions(ano). Chamamos para anos vistos na base.

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
