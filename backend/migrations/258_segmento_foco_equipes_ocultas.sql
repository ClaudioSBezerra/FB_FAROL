-- 258_segmento_foco_equipes_ocultas.sql — "Equipe entra no painel?" do Segmento Foco (Reckitt).
--
-- Equipes (CRV) listadas aqui ficam FORA do painel Segmento Foco (realizado e
-- objetivo), em qualquer nível. Pedido do Igor/Reckitt (17/11/2026): tirar a
-- equipe 92. Parâmetro por vínculo, em vez de editar a planilha de hierarquia —
-- reimportar a hierarquia não desfaz a escolha. Mesmo padrão de ufs_ocultas (253):
-- o filtro vale na leitura, sem recalcular nada. Só o Segmento Foco lê esta coluna.
ALTER TABLE farol.metas_vinculos
    ADD COLUMN IF NOT EXISTS equipes_ocultas TEXT[] NOT NULL DEFAULT '{}';
