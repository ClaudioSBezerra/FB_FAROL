-- 252_segmento_foco.sql — Programa "Segmento Foco" (Reckitt, 19/10/2026).
--
-- Positivação de GRUPOS de produtos por cliente, mensal e trimestral (o
-- trimestre é a soma dos 3 meses). Um cliente positiva um grupo no mês quando
-- compra o mínimo de 3 unidades de qualquer produto do grupo (caixa conta
-- pelas unidades — mesma regra de embalagem da Numérica/PPA). O realizado é a
-- quantidade de (cliente × grupo) positivados; o objetivo é por UF × grupo ×
-- mês. Painel novo (/farol/segmento-foco), com carga mensal própria na tela
-- "Carga do mês".
--
-- Modelo:
--   tipos_metrica 'positivacao_grupos' (nível cliente, janela mês fechado);
--   metas_grupos_produtos  — lista grupo → cod_prod, por vigência (a JC manda
--                            a lista de SKUs válidos a cada trimestre);
--   metas_grupos_objetivos — objetivo por UF × grupo, por vigência (mês).

CREATE TABLE IF NOT EXISTS farol.metas_grupos_produtos (
    id          SERIAL      PRIMARY KEY,
    empresa_id  UUID        NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    vinculo_id  INTEGER     NOT NULL REFERENCES farol.metas_vinculos(id) ON DELETE CASCADE,
    vigencia_id INTEGER     NOT NULL REFERENCES farol.metas_vigencias(id) ON DELETE CASCADE,
    grupo       TEXT        NOT NULL,
    cod_prod    TEXT        NOT NULL,
    produto     TEXT        NOT NULL DEFAULT '',
    ean         TEXT        NOT NULL DEFAULT '',
    embalagem   TEXT        NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_farol_metas_grupos_produtos UNIQUE (vigencia_id, cod_prod)
);
CREATE INDEX IF NOT EXISTS idx_farol_metas_grupos_produtos_vig ON farol.metas_grupos_produtos (vigencia_id, grupo);

CREATE TABLE IF NOT EXISTS farol.metas_grupos_objetivos (
    id          SERIAL        PRIMARY KEY,
    empresa_id  UUID          NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    vinculo_id  INTEGER       NOT NULL REFERENCES farol.metas_vinculos(id) ON DELETE CASCADE,
    vigencia_id INTEGER       NOT NULL REFERENCES farol.metas_vigencias(id) ON DELETE CASCADE,
    uf          TEXT          NOT NULL,
    grupo       TEXT          NOT NULL,
    objetivo    NUMERIC(14,4) NOT NULL,
    created_at  TIMESTAMPTZ   NOT NULL DEFAULT now(),
    CONSTRAINT uq_farol_metas_grupos_objetivos UNIQUE (vigencia_id, uf, grupo)
);

DO $$
DECLARE
    r RECORD;
BEGIN
    FOR r IN SELECT id AS empresa_id FROM companies LOOP
        INSERT INTO farol.tipos_metrica (empresa_id, nome, descricao, nivel_agregacao, janela_apuracao, formula_codigo, parametros_schema)
        VALUES (
            r.empresa_id,
            'Positivação de Grupos (Segmento Foco)',
            'Conta quantos grupos de produtos cada cliente positivou no mês (mínimo de 3 unidades de qualquer produto do grupo; tipos de venda 1, 5 e 9 — bonificação positiva mesmo com valor zero). Objetivo por UF e grupo; o trimestre soma os 3 meses.',
            'cliente',
            'mes_fechado',
            'positivacao_grupos',
            '[]'::jsonb
        )
        ON CONFLICT (empresa_id, nome) DO NOTHING;
    END LOOP;
END $$;
