-- 182_tabela_vendas_ccd.sql
-- ════════════════════════════════════════════════════════════════════════════
-- Cria vendas_ccd — armazena eventos CORTADO, CANCELADO e DEVOLVIDO
-- exportados pelo ION VENDAS no novo layout (arquivos MES_ANO_CCD.csv).
--
-- Estrutura espelha vendas_faturadas/vendas_transmitidas + coluna `evento`
-- discriminando o tipo. Assim reaproveita todo o padrão de import, dedup e
-- (Fase 2) agregação.
--
-- Não é criada MV, agg_*_mes ou índice além do mínimo — Fase 2 cuidará
-- do que for necessário para expor CCD no GRID.
-- ════════════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS vendas_ccd (
    id                BIGSERIAL PRIMARY KEY,
    empresa_id        UUID NOT NULL,
    data_evento       DATE NOT NULL,
    evento            TEXT NOT NULL,               -- CORTADO | CANCELADO | DEVOLVIDO

    -- Hierarquia comercial (idêntica às demais tabelas)
    cod_gerente       TEXT          NOT NULL DEFAULT '',
    nome_gerente      TEXT          NOT NULL DEFAULT '',
    cod_supervisor    TEXT          NOT NULL DEFAULT '',
    nome_supervisor   TEXT          NOT NULL DEFAULT '',
    qtrca_supervisor  INT           NOT NULL DEFAULT 0,
    cod_rca           TEXT          NOT NULL DEFAULT '',
    nome_rca          TEXT          NOT NULL DEFAULT '',
    qtcli_rca         INT           NOT NULL DEFAULT 0,
    cod_fornec        TEXT          NOT NULL DEFAULT '',
    nome_fornec       TEXT          NOT NULL DEFAULT '',
    cod_cli           TEXT          NOT NULL DEFAULT '',
    nome_cli          TEXT          NOT NULL DEFAULT '',
    uf                TEXT          NOT NULL DEFAULT '',
    empresa           TEXT          NOT NULL DEFAULT '',
    cod_prod          TEXT          NOT NULL DEFAULT '',
    nome_prod         TEXT          NOT NULL DEFAULT '',
    ean               TEXT          NOT NULL DEFAULT '',
    qt                NUMERIC(15,3) NOT NULL DEFAULT 0,
    pvenda            NUMERIC(15,2) NOT NULL DEFAULT 0,   -- total da linha
    pvenda_unit       NUMERIC(15,4) NOT NULL DEFAULT 0,   -- unitário do CSV
    plucro            NUMERIC(15,2) NOT NULL DEFAULT 0,
    importado_em      TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    cnpj              TEXT          NOT NULL DEFAULT '',
    cod_ramo          TEXT          NOT NULL DEFAULT '',
    ramo              TEXT          NOT NULL DEFAULT '',
    embalagem         TEXT          NOT NULL DEFAULT '',
    qt_unit           NUMERIC(15,3) NOT NULL DEFAULT 0,
    qt_unit_cx        NUMERIC(15,3) NOT NULL DEFAULT 0,
    cod_bar           TEXT          NOT NULL DEFAULT '',

    -- Novas colunas do layout jul/2026
    cod_depto         TEXT NOT NULL DEFAULT '',
    depto             TEXT NOT NULL DEFAULT '',
    cod_sec           TEXT NOT NULL DEFAULT '',
    secao             TEXT NOT NULL DEFAULT '',
    cod_categoria     TEXT NOT NULL DEFAULT '',
    categoria         TEXT NOT NULL DEFAULT '',
    cod_cliprinc      TEXT NOT NULL DEFAULT '',
    fantasia          TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_vccd_emp_data
    ON vendas_ccd (empresa_id, data_evento);

CREATE INDEX IF NOT EXISTS idx_vccd_emp_evento_data
    ON vendas_ccd (empresa_id, evento, data_evento);

COMMENT ON TABLE vendas_ccd IS 'Eventos de Cortado/Cancelado/Devolvido exportados pelo ION VENDAS (arquivos _CCD.csv, layout jul/2026). Estrutura espelha vendas_faturadas + coluna evento discriminando o tipo.';
COMMENT ON COLUMN vendas_ccd.evento IS 'CORTADO | CANCELADO | DEVOLVIDO — determinado a partir do campo ESTADO do CSV.';
