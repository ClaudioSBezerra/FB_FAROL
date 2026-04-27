-- Migration 107: sp_historico
-- Story 7.1 — Migrations de Histórico
--
-- sp_historico: registra cada ciclo de calibragem executado por CD.
-- Um ciclo é criado automaticamente quando o motor roda (status = 'em_andamento')
-- e fechado quando todas as propostas são aprovadas/rejeitadas (status = 'concluido').
-- Ciclos sem execução por CD são detectáveis via query de compliance.

CREATE TABLE IF NOT EXISTS smartpick.sp_historico (
    id              BIGSERIAL       PRIMARY KEY,
    job_id          UUID            REFERENCES smartpick.sp_csv_jobs(id) ON DELETE SET NULL,
    cd_id           INTEGER         NOT NULL
                        REFERENCES smartpick.sp_centros_dist(id) ON DELETE CASCADE,
    empresa_id      UUID            NOT NULL
                        REFERENCES public.companies(id) ON DELETE CASCADE,
    -- Snapshot de contagens ao fechar o ciclo
    total_propostas INTEGER         NOT NULL DEFAULT 0,
    aprovadas       INTEGER         NOT NULL DEFAULT 0,
    rejeitadas      INTEGER         NOT NULL DEFAULT 0,
    pendentes       INTEGER         NOT NULL DEFAULT 0,
    -- Distribuição por curva
    curva_a         INTEGER         NOT NULL DEFAULT 0,
    curva_b         INTEGER         NOT NULL DEFAULT 0,
    curva_c         INTEGER         NOT NULL DEFAULT 0,
    -- Workflow
    executado_por   UUID            REFERENCES public.users(id) ON DELETE SET NULL,
    executado_em    TIMESTAMPTZ     NOT NULL DEFAULT now(),
    concluido_em    TIMESTAMPTZ,
    status          TEXT            NOT NULL DEFAULT 'em_andamento'
                        CHECK (status IN ('em_andamento', 'concluido', 'nao_executado')),
    observacao      TEXT,
    created_at      TIMESTAMPTZ     NOT NULL DEFAULT now()
);

-- Índices para dashboard de compliance
CREATE INDEX IF NOT EXISTS idx_sp_historico_cd_status
    ON smartpick.sp_historico (cd_id, status, executado_em DESC);

CREATE INDEX IF NOT EXISTS idx_sp_historico_empresa_data
    ON smartpick.sp_historico (empresa_id, executado_em DESC);
