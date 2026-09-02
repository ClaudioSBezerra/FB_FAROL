-- Migration 218: Recorte organizacional do vínculo — Épico 2, Story 2.3
--
-- FR5: a configuração de meta precisa suportar recorte PARCIAL da operação
-- (ex: o programa Unilever é restrito a UF=GO + GGVs específicos), não só
-- "empresa inteira". ALTER na tabela existente (não tabela nova) — o
-- recorte é um atributo do vínculo, não uma entidade própria.
--
-- recorte_uf NULL ou recorte_ggvs '{}' (vazio) = sem restrição naquele eixo
-- (empresa toda). Os dois são independentes: pode restringir só por UF, só
-- por GGV, os dois juntos, ou nenhum.

ALTER TABLE farol.metas_vinculos
    ADD COLUMN IF NOT EXISTS recorte_uf TEXT,
    ADD COLUMN IF NOT EXISTS recorte_ggvs TEXT[] NOT NULL DEFAULT '{}';
