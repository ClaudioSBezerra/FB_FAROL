-- Adiciona suporte a logotipo por empresa (armazenado como BYTEA no banco)
ALTER TABLE companies ADD COLUMN IF NOT EXISTS logo_data  BYTEA;
ALTER TABLE companies ADD COLUMN IF NOT EXISTS logo_mime  VARCHAR(50);
ALTER TABLE companies ADD COLUMN IF NOT EXISTS logo_nome  VARCHAR(255);
