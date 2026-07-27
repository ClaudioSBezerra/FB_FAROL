-- 196_fix_idx_uf_ordem_colunas.sql
-- ════════════════════════════════════════════════════════════════════════════
-- Corrige a ORDEM DAS COLUNAS dos índices de UF criados na mig 195.
--
-- A 195 criou (empresa_id, data_faturamento, uf) — uf por ÚLTIMO. Nas queries
-- do filtro cruzado o padrão é: empresa_id = $1 (igualdade), data em RANGE,
-- uf = ANY(...) (igualdade/IN). Com uf depois do range de data, o índice não
-- consegue restringir por uf (o range "quebra" o prefixo) — confirmado em
-- produção 27/07/2026: EXPLAIN ANALYZE mostrou o planner IGNORANDO idx_vf_uf
-- e fazendo Parallel Seq Scan de 17.8M linhas.
--
-- Ordem correta: (empresa_id, uf, data) — igualdades primeiro, range por
-- último. O = ANY vira N range-scans estreitos (um por UF). Para UFs pequenas
-- e períodos curtos (mês atual — uso mais comum) o ganho é grande; para
-- UF+período que cubram ~25%+ da tabela o planner ainda vai preferir seq scan
-- (correto — nesse caso quem salva é o agg V08/V09 da mig 197).
--
-- Aproveita e DROPA os índices de "Filial" (coluna empresa) da 195: o filtro
-- Filial foi removido da UI em 27/07/2026 (FarolExecutivo.tsx) — manter os
-- índices só encareceria o import (2 índices × 2 tabelas × 18M linhas).
-- ════════════════════════════════════════════════════════════════════════════

DROP INDEX IF EXISTS idx_vf_uf;
DROP INDEX IF EXISTS idx_vt_uf;
DROP INDEX IF EXISTS idx_vf_filial;
DROP INDEX IF EXISTS idx_vt_filial;

CREATE INDEX IF NOT EXISTS idx_vf_uf
    ON vendas_faturadas (empresa_id, uf, data_faturamento)
    WHERE uf <> '';

CREATE INDEX IF NOT EXISTS idx_vt_uf
    ON vendas_transmitidas (empresa_id, uf, data_transmissao)
    WHERE uf <> '';
