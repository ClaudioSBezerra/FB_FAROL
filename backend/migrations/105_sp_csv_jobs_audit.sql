-- Migration 105: sp_csv_jobs e sp_audit_log
-- Story 4.1 — Migrations de Jobs CSV e Audit Log
--
-- sp_csv_jobs: controla o ciclo de vida de cada upload de CSV.
--   Status: pending → processing → done | failed
--   O worker goroutine consulta registros 'pending', processa e atualiza status.
--
-- sp_audit_log: registro imutável de operações de escrita (aprovação,
--   edição inline, duplicação de CD, etc.). Nunca deletar registros aqui.
--
-- NOTA: sp_enderecos foi criada na migration 100 com FK job_id sem constraint
--   formal. Esta migration adiciona a FK formal de sp_enderecos.job_id → sp_csv_jobs.id.

-- ─── sp_csv_jobs ─────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS smartpick.sp_csv_jobs (
    id              UUID            PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id      UUID            NOT NULL REFERENCES public.companies(id) ON DELETE CASCADE,
    filial_id       INTEGER         NOT NULL REFERENCES smartpick.sp_filiais(id) ON DELETE CASCADE,
    cd_id           INTEGER         NOT NULL REFERENCES smartpick.sp_centros_dist(id) ON DELETE CASCADE,
    uploaded_by     UUID            NOT NULL REFERENCES public.users(id) ON DELETE SET NULL,
    filename        TEXT            NOT NULL,
    file_path       TEXT            NOT NULL,           -- caminho físico no servidor
    total_linhas    INTEGER,                            -- preenchido após parse
    linhas_ok       INTEGER,
    linhas_erro     INTEGER,
    status          TEXT            NOT NULL DEFAULT 'pending'
                        CHECK (status IN ('pending','processing','done','failed')),
    erro_msg        TEXT,                              -- mensagem de erro se failed
    started_at      TIMESTAMPTZ,
    finished_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ     NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_sp_csv_jobs_empresa    ON smartpick.sp_csv_jobs (empresa_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_sp_csv_jobs_status     ON smartpick.sp_csv_jobs (status) WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS idx_sp_csv_jobs_cd         ON smartpick.sp_csv_jobs (cd_id);

-- ─── FK retroativa: sp_enderecos.job_id → sp_csv_jobs.id ─────────────────────
-- Migration 100 criou sp_enderecos com job_id UUID mas sem FK formal
-- (sp_csv_jobs não existia ainda). Adicionamos agora.
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.table_constraints
    WHERE constraint_name = 'fk_sp_enderecos_job'
      AND table_schema = 'smartpick'
  ) THEN
    ALTER TABLE smartpick.sp_enderecos
      ADD CONSTRAINT fk_sp_enderecos_job
      FOREIGN KEY (job_id) REFERENCES smartpick.sp_csv_jobs(id)
      ON DELETE CASCADE;
  END IF;
END
$$;

-- ─── sp_audit_log ─────────────────────────────────────────────────────────────
-- Registra operações de escrita sensíveis. Imutável: sem UPDATE, sem DELETE.
-- Campos:
--   entidade    → nome da tabela afetada (sp_propostas, sp_centros_dist, etc.)
--   entidade_id → ID do registro afetado
--   acao        → verbo da operação (aprovar, editar, duplicar, rejeitar, etc.)
--   payload     → JSON com valores antes/depois ou contexto da operação
CREATE TABLE IF NOT EXISTS smartpick.sp_audit_log (
    id          BIGSERIAL       PRIMARY KEY,
    empresa_id  UUID            NOT NULL REFERENCES public.companies(id) ON DELETE CASCADE,
    user_id     UUID            REFERENCES public.users(id) ON DELETE SET NULL,
    entidade    TEXT            NOT NULL,
    entidade_id TEXT            NOT NULL,
    acao        TEXT            NOT NULL,
    payload     JSONB,
    ip_addr     TEXT,
    created_at  TIMESTAMPTZ     NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_sp_audit_empresa   ON smartpick.sp_audit_log (empresa_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_sp_audit_entidade  ON smartpick.sp_audit_log (entidade, entidade_id);
CREATE INDEX IF NOT EXISTS idx_sp_audit_user      ON smartpick.sp_audit_log (user_id);
