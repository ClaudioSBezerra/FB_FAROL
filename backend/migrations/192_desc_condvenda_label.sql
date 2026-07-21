-- 192_desc_condvenda_label.sql
-- ════════════════════════════════════════════════════════════════════════════
-- DESC_CONDVENDA — descrição oficial do ERP para o tipo de venda (ex.:
-- "VENDA PADRAO", "BONIFICACAO SIMPLES", "TRANSFERENCIA:"). Passa a ser a fonte
-- da verdade do RÓTULO no dropdown "Tipo de Venda", cobrindo qualquer código
-- (inclusive novos) exatamente como o ION VENDAS exporta.
--
--   1. vendas_faturadas ADD desc_condvenda (import grava a partir desta versão).
--   2. upsert_tipo_venda_dims: label = MAX(desc_condvenda) do mês; se vazio
--      (CSV antigo sem a coluna), cai no farol.tipo_venda_label(código).
--
-- O código (CONDVENDA) segue em vendas_faturadas.tipo_venda — a classificação
-- da venda líquida (venda real vs bonif/transf/remessa) usa o CÓDIGO, não muda.
-- ════════════════════════════════════════════════════════════════════════════

ALTER TABLE vendas_faturadas
    ADD COLUMN IF NOT EXISTS desc_condvenda TEXT NOT NULL DEFAULT '';

CREATE OR REPLACE FUNCTION farol.upsert_tipo_venda_dims(
    p_empresa_id UUID,
    p_ano        INT,
    p_mes        INT
) RETURNS VOID AS $$
DECLARE
    p_ini DATE := make_date(p_ano, p_mes, 1);
    p_fim DATE := (p_ini + INTERVAL '1 month' - INTERVAL '1 day')::date;
BEGIN
    INSERT INTO farol.agg_fat_dims_mes AS t (empresa_id, ano, mes, dim, key, label)
    SELECT p_empresa_id, p_ano, p_mes, 'tipo_venda', v.tipo_venda,
           COALESCE(NULLIF(MAX(v.desc_condvenda), ''), farol.tipo_venda_label(v.tipo_venda))
      FROM vendas_faturadas v
     WHERE v.empresa_id = p_empresa_id
       AND v.data_faturamento BETWEEN p_ini AND p_fim
       AND v.tipo_venda <> ''
     GROUP BY v.tipo_venda
    ON CONFLICT (ano, empresa_id, mes, dim, key) DO UPDATE SET label = EXCLUDED.label;
END;
$$ LANGUAGE plpgsql;
