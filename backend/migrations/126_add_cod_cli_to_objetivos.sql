-- Migration 126: substitui qtd_clientes pelo código individual do cliente (cod_cli).
-- CODCLI no CSV (coluna [9]) é o código do cliente, não uma contagem pré-agregada.
-- A quantidade de clientes passa a ser calculada nas views via COUNT(DISTINCT cod_cli).

-- 1. Adiciona coluna cod_cli (texto; '' para registros anteriores)
ALTER TABLE objetivos_importados
    ADD COLUMN IF NOT EXISTS cod_cli TEXT NOT NULL DEFAULT '';

-- 2. Remove a constraint UNIQUE antiga (não inclui cod_cli) e recria incluindo cod_cli.
--    O nome é auto-gerado pelo PostgreSQL; buscamos dinamicamente via pg_constraint.
DO $$
DECLARE v_name TEXT;
BEGIN
    SELECT c.conname INTO v_name
    FROM pg_constraint c
    JOIN pg_class t ON t.oid = c.conrelid
    WHERE t.relname = 'objetivos_importados'
      AND c.contype = 'u'
    ORDER BY c.oid
    LIMIT 1;
    IF v_name IS NOT NULL THEN
        EXECUTE format('ALTER TABLE objetivos_importados DROP CONSTRAINT %I', v_name);
    END IF;
END $$;

ALTER TABLE objetivos_importados
    ADD CONSTRAINT objetivos_importados_unique
    UNIQUE (empresa_id, tipo_periodo, ano, periodo_seq,
            cod_supervisor, cod_rca, cod_depto, cod_sec, cod_fornec, cod_prod, cod_cli);

-- 3. Atualiza as 3 views para calcular qtd_clientes via COUNT(DISTINCT cod_cli).
--    NULLIF('', '') = NULL — exclui linhas de imports antigos sem cod_cli real.

CREATE OR REPLACE VIEW vw_obj_rca_produto AS
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

CREATE OR REPLACE VIEW vw_obj_rca_fornecedor AS
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
    MAX(oi.fornecedor)                                          AS fornecedor,
    COUNT(DISTINCT oi.cod_prod)                                 AS qtd_produtos,
    COUNT(DISTINCT NULLIF(oi.cod_cli, ''))                      AS qtd_clientes,
    SUM(oi.vl_anterior)                                         AS vl_anterior,
    SUM(oi.vl_corrente)                                         AS vl_corrente
FROM objetivos_importados oi
LEFT JOIN gestores g ON g.empresa_id = oi.empresa_id AND g.cod_supervisor = oi.cod_supervisor
LEFT JOIN rcas     r ON r.empresa_id = oi.empresa_id AND r.cod_rca        = oi.cod_rca
GROUP BY
    oi.empresa_id, oi.tipo_periodo, oi.ano, oi.periodo_seq,
    oi.cod_supervisor, g.nome,
    oi.cod_rca, r.nome,
    oi.cod_fornec;

CREATE OR REPLACE VIEW vw_obj_supervisor AS
SELECT
    oi.empresa_id,
    oi.tipo_periodo,
    oi.ano,
    oi.periodo_seq,
    oi.cod_supervisor,
    COALESCE(g.nome, 'Supervisor ' || oi.cod_supervisor::text) AS nome_supervisor,
    oi.cod_fornec,
    MAX(oi.fornecedor)                                          AS fornecedor,
    COUNT(DISTINCT oi.cod_rca)                                  AS qtd_rcas,
    COUNT(DISTINCT oi.cod_prod)                                 AS qtd_produtos,
    COUNT(DISTINCT NULLIF(oi.cod_cli, ''))                      AS qtd_clientes,
    SUM(oi.vl_anterior)                                         AS vl_anterior,
    SUM(oi.vl_corrente)                                         AS vl_corrente
FROM objetivos_importados oi
LEFT JOIN gestores g ON g.empresa_id = oi.empresa_id AND g.cod_supervisor = oi.cod_supervisor
GROUP BY
    oi.empresa_id, oi.tipo_periodo, oi.ano, oi.periodo_seq,
    oi.cod_supervisor, g.nome,
    oi.cod_fornec;
