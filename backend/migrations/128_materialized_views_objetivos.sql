-- Migration 128: converte vw_obj_rca_fornecedor e vw_obj_supervisor em
-- materialized views para que o dado fique pré-computado em disco.
-- O handler de import chama REFRESH MATERIALIZED VIEW após cada importação.
-- IDEMPOTENTE: remove materialized view e view regular antes de recriar.

-- 1. Remove views em qualquer forma (regular 'v' ou materializada 'm')
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

-- 2. Cria materialized views com os mesmos nomes e queries

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

-- 3. Índices de consulta (não unique — refresh não-concurrent não exige)
CREATE INDEX IF NOT EXISTS idx_mv_rca_forn_periodo
    ON vw_obj_rca_fornecedor (empresa_id, tipo_periodo, ano, periodo_seq);

CREATE INDEX IF NOT EXISTS idx_mv_supervisor_periodo
    ON vw_obj_supervisor (empresa_id, tipo_periodo, ano, periodo_seq);
