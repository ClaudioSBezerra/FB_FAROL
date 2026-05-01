-- Migration 127: cod_cli TEXT → INTEGER (CODCLI é sempre numérico).
-- Atualiza a constraint única e as views para NULLIF(cod_cli, 0).

-- 1. Remove constraint existente antes de mudar o tipo
ALTER TABLE objetivos_importados DROP CONSTRAINT IF EXISTS objetivos_importados_unique;

-- 2. Converte coluna ('' → 0 para registros antigos sem valor)
ALTER TABLE objetivos_importados
    ALTER COLUMN cod_cli TYPE INTEGER
    USING CASE WHEN cod_cli = '' THEN 0 ELSE cod_cli::INTEGER END;

ALTER TABLE objetivos_importados
    ALTER COLUMN cod_cli SET DEFAULT 0;

-- 3. Recria constraint com o novo tipo
ALTER TABLE objetivos_importados
    ADD CONSTRAINT objetivos_importados_unique
    UNIQUE (empresa_id, tipo_periodo, ano, periodo_seq,
            cod_supervisor, cod_rca, cod_depto, cod_sec, cod_fornec, cod_prod, cod_cli);

-- 4. Atualiza views: NULLIF(cod_cli, 0) para excluir zeros (imports antigos / sem cliente)

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
    MAX(oi.fornecedor)                      AS fornecedor,
    COUNT(DISTINCT oi.cod_prod)             AS qtd_produtos,
    COUNT(DISTINCT NULLIF(oi.cod_cli, 0))   AS qtd_clientes,
    SUM(oi.vl_anterior)                     AS vl_anterior,
    SUM(oi.vl_corrente)                     AS vl_corrente
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
    MAX(oi.fornecedor)                      AS fornecedor,
    COUNT(DISTINCT oi.cod_rca)              AS qtd_rcas,
    COUNT(DISTINCT oi.cod_prod)             AS qtd_produtos,
    COUNT(DISTINCT NULLIF(oi.cod_cli, 0))   AS qtd_clientes,
    SUM(oi.vl_anterior)                     AS vl_anterior,
    SUM(oi.vl_corrente)                     AS vl_corrente
FROM objetivos_importados oi
LEFT JOIN gestores g ON g.empresa_id = oi.empresa_id AND g.cod_supervisor = oi.cod_supervisor
GROUP BY
    oi.empresa_id, oi.tipo_periodo, oi.ano, oi.periodo_seq,
    oi.cod_supervisor, g.nome,
    oi.cod_fornec;
