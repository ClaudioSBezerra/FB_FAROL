-- Migration 100: Criação do schema smartpick e tabela sp_enderecos
-- Story 1.2 - Criação do Banco de Dados e Schema SmartPick
--
-- Estrutura baseada no export WMS (Calibragem_WMS_v2.csv):
-- Colunas 1-27 são importadas do CSV; a sugestão de calibragem é gerada pelo motor.
--
-- Coluna            CSV            Tipo
-- ─────────────────────────────────────────────────────────
-- cod_filial        CODFILIAL      INTEGER
-- codepto           CODEPTO        INTEGER
-- departamento      DEPARTAMENTO   TEXT
-- codsec            CODSEC         INTEGER
-- secao             SECAO          TEXT
-- codprod           CODPROD        INTEGER  (chave do produto no WMS)
-- produto           PRODUTO        TEXT
-- embalagem         EMBALAGEM      TEXT     (ex: "UN/0001/UN")
-- fora_linha        FORALINHA      BOOLEAN  (N→false, S→true)
-- rua               RUA            INTEGER  (endereço picking)
-- predio            PREDIO         INTEGER  (endereço picking)
-- apto              APTO           INTEGER  (endereço picking)
-- capacidade        CAPACIDADE     INTEGER
-- norma_palete      NORMA_PALETE   INTEGER
-- ponto_reposicao   PONTOREPOSICAO INTEGER
-- classe_venda      CLASSEVENDA    CHAR(1)  (A/B/C)
-- classe_venda_dias CLASSEVENDA_DIAS INTEGER
-- qt_giro_dia       QTGIRODIA_SISTEMA      NUMERIC(12,4)
-- qt_acesso_90      QTACESSO_PICKING_PERIODO_90  INTEGER
-- qt_dias           QT_DIAS        INTEGER
-- qt_prod           QT_PROD        INTEGER
-- qt_prod_cx        QT_PROD_CX     INTEGER
-- med_venda_cx      MED_VENDA_DIAS_CX      NUMERIC(12,4)
-- med_venda_dias    MED_VENDA_DIAS         NUMERIC(12,4)
-- med_dias_estoque  MED_DIAS_ESTOQUE       NUMERIC(12,4)
-- med_venda_cx_aa   MED_VENDA_DIAS_CX_ANOANT_MESSEG NUMERIC(12,4)
-- unidade_master    UNIDADE_MASTER INTEGER
-- (SUGESTÃO CALIBRAGEM é gerada pelo motor → sp_propostas)

-- ─── Schema ───────────────────────────────────────────────────────────────────
CREATE SCHEMA IF NOT EXISTS smartpick;

-- ─── Extensões necessárias ────────────────────────────────────────────────────
CREATE EXTENSION IF NOT EXISTS "pgcrypto";  -- gen_random_uuid()

-- ─── sp_enderecos ─────────────────────────────────────────────────────────────
-- Armazena os dados importados do CSV do WMS, um registro por linha do arquivo.
-- Cada job de importação gera um conjunto de registros vinculados via job_id.
CREATE TABLE IF NOT EXISTS smartpick.sp_enderecos (
    id                  BIGSERIAL       PRIMARY KEY,
    job_id              UUID            NOT NULL,           -- FK para sp_csv_jobs (criada na migration 102)
    filial_id           INTEGER         NOT NULL,           -- ID interno SmartPick da filial
    cod_filial          INTEGER         NOT NULL,           -- CODFILIAL do WMS
    codepto             INTEGER,                            -- CODEPTO
    departamento        TEXT,                               -- DEPARTAMENTO
    codsec              INTEGER,                            -- CODSEC
    secao               TEXT,                               -- SECAO
    codprod             INTEGER         NOT NULL,           -- CODPROD (chave produto WMS)
    produto             TEXT,                               -- PRODUTO
    embalagem           TEXT,                               -- EMBALAGEM (ex: "UN/0001/UN")
    fora_linha          BOOLEAN         NOT NULL DEFAULT FALSE, -- FORALINHA (N→false, S→true)
    rua                 INTEGER,                            -- RUA (endereço de picking)
    predio              INTEGER,                            -- PREDIO (endereço de picking)
    apto                INTEGER,                            -- APTO (endereço de picking)
    capacidade          INTEGER,                            -- CAPACIDADE atual
    norma_palete        INTEGER,                            -- NORMA_PALETE
    ponto_reposicao     INTEGER,                            -- PONTOREPOSICAO
    classe_venda        CHAR(1),                            -- CLASSEVENDA (A/B/C)
    classe_venda_dias   INTEGER,                            -- CLASSEVENDA_DIAS
    qt_giro_dia         NUMERIC(12,4),                      -- QTGIRODIA_SISTEMA
    qt_acesso_90        INTEGER,                            -- QTACESSO_PICKING_PERIODO_90
    qt_dias             INTEGER,                            -- QT_DIAS
    qt_prod             INTEGER,                            -- QT_PROD
    qt_prod_cx          INTEGER,                            -- QT_PROD_CX
    med_venda_cx        NUMERIC(12,4),                      -- MED_VENDA_DIAS_CX
    med_venda_dias      NUMERIC(12,4),                      -- MED_VENDA_DIAS
    med_dias_estoque    NUMERIC(12,4),                      -- MED_DIAS_ESTOQUE
    med_venda_cx_aa     NUMERIC(12,4),                      -- MED_VENDA_DIAS_CX_ANOANT_MESSEG
    unidade_master      INTEGER,                            -- UNIDADE_MASTER
    created_at          TIMESTAMPTZ     NOT NULL DEFAULT now()
);

-- Índices de acesso mais frequentes
CREATE INDEX IF NOT EXISTS idx_sp_enderecos_job_id      ON smartpick.sp_enderecos(job_id);
CREATE INDEX IF NOT EXISTS idx_sp_enderecos_filial_prod ON smartpick.sp_enderecos(filial_id, codprod);
CREATE INDEX IF NOT EXISTS idx_sp_enderecos_classe      ON smartpick.sp_enderecos(filial_id, classe_venda);
