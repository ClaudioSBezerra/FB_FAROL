-- 239_metas_itens_data_ultima_venda.sql
-- ════════════════════════════════════════════════════════════════════════════
-- Pedido do Claudio 22/09/2026: no drill-down de Itens (Sortimento), cada
-- PRODUTO precisa da sua própria data de última venda — não só o "Dt.Ult.Cmp"
-- do Cliente (fato único, mostrado 1x, ver resolverDataUltimaCompraClientes)
-- mas "quando ESTE produto específico foi vendido pela última vez pra este
-- cliente", útil sobretudo nos itens "Não coberto": mostra se o item já foi
-- comprado antes (só não neste período) ou nunca foi.
--
-- Grão empresa×fornecedor×rede×cliente×supv×rca×produto×data_competência (a
-- visão descrita pelo Claudio) — vigencia_id/cnpj/ean já são essa chave
-- (rede/supv/rca/fornecedor/data_competência vêm de JOIN com
-- metas_clientes_validos/metas_vigencias/metas_vinculos/industria_fornecedores
-- na leitura, não precisam duplicar aqui).
ALTER TABLE farol.metas_itens_realizado
    ADD COLUMN data_ultima_venda DATE;

COMMENT ON COLUMN farol.metas_itens_realizado.data_ultima_venda IS
    'Última venda deste EAN/grupo pra este CNPJ, até o fim da vigência (sem vazar período futuro) — NULL se nunca vendido.';
