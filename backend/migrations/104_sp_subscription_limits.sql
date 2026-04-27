-- Migration 104: sp_subscription_limits
-- Story 3.5 — Gestão de Planos e Limites de CDs
--
-- Um registro por empresa define os limites do plano SaaS.
-- O backend valida estes limites antes de criar novos CDs.
--
-- Planos disponíveis:
--   basic      → até 1 filial,  3 CDs,  5 usuários
--   pro        → até 5 filiais, 10 CDs, 20 usuários
--   enterprise → ilimitado (max_* = -1 significa sem limite)

CREATE TABLE IF NOT EXISTS smartpick.sp_subscription_limits (
    id              SERIAL          PRIMARY KEY,
    empresa_id      UUID            NOT NULL
                        REFERENCES public.companies(id) ON DELETE CASCADE,
    plano           TEXT            NOT NULL DEFAULT 'basic'
                        CHECK (plano IN ('basic', 'pro', 'enterprise')),
    max_filiais     INTEGER         NOT NULL DEFAULT 1,   -- -1 = ilimitado
    max_cds         INTEGER         NOT NULL DEFAULT 3,   -- -1 = ilimitado
    max_usuarios    INTEGER         NOT NULL DEFAULT 5,   -- -1 = ilimitado
    ativo           BOOLEAN         NOT NULL DEFAULT TRUE,
    valido_ate      TIMESTAMPTZ,                          -- NULL = sem vencimento
    created_at      TIMESTAMPTZ     NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ     NOT NULL DEFAULT now(),

    CONSTRAINT uq_sp_subscription_empresa UNIQUE (empresa_id)
);

CREATE OR REPLACE TRIGGER trg_sp_subscription_updated_at
  BEFORE UPDATE ON smartpick.sp_subscription_limits
  FOR EACH ROW EXECUTE FUNCTION smartpick.set_updated_at();

-- ─── Seed: plano básico para novas empresas via trigger ───────────────────────
-- Quando uma empresa é criada (companies), insere automaticamente o plano basic.
-- Usa a função do schema padrão para não impactar migrations anteriores.
CREATE OR REPLACE FUNCTION smartpick.seed_subscription_for_company()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
  INSERT INTO smartpick.sp_subscription_limits (empresa_id, plano, max_filiais, max_cds, max_usuarios)
  VALUES (NEW.id, 'basic', 1, 3, 5)
  ON CONFLICT (empresa_id) DO NOTHING;
  RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_seed_subscription ON public.companies;
CREATE TRIGGER trg_seed_subscription
  AFTER INSERT ON public.companies
  FOR EACH ROW EXECUTE FUNCTION smartpick.seed_subscription_for_company();
