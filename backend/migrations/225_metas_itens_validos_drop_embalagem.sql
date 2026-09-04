-- Migration 225: remove tipo_embalagem de metas_itens_validos
--
-- Ajuste pós-orientação direta do Heverton (2026-09-04): a regra de
-- quantidade mínima do Sortimento ("regra das 3 unidades", FR12) não devia
-- ter sido modelada como campo importado pela JC no CSV mensal de Itens
-- Válidos — essa informação já existe no cadastro de produto, importado
-- todo dia na carga diária de vendas (colunas embalagem/qt_unit_cx,
-- migration 168_add_visual_fields.sql). Pedir a mesma informação de novo,
-- todo mês, num arquivo à parte, duplicava uma fonte que o Farol já tem —
-- e sujeitava o cálculo a divergir dela.
--
-- tipo_embalagem sai da tabela; o motor de apuração
-- (farol_metas_calculo.go) passa a resolver "este item vende em UN?" via
-- JOIN com vendas_faturadas/vendas_transmitidas por cod_prod.

ALTER TABLE farol.metas_itens_validos DROP COLUMN IF EXISTS tipo_embalagem;
