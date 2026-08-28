-- Migration 209: Cadastro de Indústrias — mapeamento de fornecedores
--
-- Problema: o mesmo fabricante às vezes tem cod_fornec diferente por filial
-- (ex.: Reckitt = 47753 num grupo de filiais, 44957 noutro). Não existe hoje
-- onde registrar essa equivalência. Esta migration cria o cadastro de
-- "Indústrias", que mapeia N cod_fornec (TEXT solto, sem FK — não existe
-- tabela mestra de fornecedores) pra 1 indústria canônica.
--
-- Escopo: SÓ o cadastro (CRUD). Nenhum filtro cruzado, nenhuma hierarquia
-- V01-V07 e nenhuma tela/query existente passam a consumir esta tabela
-- ainda — isso é goal 2, deferido (ver deferred-work.md, entrada 2026-08-28).

CREATE TABLE IF NOT EXISTS farol.industrias (
    id           SERIAL       PRIMARY KEY,
    empresa_id   UUID         NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    nome         TEXT         NOT NULL,
    razao_social TEXT,
    ativo        BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),

    CONSTRAINT uq_farol_industrias_empresa_nome UNIQUE (empresa_id, nome)
);

CREATE INDEX IF NOT EXISTS idx_farol_industrias_empresa ON farol.industrias (empresa_id);

CREATE TABLE IF NOT EXISTS farol.industria_fornecedores (
    id           SERIAL       PRIMARY KEY,
    empresa_id   UUID         NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    industria_id INTEGER      NOT NULL REFERENCES farol.industrias(id) ON DELETE CASCADE,
    cod_fornec   TEXT         NOT NULL,
    rotulo       TEXT,        -- anotação livre do usuário (ex.: "MTZ/MS/BA"); só documentação, não usado em query

    CONSTRAINT uq_farol_industria_fornecedores_empresa_cod UNIQUE (empresa_id, cod_fornec)
);

CREATE INDEX IF NOT EXISTS idx_farol_industria_fornecedores_industria ON farol.industria_fornecedores (industria_id);
