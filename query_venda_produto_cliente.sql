-- ==========================================
-- VENDA PROUTO x CLIENTE - MÊS A MÊS
-- Produto: 467601 - SABAO PO OMO 2,2KG CX LAVAGEM PERFEITA
-- Cliente: 184264 - L A COMERCIO DE PRODUTOS DE SECOS E MOLHADOS LTDA
-- ==========================================

-- Primeiro, identifique o empresa_id correto:
SELECT id, nome FROM companies ORDER BY nome;

-- Substitua '<SEU_EMPRESA_ID>' abaixo pelo UUID da empresa


-- ==========================================
-- QUERY PRINCIPAL - VENDAS MÊS A MÊS 2025-2026
-- ==========================================
SELECT
    EXTRACT(YEAR FROM vf.data_faturamento)::INTEGER AS ano,
    EXTRACT(MONTH FROM vf.data_faturamento)::INTEGER AS mes,
    TO_CHAR(vf.data_faturamento, 'MM/YYYY') AS periodo,

    -- Dados do produto
    vf.cod_prod,
    vf.nome_prod,

    -- Dados do cliente
    vf.cod_cli,
    vf.nome_cli,
    vf.cnpj,

    -- Dados do fornecedor (indústria)
    vf.cod_fornec,
    vf.nome_fornec,

    -- Venda do período
    COUNT(*) AS qt_nfs,
    SUM(vf.qt) AS qt_total,
    SUM(vf.pvenda) AS valor_venda,
    SUM(vf.plucro) AS valor_lucro,

    -- Ticket médio
    CASE WHEN COUNT(*) > 0 THEN SUM(vf.pvenda) / COUNT(*) ELSE 0 END AS ticket_medio_nf,

    -- Preço médio unitário
    CASE WHEN SUM(vf.qt) > 0 THEN SUM(vf.pvenda) / SUM(vf.qt) ELSE 0 END AS preco_medio_unit

FROM public.vendas_faturadas vf
WHERE vf.empresa_id = '<SEU_EMPRESA_ID>'  -- <-- SUBSTITUIR PELO UUID DA EMPRESA
  AND vf.cod_prod = '467601'
  AND vf.cod_cli = '184264'
  AND vf.data_faturamento BETWEEN '2025-01-01' AND '2026-12-31'
GROUP BY
    EXTRACT(YEAR FROM vf.data_faturamento),
    EXTRACT(MONTH FROM vf.data_faturamento),
    vf.cod_prod,
    vf.nome_prod,
    vf.cod_cli,
    vf.nome_cli,
    vf.cnpj,
    vf.cod_fornec,
    vf.nome_fornec
ORDER BY ano DESC, mes DESC;


