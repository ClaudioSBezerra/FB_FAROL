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
# 10/09/2026 — passou a também RECONSTRUIR vendas_faturadas/vendas_transmitidas
# como tabelas PARTICIONADAS POR ANO (RANGE(data)), em vez de só truncar as
# tabelas normais existentes. Isto é a "estrutura definitiva" pedida pelo
# Claudio: com a base zerada de qualquer forma (reset completo + reimport via
# Oracle), dá pra nascer já particionada por ano, sem o custo/risco de
# converter uma tabela populada (que exigiria ATTACH PARTITION — ver
# scripts/particionar_vendas_2025_2026.sql, um script ANTERIOR que fazia
# exatamente isso pra não perder os ~49GB que existiam até então; agora que
# vamos zerar tudo mesmo, esse script fica obsoleto/histórico). Cada reset
# futuro (se algum dia rodar de novo) recria a estrutura particionada do
# zero automaticamente — não é um hack de uma vez só.
#
# A reconstrução particionada em si vive na FUNÇÃO farol._rebuild_vendas_
# particionado(), definida na migration 232 — este script só a CHAMA (depois
# do operador confirmar "APAGAR TUDO"). Fonte única da verdade: a mesma função
# roda no boot de qualquer banco novo/disposable pra nascer já particionado.
# A função dropa as 3 MVs (mv_fat_carteira_rca, mv_trans_carteira_rca,
# mv_fat_uf_mes) antes do DROP TABLE (dependem das tabelas por OID) e as
# recria contra o novo pai particionado.
#
# ⚠ PRÉ-REQUISITO: a migration 232 já tem que ter rodado neste banco (i.e.
#   o deploy com ela já subiu) — senão farol._rebuild_vendas_particionado()
#   não existe ainda. No primeiro deploy da 232 contra a base cheia, ela só
#   emite um WARNING e não faz nada (não converte tabela com dado); é este
#   script, rodado à mão logo depois, que efetivamente migra a estrutura.
#
# Uso, no servidor:
#   1. git push da branch com a migration 232 → esperar o deploy do Coolify
#   2. source /root/farol-env.sh
#   3. bash reset_base_completo.sh
set -euo pipefail

: "${DB:?defina DB — rode 'source /root/farol-env.sh'}"
: "${API:?defina API — rode 'source /root/farol-env.sh'}"

psql() { docker exec -i "$DB" psql -v ON_ERROR_STOP=1 -U postgres -d fb_farol "$@"; }

echo "═══════════════════════════════════════════════════════════════"
echo "  RESET COMPLETO DA BASE — isto APAGA todas as vendas e agregados"
echo "  e RECRIA vendas_faturadas/vendas_transmitidas particionadas por ano"
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

# ── 1. TRUNCATE dos agregados + DROP/RECRIA particionada das tabelas cruas ──
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
echo "── truncando agregados + reconstruindo vendas_* particionadas ──"
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

TRUNCATE TABLE vendas_ccd;
TRUNCATE TABLE farol.consolidacao_pendente;
TRUNCATE TABLE farol.consolidacao_log;
TRUNCATE TABLE vendas_import_jobs;

-- Dropa e recria vendas_faturadas/vendas_transmitidas particionadas por ano
-- + as 3 MVs dependentes. Toda a DDL vive nesta função (migration 232) —
-- fonte única da verdade, compartilhada com o boot de bancos novos.
SELECT farol._rebuild_vendas_particionado();

COMMIT;
SQL

# ── 2. RESTART DA API — NÃO É OPCIONAL ─────────────────────────────────────
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

# ── 3. Conferência ─────────────────────────────────────────────────────────
echo
echo "── conferência: tudo tem que estar em zero, e as tabelas cruas já"
echo "   têm que aparecer PARTICIONADAS ────────────────────────────"
psql -c "SELECT
   (SELECT COUNT(*) FROM vendas_faturadas)          AS faturadas,
   (SELECT COUNT(*) FROM vendas_transmitidas)       AS transmitidas,
   (SELECT COUNT(*) FROM vendas_ccd)                AS ccd,
   (SELECT COUNT(*) FROM farol.agg_fat_v01_l0_mes)  AS agg_v01,
   (SELECT COUNT(*) FROM farol.agg_fat_v08_l0_mes)  AS agg_uf,
   (SELECT COUNT(*) FROM farol.agg_fat_v10_l0_mes)  AS agg_filial,
   (SELECT COUNT(*) FROM farol.consolidacao_pendente) AS pendentes;"
psql -c "SELECT relname, relkind FROM pg_class WHERE relname IN ('vendas_faturadas','vendas_transmitidas') AND relkind = 'p';"
psql -c "SELECT inhrelid::regclass AS particao FROM pg_inherits WHERE inhparent = 'vendas_faturadas'::regclass ORDER BY 1;"

cat <<'FIM'

═══════════════════════════════════════════════════════════════
Base zerada e RECONSTRUÍDA JÁ PARTICIONADA POR ANO. Para recarregar,
em DUAS ondas (2026 primeiro — ativa o Painel de Indústria hoje — e 2025
depois, sem pressa, durante o fim de semana).

⚠ ANTES de disparar as ondas: pôr JC_JOBS_PAUSED=1 no ambiente (Coolify) e
  redeployar, pra carga diária das 04:30 / reextração / prewarm NÃO
  competirem com o backfill. Tirar a variável e redeployar quando as duas
  ondas terminarem (ver ONBOARDING/memória).

  TOKEN=$(curl -s -X POST http://localhost:8087/api/auth/login \
    -H 'Content-Type: application/json' \
    -d '{"email":"importacao.dados@jcdistribuicao.com.br","password":"123456"}' \
    | grep -o '"token":"[^"]*"' | cut -d'"' -f4)
  [ -z "$TOKEN" ] && { echo "LOGIN FALHOU"; exit 1; }

  # Onda 1 — 2026 de janeiro até ONTEM (~9 meses, algumas horas). Até ontem,
  # não até hoje: o dia corrente ainda está incompleto na origem e a carga
  # diária pega D-1 de qualquer forma.
  curl -s -X POST \
    "http://localhost:8087/api/v2/jc/carga?de=2026-01-01&ate=$(date -d yesterday +%F)&passo=mes" \
    -H "Authorization: Bearer $TOKEN"

  # Onda 2 — depois que a onda 1 terminar, 2025 completo (~12 meses):
  curl -s -X POST \
    "http://localhost:8087/api/v2/jc/carga?de=2025-01-01&ate=2025-12-31&passo=mes" \
    -H "Authorization: Bearer $TOKEN"

Conte ~40 min por mês (medido em 06/08: 19 meses em ~11h40). A carga popula
V01..V11 sozinha — inclusive as aggs de FILIAL — e a consolidação final roda
uma vez só, no fim de CADA onda.

⚠ NÃO dê git push enquanto rodar: o redeploy do Coolify recria os containers
  e mata o processo no meio.

DEPOIS que as DUAS ondas terminarem, criar os índices do fallback de filial
(2+ filiais selecionadas seguem no scan). Fora da carga, porque CREATE INDEX
durante o COPY encarece a importação — e agora as tabelas são particionadas,
então CONCURRENTLY funciona direto no pai (Postgres 14+ propaga pra cada
partição sozinho):

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
