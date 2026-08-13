#!/usr/bin/env bash
# diagnostico_indices.sh — fotografia dos índices do FAROL em produção.
#
# SOMENTE LEITURA. Não cria, não dropa, não altera nada.
#
# ⚠ NÃO DROPE ÍNDICE COM BASE NESTE SCRIPT ENQUANTO O SISTEMA NÃO ESTIVER EM
#   USO REAL. Rodado em 13/08/2026 ele apontou 166 índices com idx_scan=0 e
#   4,7 GB "reclamáveis" — número sem valor naquele momento, porque o FAROL
#   ainda não tinha entrado em produção: os RCAs em campo (/m/CNPJ/RCA/cod)
#   nunca haviam acessado, ninguém tinha feito drill até Cliente/Produto, e
#   os filtros combinados que os gestores realmente usam eram desconhecidos.
#   idx_scan=0 ali significava "ninguém exercitou este caminho ainda", não
#   "este índice é inútil".
#
#   O erro é assimétrico: dropar leva segundos, recriar é CREATE INDEX em
#   24M linhas com o sistema no ar.
#
#   Para que o item 2 passe a valer:
#     1. Espere o sistema estar em uso real (gestores + RCAs em campo).
#     2. Zere o marco:  SELECT pg_stat_reset();
#        (zera também n_dead_tup — faça logo após um VACUUM, quando ele já
#         está perto de zero, para não atrasar o autovacuum.)
#     3. Deixe rodar 2-4 semanas cobrindo fechamento de mês, que é quando
#        aparecem as consultas mais pesadas.
#     4. Só então rode este script de novo e considere drops.
#
# Uso:  bash scripts/diagnostico_indices.sh
set -uo pipefail

source /root/farol-env.sh 2>/dev/null || {
  DB=${DB:-db-a0wcggw4wo040gwwwokckgk8-201557598604}
}
PSQL="docker exec -i $DB psql -U postgres -d fb_farol"

echo "════════════════════════════════════════════════════════════════════"
echo " 1. ÍNDICES DE FILIAL — existem?"
echo "════════════════════════════════════════════════════════════════════"
echo "A mig 196 dropou idx_v[ft]_filial; a 199 pediu para recriar À MÃO."
echo "Se vier VAZIO, o fallback de Filial (2+ filiais, ou nível sem agg)"
echo "está varrendo 24M linhas sem índice — foi o que deu 17,9s em 08/08."
echo
$PSQL -c "
SELECT indexname, tablename, pg_size_pretty(pg_relation_size(indexname::regclass)) AS tamanho
  FROM pg_indexes
 WHERE tablename IN ('vendas_faturadas','vendas_transmitidas')
   AND indexdef LIKE '%empresa,%'
 ORDER BY tablename;"

echo
echo "════════════════════════════════════════════════════════════════════"
echo " 2. ÍNDICES NUNCA USADOS nas duas tabelas grandes (18GB + 15GB)"
echo "════════════════════════════════════════════════════════════════════"
echo "idx_scan = 0 significa que o planner NUNCA escolheu esse índice desde"
echo "o último reset de estatísticas. Cada um deles custa espaço em disco E"
echo "torna todo COPY da carga mais lento (todo INSERT atualiza todo índice)."
echo
echo "Suspeita principal: idx_v[ft]_mixtotal_* (migs 171/172/173) foram"
echo "criados para queryMixTotal, função REMOVIDA do código."
echo
$PSQL -c "
SELECT relname AS tabela, indexrelname AS indice, idx_scan AS vezes_usado,
       pg_size_pretty(pg_relation_size(indexrelid)) AS tamanho
  FROM pg_stat_user_indexes
 WHERE relname IN ('vendas_faturadas','vendas_transmitidas')
 ORDER BY idx_scan, pg_relation_size(indexrelid) DESC;"

echo
echo "════════════════════════════════════════════════════════════════════"
echo " 3. Desde QUANDO essas estatísticas acumulam?"
echo "════════════════════════════════════════════════════════════════════"
echo "Sem isso, idx_scan=0 é ambíguo: pode ser índice órfão OU estatística"
echo "recém-zerada. Só confie no item 2 se a janela cobrir dias de uso real."
echo
$PSQL -c "SELECT stats_reset, now() - stats_reset AS janela FROM pg_stat_database WHERE datname='fb_farol';"

echo
echo "════════════════════════════════════════════════════════════════════"
echo " 4. Peso dos índices vs. dados (as 12 maiores tabelas)"
echo "════════════════════════════════════════════════════════════════════"
$PSQL -c "
SELECT relname AS tabela,
       pg_size_pretty(pg_relation_size(relid))                          AS dados,
       pg_size_pretty(pg_indexes_size(relid))                           AS indices,
       round(100.0*pg_indexes_size(relid)/NULLIF(pg_relation_size(relid),0)) AS pct_idx
  FROM pg_catalog.pg_statio_user_tables
 ORDER BY pg_total_relation_size(relid) DESC
 LIMIT 12;"

echo
echo "════════════════════════════════════════════════════════════════════"
echo " 5. Materialized views ANTIGAS ainda existem?"
echo "════════════════════════════════════════════════════════════════════"
echo "mv_simples / mv_supervisor_periodo / mv_rca_forn_periodo não são"
echo "lidas nem refreshadas por nenhum código Go atual (refreshAllFarolViews"
echo "só toca mv_[fat|trans]_carteira_rca). Se existirem, são peso morto."
echo
$PSQL -c "
SELECT schemaname, matviewname,
       pg_size_pretty(pg_total_relation_size((schemaname||'.'||matviewname)::regclass)) AS tamanho
  FROM pg_matviews
 ORDER BY pg_total_relation_size((schemaname||'.'||matviewname)::regclass) DESC;"

echo
echo "════════════════════════════════════════════════════════════════════"
echo " 6. Total reclamável se dropar tudo que tem idx_scan = 0"
echo "════════════════════════════════════════════════════════════════════"
$PSQL -c "
SELECT count(*) AS qtd_indices_nunca_usados,
       pg_size_pretty(COALESCE(sum(pg_relation_size(indexrelid)),0)) AS espaco_total
  FROM pg_stat_user_indexes
 WHERE idx_scan = 0
   AND indexrelname NOT LIKE '%_pkey';"

echo
echo "── fim do diagnóstico (nada foi alterado) ──"
