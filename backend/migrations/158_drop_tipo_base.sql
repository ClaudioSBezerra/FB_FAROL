-- 158_drop_tipo_base.sql
-- Remove tipo_base de TUDO — herança do modelo mensal antigo que não faz
-- mais sentido com granularidade diária + ranges livres.
--
-- PRINCÍPIO:
--   A "atual vs comparativa" é uma propriedade da CONSULTA (qual range o
--   usuário escolheu agora), não do DADO. A linha de venda do dia 15/05/26
--   é "atual" em junho/26 e vira "comparativa" em junho/27 — automaticamente,
--   sem reclassificação.
--
-- ABORDAGEM:
--   DROP TABLE ... CASCADE — derruba tabelas + as 28 MVs derivadas.
--   Recria as tabelas sem tipo_base. A migration 159 recria as 28 MVs.
--   Em produção, o usuário vai resetar a base — sem migração de dados.

DROP TABLE IF EXISTS vendas_faturadas    CASCADE;
DROP TABLE IF EXISTS vendas_transmitidas CASCADE;

-- ═══════════════════════════════════════════════════════════════════════════════
-- vendas_faturadas (sem tipo_base)
-- ═══════════════════════════════════════════════════════════════════════════════

CREATE TABLE vendas_faturadas (
    id               BIGSERIAL PRIMARY KEY,
    empresa_id       UUID         NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    data_faturamento DATE         NOT NULL,

    cod_gerente      TEXT         NOT NULL DEFAULT '',
    nome_gerente     TEXT         NOT NULL DEFAULT '',
    cod_supervisor   TEXT         NOT NULL DEFAULT '',
    nome_supervisor  TEXT         NOT NULL DEFAULT '',
    qtrca_supervisor INTEGER      NOT NULL DEFAULT 0,
    cod_rca          TEXT         NOT NULL DEFAULT '',
    nome_rca         TEXT         NOT NULL DEFAULT '',
    qtcli_rca        INTEGER      NOT NULL DEFAULT 0,

    cod_fornec       TEXT         NOT NULL DEFAULT '',
    nome_fornec      TEXT         NOT NULL DEFAULT '',

    cod_cli          TEXT         NOT NULL DEFAULT '',
    nome_cli         TEXT         NOT NULL DEFAULT '',
    uf               TEXT         NOT NULL DEFAULT '',
    empresa          TEXT         NOT NULL DEFAULT '',

    cod_prod         TEXT         NOT NULL DEFAULT '',
    nome_prod        TEXT         NOT NULL DEFAULT '',
    ean              TEXT         NOT NULL DEFAULT '',

    qt               NUMERIC(15,3) NOT NULL DEFAULT 0,
    pvenda           NUMERIC(15,2) NOT NULL DEFAULT 0,
    plucro           NUMERIC(15,2) NOT NULL DEFAULT 0,

    importado_em     TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_vf_emp_data             ON vendas_faturadas (empresa_id, data_faturamento);
CREATE INDEX idx_vf_emp_fornec           ON vendas_faturadas (empresa_id, cod_fornec);
CREATE INDEX idx_vf_emp_supervisor       ON vendas_faturadas (empresa_id, cod_supervisor);
CREATE INDEX idx_vf_emp_rca              ON vendas_faturadas (empresa_id, cod_rca);
CREATE INDEX idx_vf_emp_cli_data         ON vendas_faturadas (empresa_id, cod_cli, data_faturamento);
CREATE INDEX idx_vf_emp_data_produto     ON vendas_faturadas (empresa_id, data_faturamento, cod_prod) WHERE cod_prod <> '';

-- ═══════════════════════════════════════════════════════════════════════════════
-- vendas_transmitidas (sem tipo_base)
-- ═══════════════════════════════════════════════════════════════════════════════

CREATE TABLE vendas_transmitidas (
    id               BIGSERIAL PRIMARY KEY,
    empresa_id       UUID         NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    data_transmissao DATE         NOT NULL,

    cod_gerente      TEXT         NOT NULL DEFAULT '',
    nome_gerente     TEXT         NOT NULL DEFAULT '',
    cod_supervisor   TEXT         NOT NULL DEFAULT '',
    nome_supervisor  TEXT         NOT NULL DEFAULT '',
    qtrca_supervisor INTEGER      NOT NULL DEFAULT 0,
    cod_rca          TEXT         NOT NULL DEFAULT '',
    nome_rca         TEXT         NOT NULL DEFAULT '',
    qtcli_rca        INTEGER      NOT NULL DEFAULT 0,

    cod_fornec       TEXT         NOT NULL DEFAULT '',
    nome_fornec      TEXT         NOT NULL DEFAULT '',

    cod_cli          TEXT         NOT NULL DEFAULT '',
    nome_cli         TEXT         NOT NULL DEFAULT '',
    uf               TEXT         NOT NULL DEFAULT '',
    empresa          TEXT         NOT NULL DEFAULT '',

    cod_prod         TEXT         NOT NULL DEFAULT '',
    nome_prod        TEXT         NOT NULL DEFAULT '',
    ean              TEXT         NOT NULL DEFAULT '',

    qt               NUMERIC(15,3) NOT NULL DEFAULT 0,
    pvenda           NUMERIC(15,2) NOT NULL DEFAULT 0,
    plucro           NUMERIC(15,2) NOT NULL DEFAULT 0,

    importado_em     TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_vt_emp_data             ON vendas_transmitidas (empresa_id, data_transmissao);
CREATE INDEX idx_vt_emp_fornec           ON vendas_transmitidas (empresa_id, cod_fornec);
CREATE INDEX idx_vt_emp_supervisor       ON vendas_transmitidas (empresa_id, cod_supervisor);
CREATE INDEX idx_vt_emp_rca              ON vendas_transmitidas (empresa_id, cod_rca);
CREATE INDEX idx_vt_emp_cli_data         ON vendas_transmitidas (empresa_id, cod_cli, data_transmissao);
CREATE INDEX idx_vt_emp_data_produto     ON vendas_transmitidas (empresa_id, data_transmissao, cod_prod) WHERE cod_prod <> '';

-- ═══════════════════════════════════════════════════════════════════════════════
-- vendas_import_jobs — remove tipo_base e seu CHECK
-- ═══════════════════════════════════════════════════════════════════════════════

ALTER TABLE vendas_import_jobs DROP CONSTRAINT IF EXISTS vendas_import_jobs_tipo_base_check;
ALTER TABLE vendas_import_jobs DROP COLUMN IF EXISTS tipo_base;
