-- 191_tipo_venda_label_normal.sql
-- ════════════════════════════════════════════════════════════════════════════
-- Ajuste de rótulo conforme a legenda oficial do ION VENDAS (jul/2026):
-- o CÓDIGO da coluna CONDVENDA "1" é "NORMAL" (não "Padrão").
-- DESC_CONDVENDA exemplos: 1: NORMAL, 5: BONIFICAÇÃO, 10: TRANSFERÊNCIA.
--
-- Só redefine farol.tipo_venda_label (mig 188) trocando 1 → 'Normal'. Os demais
-- rótulos seguem a lista de negócio já acordada. O label é gravado no dropdown
-- (agg_fat_dims_mes) na próxima importação (upsert_tipo_venda_dims).
-- ════════════════════════════════════════════════════════════════════════════

CREATE OR REPLACE FUNCTION farol.tipo_venda_label(p_cod TEXT) RETURNS TEXT AS $$
    SELECT CASE p_cod
        WHEN '1'  THEN 'Normal'
        WHEN '4'  THEN 'Simples Fatura'
        WHEN '5'  THEN 'Bonificação'
        WHEN '7'  THEN 'Entrega Futura'
        WHEN '8'  THEN 'Simples Entrega'
        WHEN '9'  THEN 'CFOP Específico'
        WHEN '10' THEN 'Transferência'
        WHEN '11' THEN 'Venda com Troca'
        WHEN '13' THEN 'Remessa Manifesto'
        WHEN '14' THEN 'Venda Manifesto'
        WHEN '20' THEN 'Consignada'
        ELSE 'Tipo ' || p_cod
    END;
$$ LANGUAGE sql IMMUTABLE;
