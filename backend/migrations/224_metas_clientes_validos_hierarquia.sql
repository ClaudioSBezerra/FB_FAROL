-- Migration 224: Clientes Válidos — hierarquia completa importada (GGV/CRV/RCA)
--
-- Ajuste pós-orientação direta do Heverton (2026-09-04) e confronto com o
-- modelo real da JC (aba "BASE DE LOJAS" do arquivo "Unico Acompanhamento
-- Ponderadas Unilever HC_V1.xlsx", enviado por scp em 2026-09-04): a lista
-- mensal de Clientes Válidos não traz só "rede_nome" livre e "cod_rca" —
-- ela traz o COD PRINC (chave real da Rede no ERP — Claudio confirmou
-- "Rede é COD_PRINC, inclusive podem ter clientes que são redes apontando
-- para ele mesmo", ou seja, uma Rede pode ter 1 CNPJ só) e o trio
-- GGV/CRV/RCA (código + nome) já resolvido por CNPJ.
--
-- Isso reverte a decisão original da Story 3.2 de resolver CRV/GGV via JOIN
-- na hierarquia denormalizada das linhas de venda: o parágrafo novo do
-- descritivo Unilever ("a apuração precisa ser referente ao total vendido
-- ao cliente, independente de quem vendeu — GGV, CRV e RCA") deixou claro
-- que o "dono" do cliente para fins de rollup GGV/CRV/RCA é o da planilha
-- mensal do fornecedor, não quem efetivamente vendeu (podem divergir).
--
-- rede_nome é renomeado para cod_princ (mesma coluna — RENAME preserva
-- índices/FKs — só o nome fica mais preciso: não é mais "nome livre de
-- rede", é o código do ERP usado como chave).

ALTER TABLE farol.metas_clientes_validos RENAME COLUMN rede_nome TO cod_princ;

ALTER TABLE farol.metas_clientes_validos
    ADD COLUMN IF NOT EXISTS razao      TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS fantasia   TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS cod_ggv    TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS nome_ggv   TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS cod_crv    TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS nome_crv   TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS nome_rca   TEXT NOT NULL DEFAULT '';

-- Os índices antigos (idx_farol_metas_clientes_validos_rede, criado sobre
-- (vigencia_id, rede_nome)) seguem a coluna renomeada automaticamente no
-- Postgres — não precisam ser recriados, só ficaram com nome desatualizado
-- (cosmético, não afeta funcionamento).
