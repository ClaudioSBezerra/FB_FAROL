-- Migration 214: Catálogo de Tipos de Métrica — Painel de Gestão de Metas por Indústria
--
-- Primeira tabela do módulo "Painel de Gestão de Metas por Indústria" (Épico 1
-- da quebra em épicos, ver _bmad-output/planning-artifacts/epics.md). Um Tipo
-- de Métrica é uma forma de cálculo reutilizável (ex.: "Cobertura por Rede",
-- "Sortimento por Rede") que 2+ indústrias/fornecedores podem instanciar com
-- parâmetros próprios (Épico 2). A genericidade fica inteira dentro de
-- parametros_schema (JSONB) — a tabela NUNCA ganha coluna nova por causa de
-- um Tipo de Métrica com forma diferente (teste de generalidade do FR1, ver
-- prd.md linha 70: um tipo hipotético "Frequência de Visita por Cliente",
-- com nível de agregação e parâmetro totalmente diferentes de Cobertura/
-- Sortimento, precisa caber aqui sem migration adicional).
--
-- nivel_agregacao é restrito (CHECK) porque vem da hierarquia organizacional
-- real do Farol (GGV/CRV/RCA/Rede/Cliente), não é parâmetro livre do tipo.
--
-- Escopo: só o catálogo (CRUD). Vínculo com Indústria/Fornecedor, metas e
-- vigências são a Story 2.1+ (Épico 2) — não criados aqui.

CREATE TABLE IF NOT EXISTS farol.tipos_metrica (
    id                 SERIAL       PRIMARY KEY,
    empresa_id         UUID         NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    nome               TEXT         NOT NULL,
    descricao          TEXT,
    nivel_agregacao    TEXT         NOT NULL,
    parametros_schema  JSONB        NOT NULL DEFAULT '[]',
    ativo              BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at         TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ  NOT NULL DEFAULT now(),

    CONSTRAINT uq_farol_tipos_metrica_empresa_nome UNIQUE (empresa_id, nome),
    CONSTRAINT ck_farol_tipos_metrica_nivel_agregacao
        CHECK (nivel_agregacao IN ('ggv', 'crv', 'rca', 'rede', 'cliente'))
);

CREATE INDEX IF NOT EXISTS idx_farol_tipos_metrica_empresa ON farol.tipos_metrica (empresa_id);
