-- ════════════════════════════════════════════════════════════════════════════
-- Índices para queryAggregatedVendas (filtro cruzado) — VERSÃO CONCURRENTLY
-- ════════════════════════════════════════════════════════════════════════════
--
-- Mesmo conteúdo da migration 180, mas com CONCURRENTLY (não bloqueia a tabela
-- durante a criação). Rodar manualmente no servidor:
--
--   psql -d <DB_NAME> -f idx_vendas_concurrently.sql
--
-- CONCURRENTLY não pode rodar dentro de transação, então cada statement é
-- independente. Quando o Coolify reiniciar o backend e a migration 180 rodar,
-- os CREATE INDEX IF NOT EXISTS verão que já existem e pularão (idempotente).
-- O ANALYZE da migration 180 roda em seguida (~1s).
--
-- Tempo estimado: ~3-5min por índice (8 índices total).
-- Pode rodar com o sistema em produção — só consome CPU/IO adicionais.

\echo 'Criando índices em vendas_faturadas (concurrently)...'

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_vf_data_fornec
  ON vendas_faturadas (empresa_id, data_faturamento, cod_fornec)
  WHERE cod_fornec <> '';
\echo '  ✓ idx_vf_data_fornec'

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_vf_data_rca
  ON vendas_faturadas (empresa_id, data_faturamento, cod_rca)
  WHERE cod_rca <> '';
\echo '  ✓ idx_vf_data_rca'

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_vf_data_sup
  ON vendas_faturadas (empresa_id, data_faturamento, cod_supervisor)
  WHERE cod_supervisor <> '';
\echo '  ✓ idx_vf_data_sup'

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_vf_data_ger
  ON vendas_faturadas (empresa_id, data_faturamento, cod_gerente)
  WHERE cod_gerente <> '';
\echo '  ✓ idx_vf_data_ger'

\echo 'Criando índices em vendas_transmitidas (concurrently)...'

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_vt_data_fornec
  ON vendas_transmitidas (empresa_id, data_transmissao, cod_fornec)
  WHERE cod_fornec <> '';
\echo '  ✓ idx_vt_data_fornec'

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_vt_data_rca
  ON vendas_transmitidas (empresa_id, data_transmissao, cod_rca)
  WHERE cod_rca <> '';
\echo '  ✓ idx_vt_data_rca'

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_vt_data_sup
  ON vendas_transmitidas (empresa_id, data_transmissao, cod_supervisor)
  WHERE cod_supervisor <> '';
\echo '  ✓ idx_vt_data_sup'

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_vt_data_ger
  ON vendas_transmitidas (empresa_id, data_transmissao, cod_gerente)
  WHERE cod_gerente <> '';
\echo '  ✓ idx_vt_data_ger'

\echo 'Atualizando estatísticas do planner...'
ANALYZE vendas_faturadas;
ANALYZE vendas_transmitidas;

\echo '✅ Concluído! Os índices da migration 180 serão no-ops quando o backend reiniciar.'
