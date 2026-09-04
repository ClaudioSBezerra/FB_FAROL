-- Migration 221: Itens Válidos (EAN + embalagem) — Épico 3, Story 3.3
--
-- Mesmo modelo de escopo da Story 3.2 (vinculo_id/vigencia_id FK — lista
-- mensal por Indústria/Fornecedor × vigência, FR12). Um EAN pode mapear pra
-- mais de um cod_prod (variantes/embalagens diferentes) — por isso a
-- unicidade é (vigencia_id, ean, cod_prod), não (vigencia_id, ean).
--
-- tipo_embalagem é CHECK constraint (não texto livre feito rede_nome):
-- diferente de "Rede" (que varia por fornecedor), embalagem é um conceito
-- fixo do catálogo de produtos do distribuidor (UN/CX/Pacote/Display) — não
-- há razão de negócio pra um fornecedor inventar um tipo novo. É esse campo
-- que decide se a regra de quantidade mínima do Sortimento (FR2) se aplica.

CREATE TABLE IF NOT EXISTS farol.metas_itens_validos (
    id              SERIAL       PRIMARY KEY,
    empresa_id      UUID         NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    vinculo_id      INTEGER      NOT NULL REFERENCES farol.metas_vinculos(id) ON DELETE CASCADE,
    vigencia_id     INTEGER      NOT NULL REFERENCES farol.metas_vigencias(id) ON DELETE CASCADE,
    ean             TEXT         NOT NULL,
    cod_prod        TEXT         NOT NULL,
    tipo_embalagem  TEXT         NOT NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),

    CONSTRAINT uq_farol_metas_itens_validos_vigencia_ean_prod UNIQUE (vigencia_id, ean, cod_prod),
    CONSTRAINT ck_farol_metas_itens_validos_embalagem CHECK (tipo_embalagem IN ('UN', 'CX', 'PACOTE', 'DISPLAY'))
);

CREATE INDEX IF NOT EXISTS idx_farol_metas_itens_validos_vigencia ON farol.metas_itens_validos (vigencia_id);
CREATE INDEX IF NOT EXISTS idx_farol_metas_itens_validos_ean ON farol.metas_itens_validos (vigencia_id, ean);
CREATE INDEX IF NOT EXISTS idx_farol_metas_itens_validos_prod ON farol.metas_itens_validos (cod_prod);
