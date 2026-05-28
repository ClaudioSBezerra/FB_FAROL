-- 146_mv_farol_lookup_idx.sql
-- Índices de suporte ao lookupNome/lookupParent (painel ION VENDAS público).
-- Essas funções buscam nome_supervisor a partir de cod_supervisor, e o supervisor
-- pai de um cod_rca — sem filtrar por (tipo_base, ano, mes).
-- Sem esses índices, cada chamada fazia seq-scan em mv_farol_cli (170K+ linhas/período).

CREATE INDEX IF NOT EXISTS idx_mvcli_by_supervisor
    ON farol.mv_farol_cli (empresa_id, cod_supervisor)
    WHERE nome_supervisor != '';

CREATE INDEX IF NOT EXISTS idx_mvcli_by_rca
    ON farol.mv_farol_cli (empresa_id, cod_rca)
    WHERE nome_rca != '' AND cod_supervisor != '';

ANALYZE farol.mv_farol_cli;
