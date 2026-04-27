-- Migration 102: sp_filiais e sp_centros_dist
-- Story 3.1 — Migrations de Filiais e CDs
--
-- sp_filiais: representa as filiais WMS de uma empresa (tenant).
--   cod_filial vem do CSV (CODFILIAL); é único dentro da empresa.
--
-- sp_centros_dist: Centros de Distribuição (CDs) vinculados a uma filial.
--   Um CD pode ser duplicado (self-referência via fonte_cd_id).
--   Cada CD tem seu conjunto de parâmetros de motor (migration 103).
--
-- Após esta migration: adiciona FK em sp_user_filiais.filial_id → sp_filiais.id

-- ─── sp_filiais ───────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS smartpick.sp_filiais (
    id          SERIAL          PRIMARY KEY,
    empresa_id  UUID            NOT NULL REFERENCES public.companies(id) ON DELETE CASCADE,
    cod_filial  INTEGER         NOT NULL,           -- CODFILIAL do WMS (ex: 11)
    nome        TEXT            NOT NULL,
    ativo       BOOLEAN         NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ     NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ     NOT NULL DEFAULT now(),

    CONSTRAINT uq_sp_filiais_empresa_cod UNIQUE (empresa_id, cod_filial)
);

CREATE INDEX IF NOT EXISTS idx_sp_filiais_empresa ON smartpick.sp_filiais (empresa_id);

CREATE OR REPLACE TRIGGER trg_sp_filiais_updated_at
  BEFORE UPDATE ON smartpick.sp_filiais
  FOR EACH ROW EXECUTE FUNCTION smartpick.set_updated_at();

-- ─── FK retroativa em sp_user_filiais ────────────────────────────────────────
-- Vincula o filial_id de sp_user_filiais ao id de sp_filiais.
-- O constraint é deferred para permitir inserção na ordem correta.
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.table_constraints
    WHERE constraint_name = 'fk_sp_user_filiais_filial'
      AND table_schema = 'smartpick'
  ) THEN
    ALTER TABLE smartpick.sp_user_filiais
      ADD CONSTRAINT fk_sp_user_filiais_filial
      FOREIGN KEY (filial_id) REFERENCES smartpick.sp_filiais(id)
      ON DELETE CASCADE
      DEFERRABLE INITIALLY DEFERRED;
  END IF;
END
$$;

-- ─── sp_centros_dist ─────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS smartpick.sp_centros_dist (
    id          SERIAL          PRIMARY KEY,
    filial_id   INTEGER         NOT NULL REFERENCES smartpick.sp_filiais(id) ON DELETE CASCADE,
    empresa_id  UUID            NOT NULL REFERENCES public.companies(id) ON DELETE CASCADE,
    nome        TEXT            NOT NULL,
    descricao   TEXT,
    ativo       BOOLEAN         NOT NULL DEFAULT TRUE,
    -- Suporte a duplicação de CD (self-referência)
    fonte_cd_id INTEGER         REFERENCES smartpick.sp_centros_dist(id) ON DELETE SET NULL,
    criado_por  UUID            REFERENCES public.users(id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ     NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ     NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_sp_cds_filial   ON smartpick.sp_centros_dist (filial_id);
CREATE INDEX IF NOT EXISTS idx_sp_cds_empresa  ON smartpick.sp_centros_dist (empresa_id);

CREATE OR REPLACE TRIGGER trg_sp_cds_updated_at
  BEFORE UPDATE ON smartpick.sp_centros_dist
  FOR EACH ROW EXECUTE FUNCTION smartpick.set_updated_at();
