-- 178_idx_dims_perf.sql
-- ════════════════════════════════════════════════════════════════════════════
-- PERF: índices para acelerar dims (cod_cli) e fetchCards
-- ════════════════════════════════════════════════════════════════════════════
--
-- Problema: dims cod_cli com subquery IN (SELECT ... FROM v01_l4_mes WHERE ...)
-- leva 1-12s porque scannea toda tabela agg_fat_v01_l4_mes para filtrar
-- clientes com movimento (pvenda <> 0 OR qt > 0).
--
-- Solução: índice parcial para a subquery e índice de covering para dims.

-- Índice parcial para subquery de clientes com movimento em v01_l4_mes
-- Usado por: dims cod_cli (linhas 1915-1918 em farol_v2_api.go)
CREATE INDEX CONCURRENTLY IF NOT EXISTS
  agg_fat_v01_l4_mes_emp_cli_mov
ON farol.agg_fat_v01_l4_mes (empresa_id, cod_cli)
WHERE (pvenda <> 0 OR qt > 0);

CREATE INDEX CONCURRENTLY IF NOT EXISTS
  agg_trans_v01_l4_mes_emp_cli_mov
ON farol.agg_trans_v01_l4_mes (empresa_id, cod_cli)
WHERE (pvenda <> 0 OR qt > 0);

-- Covering index para agg_fat_dims_mes (usado por todos os dims)
-- A query sempre faz: WHERE empresa_id=$1 AND dim=$2 AND key != '' GROUP BY key ORDER BY label
CREATE INDEX CONCURRENTLY IF NOT EXISTS
  agg_fat_dims_mes_emp_dim_key_label
ON farol.agg_fat_dims_mes (empresa_id, dim, key, label)
WHERE key != '';

CREATE INDEX CONCURRENTLY IF NOT EXISTS
  agg_trans_dims_mes_emp_dim_key_label
ON farol.agg_trans_dims_mes (empresa_id, dim, key, label)
WHERE key != '';

-- ANALYZE para atualizar estatísticas
ANALYZE farol.agg_fat_v01_l4_mes;
ANALYZE farol.agg_trans_v01_l4_mes;
ANALYZE farol.agg_fat_dims_mes;
ANALYZE farol.agg_trans_dims_mes;
