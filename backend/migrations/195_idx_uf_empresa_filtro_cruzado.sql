-- 195_idx_uf_empresa_filtro_cruzado.sql
-- ════════════════════════════════════════════════════════════════════════════
-- ÍNDICES DE APOIO — filtro cruzado por UF e "Filial" (coluna `empresa`).
--
-- UF e "Filial" (rótulo da UI para a coluna `empresa`, texto livre do CSV do
-- ION VENDAS) nunca existem em nenhuma tabela agg_* — só em vendas_faturadas/
-- vendas_transmitidas. Por isso, filtrar por qualquer um dos dois SEMPRE cai
-- em queryAggregatedVendas (scan direto de vendas_*), nunca no caminho rápido
-- das agregações pré-computadas. Confirmado no código:
--   colsInAggTable (farol_v2_api.go): "uf/empresa NUNCA estão em tabela agg"
--   pickAggForCrossFilter: "Só falha (e cai para vendas_*) quando nenhuma agg
--     tem a coluna (ex: filtro por UF/Filial)."
--
-- Até aqui, esse scan não tinha NENHUM índice de apoio em uf/empresa — o
-- Postgres varria o range de data (via idx_v[ft]_emp_base_data) e filtrava
-- uf/empresa linha a linha. Mesmo problema que tipo_venda tinha antes da
-- mig 187, que criou um índice parcial dedicado para ele. Este arquivo aplica
-- o MESMO tratamento pra uf e empresa, nos dois fluxos — análise 27/07/2026
-- (usuário: "UF e Filiais serão muito utilizados").
--
-- Índice parcial (coluna <> '') porque o filtro só faz sentido quando há
-- valor — evita indexar as linhas com uf/empresa vazio (ruído, sem ganho).
-- ════════════════════════════════════════════════════════════════════════════

CREATE INDEX IF NOT EXISTS idx_vf_uf
    ON vendas_faturadas (empresa_id, data_faturamento, uf)
    WHERE uf <> '';

CREATE INDEX IF NOT EXISTS idx_vf_filial
    ON vendas_faturadas (empresa_id, data_faturamento, empresa)
    WHERE empresa <> '';

CREATE INDEX IF NOT EXISTS idx_vt_uf
    ON vendas_transmitidas (empresa_id, data_transmissao, uf)
    WHERE uf <> '';

CREATE INDEX IF NOT EXISTS idx_vt_filial
    ON vendas_transmitidas (empresa_id, data_transmissao, empresa)
    WHERE empresa <> '';
