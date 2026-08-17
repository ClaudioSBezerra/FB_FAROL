#!/usr/bin/env bash
# comparativo_origem.sh — confronta o Oracle da JC com a nossa base, mês a mês.
#
# SOMENTE LEITURA dos dois lados. Não importa, não corrige, não apaga.
#
# Uso:  bash scripts/comparativo_origem.sh [ANO-MES-INICIAL]
#       bash scripts/comparativo_origem.sh 2026-01     (default: 2025-01)
set -uo pipefail

DESDE="${1:-2025-01}"
DESDE_SQL="${DESDE}-01"

source /root/farol-env.sh 2>/dev/null || {
  DB=${DB:-}; EMP=${EMP:-}
}
: "${DB:?defina DB (container do Postgres)}"
: "${EMP:?defina EMP (empresa_id)}"

echo "╔══════════════════════════════════════════════════════════════════════╗"
echo "║ COMPARATIVO ORIGEM (Oracle JC) × FAROL (Postgres)                    ║"
echo "║ desde $DESDE_SQL — somente leitura                                   "
echo "╚══════════════════════════════════════════════════════════════════════╝"
echo
echo "A classificação por evento replica detectEvento() do importador:"
echo "  ESTADO/PERIODO contém TRANS  → TRANSMITIDO  (vendas_transmitidas)"
echo "  ESTADO contém CORT/CANCEL/DEVOL → CCD       (vendas_ccd)"
echo "  qualquer outro               → FATURADO     (vendas_faturadas)"
echo
echo "Sem essa classificação a soma do Oracle junta faturado + transmitido e a"
echo "comparação fica sem sentido — foi o que aconteceu em 12/08/2026, quando"
echo "o total da origem veio 2,4× maior por esse motivo."
echo

# ─── Lado ORIGEM ────────────────────────────────────────────────────────────
echo "═══════════════════ ORACLE (origem) ═══════════════════"
JC_USER=IAUSER JC_PASS=IAUSER JC_SQL="
SELECT TO_CHAR(DATA,'YYYY-MM') AS MES,
       CASE
         WHEN UPPER(ESTADO) LIKE '%TRANS%' OR UPPER(PERIODO) LIKE '%TRANS%' THEN 'TRANSMITIDO'
         WHEN UPPER(ESTADO) LIKE '%CORT%'   THEN 'CCD'
         WHEN UPPER(ESTADO) LIKE '%CANCEL%' THEN 'CCD'
         WHEN UPPER(ESTADO) LIKE '%DEVOL%'  THEN 'CCD'
         ELSE 'FATURADO'
       END AS EVENTO,
       COUNT(*) AS LINHAS,
       ROUND(SUM(PVENDA_TOTAL),2) AS VALOR
  FROM IAUSER.COMPRAS_FAROL_VW
 WHERE DATA >= DATE '$DESDE_SQL'
 GROUP BY TO_CHAR(DATA,'YYYY-MM'),
       CASE
         WHEN UPPER(ESTADO) LIKE '%TRANS%' OR UPPER(PERIODO) LIKE '%TRANS%' THEN 'TRANSMITIDO'
         WHEN UPPER(ESTADO) LIKE '%CORT%'   THEN 'CCD'
         WHEN UPPER(ESTADO) LIKE '%CANCEL%' THEN 'CCD'
         WHEN UPPER(ESTADO) LIKE '%DEVOL%'  THEN 'CCD'
         ELSE 'FATURADO'
       END
 ORDER BY 1, 2" /tmp/jc-probe

echo
echo "═══════════════════ POSTGRES (Farol) ═══════════════════"
docker exec -i "$DB" psql -U postgres -d fb_farol -c "
SELECT mes, evento, SUM(linhas) AS linhas, ROUND(SUM(valor)::numeric,2) AS valor
  FROM (
    SELECT to_char(data_faturamento,'YYYY-MM') AS mes, 'FATURADO' AS evento,
           COUNT(*) AS linhas, COALESCE(SUM(pvenda),0) AS valor
      FROM vendas_faturadas
     WHERE empresa_id='$EMP' AND data_faturamento >= DATE '$DESDE_SQL'
     GROUP BY 1
    UNION ALL
    SELECT to_char(data_transmissao,'YYYY-MM'), 'TRANSMITIDO',
           COUNT(*), COALESCE(SUM(pvenda),0)
      FROM vendas_transmitidas
     WHERE empresa_id='$EMP' AND data_transmissao >= DATE '$DESDE_SQL'
     GROUP BY 1
    UNION ALL
    SELECT to_char(data_evento,'YYYY-MM'), 'CCD',
           COUNT(*), COALESCE(SUM(pvenda),0)
      FROM vendas_ccd
     WHERE empresa_id='$EMP' AND data_evento >= DATE '$DESDE_SQL'
     GROUP BY 1
  ) u
 GROUP BY mes, evento
 ORDER BY mes, evento;"

echo
echo "═══════════════════ ÚLTIMO DIA DE CADA LADO ═══════════════════"
echo "A diferença aqui é o que ainda NÃO foi importado — não é divergência."
JC_USER=IAUSER JC_PASS=IAUSER JC_SQL="
SELECT TO_CHAR(MAX(DATA),'YYYY-MM-DD') AS ULTIMO_DIA_ORIGEM
  FROM IAUSER.COMPRAS_FAROL_VW" /tmp/jc-probe

docker exec -i "$DB" psql -U postgres -d fb_farol -c "
SELECT MAX(data_faturamento) AS ultimo_faturado,
       (SELECT MAX(data_transmissao) FROM vendas_transmitidas WHERE empresa_id='$EMP') AS ultimo_transmitido
  FROM vendas_faturadas WHERE empresa_id='$EMP';"

echo
echo "── fim (nada foi alterado nos dois lados) ──"
