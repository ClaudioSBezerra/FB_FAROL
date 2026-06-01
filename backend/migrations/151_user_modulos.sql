-- Módulos por usuário: vendas | marketing | bi
ALTER TABLE users ADD COLUMN IF NOT EXISTS modulos TEXT[] DEFAULT ARRAY['vendas']::TEXT[];
UPDATE users SET modulos = ARRAY['vendas','marketing','bi']::TEXT[]
  WHERE sp_role = 'admin_fbtax' OR role = 'admin';
CREATE INDEX IF NOT EXISTS idx_users_modulos ON users USING GIN(modulos);
