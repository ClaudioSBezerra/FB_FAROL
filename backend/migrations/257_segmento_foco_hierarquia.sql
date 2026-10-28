-- 257_segmento_foco_hierarquia.sql — RCA → GGV/CRV/UF do Segmento Foco (Reckitt).
--
-- Igor/Reckitt (28/10/2026): a UF do programa é a do GGV que atua no estado
-- (GGV 3 e 4 = GO, 2 = TO, 347 = MS, 5 = PA) e todas as equipes abaixo dele
-- entram nessa UF — não vale a UF do cadastro do cliente. Vem da planilha
-- HIERARQUIA RCA, importada pela Carga do mês. Sem linhas, o painel mantém a
-- regra antiga (UF da matriz).
CREATE TABLE IF NOT EXISTS farol.metas_rca_hierarquia (
    empresa_id UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    vinculo_id INT  NOT NULL REFERENCES farol.metas_vinculos(id) ON DELETE CASCADE,
    cod_rca    TEXT NOT NULL,
    cod_ggv    TEXT NOT NULL DEFAULT '',
    nome_ggv   TEXT NOT NULL DEFAULT '',
    cod_crv    TEXT NOT NULL DEFAULT '',
    nome_crv   TEXT NOT NULL DEFAULT '',
    uf         TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (empresa_id, vinculo_id, cod_rca)
);
