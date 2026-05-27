-- 142_mv_farol_idx_covering.sql
-- Índice de cobertura para as queries da API do Farol.
--
-- O queryAggregated faz 3 scans em mv_farol_resumo (pos CTE, mix_inner CTE
-- e SELECT principal). Este índice permite index-only scan em todos eles ao
-- agrupar em qualquer nível hierárquico (fornec, gerente, supervisor, rca).
--
-- Colunas INCLUDE (pvenda, estado, cod_cli, qt, cod_prod, qtcli_rca) evitam
-- heap fetch para os campos agregados, reduzindo I/O de 3 full-scan → 3 idx-scan.

CREATE INDEX IF NOT EXISTS idx_mvfr_api_covering ON farol.mv_farol_resumo
  (empresa_id, tipo_base, ano, mes, cod_fornec, cod_gerente, cod_supervisor, cod_rca)
  INCLUDE (pvenda, estado, cod_cli, qt, cod_prod, qtcli_rca);

ANALYZE farol.mv_farol_resumo;
