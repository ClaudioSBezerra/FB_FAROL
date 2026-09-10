-- 233_fix_overload_ambiguo_v12_sazonalidade.sql
-- ════════════════════════════════════════════════════════════════════════════
-- BUG PRÉ-EXISTENTE (desde a migration 213, não introduzido nesta sessão):
-- 213 usou `CREATE OR REPLACE FUNCTION` para acrescentar o parâmetro opcional
-- `p_empresa TEXT DEFAULT NULL` a farol.upsert_aggs_mes_v12 e
-- farol.upsert_sazonalidade_produto_ano — mas `CREATE OR REPLACE` só substitui
-- quando a ASSINATURA bate exatamente. Como o número de parâmetros mudou, o
-- Postgres criou um OVERLOAD NOVO em vez de substituir o antigo. Resultado:
-- as duas versões (3 args / 4 args-com-default; e 2 args / 3 args-com-default)
-- continuam existindo lado a lado, e uma chamada com 3 (ou 2) argumentos passa
-- a bater em AMBAS — o Postgres recusa com "function is not unique" (42725).
--
-- Descoberto em 10/09/2026 durante o backfill de 2026 pós-particionamento: a
-- consolidação diária normal (farol_v2_api.go, upsertAggsMesParallel) já
-- chamava essas funções com 3/2 argumentos, então este erro provavelmente já
-- vinha acontecendo TODO DIA desde que a 213 foi deployada — só nunca travou
-- nada porque o erro é apenas logado (upsertAggsMesParallel não propaga
-- falha de UM agregado como falha do import inteiro). V12 (mix de produto
-- por filial) e a Sazonalidade por produto/ano provavelmente estavam vazios
-- ou desatualizados em produção há algum tempo.
--
-- Fix: remove as versões de 3/2 argumentos (sem p_empresa) — são um subconjunto
-- estrito das versões de 4/3 argumentos da 213 (mesmo corpo, mesmo resultado
-- quando p_empresa é omitido/NULL). Todo call site do repo já usa só 3, 4, 2
-- ou 3 argumentos respectivamente — nenhum deles quebra ao sobrar só a versão
-- com DEFAULT.
-- ════════════════════════════════════════════════════════════════════════════

DROP FUNCTION IF EXISTS farol.upsert_aggs_mes_v12(uuid, integer, integer);
DROP FUNCTION IF EXISTS farol.upsert_sazonalidade_produto_ano(uuid, integer);
