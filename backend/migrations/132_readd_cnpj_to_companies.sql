-- Migration 132: re-adiciona coluna cnpj em companies (removida na 023).
-- Necessária para identificar empresas via CNPJ na chamada do Farol Mobile pelo ION.
-- IDEMPOTENTE: só adiciona se não existir.

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'companies' AND column_name = 'cnpj'
    ) THEN
        ALTER TABLE companies ADD COLUMN cnpj VARCHAR(14);
    END IF;
END $$;

-- Índice único parcial: permite múltiplos NULL mas garante CNPJ único quando informado
CREATE UNIQUE INDEX IF NOT EXISTS idx_companies_cnpj_unique
    ON companies (cnpj) WHERE cnpj IS NOT NULL;
