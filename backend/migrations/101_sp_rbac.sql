-- Migration 101: RBAC SmartPick
-- Story 2.1 — Migrations de RBAC SmartPick
--
-- Perfis SmartPick (sp_role):
--   admin_fbtax     → Administrador FbTax; acesso total a todas as empresas
--   gestor_geral    → Gestor geral da empresa; vê todas as filiais do seu tenant
--   gestor_filial   → Gestor de filial(is) específica(s); escopo restrito via sp_user_filiais
--   somente_leitura → Apenas visualização; sem aprovação ou edição
--
-- Regras de escopo (sp_user_filiais):
--   all_filiais = TRUE  → acesso a todas as filiais da empresa (para gestor_geral)
--   filial_id IS NULL   → vínculo de empresa sem filial específica
--   filial_id NOT NULL  → acesso apenas àquela filial (para gestor_filial)

-- ─── Tipo ENUM para os perfis SmartPick ──────────────────────────────────────
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'sp_role_type') THEN
    CREATE TYPE smartpick.sp_role_type AS ENUM (
      'admin_fbtax',
      'gestor_geral',
      'gestor_filial',
      'somente_leitura'
    );
  END IF;
END
$$;

-- ─── Coluna sp_role na tabela users ──────────────────────────────────────────
-- Adicionada ao public.users para que o JWT possa carregar o perfil SmartPick.
-- Padrão 'somente_leitura' garante menor privilégio para usuários novos.
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = 'public' AND table_name = 'users' AND column_name = 'sp_role'
  ) THEN
    ALTER TABLE public.users
      ADD COLUMN sp_role smartpick.sp_role_type NOT NULL DEFAULT 'somente_leitura';
  END IF;
END
$$;

-- ─── sp_user_filiais ─────────────────────────────────────────────────────────
-- Tabela de escopo: define quais filiais cada usuário pode acessar por empresa.
-- Um mesmo usuário pode ter vínculos em empresas diferentes com escopos distintos.
--
-- Semântica:
--   all_filiais = TRUE               → usuário vê todas as filiais da empresa
--   all_filiais = FALSE, filial_id   → usuário vê apenas essa filial
--   (combinações adicionais no futuro podem ser adicionadas como novas linhas)
CREATE TABLE IF NOT EXISTS smartpick.sp_user_filiais (
    id          BIGSERIAL   PRIMARY KEY,
    user_id     UUID        NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    empresa_id  UUID        NOT NULL REFERENCES public.companies(id) ON DELETE CASCADE,
    filial_id   INTEGER,                          -- NULL quando all_filiais = TRUE
    all_filiais BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT sp_user_filiais_check
      CHECK (
        (all_filiais = TRUE  AND filial_id IS NULL) OR
        (all_filiais = FALSE AND filial_id IS NOT NULL)
      )
);

-- Um usuário não pode ter duas entradas para a mesma filial da mesma empresa
CREATE UNIQUE INDEX IF NOT EXISTS uq_sp_user_filiais_user_empresa_filial
  ON smartpick.sp_user_filiais (user_id, empresa_id, filial_id)
  WHERE filial_id IS NOT NULL;

-- E não pode ter duas entradas de "all_filiais" para a mesma empresa
CREATE UNIQUE INDEX IF NOT EXISTS uq_sp_user_filiais_user_empresa_all
  ON smartpick.sp_user_filiais (user_id, empresa_id)
  WHERE all_filiais = TRUE;

CREATE INDEX IF NOT EXISTS idx_sp_user_filiais_user    ON smartpick.sp_user_filiais (user_id);
CREATE INDEX IF NOT EXISTS idx_sp_user_filiais_empresa ON smartpick.sp_user_filiais (empresa_id);

-- ─── Função utilitária: atualiza updated_at automaticamente ──────────────────
CREATE OR REPLACE FUNCTION smartpick.set_updated_at()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
  NEW.updated_at = now();
  RETURN NEW;
END;
$$;

CREATE OR REPLACE TRIGGER trg_sp_user_filiais_updated_at
  BEFORE UPDATE ON smartpick.sp_user_filiais
  FOR EACH ROW EXECUTE FUNCTION smartpick.set_updated_at();
