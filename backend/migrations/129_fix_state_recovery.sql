-- Migration 129: recuperação de estado — garante cod_cli INTEGER e views materializadas.
-- IDEMPOTENTE: seguro re-executar em qualquer estado intermediário das migrations 126-128.
-- Cobre o caso em que o servidor ficou com qtd_clientes e sem cod_cli por falha anterior.

-- 1. Remove views em qualquer forma que possam existir
DROP MATERIALIZED VIEW IF EXISTS vw_obj_supervisor;
DROP MATERIALIZED VIEW IF EXISTS vw_obj_rca_fornecedor;
DROP VIEW IF EXISTS vw_obj_supervisor;
DROP VIEW IF EXISTS vw_obj_rca_fornecedor;
DROP VIEW IF EXISTS vw_obj_rca_produto;

-- 2. Garante cod_cli como INTEGER (trata ausência ou presença como TEXT)
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'objetivos_importados' AND column_name = 'cod_cli'
    ) THEN
        ALTER TABLE objetivos_importados
            ADD COLUMN cod_cli INTEGER NOT NULL DEFAULT 0;
    ELSIF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'objetivos_importados'
          AND column_name = 'cod_cli'
          AND data_type   = 'text'
    ) THEN
        ALTER TABLE objetivos_importados
            ALTER COLUMN cod_cli TYPE INTEGER
            USING CASE WHEN cod_cli = '' THEN 0 ELSE cod_cli::INTEGER END;
        ALTER TABLE objetivos_importados
            ALTER COLUMN cod_cli SET DEFAULT 0;
    END IF;
END $$;

-- 3. Remove coluna qtd_clientes se ainda existir (substituída por cod_cli)
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'objetivos_importados' AND column_name = 'qtd_clientes'
    ) THEN
        ALTER TABLE objetivos_importados DROP COLUMN qtd_clientes;
    END IF;
END $$;

-- 4. Recria constraint UNIQUE incluindo cod_cli
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

-- 5. Recria vw_obj_rca_produto como view regular (detalhe por linha)
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

-- 6. Recria vw_obj_rca_fornecedor como materialized view
CREATE MATERIALIZED VIEW vw_obj_rca_fornecedor AS
SELECT
    oi.empresa_id,
    oi.tipo_periodo,
    oi.ano,
    oi.periodo_seq,
    oi.cod_supervisor,
    COALESCE(g.nome, 'Supervisor ' || oi.cod_supervisor::text) AS nome_supervisor,
    oi.cod_rca,
    COALESCE(r.nome, 'RCA ' || oi.cod_rca::text)              AS nome_rca,
    oi.cod_fornec,
    MAX(oi.fornecedor)                    AS fornecedor,
    COUNT(DISTINCT oi.cod_prod)           AS qtd_produtos,
    COUNT(DISTINCT NULLIF(oi.cod_cli, 0)) AS qtd_clientes,
    SUM(oi.vl_anterior)                   AS vl_anterior,
    SUM(oi.vl_corrente)                   AS vl_corrente
FROM objetivos_importados oi
LEFT JOIN gestores g ON g.empresa_id = oi.empresa_id AND g.cod_supervisor = oi.cod_supervisor
LEFT JOIN rcas     r ON r.empresa_id = oi.empresa_id AND r.cod_rca        = oi.cod_rca
GROUP BY
    oi.empresa_id, oi.tipo_periodo, oi.ano, oi.periodo_seq,
    oi.cod_supervisor, g.nome,
    oi.cod_rca, r.nome,
    oi.cod_fornec
WITH DATA;

-- 7. Recria vw_obj_supervisor como materialized view
CREATE MATERIALIZED VIEW vw_obj_supervisor AS
SELECT
    oi.empresa_id,
    oi.tipo_periodo,
    oi.ano,
    oi.periodo_seq,
    oi.cod_supervisor,
    COALESCE(g.nome, 'Supervisor ' || oi.cod_supervisor::text) AS nome_supervisor,
    oi.cod_fornec,
    MAX(oi.fornecedor)                    AS fornecedor,
    COUNT(DISTINCT oi.cod_rca)            AS qtd_rcas,
    COUNT(DISTINCT oi.cod_prod)           AS qtd_produtos,
    COUNT(DISTINCT NULLIF(oi.cod_cli, 0)) AS qtd_clientes,
    SUM(oi.vl_anterior)                   AS vl_anterior,
    SUM(oi.vl_corrente)                   AS vl_corrente
FROM objetivos_importados oi
LEFT JOIN gestores g ON g.empresa_id = oi.empresa_id AND g.cod_supervisor = oi.cod_supervisor
GROUP BY
    oi.empresa_id, oi.tipo_periodo, oi.ano, oi.periodo_seq,
    oi.cod_supervisor, g.nome,
    oi.cod_fornec
WITH DATA;

-- 8. Índices
CREATE INDEX IF NOT EXISTS idx_mv_rca_forn_periodo
    ON vw_obj_rca_fornecedor (empresa_id, tipo_periodo, ano, periodo_seq);

CREATE INDEX IF NOT EXISTS idx_mv_supervisor_periodo
    ON vw_obj_supervisor (empresa_id, tipo_periodo, ano, periodo_seq);
