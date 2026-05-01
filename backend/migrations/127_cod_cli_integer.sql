-- Migration 127: cod_cli TEXT → INTEGER (CODCLI é sempre numérico).
-- IDEMPOTENTE: só converte o tipo se a coluna ainda for TEXT.

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'objetivos_importados'
          AND column_name = 'cod_cli'
          AND data_type   = 'text'
    ) THEN
        -- Remove o DEFAULT '' antes de converter (PostgreSQL não faz cast automático de '' para INTEGER)
        ALTER TABLE objetivos_importados ALTER COLUMN cod_cli DROP DEFAULT;
        ALTER TABLE objetivos_importados
            ALTER COLUMN cod_cli TYPE INTEGER
            USING CASE WHEN cod_cli = '' THEN 0 ELSE cod_cli::INTEGER END;
        ALTER TABLE objetivos_importados
            ALTER COLUMN cod_cli SET DEFAULT 0;
    END IF;
END $$;

-- Recria constraint com o tipo definitivo (INTEGER).
-- Remove qualquer constraint antiga que inclua cod_cli como texto,
-- e garante que a versão final (com INTEGER) esteja registrada.
DO $$
DECLARE v_name TEXT;
BEGIN
    FOR v_name IN
        SELECT c.conname
        FROM pg_constraint c
        JOIN pg_class t ON t.oid = c.conrelid
        WHERE t.relname = 'objetivos_importados' AND c.contype = 'u'
    LOOP
        EXECUTE format('ALTER TABLE objetivos_importados DROP CONSTRAINT %I', v_name);
    END LOOP;
END $$;

ALTER TABLE objetivos_importados
    ADD CONSTRAINT objetivos_importados_unique
    UNIQUE (empresa_id, tipo_periodo, ano, periodo_seq,
            cod_supervisor, cod_rca, cod_depto, cod_sec, cod_fornec, cod_prod, cod_cli);

-- Atualiza as views para usar NULLIF(cod_cli, 0) (agora INTEGER).
-- As views regulares criadas pela 126 usavam NULLIF(cod_cli, ''); recriamos com 0.
DROP VIEW IF EXISTS vw_obj_rca_produto;
CREATE VIEW vw_obj_rca_produto AS
SELECT
    oi.empresa_id,
    oi.tipo_periodo,
    oi.ano,
    oi.periodo_seq,
    oi.cod_supervisor,
    COALESCE(g.nome, 'Supervisor ' || oi.cod_supervisor::text) AS nome_supervisor,
    oi.cod_rca,
    COALESCE(r.nome, 'RCA ' || oi.cod_rca::text)              AS nome_rca,
    oi.cod_depto,
    oi.departamento,
    oi.cod_sec,
    oi.secao,
    oi.cod_fornec,
    oi.fornecedor,
    oi.cod_prod,
    oi.cod_cli,
    oi.vl_anterior,
    oi.vl_corrente
FROM objetivos_importados oi
LEFT JOIN gestores g ON g.empresa_id = oi.empresa_id AND g.cod_supervisor = oi.cod_supervisor
LEFT JOIN rcas     r ON r.empresa_id = oi.empresa_id AND r.cod_rca        = oi.cod_rca;

-- Drop vw_obj_rca_fornecedor and vw_obj_supervisor regardless of type (view or materialized view).
-- They will be recreated as materialized views by migrations 128-130.
DO $$
DECLARE v_kind char;
BEGIN
    SELECT c.relkind INTO v_kind FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
    WHERE c.relname = 'vw_obj_rca_fornecedor' AND n.nspname = current_schema();
    IF    v_kind = 'v' THEN DROP VIEW              vw_obj_rca_fornecedor;
    ELSIF v_kind = 'm' THEN DROP MATERIALIZED VIEW  vw_obj_rca_fornecedor;
    END IF;

    SELECT c.relkind INTO v_kind FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
    WHERE c.relname = 'vw_obj_supervisor' AND n.nspname = current_schema();
    IF    v_kind = 'v' THEN DROP VIEW              vw_obj_supervisor;
    ELSIF v_kind = 'm' THEN DROP MATERIALIZED VIEW  vw_obj_supervisor;
    END IF;
END $$;
