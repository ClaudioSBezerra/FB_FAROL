-- Migration 133: índices adicionais para acelerar as queries do Farol web.
-- Os painéis filtram por (empresa_id, periodo, cod_supervisor) e
-- (empresa_id, periodo, cod_rca) com frequência; o índice atual cobre só periodo.

CREATE INDEX IF NOT EXISTS idx_mv_rca_forn_sup
    ON vw_obj_rca_fornecedor (empresa_id, tipo_periodo, ano, periodo_seq, cod_supervisor);

CREATE INDEX IF NOT EXISTS idx_mv_rca_forn_rca
    ON vw_obj_rca_fornecedor (empresa_id, tipo_periodo, ano, periodo_seq, cod_rca);

CREATE INDEX IF NOT EXISTS idx_mv_supervisor_sup
    ON vw_obj_supervisor (empresa_id, tipo_periodo, ano, periodo_seq, cod_supervisor);
