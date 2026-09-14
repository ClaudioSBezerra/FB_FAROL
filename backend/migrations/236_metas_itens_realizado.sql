-- Migration 236: Itens Realizados por Cliente — agregado persistido de
-- vendeu/não vendeu, Qtd e Valor, por CNPJ×EAN, numa vigência de Sortimento.
--
-- Até aqui (farol_metas_painel_itens.go) o diálogo "Itens" do Painel de
-- Metas por Indústria calculava ao vivo a cada abertura (Rede ou Loja
-- escolhida) — e a busca de nome de produto pra itens nunca vendidos no
-- escopo pedido varria vendas_faturadas/vendas_transmitidas inteiras, sem
-- filtro de data (51s medidos em produção 14/09/2026 pra uma rede de 1
-- loja só). Além do gargalo, o Claudio pediu (14/09/2026) pra reaproveitar
-- esse "o que não vendeu" em painéis futuros — principal caso: a visão do
-- RCA no mobile (ION VENDAS).
--
-- Grão CNPJ×EAN (mais fino que Rede) serve os três consumidores possíveis
-- sem recalcular: uma Rede inteira (soma das lojas), uma loja só, ou o
-- escopo de um RCA (filtra pelos CNPJs dele) — ver RecalcularItensRealizado
-- em farol_metas_painel_itens.go.
CREATE TABLE farol.metas_itens_realizado (
    id           BIGSERIAL PRIMARY KEY,
    empresa_id   UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    vinculo_id   INTEGER NOT NULL REFERENCES farol.metas_vinculos(id) ON DELETE CASCADE,
    vigencia_id  INTEGER NOT NULL REFERENCES farol.metas_vigencias(id) ON DELETE CASCADE,
    fluxo        TEXT NOT NULL,
    cnpj         VARCHAR(14) NOT NULL,
    ean          TEXT NOT NULL,
    nome         TEXT NOT NULL DEFAULT '',
    qtd          NUMERIC NOT NULL DEFAULT 0,
    valor        NUMERIC NOT NULL DEFAULT 0,
    vendeu       BOOLEAN NOT NULL DEFAULT false,
    calculado_em TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_farol_metas_itens_realizado UNIQUE (vigencia_id, fluxo, cnpj, ean)
);

-- Leitura principal: "itens de um conjunto de CNPJs nesta vigência/fluxo"
-- (Rede = N cnpjs, Loja = 1 cnpj, RCA = cnpjs do escopo dele).
CREATE INDEX idx_farol_metas_itens_realizado_leitura
    ON farol.metas_itens_realizado (vigencia_id, fluxo, cnpj);

-- Leitura futura: "quem não vendeu este EAN" (painéis por item, não por cliente).
CREATE INDEX idx_farol_metas_itens_realizado_nao_vendeu
    ON farol.metas_itens_realizado (vigencia_id, fluxo, ean) WHERE NOT vendeu;
