-- Extrato de pagamento de Gamificação — pedido do Claudio 23/09/2026: cada
-- campanha precisa ser rastreável, com um extrato pra mandar a gestores/RH
-- (e servir de documentação jurídica). É um SNAPSHOT imutável — uma vez
-- gerado, nunca é recalculado nem sobrescrito, mesmo que a campanha continue
-- rodando e farol.gamif_pontuacao mude depois (essa tabela é o "ao vivo";
-- esta aqui é o "combinado e assinado").
CREATE TABLE farol.gamif_extratos (
    id SERIAL PRIMARY KEY,
    empresa_id UUID NOT NULL,
    campanha_id INT NOT NULL REFERENCES farol.gamif_campanhas(id) ON DELETE CASCADE,
    gerado_em TIMESTAMPTZ NOT NULL DEFAULT now(),
    gerado_por TEXT NOT NULL,
    linhas JSONB NOT NULL
);

CREATE INDEX idx_gamif_extratos_campanha ON farol.gamif_extratos (campanha_id, gerado_em DESC);

COMMENT ON TABLE farol.gamif_extratos IS 'Snapshot imutável do pagamento de uma campanha de Gamificação (1 linha por RCA: pontos, bônus, nível, percentual), gerado sob demanda pelo admin — nunca recalculado depois de criado.';
