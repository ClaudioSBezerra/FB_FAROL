#!/bin/bash

# SCRIPT DE VERIFICAÇÃO DE MATERIALIZED VIEWS
# FB_FAROL - Sistema de Reforma Tributária

DB_NAME="fiscal_db"
DB_USER="postgres"

# Cores para output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${BLUE}=================================================="
echo "VERIFICAÇÃO DE MATERIALIZED VIEWS - FB_FAROL"
echo "Data/Hora: $(date)"
echo "==================================================${NC}"

# Lista de views críticas que devem existir
CRITICAL_VIEWS=(
    "mv_operacoes_simples"
    "mv_mercadorias"
)

echo ""
echo "🔍 Verificando materialized views existentes..."

# Verificar views existentes
psql -U ${DB_USER} -d ${DB_NAME} -c "
SELECT 
    schemaname,
    matviewname,
    pg_size_pretty(pg_total_relation_size(schemaname||'.'||matviewname)) as size,
    pg_stat_get_last_vacuum_time(schemaname||'.'||matviewname) as last_vacuum
FROM pg_matviews 
WHERE schemaname = 'public'
ORDER BY matviewname;
" 2>/dev/null

echo ""
echo "📊 Validando views críticas..."

for view in "${CRITICAL_VIEWS[@]}"; do
    exists=$(psql -U ${DB_USER} -d ${DB_NAME} -t -c "SELECT 1 FROM pg_matviews WHERE schemaname = 'public' AND matviewname = '$view';" 2>/dev/null)
    
    if [ "$exists" = "1" ]; then
        count=$(psql -U ${DB_USER} -d ${DB_NAME} -t -c "SELECT COUNT(*) FROM $view;" 2>/dev/null || echo "ERROR")
        size=$(psql -U ${DB_USER} -d ${DB_NAME} -t -c "SELECT pg_size_pretty(pg_total_relation_size('$view'));" 2>/dev/null || echo "ERROR")
        echo -e "${GREEN}✅ $view${NC} - Registros: $count, Tamanho: $size"
    else
        echo -e "${RED}❌ $view${NC} - NÃO ENCONTRADA!"
    fi
done

echo ""
echo "🔄 Testando refresh das views..."

for view in "${CRITICAL_VIEWS[@]}"; do
    exists=$(psql -U ${DB_USER} -d ${DB_NAME} -t -c "SELECT 1 FROM pg_matviews WHERE schemaname = 'public' AND matviewname = '$view';" 2>/dev/null)
    
    if [ "$exists" = "1" ]; then
        echo -n "🔄 Refresh $view... "
        start_time=$(date +%s)
        
        psql -U ${DB_USER} -d ${DB_NAME} -c "REFRESH MATERIALIZED VIEW CONCURRENTLY $view;" >/dev/null 2>&1
        
        if [ $? -eq 0 ]; then
            end_time=$(date +%s)
            duration=$((end_time - start_time))
            echo -e "${GREEN}OK (${duration}s)${NC}"
        else
            echo -e "${RED}FALHOU${NC}"
        fi
    fi
done

echo ""
echo "📈 Estatísticas do banco de dados..."

# Tamanho total das views
total_size=$(psql -U ${DB_USER} -d ${DB_NAME} -t -c "
SELECT pg_size_pretty(SUM(pg_total_relation_size(schemaname||'.'||matviewname))) 
FROM pg_matviews 
WHERE schemaname = 'public';
" 2>/dev/null)

echo "📊 Tamanho total das views: $total_size"

# Verificar última atualização
echo ""
echo "🕒 Últimas atualizações:"

psql -U ${DB_USER} -d ${DB_NAME} -c "
SELECT 
    matviewname,
    pg_stat_get_last_vacuum_time(schemaname||'.'||matviewname) as last_update
FROM pg_matviews 
WHERE schemaname = 'public' AND pg_stat_get_last_vacuum_time(schemaname||'.'||matviewname) IS NOT NULL
ORDER BY last_update DESC
LIMIT 10;
" 2>/dev/null

echo ""
echo "🚨 Verificando problemas potenciais..."

# Verificar views muito grandes (acima de 1GB)
large_views=$(psql -U ${DB_USER} -d ${DB_NAME} -t -c "
SELECT COUNT(*) 
FROM pg_matviews 
WHERE schemaname = 'public' 
AND pg_total_relation_size(schemaname||'.'||matviewname) > 1073741824;
" 2>/dev/null)

if [ "$large_views" -gt 0 ]; then
    echo -e "${YELLOW}⚠️  Atenção: $large_views views com mais de 1GB${NC}"
fi

# Verificar views não atualizadas recentemente (mais de 7 dias)
old_views=$(psql -U ${DB_USER} -d ${DB_NAME} -t -c "
SELECT COUNT(*) 
FROM pg_matviews 
WHERE schemaname = 'public' 
AND pg_stat_get_last_vacuum_time(schemaname||'.'||matviewname) < NOW() - INTERVAL '7 days';
" 2>/dev/null)

if [ "$old_views" -gt 0 ]; then
    echo -e "${YELLOW}⚠️  Atenção: $old_views views não atualizadas nos últimos 7 dias${NC}"
fi

echo ""
echo "=================================================="
echo -e "${GREEN}✅ VERIFICAÇÃO CONCLUÍDA!${NC}"
echo "Data/Hora: $(date)"
echo "=================================================="

# Recomendações
echo ""
echo "💡 RECOMENDAÇÕES:"
echo "1. Execute 'REFRESH MATERIALIZED VIEW CONCURRENTLY' regularmente"
echo "2. Monitore o tamanho das views para evitar crescimento excessivo"
echo "3. Agende refreshs em horários de baixo tráfego"
echo "4. Considere particionamento para tabelas muito grandes"
echo ""