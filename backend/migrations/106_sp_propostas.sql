-- Migration 106: sp_propostas
-- Story 4.4 — Migrations de Propostas
--
-- sp_propostas: armazena a proposta de recalibração gerada pelo motor
--   para cada endereço de picking (sp_enderecos) de um job.
--
-- Status do ciclo de vida:
--   pendente   → gerada pelo motor, aguarda revisão do gestor
--   aprovada   → gestor aprovou (individual ou em lote)
--   rejeitada  → gestor rejeitou / decidiu manter capacidade atual
--
-- Regra crítica (architecture.md):
--   Para Curva A com curva_a_nunca_reduz = TRUE, o motor nunca gera
--   sugestao_calibragem < capacidade_atual. O handler do motor aplica
--   esta regra antes de inserir em sp_propostas.
--
-- Campos calculados pelo motor:
--   sugestao_calibragem → nova capacidade proposta
--   delta               → diferença (sugestao - capacidade_atual)
--   justificativa       → texto gerado pelo motor explicando o cálculo

CREATE TABLE IF NOT EXISTS smartpick.sp_propostas (
    id                  BIGSERIAL       PRIMARY KEY,
    job_id              UUID            NOT NULL
                            REFERENCES smartpick.sp_csv_jobs(id) ON DELETE CASCADE,
    endereco_id         BIGINT          NOT NULL
                            REFERENCES smartpick.sp_enderecos(id) ON DELETE CASCADE,
    empresa_id          UUID            NOT NULL
                            REFERENCES public.companies(id) ON DELETE CASCADE,
    cd_id               INTEGER         NOT NULL
                            REFERENCES smartpick.sp_centros_dist(id) ON DELETE CASCADE,
    -- Dados do endereço (desnormalizados para leitura rápida no dashboard)
    cod_filial          INTEGER         NOT NULL,
    codprod             INTEGER         NOT NULL,
    produto             TEXT,
    rua                 INTEGER,
    predio              INTEGER,
    apto                INTEGER,
    classe_venda        CHAR(1),                        -- A/B/C
    -- Calibragem
    capacidade_atual    INTEGER,                        -- CAPACIDADE do CSV
    sugestao_calibragem INTEGER         NOT NULL,       -- calculado pelo motor
    delta               INTEGER
        GENERATED ALWAYS AS (sugestao_calibragem - COALESCE(capacidade_atual, 0)) STORED,
    justificativa       TEXT,                           -- explicação do motor
    -- Workflow de aprovação
    status              TEXT            NOT NULL DEFAULT 'pendente'
                            CHECK (status IN ('pendente', 'aprovada', 'rejeitada')),
    aprovado_por        UUID            REFERENCES public.users(id) ON DELETE SET NULL,
    aprovado_em         TIMESTAMPTZ,
    -- Edição inline pelo gestor (antes de aprovar)
    sugestao_editada    INTEGER,                        -- NULL = não editado
    editado_por         UUID            REFERENCES public.users(id) ON DELETE SET NULL,
    editado_em          TIMESTAMPTZ,
    created_at          TIMESTAMPTZ     NOT NULL DEFAULT now()
);

-- Índice principal do dashboard de urgência
CREATE INDEX IF NOT EXISTS idx_sp_propostas_job_status
  ON smartpick.sp_propostas (job_id, status);

CREATE INDEX IF NOT EXISTS idx_sp_propostas_cd_status
  ON smartpick.sp_propostas (cd_id, status, classe_venda);

CREATE INDEX IF NOT EXISTS idx_sp_propostas_empresa_status
  ON smartpick.sp_propostas (empresa_id, status, created_at DESC);

-- Índice para detectar urgência de falta (delta < 0 = capacidade sendo reduzida = risco falta)
-- e urgência de espaço (delta > 0 = capacidade sendo aumentada = risco espaço)
CREATE INDEX IF NOT EXISTS idx_sp_propostas_urgencia
  ON smartpick.sp_propostas (empresa_id, cd_id, delta, status)
  WHERE status = 'pendente';
