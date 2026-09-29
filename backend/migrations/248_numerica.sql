-- Migration 248: Numérica — Cobertura e Sortimento por Cliente/CNPJ
-- (Épico 7 addendum, ver _bmad-output/planning-artifacts/epics.md, Story 7.1-7.3)
--
-- Numérica é um 2º par de Tipo de Métrica do Programa Único, calculado por
-- Cliente/CNPJ direto (nivel_agregacao='cliente', já suportado desde a
-- migration 214 — prova real do "teste de generalidade" do FR1/Story 1.1).
-- Vinculado às MESMAS Indústrias FOOD/HC já existentes (migration 210), não
-- cria Indústria nova (decisão confirmada com o usuário 2026-09-29).
--
-- 3 coisas novas que a Numérica precisa e Cobertura/Sortimento por Rede não
-- tinham:
--
-- 1. janela_apuracao — bimestre móvel (mês corrente + mês anterior) em vez
--    de mês fechado. É característica do TIPO DE MÉTRICA (FR14a), não do
--    vínculo — por isso a coluna vai em tipos_metrica, não em
--    metas_vinculos/metas_vigencias.
--
-- 2. Clientes Numéricas — formato de lista diferente de Clientes Válidos
--    (migration 220): CNPJ individual com Classificação PDV (A/B/C), SEM
--    rede_nome (não existe Rede na Numérica). GGV/CRV/RCA vêm PRONTOS do
--    arquivo da JC (denormalizados), não resolvidos por JOIN com a
--    hierarquia organizacional do Farol como Clientes Válidos faz — o
--    RCA/CRV/GGV do lado da JC pode não ter correspondência 1:1 com
--    managers/users do Farol. Tabela própria em vez de reaproveitar
--    metas_clientes_validos (que exige rede_nome/cod_rca NOT NULL e não
--    tem coluna de Classificação).
--
-- 3. PPAs — agrupamento de produto (cod_prod → PPA → EAN) diferente da
--    lista de Itens Válidos (migration 221, que é só EAN↔cod_prod sem nome
--    de família). Tabela própria — Sortimento por Rede continua usando
--    Itens Válidos como está, sem migração de dado entre as duas.
--    regiao/ae/bu são NULLABLE e não usados pelo motor ainda — colunas
--    informativas capturadas do arquivo-fonte, aguardando resposta da JC
--    (Questão em aberto #4 do PRD) sobre se entram no cálculo.

ALTER TABLE farol.tipos_metrica
    ADD COLUMN IF NOT EXISTS janela_apuracao TEXT NOT NULL DEFAULT 'mes_fechado';

ALTER TABLE farol.tipos_metrica
    DROP CONSTRAINT IF EXISTS ck_farol_tipos_metrica_janela_apuracao;
ALTER TABLE farol.tipos_metrica
    ADD CONSTRAINT ck_farol_tipos_metrica_janela_apuracao
        CHECK (janela_apuracao IN ('mes_fechado', 'bimestre_movel'));

CREATE TABLE IF NOT EXISTS farol.metas_clientes_numericas (
    id                SERIAL       PRIMARY KEY,
    empresa_id        UUID         NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    vinculo_id        INTEGER      NOT NULL REFERENCES farol.metas_vinculos(id) ON DELETE CASCADE,
    vigencia_id       INTEGER      NOT NULL REFERENCES farol.metas_vigencias(id) ON DELETE CASCADE,
    cnpj              VARCHAR(14)  NOT NULL,
    cod_cl            TEXT         NOT NULL DEFAULT '',
    classificacao_pdv TEXT         NOT NULL,
    razao             TEXT         NOT NULL DEFAULT '',
    fantasia          TEXT         NOT NULL DEFAULT '',
    cod_ggv           TEXT         NOT NULL DEFAULT '',
    nome_ggv          TEXT         NOT NULL DEFAULT '',
    cod_crv           TEXT         NOT NULL DEFAULT '',
    nome_crv          TEXT         NOT NULL DEFAULT '',
    cod_rca           TEXT         NOT NULL DEFAULT '',
    nome_rca          TEXT         NOT NULL DEFAULT '',
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT now(),

    CONSTRAINT uq_farol_metas_clientes_numericas_vigencia_cnpj UNIQUE (vigencia_id, cnpj),
    CONSTRAINT ck_farol_metas_clientes_numericas_classificacao
        CHECK (classificacao_pdv IN ('Num. A', 'Num. B', 'Num. C'))
);

CREATE INDEX IF NOT EXISTS idx_farol_metas_clientes_numericas_vigencia ON farol.metas_clientes_numericas (vigencia_id);
CREATE INDEX IF NOT EXISTS idx_farol_metas_clientes_numericas_classificacao ON farol.metas_clientes_numericas (vigencia_id, classificacao_pdv);

CREATE TABLE IF NOT EXISTS farol.metas_ppas (
    id           SERIAL       PRIMARY KEY,
    empresa_id   UUID         NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    vinculo_id   INTEGER      NOT NULL REFERENCES farol.metas_vinculos(id) ON DELETE CASCADE,
    vigencia_id  INTEGER      NOT NULL REFERENCES farol.metas_vigencias(id) ON DELETE CASCADE,
    ppa_nome     TEXT         NOT NULL,
    cod_prod     TEXT         NOT NULL,
    ean          TEXT         NOT NULL DEFAULT '',
    embalagem    TEXT         NOT NULL DEFAULT '',
    regiao       TEXT         NOT NULL DEFAULT '',
    ae           TEXT         NOT NULL DEFAULT '',
    bu           TEXT         NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),

    CONSTRAINT uq_farol_metas_ppas_vigencia_codprod UNIQUE (vigencia_id, cod_prod)
);

