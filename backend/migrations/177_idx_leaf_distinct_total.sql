-- Migration 177 — Índices complementares p/ o totalizador de Clientes Ativos
--
-- O totalizador (queryDistinctCliPositivados) faz COUNT(DISTINCT cnpj) SEM
-- agrupamento sobre a folha (recorte do drill/filtros). A migration 176 indexou
-- (empresa_id, <nível-topo>, cnpj) — ótima p/ o GRID (GROUP BY) e p/ o drill
-- num fornecedor, mas no nível raiz (sem drill) o distinct sem grupo precisa
-- deduplicar cnpj entre todos os grupos (hash). Um índice (empresa_id, cnpj)
-- parcial deixa o cnpj já ordenado → distinct em streaming (index-only).
--
-- Roda em toda carga de tela (totalizador), então é alto impacto.
-- CREATE na partição-pai propaga p/ as partições; IF NOT EXISTS = idempotente.

CREATE INDEX IF NOT EXISTS idx_aggfat_v01l4_cnpjpos ON farol.agg_fat_v01_l4_mes   (empresa_id, cnpj) WHERE positivados > 0;
CREATE INDEX IF NOT EXISTS idx_aggtra_v01l4_cnpjpos ON farol.agg_trans_v01_l4_mes (empresa_id, cnpj) WHERE positivados > 0;
CREATE INDEX IF NOT EXISTS idx_aggfat_v02l3_cnpjpos ON farol.agg_fat_v02_l3_mes   (empresa_id, cnpj) WHERE positivados > 0;
CREATE INDEX IF NOT EXISTS idx_aggtra_v02l3_cnpjpos ON farol.agg_trans_v02_l3_mes (empresa_id, cnpj) WHERE positivados > 0;
CREATE INDEX IF NOT EXISTS idx_aggfat_v03l3_cnpjpos ON farol.agg_fat_v03_l3_mes   (empresa_id, cnpj) WHERE positivados > 0;
CREATE INDEX IF NOT EXISTS idx_aggtra_v03l3_cnpjpos ON farol.agg_trans_v03_l3_mes (empresa_id, cnpj) WHERE positivados > 0;
CREATE INDEX IF NOT EXISTS idx_aggfat_v05l3_cnpjpos ON farol.agg_fat_v05_l3_mes   (empresa_id, cnpj) WHERE positivados > 0;
CREATE INDEX IF NOT EXISTS idx_aggtra_v05l3_cnpjpos ON farol.agg_trans_v05_l3_mes (empresa_id, cnpj) WHERE positivados > 0;

-- Stats frescas p/ o planner escolher os índices novos (176/177).
ANALYZE farol.agg_fat_v01_l4_mes;
ANALYZE farol.agg_trans_v01_l4_mes;
ANALYZE farol.agg_fat_v02_l3_mes;
ANALYZE farol.agg_trans_v02_l3_mes;
ANALYZE farol.agg_fat_v03_l3_mes;
ANALYZE farol.agg_trans_v03_l3_mes;
ANALYZE farol.agg_fat_v05_l3_mes;
ANALYZE farol.agg_trans_v05_l3_mes;
