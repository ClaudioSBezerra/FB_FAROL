-- Meta/Realizado no extrato de Gamificação — pedido do Claudio 23/09/2026:
-- "seria interessante no extrato do RCA colocar a quantidade objetivo e o
-- que o RCA vendeu". Guarda o par (meta, realizado) da regra PRINCIPAL
-- (a que gerou nivel_principal/percentual_principal) — genérico o
-- suficiente pra qualquer tipo de regra: lojas cobertas/total de lojas,
-- redes cobertas/total de redes, ou qtd vendida/qtd mínima.
ALTER TABLE farol.gamif_pontuacao
    ADD COLUMN realizado_principal NUMERIC(14,2),
    ADD COLUMN meta_principal NUMERIC(14,2);

COMMENT ON COLUMN farol.gamif_pontuacao.realizado_principal IS 'o que o RCA de fato atingiu na regra principal — qtd vendida, lojas cobertas, ou redes cobertas, dependendo do tipo';
COMMENT ON COLUMN farol.gamif_pontuacao.meta_principal IS 'o objetivo/denominador da regra principal — qtd_minima, total de lojas, ou total de redes';
