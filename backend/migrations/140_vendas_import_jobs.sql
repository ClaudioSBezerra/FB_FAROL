-- 140_vendas_import_jobs.sql
-- Rastreamento de jobs de importação (padrão FB_APU01):
--   pending → processing → done | error | cancelled
-- Permite cancelamento e polling de progresso sem SSE.

CREATE TABLE IF NOT EXISTS vendas_import_jobs (
    id            UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id    UUID         NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    tipo_base     TEXT         NOT NULL CHECK (tipo_base IN ('ATUAL','COMPARATIVA')),
    ano           INTEGER      NOT NULL,
    mes           INTEGER      NOT NULL CHECK (mes BETWEEN 1 AND 12),
    status        TEXT         NOT NULL DEFAULT 'pending'
                               CHECK (status IN ('pending','processing','done','error','cancelled')),
    progress      INTEGER      NOT NULL DEFAULT 0 CHECK (progress BETWEEN 0 AND 100),
    total_lines   INTEGER      NOT NULL DEFAULT 0,
    importados    INTEGER      NOT NULL DEFAULT 0,
    message       TEXT         NOT NULL DEFAULT '',
    criado_em     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    atualizado_em TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_vij_empresa_status
    ON vendas_import_jobs(empresa_id, status, criado_em DESC);
