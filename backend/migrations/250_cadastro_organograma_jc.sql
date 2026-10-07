-- 250_cadastro_organograma_jc.sql — cadastro autoritativo de Gerente/GGV,
-- Supervisor/CRV e RCA, sincronizado diariamente a partir do CADRCA_JC
-- (Oracle da JC, liberado pelo Keslley/TI 21/09/2026).
--
-- Achado real 2026-10-07 (Claudio): quando um código de Supervisor/GGV/RCA
-- troca de dono (mesma pessoa saiu, código foi reaproveitado), o Farol
-- continuava mostrando o nome ANTIGO em alguns lugares — porque até aqui o
-- nome só existia como efeito colateral do CSV de venda do dia (se o código
-- não tem venda recente, o nome nunca é atualizado) ou de um cadastro manual
-- (gestores/rcas, upload de CSV avulso). Essa tabela passa a ser a fonte da
-- verdade: um sync diário lê o CADRCA_JC (que a JC mantém em tempo real) e
-- sobrescreve o nome atual aqui, independente de ter venda ou upload manual.
--
-- Não troca os dados históricos (farol.agg_fat_dims_mes/agg_trans_dims_mes
-- continuam guardando o nome que estava em vigor em cada mês — ver
-- lookupNome/fetchDim em farol_v2_api.go) — só passa a ser a PRIMEIRA fonte
-- consultada para "qual é o nome ATUAL deste código", com fallback pro
-- comportamento antigo se o código não estiver (mais) no CADRCA_JC.
CREATE TABLE IF NOT EXISTS farol.cadastro_organograma_jc (
    empresa_id      UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    nivel           TEXT NOT NULL CHECK (nivel IN ('gerente', 'supervisor', 'rca')),
    codigo          TEXT NOT NULL,
    nome            TEXT NOT NULL,
    atualizado_em   TIMESTAMPTZ NOT NULL, -- DT_REG do CADRCA_JC — quando a JC alterou esse registro
    sincronizado_em TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (empresa_id, nivel, codigo)
);

CREATE INDEX IF NOT EXISTS idx_cadastro_organograma_jc_nivel
    ON farol.cadastro_organograma_jc(empresa_id, nivel);
