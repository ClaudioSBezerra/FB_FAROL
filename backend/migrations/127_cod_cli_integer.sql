-- Migration 127: cod_cli TEXT → INTEGER (CODCLI é sempre numérico).
-- IDEMPOTENTE: só converte o tipo se a coluna ainda for TEXT.
--
-- GUARDA: as tabelas objetivos_importados/gestores/rcas foram removidas
-- pela migration 138. Em ambientes onde 138 já rodou mas 127 ficou
-- pendente, esta migration precisa virar no-op. Envolvemos tudo num
-- DO block que sai cedo se a tabela base não existir mais.

DO $mig127$
DECLARE
    v_name TEXT;
    v_kind CHAR;
BEGIN
    IF to_regclass('public.objetivos_importados') IS NULL
       OR to_regclass('public.gestores') IS NULL
       OR to_regclass('public.rcas') IS NULL THEN
        RAISE NOTICE 'Migration 127: tabelas legacy removidas pela mig 138 — skip';
        RETURN;
    END IF;

    -- Converte cod_cli TEXT → INTEGER (só se ainda for TEXT)
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'objetivos_importados'
          AND column_name = 'cod_cli'
          AND data_type   = 'text'
    ) THEN
        ALTER TABLE objetivos_importados ALTER COLUMN cod_cli DROP DEFAULT;
        ALTER TABLE objetivos_importados
            ALTER COLUMN cod_cli TYPE INTEGER
            USING CASE WHEN cod_cli = '' THEN 0 ELSE cod_cli::INTEGER END;
        ALTER TABLE objetivos_importados
            ALTER COLUMN cod_cli SET DEFAULT 0;
    END IF;

    -- Remove qualquer constraint UNIQUE antiga em objetivos_importados
    FOR v_name IN
        SELECT c.conname
        FROM pg_constraint c
        JOIN pg_class t ON t.oid = c.conrelid
        WHERE t.relname = 'objetivos_importados' AND c.contype = 'u'
    LOOP
        EXECUTE format('ALTER TABLE objetivos_importados DROP CONSTRAINT %I', v_name);
    END LOOP;

    EXECUTE $ddl$
        ALTER TABLE objetivos_importados
            ADD CONSTRAINT objetivos_importados_unique
            UNIQUE (empresa_id, tipo_periodo, ano, periodo_seq,
                    cod_supervisor, cod_rca, cod_depto, cod_sec, cod_fornec, cod_prod, cod_cli)
    $ddl$;

    -- Recria vw_obj_rca_produto com cod_cli INTEGER
    EXECUTE 'DROP VIEW IF EXISTS vw_obj_rca_produto';
    EXECUTE $ddl$
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
        LEFT JOIN rcas     r ON r.empresa_id = oi.empresa_id AND r.cod_rca        = oi.cod_rca
    $ddl$;

    -- Drop vw_obj_rca_fornecedor e vw_obj_supervisor em qualquer forma
    -- (view regular ou materializada) — serão recriadas como MV pela mig 128.
    SELECT c.relkind INTO v_kind FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
    WHERE c.relname = 'vw_obj_rca_fornecedor' AND n.nspname = current_schema();
    IF    v_kind = 'v' THEN EXECUTE 'DROP VIEW              vw_obj_rca_fornecedor';
    ELSIF v_kind = 'm' THEN EXECUTE 'DROP MATERIALIZED VIEW vw_obj_rca_fornecedor';
    END IF;

    SELECT c.relkind INTO v_kind FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
    WHERE c.relname = 'vw_obj_supervisor' AND n.nspname = current_schema();
    IF    v_kind = 'v' THEN EXECUTE 'DROP VIEW              vw_obj_supervisor';
    ELSIF v_kind = 'm' THEN EXECUTE 'DROP MATERIALIZED VIEW vw_obj_supervisor';
    END IF;
END $mig127$;
