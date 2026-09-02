-- Migration 219: Tipo de venda configurável por vínculo — Épico 2, Story 2.4
--
-- FR6: o tipo de venda válido pra apuração (ex: só Tipo 1 e 9 no caso
-- Unilever) deve ser configurável por vínculo, não fixo no sistema. `tipo_venda`
-- já existe como TEXT em vendas_faturadas/vendas_transmitidas (ver migration
-- 187/203) — aqui só guardamos QUAIS códigos são válidos pra este vínculo.
--
-- Array vazio = usa o filtro "Líquido" padrão do Farol (fallback, não uma
-- lista explícita de códigos) — é o motor de apuração (Épico 4) que decide
-- isso na hora de montar a query, não esta tabela.

ALTER TABLE farol.metas_vinculos
    ADD COLUMN IF NOT EXISTS tipos_venda_validos TEXT[] NOT NULL DEFAULT '{}';
