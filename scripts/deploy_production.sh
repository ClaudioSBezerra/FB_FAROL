#!/bin/bash

# SCRIPT DE DEPLOY AUTOMATIZADO PARA PRODUÇÃO
# FB_FAROL - Sistema de Reforma Tributária

set -e  # Exit on error

# CONFIGURAÇÕES
PROJECT_DIR="/opt/fb_farol"
BACKUP_DIR="/opt/fb_farol/backups"
LOG_FILE="/var/log/fb_farol_deploy.log"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

# Cores para output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Função de log
log() {
    echo -e "${BLUE}[$(date '+%Y-%m-%d %H:%M:%S')]${NC} $1" | tee -a ${LOG_FILE}
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1" | tee -a ${LOG_FILE}
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1" | tee -a ${LOG_FILE}
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1" | tee -a ${LOG_FILE}
}

# Verificar se está rodando como root
if [[ $EUID -ne 0 ]]; then
   log_error "Este script precisa ser executado como root!"
   exit 1
fi

log "=================================================="
log "DEPLOY FB_FAROL - AMBIENTE DE PRODUÇÃO"
log "Data/Hora: $(date)"
log "=================================================="

# FASE 1: PREPARAÇÃO
log ""
log "🔍 FASE 1: PREPARAÇÃO E BACKUP"

# 1.1 Verificar diretórios
log "1.1 Verificando estrutura de diretórios..."
mkdir -p ${BACKUP_DIR}
mkdir -p ${PROJECT_DIR}
mkdir -p /var/log

# 1.2 Backup completo
log "1.2 Executando backup completo do banco..."
cd ${PROJECT_DIR}
if [ -f "./scripts/backup_production.sh" ]; then
    chmod +x scripts/backup_production.sh
    ./scripts/backup_production.sh
    if [ $? -eq 0 ]; then
        log_success "Backup completo realizado com sucesso!"
    else
        log_error "Falha no backup! Abortando deploy."
        exit 1
    fi
else
    log_error "Script de backup não encontrado! Abortando deploy."
    exit 1
fi

# 1.3 Verificar variáveis de ambiente
log "1.3 Verificando variáveis de ambiente..."
if [ ! -f ".env.production" ]; then
    log_error "Arquivo .env.production não encontrado!"
    exit 1
fi

# FASE 2: BUILD
log ""
log "🔨 FASE 2: BUILD DA APLICAÇÃO"

# 2.1 Parar serviços atuais
log "2.1 Parando serviços atuais..."
docker-compose -f docker-compose.yml down 2>/dev/null || true
docker-compose -f docker-compose.prod.yml down 2>/dev/null || true

# 2.2 Limpar imagens antigas
log "2.2 Limpando imagens Docker antigas..."
docker system prune -f 2>/dev/null || true

# 2.3 Buildar imagens de produção
log "2.3 Buildando imagens de produção..."
docker-compose -f docker-compose.prod.yml build --no-cache --parallel
if [ $? -eq 0 ]; then
    log_success "Build concluído com sucesso!"
else
    log_error "Falha no build das imagens!"
    exit 1
fi

# FASE 3: DEPLOY
log ""
log "🚀 FASE 3: DEPLOY EM PRODUÇÃO"

# 3.1 Iniciar serviços de produção
log "3.1 Iniciando serviços de produção..."
docker-compose -f docker-compose.prod.yml up -d

# 3.2 Aguardar serviços iniciarem
log "3.2 Aguardando inicialização dos serviços..."
sleep 30

# 3.3 Verificar health checks
log "3.3 Verificando saúde dos serviços..."

# Verificar backend
BACKEND_HEALTH=""
for i in {1..10}; do
    if curl -f http://localhost:8081/api/health >/dev/null 2>&1; then
        BACKEND_HEALTH="OK"
        break
    fi
    sleep 10
    log "Tentativa $i/10: Aguardando backend..."
done

if [ "$BACKEND_HEALTH" = "OK" ]; then
    log_success "Backend está saudável!"
else
    log_error "Backend não está respondendo!"
    docker-compose -f docker-compose.prod.yml logs api --tail=50
    exit 1
fi

# Verificar banco
DB_HEALTH=""
for i in {1..5}; do
    if docker exec fb_farol-db-prod pg_isready -U ${DB_USER} -d ${DB_NAME} >/dev/null 2>&1; then
        DB_HEALTH="OK"
        break
    fi
    sleep 10
    log "Tentativa $i/5: Aguardando banco de dados..."
done

if [ "$DB_HEALTH" = "OK" ]; then
    log_success "Banco de dados está saudável!"
else
    log_error "Banco de dados não está respondendo!"
    docker-compose -f docker-compose.prod.yml logs db --tail=50
    exit 1
fi

# FASE 4: VERIFICAÇÃO
log ""
log "✅ FASE 4: VERIFICAÇÃO PÓS-DEPLOY"

# 4.1 Verificar materialized views
log "4.1 Verificando materialized views..."
VIEW_COUNT=$(docker exec fb_farol-db-prod psql -U ${DB_USER} -d ${DB_NAME} -t -c "SELECT COUNT(*) FROM pg_matviews WHERE schemaname = 'public';" 2>/dev/null || echo "0")
if [ "$VIEW_COUNT" -gt 0 ]; then
    log_success "Materialized views encontradas: $VIEW_COUNT"
else
    log_warning "Nenhuma materialized view encontrada. Verifique as migrations."
fi

# 4.2 Testar endpoints críticos
log "4.2 Testando endpoints críticos..."

# Health check
if curl -f http://localhost:8081/api/health >/dev/null 2>&1; then
    log_success "✅ Health check OK"
else
    log_error "❌ Health check falhou"
fi

# Auth endpoint (se disponível)
if curl -f http://localhost:8081/api/auth/me >/dev/null 2>&1; then
    log_success "✅ Auth endpoint OK"
else
    log_warning "⚠️ Auth endpoint falhou (pode ser normal sem token)"
fi

# 4.3 Verificar status dos containers
log "4.3 Status dos containers:"
docker-compose -f docker-compose.prod.yml ps

# FASE 5: LIMPEZA
log ""
log "🧹 FASE 5: LIMPEZA E OTIMIZAÇÃO"

# 5.1 Remover imagens não utilizadas
log "5.1 Removendo imagens Docker não utilizadas..."
docker image prune -f 2>/dev/null || true

# 5.2 Limpar logs antigos (manter últimos 7 dias)
log "5.2 Limpando logs antigos..."
find /var/log -name "fb_farol_deploy.log*" -mtime +7 -delete 2>/dev/null || true

log ""
log "=================================================="
log "🎉 DEPLOY CONCLUÍDO COM SUCESSO!"
log "Data/Hora: $(date)"
log ""
log "📊 Serviços em produção:"
docker-compose -f docker-compose.prod.yml ps
log ""
log "🌐 Acesse a aplicação em: https://fbtax.cloud"
log "📈 Monitoramento: http://servidor:3001 (Grafana)"
log "📊 Métricas: http://servidor:9090 (Prometheus)"
log ""
log "⚠️  IMPORTANTE: Monitore os logs nos próximos minutos!"
log "=================================================="

# Enviar notificação (se configurado)
if [ -n "${SLACK_WEBHOOK}" ]; then
    curl -X POST -H 'Content-type: application/json' \
        --data "{\"text\":\"🚀 FB_FAROL deployed to production successfully! $(date)\"}" \
        ${SLACK_WEBHOOK} 2>/dev/null || true
fi

exit 0