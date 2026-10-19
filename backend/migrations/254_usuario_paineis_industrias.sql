-- 254_usuario_paineis_industrias.sql — quais painéis e indústrias cada usuário vê
-- (pedido do Claudio, 19/10/2026: "um parâmetro no usuário").
--
-- PAINÉIS = valores em users.modulos (coluna da migration 151, já editável na
-- tela de Usuários): vendas (Painel Vendas), unilever (Painel Unilever),
-- reckitt (Painel Reckitt), relatorios (Relatórios), bi. Os usuários que já
-- existiam ganham 'unilever' e 'relatorios' pra continuar vendo o que viam.
-- O Painel Reckitt NÃO entra pra ninguém: só admin (ou quem for marcado).
--
-- INDÚSTRIAS = users.industrias_permitidas (ids de farol.industrias).
-- NULL = todas as indústrias dos painéis que o usuário vê.

ALTER TABLE users ADD COLUMN IF NOT EXISTS industrias_permitidas INTEGER[] NULL;

UPDATE users SET modulos = ARRAY['vendas']::TEXT[] WHERE modulos IS NULL;
UPDATE users SET modulos = array_append(modulos, 'unilever')   WHERE NOT ('unilever'   = ANY(modulos));
UPDATE users SET modulos = array_append(modulos, 'relatorios') WHERE NOT ('relatorios' = ANY(modulos));

ALTER TABLE users ALTER COLUMN modulos SET DEFAULT ARRAY['vendas','unilever','relatorios']::TEXT[];
