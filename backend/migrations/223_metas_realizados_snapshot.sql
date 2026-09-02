-- Migration 223: Snapshot de Realizado congelado — Épico 4, Story 4.3
--
-- FR17: meses fechados devem ter resultado congelado — recalcular só por
-- ação explícita, nunca automático/silencioso. Até aqui (Story 4.1/4.2) o
-- Realizado era sempre calculado ao vivo, a cada request — não existia
-- "congelamento" de verdade porque não existia nada persistido pra
-- congelar. Esta tabela é esse snapshot.
--
-- Guardado como JSONB do resultado inteiro (não uma linha por Rede) —
-- mais simples de implementar/consultar, e "snapshot" é exatamente a
-- semântica certa (blob imutável de um cálculo específico), não dado
-- relacional que precisa ser somado depois.
--
-- UNIQUE (vigencia_id, fluxo, nivel): um snapshot por combinação. Reprocessar
-- manualmente faz UPDATE (motivo muda pra 'reprocessamento_manual',
-- calculado_em atualiza) — a linha antiga não fica de "histórico duplo"
-- porque o snapshot É o congelamento atual, não um log de todas as vezes
-- que já foi calculado (isso já existe em farol.sp_audit_log).

CREATE TABLE IF NOT EXISTS farol.metas_realizados_snapshot (
    id             SERIAL       PRIMARY KEY,
    empresa_id     UUID         NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    vinculo_id     INTEGER      NOT NULL REFERENCES farol.metas_vinculos(id) ON DELETE CASCADE,
    vigencia_id    INTEGER      NOT NULL REFERENCES farol.metas_vigencias(id) ON DELETE CASCADE,
    fluxo          TEXT         NOT NULL,
    nivel          TEXT         NOT NULL,
    resultado_json JSONB        NOT NULL,
    motivo         TEXT         NOT NULL DEFAULT 'congelamento_automatico',
    calculado_em   TIMESTAMPTZ  NOT NULL DEFAULT now(),

    CONSTRAINT uq_farol_metas_realizados_snapshot UNIQUE (vigencia_id, fluxo, nivel),
    CONSTRAINT ck_farol_metas_realizados_snapshot_motivo CHECK (motivo IN ('congelamento_automatico', 'reprocessamento_manual'))
);

CREATE INDEX IF NOT EXISTS idx_farol_metas_realizados_snapshot_vigencia ON farol.metas_realizados_snapshot (vigencia_id);