CREATE INDEX IF NOT EXISTS idx_farol_metas_ppas_vigencia ON farol.metas_ppas (vigencia_id);
CREATE INDEX IF NOT EXISTS idx_farol_metas_ppas_nome ON farol.metas_ppas (vigencia_id, ppa_nome);

-- Seed dos 2 Tipos de Métrica — Story 7.1. Vínculo com Indústria FOOD/HC e
-- os parametros_valores reais (limiares por Classificação, teto de PPA)
-- ficam pra ser criados pelo admin via tela de Config (mesma tela genérica
-- de Story 2.1, data-driven por parametros_schema — sem código novo), igual
-- foi feito manualmente pra Cobertura/Sortimento por Rede.
DO $$
DECLARE
    r RECORD;
BEGIN
    FOR r IN SELECT id AS empresa_id FROM companies LOOP

        INSERT INTO farol.tipos_metrica (empresa_id, nome, descricao, nivel_agregacao, janela_apuracao, formula_codigo, parametros_schema)
        VALUES (
            r.empresa_id,
            'Cobertura Numérica',
            'Um Cliente/CNPJ é considerado coberto (positivado) quando compra, no bimestre móvel, valor igual ou maior que o limiar da sua Classificação PDV (Num. A/B/C). Exclui venda Tipo 5 (bonificada).',
            'cliente',
            'bimestre_movel',
            'cobertura_numerica',
            '[
                {"key":"limiar_num_a","label":"Limiar Classificação Num. A (R$)","type":"number"},
                {"key":"limiar_num_b","label":"Limiar Classificação Num. B (R$)","type":"number"},
                {"key":"limiar_num_c","label":"Limiar Classificação Num. C (R$)","type":"number"}
            ]'::jsonb
        )
        ON CONFLICT (empresa_id, nome) DO NOTHING;

        INSERT INTO farol.tipos_metrica (empresa_id, nome, descricao, nivel_agregacao, janela_apuracao, formula_codigo, parametros_schema)
        VALUES (
            r.empresa_id,
            'Sortimento Numérica (PPA)',
            'Só clientes Classificação A e B são apurados. Conta quantos PPAs (famílias de produto) distintos o cliente comprou no bimestre móvel, com teto configurável. Inclui venda Tipo 5 (bonificada) — diferente da Cobertura Numérica.',
            'cliente',
            'bimestre_movel',
            'sortimento_numerica_ppa',
            '[{"key":"teto_ppas","label":"Teto de PPAs distintos contados por cliente","type":"integer"}]'::jsonb
        )
        ON CONFLICT (empresa_id, nome) DO NOTHING;

    END LOOP;
END $$;
