-- Migration 171 — Índices para queryMixTotal (fix de performance)
--
-- A função queryMixTotal (introduzida com o feature "Mix Médio X de Y")
-- executa:
--   SELECT v.<groupCol>, COUNT(DISTINCT v.cod_prod)
--   FROM vendas_faturadas v
--   WHERE v.empresa_id=$1 AND v.<groupCol> <> '' AND v.cod_prod <> '' AND v.qt > 0
--     AND v.data_faturamento BETWEEN $A AND $B
--   GROUP BY v.<groupCol>
--
-- Sem índice composto adequado faz seq scan em 5.8M linhas → 20-40s/request.
-- Estes índices reduzem para ~200-500ms (mesmo padrão dos índices em agg_*_mes).
--
-- groupCol varia conforme view + drill:
--   V01 (Por Indústria): cod_fornec
--   V03 (Por Gerência) / V05 mobile (Por Fornec sob sup): cod_supervisor, cod_fornec
--   V02 (Por Equipe): cod_supervisor, cod_rca
--
-- Criamos 2 índices principais (cobrem ~90% dos casos):
--   (empresa_id, data_X, cod_fornec, cod_prod)     — drill em fornec
--   (empresa_id, data_X, cod_supervisor, cod_prod) — drill em sup (visão mobile)
--
-- WHERE qt > 0 AND cod_prod <> '' replica o filtro da query → índice menor.

CREATE INDEX IF NOT EXISTS idx_vf_mixtotal_fornec
  ON vendas_faturadas (empresa_id, data_faturamento, cod_fornec, cod_prod)
  WHERE qt > 0 AND cod_prod <> '' AND cod_fornec <> '';

CREATE INDEX IF NOT EXISTS idx_vf_mixtotal_supervisor
  ON vendas_faturadas (empresa_id, data_faturamento, cod_supervisor, cod_prod)
  WHERE qt > 0 AND cod_prod <> '' AND cod_supervisor <> '';

CREATE INDEX IF NOT EXISTS idx_vt_mixtotal_fornec
  ON vendas_transmitidas (empresa_id, data_transmissao, cod_fornec, cod_prod)
  WHERE qt > 0 AND cod_prod <> '' AND cod_fornec <> '';

CREATE INDEX IF NOT EXISTS idx_vt_mixtotal_supervisor
  ON vendas_transmitidas (empresa_id, data_transmissao, cod_supervisor, cod_prod)
  WHERE qt > 0 AND cod_prod <> '' AND cod_supervisor <> '';

-- ANALYZE para o planner saber das novas estatísticas
ANALYZE vendas_faturadas;
ANALYZE vendas_transmitidas;
