-- Migration 134: corrige função seed_subscription_for_company após rename smartpick → farol
--
-- A migration 124 renomeou o schema 'smartpick' para 'farol'. O rename atualiza
-- nomes de tabelas/funções, MAS o corpo das funções (PL/pgSQL) é texto e mantém
-- as referências hardcoded ao schema antigo.
--
-- A função smartpick.seed_subscription_for_company() (criada na 104) virou
-- farol.seed_subscription_for_company(), mas o INSERT no corpo ainda referencia
-- smartpick.sp_subscription_limits que não existe mais → erro 42P01 ao cadastrar
-- nova empresa (o trigger trg_seed_subscription dispara após INSERT em companies).
--
-- Esta migration recria a função apontando para o schema correto 'farol'.

CREATE OR REPLACE FUNCTION farol.seed_subscription_for_company()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
  INSERT INTO farol.sp_subscription_limits (empresa_id, plano, max_filiais, max_cds, max_usuarios)
  VALUES (NEW.id, 'basic', 1, 3, 5)
  ON CONFLICT (empresa_id) DO NOTHING;
  RETURN NEW;
END;
$$;

-- Recria trigger garantindo que aponta para a função no schema farol
DROP TRIGGER IF EXISTS trg_seed_subscription ON public.companies;
CREATE TRIGGER trg_seed_subscription
  AFTER INSERT ON public.companies
  FOR EACH ROW EXECUTE FUNCTION farol.seed_subscription_for_company();
