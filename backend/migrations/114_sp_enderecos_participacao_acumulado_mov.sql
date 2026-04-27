-- Migration 114: novos campos do layout WMS — PARTICIPACAO, ACUMULADO, QT_MOV_PICKING_90
--
-- Três colunas adicionadas ao export do WMS (Calibragem_WMS_v3):
--   PARTICIPACAO     → % de participação do produto nas vendas totais (base do cálculo ABC)
--   ACUMULADO        → % participação acumulada (define corte de curva A/B/C)
--   QT_MOV_PICKING_90 → total de peças/caixas movimentadas no picking em 90 dias
--                       (diferente de QTACESSO_PICKING_PERIODO_90, que conta acessos ao endereço)

ALTER TABLE smartpick.sp_enderecos
    ADD COLUMN IF NOT EXISTS participacao      NUMERIC(8,4),   -- PARTICIPACAO  (0–100, ex: 15.2345)
    ADD COLUMN IF NOT EXISTS acumulado         NUMERIC(8,4),   -- ACUMULADO     (0–100, ex: 72.8901)
    ADD COLUMN IF NOT EXISTS qt_mov_picking_90 INTEGER;        -- QT_MOV_PICKING_90
