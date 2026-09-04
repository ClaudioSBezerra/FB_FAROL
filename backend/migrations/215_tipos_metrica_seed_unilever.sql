-- Migration 215: seed dos Tipos de Métrica de referência Unilever
--
-- Story 1.2 do Épico 1 (Painel de Gestão de Metas por Indústria, ver
-- _bmad-output/planning-artifacts/epics.md). "Cobertura por Rede" e
-- "Sortimento por Rede" são os dois Tipos de Métrica do caso de referência
-- (FR2) — pré-cadastrados pra não exigir digitação manual antes de
-- configurar o vínculo Unilever (Épico 2).
--
-- A "lista de itens válidos" e a lista de Clientes/Redes válidas do FR2 NÃO
-- entram aqui — são dado mensal importado via Épico 3 (FR11/FR12), não
-- parâmetro do Tipo de Métrica. O parametros_schema de Sortimento só carrega
-- a regra numérica de cálculo (quantidade mínima pra positivar).
--
-- ON CONFLICT (empresa_id, nome) DO NOTHING: reexecução ou tipo já
-- cadastrado manualmente não duplica nem sobrescreve — mesmo padrão da
-- migration 210 (seed de Indústrias).

DO $$
DECLARE
    r RECORD;
BEGIN
    FOR r IN SELECT id AS empresa_id FROM companies LOOP

        INSERT INTO farol.tipos_metrica (empresa_id, nome, descricao, nivel_agregacao, parametros_schema)
        VALUES (
            r.empresa_id,
            'Cobertura por Rede',
            'Uma Rede é considerada coberta quando a média de compra entre suas lojas ultrapassa um limiar em R$ (limiar próprio por fornecedor).',
            'rede',
            '[{"key":"limiar_valor_medio","label":"Limiar de valor médio de compra por loja (R$)","type":"number"}]'::jsonb
        )
        ON CONFLICT (empresa_id, nome) DO NOTHING;

        INSERT INTO farol.tipos_metrica (empresa_id, nome, descricao, nivel_agregacao, parametros_schema)
        VALUES (
            r.empresa_id,
            'Sortimento por Rede',
            'Média de itens (EAN) distintos comprados entre as lojas da Rede, de uma lista de itens válida enviada mensalmente. Um EAN só conta como positivado numa loja com quantidade mínima de unidades vendidas (regra que não se aplica a itens vendidos em caixa/pacote/display).',
            'rede',
            '[{"key":"qtd_minima_positivacao","label":"Quantidade mínima de unidades pra um item contar como positivado","type":"integer"}]'::jsonb
        )
        ON CONFLICT (empresa_id, nome) DO NOTHING;

    END LOOP;
END $$;
