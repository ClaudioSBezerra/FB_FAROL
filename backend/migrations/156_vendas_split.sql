-- 156_vendas_split.sql
-- Reestruturação 2026 — Opção B: duas tabelas separadas por natureza do registro.
--
-- DECISÃO ARQUITETURAL:
--   Cada linha do CSV representa UM evento exclusivo: ou um FATURAMENTO,
--   ou uma TRANSMISSÃO de pedido. Esses dois universos têm semânticas,
--   ciclos de vida e datas próprias. Mantê-los em tabelas separadas:
--     - elimina a ambiguidade da coluna data_processo (que dependia do estado)
--     - elimina a coluna estado da chave de TODAS as MVs
--     - permite políticas independentes de retenção, índices e refresh
--     - dobra o número de MVs, mas cada uma fica menor (cache hit melhor)
--
-- ESTRUTURA:
--   vendas_faturadas      — eventos de FATURAMENTO (NF emitida)
--   vendas_transmitidas   — eventos de TRANSMISSÃO (pedido digitado pelo RCA)
--
--   Ambas têm a MESMA estrutura de hierarquia/indicadores. A única diferença
--   é o NOME da coluna de data, que reflete a semântica:
--     vendas_faturadas.data_faturamento
--     vendas_transmitidas.data_transmissao
--
-- ATENÇÃO — DROP CASCADE:
--   Remove vendas_importadas e as 14 MVs criadas na migration 155.
--   Em produção isso seria destrutivo; aqui (localhost, fase de design)
--   é só limpeza.

DROP TABLE IF EXISTS vendas_importadas CASCADE;

-- ═══════════════════════════════════════════════════════════════════════════════
-- vendas_faturadas — eventos de FATURAMENTO
-- ═══════════════════════════════════════════════════════════════════════════════

CREATE TABLE vendas_faturadas (
    id               BIGSERIAL PRIMARY KEY,
    empresa_id       UUID         NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    tipo_base        TEXT         NOT NULL CHECK (tipo_base IN ('ATUAL','COMPARATIVA')),
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

CREATE INDEX idx_vf_emp_base_data        ON vendas_faturadas (empresa_id, tipo_base, data_faturamento);
CREATE INDEX idx_vf_emp_fornec           ON vendas_faturadas (empresa_id, cod_fornec);
CREATE INDEX idx_vf_emp_supervisor       ON vendas_faturadas (empresa_id, cod_supervisor);
CREATE INDEX idx_vf_emp_rca              ON vendas_faturadas (empresa_id, cod_rca);
CREATE INDEX idx_vf_emp_cli_data         ON vendas_faturadas (empresa_id, cod_cli, tipo_base, data_faturamento);
CREATE INDEX idx_vf_emp_data_produto     ON vendas_faturadas (empresa_id, tipo_base, data_faturamento, cod_prod) WHERE cod_prod <> '';

-- ═══════════════════════════════════════════════════════════════════════════════
-- vendas_transmitidas — eventos de TRANSMISSÃO de pedidos
-- ═══════════════════════════════════════════════════════════════════════════════

CREATE TABLE vendas_transmitidas (
    id               BIGSERIAL PRIMARY KEY,
    empresa_id       UUID         NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    tipo_base        TEXT         NOT NULL CHECK (tipo_base IN ('ATUAL','COMPARATIVA')),
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

CREATE INDEX idx_vt_emp_base_data        ON vendas_transmitidas (empresa_id, tipo_base, data_transmissao);
CREATE INDEX idx_vt_emp_fornec           ON vendas_transmitidas (empresa_id, cod_fornec);
CREATE INDEX idx_vt_emp_supervisor       ON vendas_transmitidas (empresa_id, cod_supervisor);
CREATE INDEX idx_vt_emp_rca              ON vendas_transmitidas (empresa_id, cod_rca);
CREATE INDEX idx_vt_emp_cli_data         ON vendas_transmitidas (empresa_id, cod_cli, tipo_base, data_transmissao);
CREATE INDEX idx_vt_emp_data_produto     ON vendas_transmitidas (empresa_id, tipo_base, data_transmissao, cod_prod) WHERE cod_prod <> '';
