-- 122_add_empresa_id_to_cadastros.sql
-- Adiciona isolamento multi-tenant (empresa_id) às tabelas de cadastro.
-- Dados sem empresa_id não podem ser migrados; são removidos para permitir a alteração de schema.

TRUNCATE gestor_rca, rcas, gestores;

-- Remove constraints antigas (FKs e PKs simples)
ALTER TABLE gestor_rca DROP CONSTRAINT IF EXISTS gestor_rca_cod_supervisor_fkey;
ALTER TABLE gestor_rca DROP CONSTRAINT IF EXISTS gestor_rca_cod_rca_fkey;
ALTER TABLE gestor_rca DROP CONSTRAINT IF EXISTS gestor_rca_pkey;
ALTER TABLE gestores   DROP CONSTRAINT IF EXISTS gestores_pkey;
ALTER TABLE rcas       DROP CONSTRAINT IF EXISTS rcas_pkey;

-- Adiciona empresa_id em todas as tabelas
ALTER TABLE gestores   ADD COLUMN empresa_id UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE;
ALTER TABLE rcas       ADD COLUMN empresa_id UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE;
ALTER TABLE gestor_rca ADD COLUMN empresa_id UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE;

-- PKs compostas
ALTER TABLE gestores   ADD PRIMARY KEY (empresa_id, cod_supervisor);
ALTER TABLE rcas       ADD PRIMARY KEY (empresa_id, cod_rca);
ALTER TABLE gestor_rca ADD PRIMARY KEY (empresa_id, cod_supervisor, cod_rca);

-- FKs compostas em gestor_rca
ALTER TABLE gestor_rca ADD CONSTRAINT fk_gestor_rca_gestor
    FOREIGN KEY (empresa_id, cod_supervisor) REFERENCES gestores(empresa_id, cod_supervisor) ON DELETE RESTRICT;
ALTER TABLE gestor_rca ADD CONSTRAINT fk_gestor_rca_rca
    FOREIGN KEY (empresa_id, cod_rca) REFERENCES rcas(empresa_id, cod_rca) ON DELETE CASCADE;

-- Atualiza índices
DROP INDEX IF EXISTS idx_gestor_rca_rca;
DROP INDEX IF EXISTS idx_gestores_uf;
DROP INDEX IF EXISTS idx_rcas_tipo;
DROP INDEX IF EXISTS idx_rcas_ativo;

CREATE INDEX IF NOT EXISTS idx_gestores_empresa    ON gestores(empresa_id);
CREATE INDEX IF NOT EXISTS idx_gestores_emp_uf     ON gestores(empresa_id, uf);
CREATE INDEX IF NOT EXISTS idx_rcas_empresa        ON rcas(empresa_id);
CREATE INDEX IF NOT EXISTS idx_rcas_emp_tipo       ON rcas(empresa_id, tipo);
CREATE INDEX IF NOT EXISTS idx_rcas_emp_ativo      ON rcas(empresa_id, ativo);
CREATE INDEX IF NOT EXISTS idx_gestor_rca_emp_rca  ON gestor_rca(empresa_id, cod_rca);
