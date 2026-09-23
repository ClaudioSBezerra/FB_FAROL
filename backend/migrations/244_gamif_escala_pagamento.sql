-- Escala de pagamento por nível (bronze/prata/ouro/diamante) — pedido do
-- Claudio 23/09/2026: pagamento gradual conforme % do objetivo atingido, em
-- vez de tudo-ou-nada. Guarda o nível/percentual PRINCIPAL (maior percentual
-- entre as regras da campanha) direto na linha, pra ordenar/filtrar sem
-- reabrir o JSONB de detalhe.
ALTER TABLE farol.gamif_pontuacao
    ADD COLUMN nivel_principal TEXT,
    ADD COLUMN percentual_principal NUMERIC(7,2) NOT NULL DEFAULT 0;

COMMENT ON COLUMN farol.gamif_pontuacao.nivel_principal IS 'bronze (>=60%) | prata (>=75%) | ouro (>=100%) | diamante (>=120%) | NULL se <60%';
COMMENT ON COLUMN farol.gamif_pontuacao.percentual_principal IS 'maior % de atingimento entre as regras da campanha pra este RCA — usado só pro selo/cor principal, não pro cálculo de pontos/bônus (que já vem tier-ajustado)';
