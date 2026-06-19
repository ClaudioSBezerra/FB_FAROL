-- 179_idx_positivados_groupcol.sql
-- ════════════════════════════════════════════════════════════════════════════
-- PERF: índices compostos para queryDistinctPositivados (gargalo do fetchCards)
-- ════════════════════════════════════════════════════════════════════════════
--
-- Problema: queryDistinctPositivados roda na tabela FOLHA (grão cnpj×mês):
--
--   SELECT v.<groupCol>, COUNT(DISTINCT v.cnpj)
--   FROM <leaf>
--   WHERE empresa_id=$1 AND <groupCol> <> '' AND positivados>0
--         AND (ano*100+mes) BETWEEN $a AND $b
--   GROUP BY <groupCol>
--
-- A mig 177 criou índice (empresa_id, cnpj) WHERE positivados>0, que NÃO cobre
-- o GROUP BY <groupCol> nem o filtro de mês → Postgres varre a folha inteira.
-- No login 3 fetchCards rodam em paralelo (V01/V02/V03) × 3 janelas (base, ref,
-- comp) = ~9 COUNT(DISTINCT) concorrentes na mesma folha → 13-20s no cache frio.
--
-- Solução: índice (empresa_id, <groupCol>, ano, mes, cnpj) WHERE positivados>0.
-- Cobre: filtro empresa+groupCol, range de mês, e cnpj para o DISTINCT — tudo
-- pelo índice (index-only-ish), sem tocar o heap da folha.
--
-- Foco no CAMINHO DO LOGIN (top-level de cada view):
--   V01 l0 → GROUP BY cod_fornec      na folha agg_*_v01_l4_mes
--   V02 l0 → GROUP BY cod_supervisor  na folha agg_*_v02_l3_mes
--   V03 l0 → GROUP BY cod_gerente     na folha agg_*_v03_l3_mes
--
-- NOTA: sem CONCURRENTLY (migrations rodam em transação).

-- ── V01 (folha l4) — agrupado por cod_fornec ────────────────────────────────
CREATE INDEX IF NOT EXISTS idx_aggfat_v01l4_pos_fornec
  ON farol.agg_fat_v01_l4_mes   (empresa_id, cod_fornec, ano, mes, cnpj)
  WHERE positivados > 0;
CREATE INDEX IF NOT EXISTS idx_aggtra_v01l4_pos_fornec
  ON farol.agg_trans_v01_l4_mes (empresa_id, cod_fornec, ano, mes, cnpj)
  WHERE positivados > 0;

-- ── V02 (folha l3) — agrupado por cod_supervisor ────────────────────────────
CREATE INDEX IF NOT EXISTS idx_aggfat_v02l3_pos_sup
  ON farol.agg_fat_v02_l3_mes   (empresa_id, cod_supervisor, ano, mes, cnpj)
  WHERE positivados > 0;
CREATE INDEX IF NOT EXISTS idx_aggtra_v02l3_pos_sup
  ON farol.agg_trans_v02_l3_mes (empresa_id, cod_supervisor, ano, mes, cnpj)
  WHERE positivados > 0;

-- ── V03 (folha l3) — agrupado por cod_gerente ───────────────────────────────
CREATE INDEX IF NOT EXISTS idx_aggfat_v03l3_pos_ger
  ON farol.agg_fat_v03_l3_mes   (empresa_id, cod_gerente, ano, mes, cnpj)
  WHERE positivados > 0;
CREATE INDEX IF NOT EXISTS idx_aggtra_v03l3_pos_ger
  ON farol.agg_trans_v03_l3_mes (empresa_id, cod_gerente, ano, mes, cnpj)
  WHERE positivados > 0;

-- ANALYZE para o planner usar os novos índices
ANALYZE farol.agg_fat_v01_l4_mes;
ANALYZE farol.agg_trans_v01_l4_mes;
ANALYZE farol.agg_fat_v02_l3_mes;
ANALYZE farol.agg_trans_v02_l3_mes;
ANALYZE farol.agg_fat_v03_l3_mes;
ANALYZE farol.agg_trans_v03_l3_mes;
