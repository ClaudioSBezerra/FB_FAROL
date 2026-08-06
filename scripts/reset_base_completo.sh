#!/usr/bin/env bash
# reset_base_completo.sh — ZERA a base do Farol para uma recarga do zero.
#
# ⚠⚠ DESTRUTIVO E IRREVERSÍVEL. Apaga vendas + TODOS os agregados. ⚠⚠
#
# Por que não basta truncar vendas_*: os painéis não leem das tabelas cruas,
# leem dos ~50 agregados em `farol`. E `upsert_aggs_mes` é UPSERT, não rebuild —
# reimportar por cima NÃO apaga linha agregada cuja origem sumiu, então números
# velhos ficariam somados aos novos.
#
# Uso, no servidor:
#   source /root/farol-env.sh
#   bash reset_base_completo.sh
set -euo pipefail

: "${DB:?defina DB — rode 'source /root/farol-env.sh'}"
: "${API:?defina API — rode 'source /root/farol-env.sh'}"

psql() { docker exec -i "$DB" psql -v ON_ERROR_STOP=1 -U postgres -d fb_farol "$@"; }

echo "═══════════════════════════════════════════════════════════════"
echo "  RESET COMPLETO DA BASE — isto APAGA todas as vendas e agregados"
echo "  DB=$DB   API=$API"
echo "═══════════════════════════════════════════════════════════════"
echo
echo "Estado ATUAL:"
psql -c "SELECT
   (SELECT COUNT(*) FROM vendas_faturadas)    AS faturadas,
   (SELECT COUNT(*) FROM vendas_transmitidas) AS transmitidas,
   (SELECT COUNT(*) FROM vendas_ccd)          AS ccd;"
echo
read -rp "Digite APAGAR TUDO para confirmar: " c
[ "$c" = "APAGAR TUDO" ] || { echo "abortado."; exit 1; }

# ── 1. TRUNCATE ────────────────────────────────────────────────────────────
# Os agregados são descobertos pelo CATÁLOGO, não por lista fixa: são V01..V11
# mais dims e mkt, e lista escrita à mão esquece alguma (foi o que quase
# aconteceu quando as V08/V09 entraram). relispartition=false pega só a
# tabela-mãe — TRUNCATE nela cascateia para as partições de ano.
#
# ⚠ consolidacao_pendente/log vivem no schema `farol`, NÃO em public. Sem o
#   prefixo, o psql erra; e sem ON_ERROR_STOP=1 ele SEGUE e o COMMIT vira
#   ROLLBACK silencioso, com a saída ainda mostrando zeros que parecem sucesso.
#   Foi exatamente o que aconteceu em 05/08/2026.
echo
echo "── truncando ─────────────────────────────────────────────────"
psql <<'SQL'
BEGIN;
DO $$
DECLARE t text; n int := 0;
BEGIN
  FOR t IN
    SELECT c.relname FROM pg_class c JOIN pg_namespace ns ON ns.oid = c.relnamespace
     WHERE ns.nspname = 'farol' AND c.relkind IN ('r','p')
       AND c.relispartition = false
       AND (c.relname LIKE 'agg\_%' OR c.relname LIKE '%\_dims\_%')
  LOOP
    EXECUTE format('TRUNCATE TABLE farol.%I CASCADE', t);
    n := n + 1;
  END LOOP;
  RAISE NOTICE 'agregados truncados: %', n;
END $$;

TRUNCATE TABLE vendas_faturadas, vendas_transmitidas, vendas_ccd;
TRUNCATE TABLE farol.consolidacao_pendente;
TRUNCATE TABLE farol.consolidacao_log;
TRUNCATE TABLE vendas_import_jobs;
COMMIT;
SQL

# ── 2. MVs ─────────────────────────────────────────────────────────────────
# Derivam de vendas_*; sem refresh ficam com a carteira e o filtro de UF velhos.
echo
echo "── refresh das materialized views ────────────────────────────"
psql -c "REFRESH MATERIALIZED VIEW farol.mv_fat_carteira_rca;"
psql -c "REFRESH MATERIALIZED VIEW farol.mv_trans_carteira_rca;"
psql -c "REFRESH MATERIALIZED VIEW farol.mv_fat_uf_mes;"

# ── 3. RESTART DA API — NÃO É OPCIONAL ─────────────────────────────────────
# Dois motivos, e o segundo é silencioso:
#   a) baseCache/vendasPeriodoCache têm TTL de 20h e não têm endpoint de limpeza
#      — sem restart os painéis seguem mostrando os números antigos.
#   b) os gates aggUFReady/aggFilialReady guardam o resultado POSITIVO para
#      sempre (`if val { return true }`, farol_v2_api.go). Depois do TRUNCATE
#      eles continuam abertos e os filtros de UF/filial roteiam para tabelas
#      VAZIAS → cards zerados, sem erro nenhum no log.
echo
echo "── reiniciando a API (limpa cache E os gates de agg) ──────────"
docker restart "$API"
sleep 20

# ── 4. Conferência ─────────────────────────────────────────────────────────
echo
echo "── conferência: tudo tem que estar em zero ───────────────────"
psql -c "SELECT
   (SELECT COUNT(*) FROM vendas_faturadas)          AS faturadas,
   (SELECT COUNT(*) FROM vendas_transmitidas)       AS transmitidas,
   (SELECT COUNT(*) FROM vendas_ccd)                AS ccd,
   (SELECT COUNT(*) FROM farol.agg_fat_v01_l0_mes)  AS agg_v01,
   (SELECT COUNT(*) FROM farol.agg_fat_v08_l0_mes)  AS agg_uf,
   (SELECT COUNT(*) FROM farol.agg_fat_v10_l0_mes)  AS agg_filial,
   (SELECT COUNT(*) FROM farol.consolidacao_pendente) AS pendentes;"

cat <<'FIM'

═══════════════════════════════════════════════════════════════
Base zerada. Para recarregar (ajuste as datas):

  TOKEN=$(curl -s -X POST http://localhost:8087/api/auth/login \
    -H 'Content-Type: application/json' \
    -d '{"email":"importacao.dados@jcdistribuicao.com.br","password":"123456"}' \
    | grep -o '"token":"[^"]*"' | cut -d'"' -f4)
  [ -z "$TOKEN" ] && { echo "LOGIN FALHOU"; exit 1; }

  curl -s -X POST \
    "http://localhost:8087/api/v2/jc/carga?de=2025-01-01&ate=2026-07-31&passo=mes" \
    -H "Authorization: Bearer $TOKEN"

Conte ~40 min por mês (medido em 06/08: 19 meses em ~11h40, não as 3h que
eu havia estimado). A carga popula V01..V11 sozinha — inclusive as aggs de
FILIAL — e a consolidação final roda uma vez só, no fim.

⚠ NÃO dê git push enquanto rodar: o redeploy do Coolify recria os containers
  e mata o processo no meio.

DEPOIS que terminar, criar os índices do fallback de filial (2+ filiais
selecionadas seguem no scan). Fora da carga, porque CREATE INDEX durante o
COPY encarece a importação:

  docker exec -i $DB psql -U postgres -d fb_farol -c \
    "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_vf_filial
       ON vendas_faturadas (empresa_id, empresa, data_faturamento)
       WHERE empresa <> '';"
  docker exec -i $DB psql -U postgres -d fb_farol -c \
    "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_vt_filial
       ON vendas_transmitidas (empresa_id, empresa, data_transmissao)
       WHERE empresa <> '';"
═══════════════════════════════════════════════════════════════
FIM
