-- 180_idx_vendas_crossfilter.sql
-- ════════════════════════════════════════════════════════════════════════════
-- PERF: índices para queryAggregatedVendas (caminho do FILTRO CRUZADO)
-- ════════════════════════════════════════════════════════════════════════════
--
-- Problema: quando pickAggForCrossFilter falha (ex: filtro por UF/Filial, ou
-- combinação de colunas que nenhuma agg atende), o fetchCards cai para
-- queryAggregatedVendas que scannea vendas_faturadas/transmitidas (~5.8M
-- linhas) SEM índice adequado → 37-63s.
--
-- As queries fazem:
--   SELECT v.<groupCol>, SUM(v.pvenda), COUNT(DISTINCT v.cnpj), ...
--   FROM vendas_<fluxo> v
--   WHERE v.empresa_id=$1 AND v.<groupCol> <> ''
--     AND v.data_<X> BETWEEN $a AND $b [AND <drill/filtros>]
--   GROUP BY v.<groupCol>
--
-- Índices existentes (156/171) NÃO cobrem range de data + groupCol sem cod_prod
-- no caminho → planner cai em seq scan + hash aggregate sobre milhões de linhas.
--
-- Solução: índices compostos (empresa_id, data_X, groupCol) com filtro de
-- groupCol<>'' — cobre o padrão da query. Mesmo precisando ler o heap para
-- pvenda/plucro/cnpj/cod_prod, o planner fará bitmap index scan bem menor
-- que seq scan, e o GROUP BY será mais eficiente com linhas pré-filtradas.
--
-- Foco nos 4 groupCols possíveis × 2 fluxos = 8 índices.
-- NOTA: sem CONCURRENTLY (migrations rodam em transação).

-- ── vendas_faturadas (data_faturamento) ──────────────────────────────────────
CREATE INDEX IF NOT EXISTS idx_vf_data_fornec
  ON vendas_faturadas (empresa_id, data_faturamento, cod_fornec)
  WHERE cod_fornec <> '';

CREATE INDEX IF NOT EXISTS idx_vf_data_rca
  ON vendas_faturadas (empresa_id, data_faturamento, cod_rca)
  WHERE cod_rca <> '';

CREATE INDEX IF NOT EXISTS idx_vf_data_sup
  ON vendas_faturadas (empresa_id, data_faturamento, cod_supervisor)
  WHERE cod_supervisor <> '';

CREATE INDEX IF NOT EXISTS idx_vf_data_ger
  ON vendas_faturadas (empresa_id, data_faturamento, cod_gerente)
  WHERE cod_gerente <> '';

-- ── vendas_transmitidas (data_transmissao) ───────────────────────────────────
CREATE INDEX IF NOT EXISTS idx_vt_data_fornec
  ON vendas_transmitidas (empresa_id, data_transmissao, cod_fornec)
  WHERE cod_fornec <> '';

CREATE INDEX IF NOT EXISTS idx_vt_data_rca
  ON vendas_transmitidas (empresa_id, data_transmissao, cod_rca)
  WHERE cod_rca <> '';

CREATE INDEX IF NOT EXISTS idx_vt_data_sup
  ON vendas_transmitidas (empresa_id, data_transmissao, cod_supervisor)
  WHERE cod_supervisor <> '';

CREATE INDEX IF NOT EXISTS idx_vt_data_ger
  ON vendas_transmitidas (empresa_id, data_transmissao, cod_gerente)
  WHERE cod_gerente <> '';

-- ANALYZE para o planner usar os novos índices
ANALYZE vendas_faturadas;
ANALYZE vendas_transmitidas;
