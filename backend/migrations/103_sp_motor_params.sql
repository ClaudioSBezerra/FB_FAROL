-- Migration 103: sp_motor_params
-- Story 3.3 — Migrations de Parâmetros do Motor
--
-- Um único registro por CD define os parâmetros da calibragem.
-- Regra crítica (architecture.md): curva_a_nunca_reduz = TRUE por padrão;
--   Curva A nunca tem capacidade reduzida pelo motor.
--
-- Parâmetros:
--   dias_analise          → janela de análise em dias (padrão 90)
--   curva_a_max_est       → dias máximos de estoque aceitos para Curva A
--   curva_b_max_est       → dias máximos de estoque aceitos para Curva B
--   curva_c_max_est       → dias máximos de estoque aceitos para Curva C
--   fator_seguranca       → multiplicador sobre a média de vendas (ex: 1.10 = +10%)
--   curva_a_nunca_reduz   → Curva A: motor nunca propõe redução de capacidade
--   min_capacidade        → capacidade mínima absoluta que pode ser sugerida

CREATE TABLE IF NOT EXISTS smartpick.sp_motor_params (
    id                  SERIAL          PRIMARY KEY,
    cd_id               INTEGER         NOT NULL
                            REFERENCES smartpick.sp_centros_dist(id) ON DELETE CASCADE,
    empresa_id          UUID            NOT NULL
                            REFERENCES public.companies(id) ON DELETE CASCADE,
    -- Janela temporal
    dias_analise        INTEGER         NOT NULL DEFAULT 90
                            CHECK (dias_analise BETWEEN 30 AND 365),
    -- Limites de estoque por curva (dias)
    curva_a_max_est     INTEGER         NOT NULL DEFAULT 7
                            CHECK (curva_a_max_est >= 1),
    curva_b_max_est     INTEGER         NOT NULL DEFAULT 15
                            CHECK (curva_b_max_est >= 1),
    curva_c_max_est     INTEGER         NOT NULL DEFAULT 30
                            CHECK (curva_c_max_est >= 1),
    -- Fator de segurança
    fator_seguranca     NUMERIC(5,2)    NOT NULL DEFAULT 1.10
                            CHECK (fator_seguranca BETWEEN 1.00 AND 3.00),
    -- Regras negociais
    curva_a_nunca_reduz BOOLEAN         NOT NULL DEFAULT TRUE,
    min_capacidade      INTEGER         NOT NULL DEFAULT 1
                            CHECK (min_capacidade >= 1),
    -- Auditoria
    updated_by          UUID            REFERENCES public.users(id) ON DELETE SET NULL,
    created_at          TIMESTAMPTZ     NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ     NOT NULL DEFAULT now(),

    CONSTRAINT uq_sp_motor_params_cd UNIQUE (cd_id)
);

CREATE OR REPLACE TRIGGER trg_sp_motor_params_updated_at
  BEFORE UPDATE ON smartpick.sp_motor_params
  FOR EACH ROW EXECUTE FUNCTION smartpick.set_updated_at();
