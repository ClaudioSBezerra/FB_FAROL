-- indices_v07_produto.sql — acelera "Por Departamento → Seção → Produto".
--
-- Por que: o nível Produto sob a Seção lê a base (vendas_*), filtrando por
-- cod_depto + cod_sec. Hoje não existe índice nessas colunas, então um período
-- longo (ex.: Ano × Ano) varre ~12 milhões de linhas por partição: medido em
-- produção 13/10/2026, 15 a 29 segundos. Período curto (dia/semana/mês) já é
-- rápido pelo índice de data e NÃO depende deste script.
--
-- Como rodar (fora do horário de pico; não bloqueia gravação — CONCURRENTLY):
--   ssh root@76.13.171.196
--   docker exec -i $(docker ps --format '{{.Names}}' | grep ^db-a0wcggw4wo040gwwwokckgk8) \
--       psql -U postgres -d fb_farol -f - < indices_v07_produto.sql
-- (ou copie o conteúdo e cole no psql). Cada CREATE INDEX leva alguns minutos.
-- Espaço esperado: ~1 GB por partição (4 índices); há ~40 GB livres em 13/10.
--
-- CONCURRENTLY não roda dentro de transação — por isso este arquivo NÃO é uma
-- migration do backend (migrations rodam no boot e bloqueariam o deploy).
-- Acompanhar: SELECT * FROM pg_stat_progress_create_index;
-- Desfazer:   DROP INDEX CONCURRENTLY idx_vf_2026_depto_sec_data;  (e os outros)

SET maintenance_work_mem = '1GB';

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_vf_2026_depto_sec_data
    ON vendas_faturadas_2026 (empresa_id, cod_depto, cod_sec, data_faturamento)
    INCLUDE (cod_prod, nome_prod, pvenda, plucro) WHERE cod_prod <> '';

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_vf_2025_depto_sec_data
    ON vendas_faturadas_2025 (empresa_id, cod_depto, cod_sec, data_faturamento)
    INCLUDE (cod_prod, nome_prod, pvenda, plucro) WHERE cod_prod <> '';

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_vt_2026_depto_sec_data
    ON vendas_transmitidas_2026 (empresa_id, cod_depto, cod_sec, data_transmissao)
    INCLUDE (cod_prod, nome_prod, pvenda, plucro) WHERE cod_prod <> '';

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_vt_2025_depto_sec_data
    ON vendas_transmitidas_2025 (empresa_id, cod_depto, cod_sec, data_transmissao)
    INCLUDE (cod_prod, nome_prod, pvenda, plucro) WHERE cod_prod <> '';

ANALYZE vendas_faturadas_2026;
ANALYZE vendas_faturadas_2025;
ANALYZE vendas_transmitidas_2026;
ANALYZE vendas_transmitidas_2025;
