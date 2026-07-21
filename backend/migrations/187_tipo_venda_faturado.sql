-- 187_tipo_venda_faturado.sql
-- ════════════════════════════════════════════════════════════════════════════
-- TIPO_VENDA (jul/2026) — nova coluna do CSV do ION VENDAS.
--   11 códigos: 1=Padrão, 4=Simples Fatura, 5=Bonificação, 7=Entrega Futura,
--   8=Simples Entrega, 9=CFOP específico, 10=Transferência, 11=Venda com Troca,
--   13=Remessa Manifesto, 14=Venda Manifesto, 20=Consignada.
--
-- OBJETIVO: separar faturamento efetivo de bonificação/transferência/remessa no
-- FLUXO FATURADO. O gestor filtra por "Tipo de Venda" nas abas de vendas
-- faturadas; cards/KPIs recalculam.
--
-- ────────────────────────────────────────────────────────────────────────────
-- DECISÃO DE ARQUITETURA (renegociada 2026-07-21 — ver Spec Change Log):
--   tipo_venda NÃO entra na PK das tabelas agg_fat_*. Vira um FILTRO CRUZADO:
--   a API já roteia qualquer filtro cuja coluna não exista nas agg (hoje
--   uf/empresa) para queryAggregatedVendas, que agrega DIRETO de
--   vendas_faturadas e calcula TODOS os indicadores corretamente.
--
--   Por quê: a API lê as agg com SUM(pvenda) mas AVG(base_cli/positivados/mix).
--   Se tipo_venda entrasse no grão das agg, a visão SEM filtro (GROUP BY sem
--   tipo_venda) faria AVG de positivados/mix por tipo → valores ERRADOS em dois
--   indicadores do Core Value. O caminho cruzado evita isso e é ~40x menos
--   código (sem recriar 52 tabelas nem reescrever o upsert de 920 linhas).
--
-- ESCOPO DESTA MIGRATION:
--   1. vendas_faturadas → ADD COLUMN tipo_venda (a base que o import popula e
--      que o filtro cruzado lê).
--   2. Índice de apoio ao scan do filtro cruzado (empresa+data+tipo_venda).
--
-- As tabelas agg_* NÃO mudam de schema. A população do dropdown do filtro
-- (dim='tipo_venda' em agg_fat_dims_mes) fica na migration 188.
-- ════════════════════════════════════════════════════════════════════════════

ALTER TABLE vendas_faturadas
    ADD COLUMN IF NOT EXISTS tipo_venda TEXT NOT NULL DEFAULT '';

-- Apoia o filtro cruzado: queryAggregatedVendas escaneia vendas_faturadas por
-- empresa_id + data_faturamento (range) e aplica `tipo_venda = ANY(...)`.
-- Índice parcial (tipo_venda<>'') porque o filtro só faz sentido quando há tipo.
CREATE INDEX IF NOT EXISTS idx_vf_tipo_venda
    ON vendas_faturadas (empresa_id, data_faturamento, tipo_venda)
    WHERE tipo_venda <> '';
