-- 136_farol_v2_schema.sql
-- Reescrita 2026 — tabelas do novo Farol de Vendas.
-- Substitui objetivos_importados (que fica intacta para compatibilidade de código antigo).

-- ─── Vendas importadas ────────────────────────────────────────────────────────
-- Uma row por linha do CSV de vendas.
-- Clientes sem venda chegam com qt=0, pvenda=0 (para positivação correta).
CREATE TABLE IF NOT EXISTS vendas_importadas (
    id               BIGSERIAL PRIMARY KEY,
    empresa_id       UUID         NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    tipo_base        TEXT         NOT NULL CHECK (tipo_base IN ('ATUAL','COMPARATIVA')),
    ano              INTEGER      NOT NULL,
    mes              INTEGER      NOT NULL CHECK (mes BETWEEN 1 AND 12),
    estado           TEXT         NOT NULL DEFAULT 'FATURADO' CHECK (estado IN ('FATURADO','TRANSMITIDO')),

    cod_gerente      TEXT         NOT NULL DEFAULT '',
    nome_gerente     TEXT         NOT NULL DEFAULT '',
    cod_supervisor   TEXT         NOT NULL DEFAULT '',
    nome_supervisor  TEXT         NOT NULL DEFAULT '',
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

    qt               NUMERIC(15,3) NOT NULL DEFAULT 0,
    pvenda           NUMERIC(15,2) NOT NULL DEFAULT 0,

    importado_em     TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_vi_empresa          ON vendas_importadas(empresa_id);
CREATE INDEX IF NOT EXISTS idx_vi_emp_base_periodo ON vendas_importadas(empresa_id, tipo_base, ano, mes);
CREATE INDEX IF NOT EXISTS idx_vi_emp_fornec       ON vendas_importadas(empresa_id, cod_fornec);
CREATE INDEX IF NOT EXISTS idx_vi_emp_supervisor   ON vendas_importadas(empresa_id, cod_supervisor);
CREATE INDEX IF NOT EXISTS idx_vi_emp_rca          ON vendas_importadas(empresa_id, cod_rca);

-- ─── Configuração de indústrias (trava mínima) ────────────────────────────────
-- Opcional: define qt mínima para considerar cliente como positivado.
-- Default=1 quando não cadastrado.
CREATE TABLE IF NOT EXISTS industrias_config (
    id                   BIGSERIAL PRIMARY KEY,
    empresa_id           UUID        NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    cod_fornec           TEXT        NOT NULL,
    nome_fornec          TEXT        NOT NULL DEFAULT '',
    programa_distribuicao TEXT       NOT NULL DEFAULT '',
    trava_minima_qt      NUMERIC(15,3) NOT NULL DEFAULT 1,
    atualizado_em        TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE (empresa_id, cod_fornec)
);

CREATE INDEX IF NOT EXISTS idx_ic_empresa_fornec ON industrias_config(empresa_id, cod_fornec);

-- ─── Extensão da tabela users ─────────────────────────────────────────────────
-- tipo_persona: diretoria | gerente | ggv | supervisor | NULL (admin/sem restricao)
-- cod_referencia: código operacional do gerente/supervisor (filtra visão)
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name='users' AND column_name='tipo_persona'
    ) THEN
        ALTER TABLE users ADD COLUMN tipo_persona TEXT DEFAULT NULL;
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name='users' AND column_name='cod_referencia'
    ) THEN
        ALTER TABLE users ADD COLUMN cod_referencia TEXT DEFAULT NULL;
    END IF;
END$$;
