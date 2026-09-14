-- Migration 237: Fechamento Comercial Externo — Comparativo Fechamento
-- Comercial (Painel Vendas), pedido do Claudio 14/09/2026.
--
-- Guarda os números de FECHAMENTO que o fornecedor (ex: Carlos/JC) manda
-- fora do Farol (planilha própria dele), por Rede — pra comparar lado a
-- lado com o que o motor do Farol (farol_metas_calculo.go) calculou pro
-- mesmo período. Não sobrepõe nem mistura com farol.metas_realizados_snapshot
-- (que é a apuração OFICIAL do Farol) — é só o outro lado do comparativo,
-- editável por reimportação (PUT-replace por indústria+período, mesmo
-- princípio de farol.metas_itens_validos/metas_clientes_validos).
--
-- Chave por indústria + intervalo de datas (não por vinculo_id/vigencia_id):
-- o "Resumo Redes" que o fornecedor manda tem os números de Cobertura E
-- Sortimento NA MESMA LINHA por Rede (uma linha por indústria, repetida se
-- o programa tem 2+ indústrias no mesmo arquivo — ver farol_fechamento_comercial.go
-- pro racional completo de como a importação separa isso). Datas soltas
-- (não vigencia_id) porque o fechamento do fornecedor não segue
-- necessariamente o mesmo calendário de vigência que o Farol tem cadastrado.
CREATE TABLE farol.fechamento_comercial_externo (
    id                 BIGSERIAL PRIMARY KEY,
    empresa_id         UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    industria_id       INTEGER NOT NULL REFERENCES farol.industrias(id) ON DELETE CASCADE,
    data_inicio        DATE NOT NULL,
    data_fim           DATE NOT NULL,
    cod_princ          TEXT NOT NULL,
    razao              TEXT NOT NULL DEFAULT '',
    fantasia           TEXT NOT NULL DEFAULT '',
    qt_lojas           INTEGER NOT NULL DEFAULT 0,
    objetivo_cobertura NUMERIC NOT NULL DEFAULT 0,
    valor_venda        NUMERIC NOT NULL DEFAULT 0,
    objetivo_eans      NUMERIC NOT NULL DEFAULT 0,
    qt_eans_vendidos   NUMERIC NOT NULL DEFAULT 0,
    cod_ggv            TEXT NOT NULL DEFAULT '',
    nome_ggv           TEXT NOT NULL DEFAULT '',
    cod_crv            TEXT NOT NULL DEFAULT '',
    nome_crv           TEXT NOT NULL DEFAULT '',
    cod_rca            TEXT NOT NULL DEFAULT '',
    nome_rca           TEXT NOT NULL DEFAULT '',
    importado_em       TIMESTAMPTZ NOT NULL DEFAULT now(),
    importado_por      UUID,
    CONSTRAINT uq_farol_fechamento_comercial_externo UNIQUE (industria_id, data_inicio, data_fim, cod_princ)
);

CREATE INDEX idx_farol_fechamento_comercial_externo_periodo
    ON farol.fechamento_comercial_externo (empresa_id, industria_id, data_inicio, data_fim);
