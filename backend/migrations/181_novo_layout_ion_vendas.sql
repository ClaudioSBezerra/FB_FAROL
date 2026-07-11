-- 181_novo_layout_ion_vendas.sql
-- ════════════════════════════════════════════════════════════════════════════
-- Adequação ao novo layout de exportação do ION VENDAS (jul/2026).
--
-- Novas colunas incluídas no CSV:
--   CODEPTO, DEPARTAMENTO         → hierarquia produto: departamento
--   CODSEC, SECAO                 → hierarquia produto: seção
--   CODCATEGORIA, CATEGORIA       → hierarquia produto: categoria
--   CODCLIPRINC                   → cliente principal (rede de lojas)
--   FANTASIA                      → nome fantasia do cliente
--   PVENDA_TOTAL                  → total já calculado (usado por preferência
--                                    no importer; fallback é QT × PVENDA)
--
-- Esta migration só ADICIONA colunas; não muda semântica das existentes.
-- `pvenda` continua sendo o TOTAL da linha (todas as agg_*_mes e queries
-- SUM(pvenda) permanecem funcionando).
-- Nova coluna `pvenda_unit` guarda o unitário do CSV (informativa).
--
-- Fase 2 (futura) usará estas colunas para criar hierarquias V06 "Por Rede"
-- e V07 "Por Departamento" no GRID.
-- ════════════════════════════════════════════════════════════════════════════

ALTER TABLE vendas_faturadas
    ADD COLUMN IF NOT EXISTS cod_depto      TEXT          NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS depto          TEXT          NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS cod_sec        TEXT          NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS secao          TEXT          NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS cod_categoria  TEXT          NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS categoria      TEXT          NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS cod_cliprinc   TEXT          NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS fantasia       TEXT          NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS pvenda_unit    NUMERIC(15,4) NOT NULL DEFAULT 0;

ALTER TABLE vendas_transmitidas
    ADD COLUMN IF NOT EXISTS cod_depto      TEXT          NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS depto          TEXT          NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS cod_sec        TEXT          NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS secao          TEXT          NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS cod_categoria  TEXT          NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS categoria      TEXT          NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS cod_cliprinc   TEXT          NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS fantasia       TEXT          NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS pvenda_unit    NUMERIC(15,4) NOT NULL DEFAULT 0;

COMMENT ON COLUMN vendas_faturadas.pvenda      IS 'Total da linha (pvenda_unit × qt). Preservado por compatibilidade com todas as agg_*_mes e queries SUM(pvenda) existentes.';
COMMENT ON COLUMN vendas_faturadas.pvenda_unit IS 'Preço unitário conforme CSV do ION VENDAS (novo layout jul/2026).';
COMMENT ON COLUMN vendas_faturadas.cod_cliprinc IS 'Código do cliente principal (rede). Cliente-pai de vários cod_cli. Usado em Fase 2 para hierarquia "Por Rede".';

COMMENT ON COLUMN vendas_transmitidas.pvenda      IS 'Total da linha (pvenda_unit × qt). Preservado por compatibilidade.';
COMMENT ON COLUMN vendas_transmitidas.pvenda_unit IS 'Preço unitário conforme CSV do ION VENDAS (novo layout jul/2026).';
COMMENT ON COLUMN vendas_transmitidas.cod_cliprinc IS 'Código do cliente principal (rede).';
