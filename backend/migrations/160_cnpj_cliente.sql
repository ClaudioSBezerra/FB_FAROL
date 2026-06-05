-- 160_cnpj_cliente.sql
-- Adiciona coluna CNPJ do cliente em vendas_faturadas e vendas_transmitidas.
-- O importador agora mapeia a coluna CNPJ do CSV (que já existe no WinThor)
-- e insere aqui.
--
-- Motivação: positivação por cod_cli (código local do WinThor) era impreciso.
-- CNPJ identifica unicamente a filial/cliente e permite consolidar entre
-- vendas faturadas e transmitidas com segurança.

ALTER TABLE vendas_faturadas    ADD COLUMN IF NOT EXISTS cnpj TEXT NOT NULL DEFAULT '';
ALTER TABLE vendas_transmitidas ADD COLUMN IF NOT EXISTS cnpj TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_vf_emp_cnpj ON vendas_faturadas    (empresa_id, cnpj);
CREATE INDEX IF NOT EXISTS idx_vt_emp_cnpj ON vendas_transmitidas (empresa_id, cnpj);
