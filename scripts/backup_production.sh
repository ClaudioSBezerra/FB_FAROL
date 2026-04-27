#!/bin/bash

# SCRIPT DE BACKUP COMPLETO PARA PRODUÇÃO
# FB_FAROL - Sistema de Reforma Tributária
# Data: $(date +%Y-%m-%d %H:%M:%S)

# CONFIGURAÇÕES
BACKUP_DIR="/opt/fb_farol/backups"
DB_NAME="fiscal_db"
DB_USER="postgres"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
BACKUP_FILE="fb_farol_full_backup_${TIMESTAMP}.sql"
COMPRESSED_FILE="${BACKUP_FILE}.tar.gz"

echo "==================================================="
echo "BACKUP COMPLETO DO BANCO DE DADOS - FB_FAROL"
echo "Data/Hora: $(date)"
echo "==================================================="

# Criar diretório de backups se não existir
mkdir -p ${BACKUP_DIR}

echo "1. Iniciando backup completo do banco de dados..."

# Backup completo do banco PostgreSQL
PGPASSWORD=${DB_PASSWORD} pg_dump \
    --host=localhost \
    --port=5432 \
    --username=${DB_USER} \
    --dbname=${DB_NAME} \
    --verbose \
    --no-password \
    --format=custom \
    --compress=9 \
    --lock-wait-timeout=30000 \
    --exclude-table-data=sessions \
    --exclude-table-data=audit_logs \
    --file=${BACKUP_DIR}/${BACKUP_FILE}

if [ $? -eq 0 ]; then
    echo "✅ Backup do banco concluído com sucesso!"
    echo "📁 Arquivo: ${BACKUP_DIR}/${BACKUP_FILE}"
else
    echo "❌ ERRO: Falha no backup do banco de dados!"
    exit 1
fi

echo ""
echo "2. Compactando arquivo de backup..."

# Comprimir o arquivo
cd ${BACKUP_DIR}
tar -czf ${COMPRESSED_FILE} ${BACKUP_FILE}

if [ $? -eq 0 ]; then
    echo "✅ Arquivo compactado com sucesso!"
    echo "📁 Arquivo compactado: ${BACKUP_DIR}/${COMPRESSED_FILE}"
    
    # Remover arquivo original não compactado
    rm ${BACKUP_FILE}
else
    echo "⚠️  Alerta: Falha na compactação, mantendo arquivo original!"
fi

echo ""
echo "3. Verificando integridade do backup..."

# Verificar se o arquivo existe e tem conteúdo
if [ -f "${BACKUP_DIR}/${COMPRESSED_FILE}" ] && [ -s "${BACKUP_DIR}/${COMPRESSED_FILE}" ]; then
    BACKUP_SIZE=$(du -h "${BACKUP_DIR}/${COMPRESSED_FILE}" | cut -f1)
    echo "✅ Backup validado!"
    echo "📊 Tamanho do backup: ${BACKUP_SIZE}"
else
    echo "❌ ERRO: Arquivo de backup inválido ou corrompido!"
    exit 1
fi

echo ""
echo "4. Limpando backups antigos (manter últimos 7 dias)..."

# Remover backups mais antigos que 7 dias
find ${BACKUP_DIR} -name "fb_farol_full_backup_*.tar.gz" -mtime +7 -delete

echo "✅ Limpeza concluída!"

echo ""
echo "5. Gerando arquivo de checksum..."

# Gerar checksum para validação
cd ${BACKUP_DIR}
sha256sum ${COMPRESSED_FILE} > ${COMPRESSED_FILE}.sha256

echo "✅ Checksum gerado: ${COMPRESSED_FILE}.sha256"

echo ""
echo "==================================================="
echo "BACKUP CONCLUÍDO COM SUCESSO!"
echo "Arquivo: ${BACKUP_DIR}/${COMPRESSED_FILE}"
echo "Checksum: ${BACKUP_DIR}/${COMPRESSED_FILE}.sha256"
echo "Data/Hora: $(date)"
echo "==================================================="

# Listar backups disponíveis
echo ""
echo "📋 Backups disponíveis em ${BACKUP_DIR}:"
ls -lh ${BACKUP_DIR}/fb_farol_full_backup_*.tar.gz