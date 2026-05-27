-- 138_drop_cadastros.sql
-- Remove tabelas legadas de gestores e RCAs.
-- A hierarquia de vendas agora é derivada diretamente do CSV via vendas_importadas.

DROP TABLE IF EXISTS gestor_rca   CASCADE;
DROP TABLE IF EXISTS rcas         CASCADE;
DROP TABLE IF EXISTS gestores     CASCADE;
