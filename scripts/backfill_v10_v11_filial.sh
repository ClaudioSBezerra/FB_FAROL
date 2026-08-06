#!/usr/bin/env bash
# backfill_v10_v11_filial.sh — pós-deploy da migration 199 (aggs com FILIAL).
#
# A migration cria as 16 tabelas VAZIAS de propósito: migrations rodam no
# startup e um backfill de 19 meses travaria a subida do container. Este script
# preenche o histórico e cria os índices de apoio ao fallback.
#
# ⚠ NÃO RODAR COM IMPORT EM ANDAMENTO. O upsert lê vendas_faturadas/
#   vendas_transmitidas do mês inteiro e concorreria com a carga.
#
# ⚠ Enquanto não rodar, nada quebra: o gate aggFilialReady mantém o filtro de
#   filial no scan de vendas_* (comportamento atual). O script pode esperar.
#
# Uso, no servidor:
#   source /root/farol-env.sh      # define DB e EMP
#   bash backfill_v10_v11_filial.sh
set -euo pipefail

: "${DB:?defina DB (container do Postgres) — rode 'source /root/farol-env.sh'}"

psql() { docker exec -i "$DB" psql -v ON_ERROR_STOP=1 -U postgres -d fb_farol "$@"; }

echo "═══ 1/2 · backfill das aggs V10/V11 (uma passada por mês) ═══"
echo "Conta ~19 meses. O tempo por mês é parecido com o do V08/V09."
date

psql <<'SQL'
DO $b$
DECLARE r RECORD; n INT := 0;
BEGIN
  FOR r IN SELECT DISTINCT empresa_id, ano, mes
             FROM farol.agg_fat_v01_l0_mes
            ORDER BY ano, mes
  LOOP
    PERFORM farol.upsert_aggs_mes_v10_v11(r.empresa_id, r.ano, r.mes);
    PERFORM farol.upsert_venda_liquida_cols(r.empresa_id, r.ano, r.mes);
    n := n + 1;
    RAISE NOTICE 'v10/v11 %-% ok (% de %)', r.ano, r.mes, n, n;
  END LOOP;
  RAISE NOTICE 'concluído: % mes(es)', n;
END $b$;
SQL

echo
echo "═══ 2/2 · índices de apoio ao fallback (2+ filiais → scan) ═══"
echo "CONCURRENTLY: não bloqueia escrita, mas não pode estar em transação."

# Uma sessão por índice — CONCURRENTLY exige autocommit.
psql -c "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_vf_filial
           ON vendas_faturadas (empresa_id, empresa, data_faturamento)
           WHERE empresa <> '';"
psql -c "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_vt_filial
           ON vendas_transmitidas (empresa_id, empresa, data_transmissao)
           WHERE empresa <> '';"

echo
echo "═══ verificação ═══"
psql -c "SELECT (SELECT COUNT(*) FROM farol.agg_fat_v10_l0_mes)  AS fat_v10_l0,
                (SELECT COUNT(*) FROM farol.agg_fat_v11_l1_mes)  AS fat_v11_l1,
                (SELECT COUNT(*) FROM farol.agg_trans_v10_l0_mes) AS trans_v10_l0;"

echo
echo "Se fat_v11_l1 > 0, o gate aggFilialReady abre em até 5 min (ou no próximo"
echo "restart do container) e o filtro de filial passa a usar as aggs."
date
