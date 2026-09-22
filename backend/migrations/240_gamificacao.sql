-- 240_gamificacao.sql
-- ════════════════════════════════════════════════════════════════════════════
-- MVP de Gamificação (pedido do José Costa, CEO da JC, via Claudio 22/09/2026):
-- "informação de última venda não vai fazer o RCA vender mais" — o pedido real
-- é criar competição/prêmio no estilo Mercado Livre/iFood: pontos, ranking e
-- bônus (inclusive em R$, ex. R$300 por vender microgarrafas de vodka pra
-- hotéis — um incentivo por PRODUTO específico, não só Cobertura/Sortimento
-- de Rede). Acesso restrito a admin_fbtax por enquanto (só o Claudio) — ver
-- FbtaxAdminRoute no frontend e withSP(..., "admin_fbtax") nas rotas novas.
--
-- 3 tabelas novas, schema farol (mesmo padrão do módulo de Objetivos por
-- Indústria):
--   gamif_campanhas  — a "competição" (nome, indústria, período).
--   gamif_regras     — como pontuar: reaproveita Cobertura/Sortimento já
--                      calculados (vinculo_id/vigencia_id existentes) OU um
--                      incentivo por produto específico (cod_prods + qtd
--                      mínima) — 3º tipo, novo, cobre o caso da vodka.
--   gamif_pontuacao  — 1 linha por RCA×campanha, resultado persistido
--                      (recalculado sob demanda, não ao vivo — mesmo
--                      cuidado de performance do resto do módulo).

CREATE TABLE farol.gamif_campanhas (
    id            BIGSERIAL PRIMARY KEY,
    empresa_id    UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    industria_id  INTEGER NOT NULL REFERENCES farol.industrias(id) ON DELETE CASCADE,
    nome          TEXT NOT NULL,
    data_inicio   DATE NOT NULL,
    data_fim      DATE NOT NULL,
    status        TEXT NOT NULL DEFAULT 'ativa' CHECK (status IN ('ativa', 'encerrada')),
    created_by    TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT ck_farol_gamif_campanhas_datas CHECK (data_fim >= data_inicio)
);

CREATE INDEX idx_farol_gamif_campanhas_empresa ON farol.gamif_campanhas (empresa_id);

-- tipo:
--   'cobertura_atingida'  — pontos/bônus por Rede que bateu o limiar de
--                           Cobertura do vinculo_id/vigencia_id apontado
--                           (reaproveita o Realizado já persistido —
--                           obterOuCongelarRealizado, sem calcular de novo).
--   'sortimento_atingido' — idem, pro limiar de Sortimento.
--   'produto_especifico'  — pontos/bônus por RCA que vendeu >= qtd_minima
--                           somada dos cod_prods listados, dentro do
--                           período da CAMPANHA (não de uma vigência —
--                           consulta direto em vendas_faturadas/
--                           vendas_transmitidas por cod_rca).
-- Pontos/bônus são POR OCORRÊNCIA (por Rede que atingiu, ou pelo RCA que
-- bateu a quantidade mínima do produto) — ver CalcularPontuacaoCampanha.
CREATE TABLE farol.gamif_regras (
    id           BIGSERIAL PRIMARY KEY,
    campanha_id  BIGINT NOT NULL REFERENCES farol.gamif_campanhas(id) ON DELETE CASCADE,
    tipo         TEXT NOT NULL CHECK (tipo IN ('cobertura_atingida', 'sortimento_atingido', 'produto_especifico')),
    descricao    TEXT NOT NULL DEFAULT '',
    vinculo_id   INTEGER REFERENCES farol.metas_vinculos(id) ON DELETE CASCADE,
    vigencia_id  INTEGER REFERENCES farol.metas_vigencias(id) ON DELETE CASCADE,
    cod_prods    TEXT[] NOT NULL DEFAULT '{}',
    qtd_minima   NUMERIC NOT NULL DEFAULT 0,
    fluxo        TEXT NOT NULL DEFAULT 'faturado' CHECK (fluxo IN ('faturado', 'transmitido')),
    pontos       NUMERIC NOT NULL DEFAULT 0,
    valor_bonus  NUMERIC NOT NULL DEFAULT 0,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_farol_gamif_regras_campanha ON farol.gamif_regras (campanha_id);

CREATE TABLE farol.gamif_pontuacao (
    id             BIGSERIAL PRIMARY KEY,
    empresa_id     UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    campanha_id    BIGINT NOT NULL REFERENCES farol.gamif_campanhas(id) ON DELETE CASCADE,
    cod_rca        TEXT NOT NULL,
    nome_rca       TEXT NOT NULL DEFAULT '',
    pontos_total   NUMERIC NOT NULL DEFAULT 0,
    bonus_total    NUMERIC NOT NULL DEFAULT 0,
    -- detalhe: [{regra_id, tipo, descricao, ocorrencias, pontos, bonus}, ...]
    -- — auditável (por que o RCA ganhou X pontos), consumido no drill-down.
    detalhe        JSONB NOT NULL DEFAULT '[]',
    calculado_em   TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_farol_gamif_pontuacao UNIQUE (campanha_id, cod_rca)
);

-- Leitura principal: ranking de uma campanha, ordenado por pontos.
CREATE INDEX idx_farol_gamif_pontuacao_ranking ON farol.gamif_pontuacao (campanha_id, pontos_total DESC);
