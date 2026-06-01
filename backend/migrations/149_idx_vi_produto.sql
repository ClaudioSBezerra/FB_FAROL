-- 149_idx_vi_produto.sql
-- Índice em vendas_importadas por produto — suporte às queries de detalhe de produto
-- do Painel Marketing (compradores e oportunidades por cod_prod).

CREATE INDEX IF NOT EXISTS idx_vi_emp_produto
    ON vendas_importadas (empresa_id, tipo_base, ano, mes, cod_prod)
    WHERE cod_prod != '';

ANALYZE vendas_importadas;
