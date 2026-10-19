-- 253_segmento_foco_ufs_ocultas.sql — "UF liberada?" do Segmento Foco (Reckitt).
--
-- UFs listadas aqui ficam FORA do painel Segmento Foco (realizado e objetivo):
-- Claudio, 19/10/2026 — o Pará (fornecedor 44957) ainda não tem venda na base,
-- então o bloco PA fica oculto até a carga subir. Liberar = tirar a UF da lista
-- (checkbox "Pará liberado" na tela Carga do mês). Só o Segmento Foco lê esta
-- coluna; o default '{PA}' vale pros vínculos novos desse programa.
ALTER TABLE farol.metas_vinculos
    ADD COLUMN IF NOT EXISTS ufs_ocultas TEXT[] NOT NULL DEFAULT '{PA}';
