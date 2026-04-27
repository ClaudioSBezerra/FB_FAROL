-- Migration 115: sp_ignorados + status calibrado/ignorado em sp_propostas
--
-- sp_ignorados: produtos marcados pelo gestor para serem ignorados no próximo
--   ciclo de calibragem. O motor verifica esta tabela antes de gerar qualquer
--   proposta; produtos ignorados não recebem slot de ajuste nem de redução.
--   A tela "Ignorados" permite reativar (remover da lista).
--
-- Novos status em sp_propostas:
--   'calibrado' → sugestão dentro de 5% da capacidade atual (≥ 95% assertividade)
--   'ignorado'  → produto marcado pelo gestor neste ciclo

-- ─── Tabela sp_ignorados ──────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS smartpick.sp_ignorados (
    id              BIGSERIAL       PRIMARY KEY,
    empresa_id      UUID            NOT NULL
                        REFERENCES public.companies(id) ON DELETE CASCADE,
    cd_id           INTEGER         NOT NULL
                        REFERENCES smartpick.sp_centros_dist(id) ON DELETE CASCADE,
    codprod         INTEGER         NOT NULL,
    cod_filial      INTEGER         NOT NULL,
    produto         TEXT,
    motivo          TEXT,
    ignorado_por    UUID            REFERENCES public.users(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ     NOT NULL DEFAULT now(),
    UNIQUE (empresa_id, cd_id, codprod, cod_filial)
);

CREATE INDEX IF NOT EXISTS idx_sp_ignorados_cd
    ON smartpick.sp_ignorados(empresa_id, cd_id);

-- ─── Expande constraint de status em sp_propostas ─────────────────────────────
ALTER TABLE smartpick.sp_propostas
    DROP CONSTRAINT IF EXISTS sp_propostas_status_check;

ALTER TABLE smartpick.sp_propostas
    ADD CONSTRAINT sp_propostas_status_check
    CHECK (status IN ('pendente', 'aprovada', 'rejeitada', 'calibrado', 'ignorado'));
