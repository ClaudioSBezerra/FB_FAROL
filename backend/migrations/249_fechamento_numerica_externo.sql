-- Migration 249: Fechamento Numérica Externo — Comparativo Fechamento
-- Numérica (Relatórios), Épico 7 addendum, 2026-09-30.
--
-- Mesmo princípio da migration 237 (fechamento_comercial_externo), só que
-- pro par Numérica (Cobertura Numérica + Sortimento Numérica/PPA): guarda o
-- "Resumo Numerica(s) Cliente" que o fornecedor manda fora do Farol, por
-- CNPJ/Cliente (não por Rede — a Numérica não tem esse conceito, FR24),
-- pra comparar lado a lado com farol.metas_realizados_snapshot.
--
-- Diferença de chave: CNPJ em vez de cod_princ (Numérica é por Cliente
-- direto), e COD CL preservado só como referência informativa (é o código
-- interno do cadastro da JC — usado achado real 2026-09-30: CNPJs cuja
-- venda está lançada com um cod_cliprinc diferente do COD CL cadastrado,
-- ver .memlog.md do PRD).
--
-- Ao contrário do fechamento Rede (1 arquivo cobre 2+ indústrias, duplicando
-- linha por indústria), cada arquivo da Numérica já vem escopado a UMA
-- indústria (HC ou Foods, arquivos separados do Carlos) — sem a lógica de
-- "casar objetivo com limiar" que o fechamento Rede precisa.
CREATE TABLE farol.fechamento_numerica_externo (
    id                 BIGSERIAL PRIMARY KEY,
    empresa_id         UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    industria_id       INTEGER NOT NULL REFERENCES farol.industrias(id) ON DELETE CASCADE,
    data_inicio        DATE NOT NULL,
    data_fim           DATE NOT NULL,
    cnpj               VARCHAR(14) NOT NULL,
    cod_cl             TEXT NOT NULL DEFAULT '',
    classificacao_pdv  TEXT NOT NULL DEFAULT '',
    razao              TEXT NOT NULL DEFAULT '',
    fantasia           TEXT NOT NULL DEFAULT '',
    valor_venda        NUMERIC NOT NULL DEFAULT 0,
    qt_ppas_vendidos   NUMERIC NOT NULL DEFAULT 0,
    cod_ggv            TEXT NOT NULL DEFAULT '',
    nome_ggv           TEXT NOT NULL DEFAULT '',
    cod_crv            TEXT NOT NULL DEFAULT '',
    nome_crv           TEXT NOT NULL DEFAULT '',
    cod_rca            TEXT NOT NULL DEFAULT '',
    nome_rca           TEXT NOT NULL DEFAULT '',
    importado_em       TIMESTAMPTZ NOT NULL DEFAULT now(),
    importado_por      UUID,
    CONSTRAINT uq_farol_fechamento_numerica_externo UNIQUE (industria_id, data_inicio, data_fim, cnpj)
);

CREATE INDEX idx_farol_fechamento_numerica_externo_periodo
    ON farol.fechamento_numerica_externo (empresa_id, industria_id, data_inicio, data_fim);
