-- 168_add_visual_fields.sql
-- Adiciona 6 campos puramente visuais em vendas_faturadas/transmitidas:
--   Cliente:  cod_ramo, ramo
--   Produto:  embalagem, qt_unit, qt_unit_cx, cod_bar
--
-- Decisão do gerente: apenas exibir nos detalhes de cliente/produto, sem
-- entrar em agg_*_mes nem em totalizadores. Por isso NÃO mexemos em
-- upsert_aggs_mes — esses campos ficam só na tabela base, lidos via JOIN
-- nos handlers de detalhe quando o usuário abre um cliente ou produto.

ALTER TABLE vendas_faturadas
    ADD COLUMN IF NOT EXISTS cod_ramo    TEXT          NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS ramo        TEXT          NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS embalagem   TEXT          NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS qt_unit     NUMERIC(15,3) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS qt_unit_cx  NUMERIC(15,3) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS cod_bar     TEXT          NOT NULL DEFAULT '';

ALTER TABLE vendas_transmitidas
    ADD COLUMN IF NOT EXISTS cod_ramo    TEXT          NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS ramo        TEXT          NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS embalagem   TEXT          NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS qt_unit     NUMERIC(15,3) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS qt_unit_cx  NUMERIC(15,3) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS cod_bar     TEXT          NOT NULL DEFAULT '';
