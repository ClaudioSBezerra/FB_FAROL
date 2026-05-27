-- 137_tipo_persona_constraint.sql
-- Define os valores válidos de tipo_persona e o mapeamento automático para sp_role.

-- Garante que a coluna existe (idempotente em relação à migration 136)
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name='users' AND column_name='tipo_persona'
    ) THEN
        ALTER TABLE users ADD COLUMN tipo_persona TEXT DEFAULT NULL;
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name='users' AND column_name='cod_referencia'
    ) THEN
        ALTER TABLE users ADD COLUMN cod_referencia TEXT DEFAULT NULL;
    END IF;
END$$;

-- Constraint de valores permitidos para tipo_persona
-- NULL = sem tipo definido (usuário antigo / admin de plataforma)
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.check_constraints
        WHERE constraint_name = 'check_tipo_persona_values'
    ) THEN
        ALTER TABLE users ADD CONSTRAINT check_tipo_persona_values
        CHECK (tipo_persona IS NULL OR tipo_persona IN (
            'diretor',
            'gerente_geral',
            'ggv',
            'supervisor',
            'rca',
            'ti',
            'analista_negocios',
            'admin'
        ));
    END IF;
END$$;

-- Índice para filtros por persona (busca rápida de todos os supervisores, etc.)
CREATE INDEX IF NOT EXISTS idx_users_tipo_persona ON users(tipo_persona) WHERE tipo_persona IS NOT NULL;