-- ==========================================
-- COMPARATIVO 2025 vs 2026 (LADO A LADO)
-- ==========================================
WITH vendas_2025 AS (
    SELECT
        EXTRACT(MONTH FROM vf.data_faturamento)::INTEGER AS mes,
        COALESCE(SUM(vf.pvenda), 0) AS valor_2025,
        COALESCE(SUM(vf.qt), 0) AS qt_2025,
        COUNT(*) AS nfs_2025
    FROM public.vendas_faturadas vf
    WHERE vf.empresa_id = '<SEU_EMPRESA_ID>'  -- <-- SUBSTITUIR
      AND vf.cod_prod = '467601'
      AND vf.cod_cli = '184264'
      AND EXTRACT(YEAR FROM vf.data_faturamento) = 2025
    GROUP BY EXTRACT(MONTH FROM vf.data_faturamento)
),
vendas_2026 AS (
    SELECT
        EXTRACT(MONTH FROM vf.data_faturamento)::INTEGER AS mes,
        COALESCE(SUM(vf.pvenda), 0) AS valor_2026,
        COALESCE(SUM(vf.qt), 0) AS qt_2026,
        COUNT(*) AS nfs_2026
    FROM public.vendas_faturadas vf
    WHERE vf.empresa_id = '<SEU_EMPRESA_ID>'  -- <-- SUBSTITUIR
      AND vf.cod_prod = '467601'
      AND vf.cod_cli = '184264'
      AND EXTRACT(YEAR FROM vf.data_faturamento) = 2026
    GROUP BY EXTRACT(MONTH FROM vf.data_faturamento)
)
SELECT
    COALESCE(v25.mes, v26.mes) AS mes,
    -- Valor de venda
    COALESCE(v25.valor_2025, 0) AS venda_2025,
    COALESCE(v26.valor_2026, 0) AS venda_2026,
    CASE
        WHEN COALESCE(v25.valor_2025, 0) > 0
        THEN ROUND(((COALESCE(v26.valor_2026, 0) - COALESCE(v25.valor_2025, 0)) / COALESCE(v25.valor_2025, 1)) * 100, 2)
        ELSE NULL
    END AS perc_variacao_valor,
    -- Quantidade
    COALESCE(v25.qt_2025, 0) AS qt_2025,
    COALESCE(v26.qt_2026, 0) AS qt_2026,
    CASE
        WHEN COALESCE(v25.qt_2025, 0) > 0
        THEN ROUND(((COALESCE(v26.qt_2026, 0) - COALESCE(v25.qt_2025, 0)) / COALESCE(v25.qt_2025, 1)) * 100, 2)
        ELSE NULL
    END AS perc_variacao_qt,
    -- Número de NFs
    COALESCE(v25.nfs_2025, 0) AS nfs_2025,
    COALESCE(v26.nfs_2026, 0) AS nfs_2026
FROM vendas_2025 v25
FULL OUTER JOIN vendas_2026 v26 ON v25.mes = v26.mes
ORDER BY COALESCE(v25.mes, v26.mes);


-- ==========================================
-- TOTALIZADORES GERAIS 2025-2026
-- ==========================================
SELECT
    EXTRACT(YEAR FROM vf.data_faturamento)::INTEGER AS ano,
    COUNT(DISTINCT DATE_TRUNC('month', vf.data_faturamento)) AS meses_com_venda,
    COUNT(DISTINCT vf.data_faturamento) AS dias_com_venda,
    COUNT(*) AS total_nfs,
    SUM(vf.qt) AS qt_total,
    SUM(vf.pvenda) AS valor_total,
    SUM(vf.plucro) AS lucro_total,
    CASE WHEN SUM(vf.pvenda) > 0 THEN ROUND((SUM(vf.plucro) / SUM(vf.pvenda)) * 100, 2) ELSE 0 END AS margem_pct
FROM public.vendas_faturadas vf
WHERE vf.empresa_id = '<SEU_EMPRESA_ID>'  -- <-- SUBSTITUIR
  AND vf.cod_prod = '467601'
  AND vf.cod_cli = '184264'
  AND EXTRACT(YEAR FROM vf.data_faturamento) IN (2025, 2026)
GROUP BY EXTRACT(YEAR FROM vf.data_faturamento)
ORDER BY ano DESC;


-- ==========================================
-- ÚLTIMAS COMPRAS DO CLIENTE (ÚLTIMAS 20 NFs)
-- ==========================================
SELECT
    vf.data_faturamento,
    vf.cod_fornec,
    vf.nome_fornec,
    vf.qt,
    vf.pvenda,
    vf.plucro,
    CASE WHEN vf.pvenda > 0 THEN ROUND((vf.plucro / vf.pvenda) * 100, 2) ELSE 0 END AS margem_pct
FROM public.vendas_faturadas vf
WHERE vf.empresa_id = '<SEU_EMPRESA_ID>'  -- <-- SUBSTITUIR
  AND vf.cod_prod = '467601'
  AND vf.cod_cli = '184264'
ORDER BY vf.data_faturamento DESC
LIMIT 20;
