#!/usr/bin/env bash
# Gera jan-2025.csv .. dez-2025.csv a partir de jan-2026.csv.
# Troca: coluna PERIODO (_JANEIRO → _NOMEMES) e coluna DATA (/01/2026 → /MM/2025).
# Fevereiro: filtra dias > 28. Demais meses: 30 dias (igual à fonte).
# Roda 4 em paralelo para acelerar.

set -euo pipefail

SRC="$(dirname "$0")/jan-2026.csv"
OUT="$(dirname "$0")"

declare -a MESES=(
  "01:jan:JANEIRO:30"
  "02:fev:FEVEREIRO:28"
  "03:mar:MARCO:30"
  "04:abr:ABRIL:30"
  "05:mai:MAIO:30"
  "06:jun:JUNHO:30"
  "07:jul:JULHO:30"
  "08:ago:AGOSTO:30"
  "09:set:SETEMBRO:30"
  "10:out:OUTUBRO:30"
  "11:nov:NOVEMBRO:30"
  "12:dez:DEZEMBRO:30"
)

gera() {
  local mm="$1" abbr="$2" nome="$3" max_day="$4"
  local dest="${OUT}/${abbr}-2025.csv"

  echo "[$(date +%H:%M:%S)] Iniciando ${abbr}-2025.csv (max_day=${max_day})…"

  awk -v mm="$mm" -v nome="$nome" -v max_day="$max_day" '
  BEGIN { FS=OFS=";" }
  NR==1 { print; next }
  {
    # Filtra dias além do limite do mês
    day = substr($26, 1, 2) + 0
    if (day > max_day) next
    # Substitui mês no campo DATA: DD/01/2026 → DD/MM/2025
    sub(/\/01\/2026/, "/" mm "/2025", $26)
    # Substitui nome do mês no PERIODO
    sub(/_JANEIRO/, "_" nome, $1)
    print
  }' "$SRC" > "$dest"

  local lines
  lines=$(wc -l < "$dest")
  echo "[$(date +%H:%M:%S)] Concluído ${abbr}-2025.csv — ${lines} linhas ($(du -h "$dest" | cut -f1))"
}

export -f gera
export SRC OUT

# Paralelo: 4 jobs simultâneos
JOBS=0
for entry in "${MESES[@]}"; do
  IFS=':' read -r mm abbr nome max_day <<< "$entry"
  gera "$mm" "$abbr" "$nome" "$max_day" &
  JOBS=$((JOBS+1))
  if [ $JOBS -ge 4 ]; then
    wait -n 2>/dev/null || wait   # aguarda qualquer um terminar
    JOBS=$((JOBS-1))
  fi
done
wait

echo ""
echo "=== Todos os arquivos de 2025 gerados ==="
ls -lh "${OUT}"/*-2025.csv
