-- Migration 176 — Índices p/ "Clientes Ativos" = COUNT(DISTINCT cnpj) (provisório Heverton)
--
-- queryDistinctPositivados / queryDistinctCliPositivados agrupam a tabela FOLHA
-- (grão cnpj×mês) por groupCol contando cnpj distinto, filtrando positivados>0.
-- O caso pesado é o nível TOPO de cada view, SEM drill, no período todo
-- (varre a folha inteira). Índices parciais (empresa_id, <topgroup>, cnpj)
-- WHERE positivados>0 transformam isso num index scan já agrupado/ordenado por
-- (groupCol, cnpj), permitindo distinct em streaming — em vez de seq scan + hash.
--
-- Drill mais profundo (ex: dentro de 1 fornecedor) filtra o recorte → poucas
-- linhas → rápido mesmo sem índice dedicado.
--
-- CREATE INDEX na tabela particionada-pai propaga p/ todas as partições (ano).
-- IF NOT EXISTS = idempotente.

-- V01 (Por Indústria) — topo = cod_fornec
CREATE INDEX IF NOT EXISTS idx_aggfat_v01l4_distpos  ON farol.agg_fat_v01_l4_mes   (empresa_id, cod_fornec, cnpj)     WHERE positivados > 0;
CREATE INDEX IF NOT EXISTS idx_aggtra_v01l4_distpos  ON farol.agg_trans_v01_l4_mes (empresa_id, cod_fornec, cnpj)     WHERE positivados > 0;

-- V02 (Por Equipe) — topo = cod_supervisor
CREATE INDEX IF NOT EXISTS idx_aggfat_v02l3_distpos  ON farol.agg_fat_v02_l3_mes   (empresa_id, cod_supervisor, cnpj) WHERE positivados > 0;
CREATE INDEX IF NOT EXISTS idx_aggtra_v02l3_distpos  ON farol.agg_trans_v02_l3_mes (empresa_id, cod_supervisor, cnpj) WHERE positivados > 0;

-- V03 (Por Gerência) — topo = cod_gerente
CREATE INDEX IF NOT EXISTS idx_aggfat_v03l3_distpos  ON farol.agg_fat_v03_l3_mes   (empresa_id, cod_gerente, cnpj)    WHERE positivados > 0;
CREATE INDEX IF NOT EXISTS idx_aggtra_v03l3_distpos  ON farol.agg_trans_v03_l3_mes (empresa_id, cod_gerente, cnpj)    WHERE positivados > 0;

-- V05 (Por Fornecedor sob supervisor, mobile) — topo = cod_supervisor
CREATE INDEX IF NOT EXISTS idx_aggfat_v05l3_distpos  ON farol.agg_fat_v05_l3_mes   (empresa_id, cod_supervisor, cnpj) WHERE positivados > 0;
CREATE INDEX IF NOT EXISTS idx_aggtra_v05l3_distpos  ON farol.agg_trans_v05_l3_mes (empresa_id, cod_supervisor, cnpj) WHERE positivados > 0;
