-- Migration 217: Vigências e Faixas de meta — Épico 2, Story 2.2
-- (Painel de Gestão de Metas por Indústria, ver _bmad-output/planning-artifacts/epics.md)
--
-- Uma Vigência é um período (normalmente um mês, mas genérico) em que um
-- conjunto de metas por faixa vale, pra um Vínculo (Story 2.1). Um mesmo
-- vínculo pode ter várias vigências ao longo do ano (ex: jan-mar com metas
-- diferentes de abr-jun) — por isso vigência é uma ENTIDADE própria com
-- histórico, não um campo de data solto no vínculo (decisão já tomada no
-- FR5/Story 2.2 do epics.md).
--
-- btree_gist habilita o EXCLUDE abaixo: impede, no nível do banco, duas
-- vigências do MESMO vínculo com datas sobrepostas — mais seguro que só
-- validar em Go (garante mesmo sob concorrência).
--
-- status ('aberta'/'fechada') é o mecanismo de congelamento do FR17: uma
-- vigência fechada não pode ser editada por esta tela — só reprocessamento
-- manual explícito (Épico 4, ainda não implementado). O fechamento
-- automático "ao virar o mês" (mencionado no FR5) fica pro motor de
-- apuração (Épico 4) — aqui só existe o fechamento manual.

CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE IF NOT EXISTS farol.metas_vigencias (
    id           SERIAL       PRIMARY KEY,
    empresa_id   UUID         NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    vinculo_id   INTEGER      NOT NULL REFERENCES farol.metas_vinculos(id) ON DELETE CASCADE,
    data_inicio  DATE         NOT NULL,
    data_fim     DATE         NOT NULL,
    status       TEXT         NOT NULL DEFAULT 'aberta',
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),

    CONSTRAINT ck_farol_metas_vigencias_status CHECK (status IN ('aberta', 'fechada')),
    CONSTRAINT ck_farol_metas_vigencias_datas CHECK (data_fim >= data_inicio),
    CONSTRAINT ex_farol_metas_vigencias_sem_overlap EXCLUDE USING gist (
        vinculo_id WITH =,
        daterange(data_inicio, data_fim, '[]') WITH &&
    )
);

CREATE INDEX IF NOT EXISTS idx_farol_metas_vigencias_vinculo ON farol.metas_vigencias (vinculo_id);
CREATE INDEX IF NOT EXISTS idx_farol_metas_vigencias_empresa ON farol.metas_vigencias (empresa_id);

-- Faixas de meta (FR7: recurso genérico, qualquer Tipo de Métrica pode ter
-- uma ou várias) — valor_meta é o que a Story 2.2 chama de "meta", distinto
-- do parametros_valores do vínculo (Story 2.1, que calibra o CÁLCULO).
CREATE TABLE IF NOT EXISTS farol.metas_faixas (
    id           SERIAL         PRIMARY KEY,
    empresa_id   UUID           NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    vigencia_id  INTEGER        NOT NULL REFERENCES farol.metas_vigencias(id) ON DELETE CASCADE,
    faixa        INTEGER        NOT NULL,
    valor_meta   NUMERIC(18,4)  NOT NULL,
    created_at   TIMESTAMPTZ    NOT NULL DEFAULT now(),

    CONSTRAINT uq_farol_metas_faixas_vigencia_faixa UNIQUE (vigencia_id, faixa),
    CONSTRAINT ck_farol_metas_faixas_faixa_positiva CHECK (faixa > 0)
);

CREATE INDEX IF NOT EXISTS idx_farol_metas_faixas_vigencia ON farol.metas_faixas (vigencia_id);
