#!/usr/bin/env bash
# Roda NO SERVIDOR (root) depois de um push: espera o deploy do commit novo
# ficar no ar, regrava os snapshots do Painel de Objetivos (vigências
# ABERTAS — mesmo job diário das 07:30, via FAROL_PREWARM_METAS_ONCE) e
# confere no banco que os campos novos (cod_cli, objetivo) entraram.
#
# Uso:  bash farol-prewarm-prod.sh <sha-do-commit>     (ex: a1b2c3d)
#
# Não mexe em vigências FECHADAS (snapshot congelado, FR17) — mês fechado só
# muda por reprocessamento manual explícito.
set -euo pipefail

SHA="${1:?informe o sha (ou prefixo) do commit que acabou de subir}"
APP="a0wcggw4wo040gwwwokckgk8"   # uuid do app FAROL no Coolify (estável entre deploys)

api_container() { docker ps --format '{{.Names}} {{.Image}}' | awk -v app="^api-$APP" '$1 ~ app {print $1, $2}'; }

echo ">> esperando o deploy do commit ${SHA} (máx. 15 min)..."
for i in $(seq 1 90); do
  linha="$(api_container || true)"
  if [[ "$linha" == *"${SHA}"* ]]; then break; fi
  sleep 10
done
[[ "$linha" == *"${SHA}"* ]] || { echo "!! deploy de ${SHA} não apareceu. Atual: ${linha:-nenhum}"; exit 1; }
API="${linha%% *}"
DB="$(docker ps --format '{{.Names}}' | grep "^db-$APP")"
echo ">> no ar: $API (db: $DB)"
sleep 15   # deixa o boot terminar

echo ">> rodando o prewarm dos snapshots (pode levar alguns minutos)..."
docker exec -w /root -e FAROL_PREWARM_METAS_ONCE=1 "$API" ./server 2>&1 | grep -E 'prewarm|ONCE|FATAL|panic|bind' || true

echo ">> conferindo no banco:"
docker exec "$DB" psql -U postgres -d fb_farol -c "
  select count(*) as snapshots,
         count(*) filter (where resultado_json::text like '%cod_cli%')  as com_cod_cli,
         count(*) filter (where resultado_json::text like '%\"objetivo\"%') as com_objetivo,
         max(calculado_em) as ultimo
  from farol.metas_realizados_snapshot v
  join farol.metas_vigencias vg on vg.id = v.vigencia_id and vg.status = 'aberta'"
echo ">> ok: 'com_cod_cli' e 'com_objetivo' devem ser > 0 (Sortimento sem venda pode não ter cod_cli)."
