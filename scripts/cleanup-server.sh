#!/bin/bash
# Limpeza completa do servidor srv1306085
# Execute como: bash cleanup-server.sh

set -e

echo "=== (1) Investigando uso de disco ==="
echo -e "\nTop 20 maiores diretórios em /:"
du -h --max-depth=2 / 2>/dev/null | sort -hr | head -20 || true

echo -e "\n=== (2) Uso do Docker ==="
docker system df

echo -e "\n=== (3) Logs do journalctl ==="
journalctl --disk-usage

echo -e "\n=== (4) /var/log sizes ==="
du -sh /var/log/* 2>/dev/null | sort -hr | head -10

echo -e "\n=== (5) INICIANDO LIMPEZA ==="
read -p "Continuar? (y/N) " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "Abortado."
    exit 1
fi

echo -e "\n--- Docker: system prune completo ---"
docker system prune -a --volumes -f

echo -e "\n--- Apt: cache e pacotes órfãos ---"
apt-get autoremove -y
apt-get autoclean -y
apt-get clean

echo -e "\n--- Journalctl: logs antigos (mantém últimos 7 dias) ---"
journalctl --vacuum-time=7d

echo -e "\n--- Logs antigos em /var/log (rotaciona) ---"
if [ -d /var/log ]; then
    find /var/log -type f -name "*.gz" -delete 2>/dev/null || true
    find /var/log -type f -name "*.1" -delete 2>/dev/null || true
    find /var/log -type f -name "*.old" -delete 2>/dev/null || true
fi

echo -e "\n--- Temp do sistema ---"
rm -rf /tmp/* 2>/dev/null || true
rm -rf /var/tmp/* 2>/dev/null || true

echo -e "\n=== RESULTADO FINAL ==="
df -h /
docker system df
journalctl --disk-usage
