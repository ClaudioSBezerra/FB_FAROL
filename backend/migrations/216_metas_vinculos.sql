-- Migration 216: Vínculo Indústria × Tipo de Métrica — Épico 2, Story 2.1
-- (Painel de Gestão de Metas por Indústria, ver _bmad-output/planning-artifacts/epics.md)
--
-- Um Vínculo associa uma Indústria (farol.industrias — que já representa
-- "Fornecedor específico", ex.: UNILEVER HC/396 e UNILEVER FOOD/131 já são
-- 2 linhas separadas nessa tabela, ver migration 210) a um Tipo de Métrica,
-- guardando os VALORES concretos dos parâmetros que o Tipo de Métrica exige
-- (ex.: limiar_valor_medio=9100 pra HC, =1500 pra Foods — mesmo Tipo de
-- Métrica "Cobertura por Rede", parâmetros diferentes). Isso é distinto da
-- META por faixa/vigência (Story 2.2) — parametros_valores calibra COMO a
-- métrica é calculada, meta é O QUANTO precisa atingir.
--
-- ON DELETE RESTRICT em tipo_metrica_id (não CASCADE): apagar um Tipo de
-- Métrica que já tem vínculo ativo tem impacto financeiro — deve falhar
-- alto e explícito, nunca apagar em cascata silenciosamente.
-- ON DELETE CASCADE em industria_id: se a indústria em si for removida, o
-- vínculo perde sentido (mesmo padrão de industria_fornecedores).

CREATE TABLE IF NOT EXISTS farol.metas_vinculos (
    id                 SERIAL       PRIMARY KEY,
    empresa_id         UUID         NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    industria_id       INTEGER      NOT NULL REFERENCES farol.industrias(id) ON DELETE CASCADE,
    tipo_metrica_id    INTEGER      NOT NULL REFERENCES farol.tipos_metrica(id) ON DELETE RESTRICT,
    parametros_valores JSONB        NOT NULL DEFAULT '{}',
    ativo              BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at         TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ  NOT NULL DEFAULT now(),

    CONSTRAINT uq_farol_metas_vinculos_empresa_industria_tipo UNIQUE (empresa_id, industria_id, tipo_metrica_id)
);

CREATE INDEX IF NOT EXISTS idx_farol_metas_vinculos_empresa ON farol.metas_vinculos (empresa_id);
CREATE INDEX IF NOT EXISTS idx_farol_metas_vinculos_industria ON farol.metas_vinculos (industria_id);
CREATE INDEX IF NOT EXISTS idx_farol_metas_vinculos_tipo ON farol.metas_vinculos (tipo_metrica_id);
