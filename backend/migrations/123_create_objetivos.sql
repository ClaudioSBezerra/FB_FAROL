-- 123_create_objetivos.sql
-- Tabela base de objetivos de vendas importados via CSV + 3 views de agregação.

CREATE TABLE IF NOT EXISTS objetivos_importados (
    id            BIGSERIAL PRIMARY KEY,
    empresa_id    UUID        NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    tipo_periodo  TEXT        NOT NULL CHECK (tipo_periodo IN ('MENSAL','TRIMESTRAL','SEMESTRAL','ANUAL')),
    ano           INTEGER     NOT NULL,
    periodo_seq   INTEGER     NOT NULL, -- 1-12 mensal | 1-4 trimestral | 1-2 semestral | 1 anual
    cod_supervisor INTEGER,
    cod_rca       INTEGER     NOT NULL,
    cod_depto     TEXT,
    departamento  TEXT,
    cod_sec       TEXT,
    secao         TEXT,
    cod_fornec    TEXT        NOT NULL,
    fornecedor    TEXT,
    cod_prod      TEXT        NOT NULL,
    qtd_clientes  INTEGER     NOT NULL DEFAULT 0,
    vl_anterior   NUMERIC(15,2) NOT NULL DEFAULT 0,
    vl_corrente   NUMERIC(15,2) NOT NULL DEFAULT 0,
    importado_em  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE (empresa_id, tipo_periodo, ano, periodo_seq,
            cod_supervisor, cod_rca, cod_depto, cod_sec, cod_fornec, cod_prod)
);

CREATE INDEX IF NOT EXISTS idx_obj_empresa         ON objetivos_importados(empresa_id);
CREATE INDEX IF NOT EXISTS idx_obj_emp_periodo     ON objetivos_importados(empresa_id, tipo_periodo, ano, periodo_seq);
CREATE INDEX IF NOT EXISTS idx_obj_emp_rca         ON objetivos_importados(empresa_id, cod_rca);
CREATE INDEX IF NOT EXISTS idx_obj_emp_supervisor  ON objetivos_importados(empresa_id, cod_supervisor);
CREATE INDEX IF NOT EXISTS idx_obj_emp_fornec      ON objetivos_importados(empresa_id, cod_fornec);

-- VIEW 1: Detalhe por RCA × Seção × Departamento × Fornecedor × Produto
-- Apenas enriquece com nome_supervisor e nome_rca via join; não agrega.
CREATE OR REPLACE VIEW vw_obj_rca_produto AS
SELECT
    oi.empresa_id,
    oi.tipo_periodo,
    oi.ano,
    oi.periodo_seq,
    oi.cod_supervisor,
    g.nome          AS nome_supervisor,
    oi.cod_rca,
    r.nome          AS nome_rca,
    oi.cod_depto,
    oi.departamento,
    oi.cod_sec,
    oi.secao,
    oi.cod_fornec,
    oi.fornecedor,
    oi.cod_prod,
    oi.qtd_clientes,
    oi.vl_anterior,
    oi.vl_corrente
FROM objetivos_importados oi
LEFT JOIN gestores g ON g.empresa_id = oi.empresa_id AND g.cod_supervisor = oi.cod_supervisor
LEFT JOIN rcas     r ON r.empresa_id = oi.empresa_id AND r.cod_rca        = oi.cod_rca;

-- VIEW 2: Acumulado por RCA × Fornecedor (soma todos os produtos)
CREATE OR REPLACE VIEW vw_obj_rca_fornecedor AS
SELECT
    oi.empresa_id,
    oi.tipo_periodo,
    oi.ano,
    oi.periodo_seq,
    oi.cod_supervisor,
    g.nome                       AS nome_supervisor,
    oi.cod_rca,
    r.nome                       AS nome_rca,
    oi.cod_fornec,
    MAX(oi.fornecedor)            AS fornecedor,
    COUNT(DISTINCT oi.cod_prod)  AS qtd_produtos,
    SUM(oi.qtd_clientes)         AS qtd_clientes,
    SUM(oi.vl_anterior)          AS vl_anterior,
    SUM(oi.vl_corrente)          AS vl_corrente
FROM objetivos_importados oi
LEFT JOIN gestores g ON g.empresa_id = oi.empresa_id AND g.cod_supervisor = oi.cod_supervisor
LEFT JOIN rcas     r ON r.empresa_id = oi.empresa_id AND r.cod_rca        = oi.cod_rca
GROUP BY
    oi.empresa_id, oi.tipo_periodo, oi.ano, oi.periodo_seq,
    oi.cod_supervisor, g.nome,
    oi.cod_rca, r.nome,
    oi.cod_fornec;

-- VIEW 3: Acumulado do Supervisor × Fornecedor (soma toda a equipe de RCAs)
CREATE OR REPLACE VIEW vw_obj_supervisor AS
SELECT
    oi.empresa_id,
    oi.tipo_periodo,
    oi.ano,
    oi.periodo_seq,
    oi.cod_supervisor,
    g.nome                       AS nome_supervisor,
    oi.cod_fornec,
    MAX(oi.fornecedor)            AS fornecedor,
    COUNT(DISTINCT oi.cod_rca)   AS qtd_rcas,
    COUNT(DISTINCT oi.cod_prod)  AS qtd_produtos,
    SUM(oi.qtd_clientes)         AS qtd_clientes,
    SUM(oi.vl_anterior)          AS vl_anterior,
    SUM(oi.vl_corrente)          AS vl_corrente
FROM objetivos_importados oi
LEFT JOIN gestores g ON g.empresa_id = oi.empresa_id AND g.cod_supervisor = oi.cod_supervisor
GROUP BY
    oi.empresa_id, oi.tipo_periodo, oi.ano, oi.periodo_seq,
    oi.cod_supervisor, g.nome,
    oi.cod_fornec;
