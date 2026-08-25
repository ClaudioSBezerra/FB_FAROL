-- 208_cnpj_receita.sql
-- ════════════════════════════════════════════════════════════════════════════
-- CADASTRO DA RECEITA POR CNPJ (decisão de 25/08/2026).
--
-- POR QUE EXISTE. O Core Value do Farol inclui "clientes sem venda". Hoje um
-- cliente que parou de comprar aparece como meta a recuperar — e o supervisor
-- é cobrado por isso. Amostra aleatória de 40 clientes que compraram em 2025 e
-- nada em 2026, conferidos na Receita em 25/08/2026:
--
--     35 ATIVA    2 BAIXADA    3 INAPTA
--
-- Nos CINCO casos a mudança de situação cadastral cai na janela em que a compra
-- parou (supermercado baixado em 23/12/2025, lanchonete em 19/01/2026, três
-- inaptos entre maio e junho de 2026). Não é correlação: é a explicação.
--
-- São 8.507 clientes nessa condição, R$ 107,1 milhões faturados em 2025. Sem
-- este cadastro, o painel não distingue "perdemos o cliente" (falha de venda,
-- alguém tem que agir) de "o cliente deixou de existir" (não havia o que fazer,
-- e mandar o RCA lá é rota desperdiçada).
--
-- SEM empresa_id, DE PROPÓSITO. O CNPJ da padaria é o mesmo para qualquer
-- distribuidora — é dado de referência, não de tenant. Se entrar outro cliente
-- no sistema, o cache já está quente. Não vaza escopo porque todo join passa
-- por vendas_*, que é escopada por empresa_id.
--
-- SEM O QUADRO SOCIETÁRIO (qsa), TAMBÉM DE PROPÓSITO. A API devolve nome de
-- sócio, faixa etária e CPF mascarado. É registro público, mas são pessoas
-- naturais: guardar exige finalidade declarada, e para análise de venda não
-- serve a nada. Se algum dia for preciso, que seja uma decisão nova.
--
-- FONTE. BrasilAPI (brasilapi.com.br/api/cnpj/v1/), que serve o dump mensal da
-- Receita Federal — não é consulta ao vivo. Para situação cadastral isso basta,
-- porque ela muda devagar; `consultado_em` registra quando lemos.
-- ════════════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS farol.cnpj_receita (
    cnpj                  CHAR(14)    PRIMARY KEY,
    cnpj_raiz             CHAR(8)     GENERATED ALWAYS AS (left(cnpj, 8)) STORED,

    razao_social          TEXT,
    nome_fantasia         TEXT,

    -- 2=ATIVA  3=SUSPENSA  4=INAPTA  8=BAIXADA
    situacao_cod          SMALLINT,
    situacao_desc         TEXT,
    situacao_data         DATE,
    situacao_motivo       TEXT,

    cnae_cod              TEXT,
    cnae_desc             TEXT,
    cnaes_secundarios     JSONB,
    natureza_juridica     TEXT,
    porte                 TEXT,
    capital_social        NUMERIC(18,2),
    data_inicio_atividade DATE,

    -- 1=matriz  2=filial. Com cnpj_raiz, permite conferir o agrupamento
    -- "Por Rede" do painel, que hoje vem do cadastro do WinThor.
    matriz_filial         SMALLINT,

    municipio             TEXT,
    uf                    CHAR(2),
    cep                   TEXT,
    logradouro            TEXT,
    numero                TEXT,
    bairro                TEXT,
    municipio_ibge        INTEGER,
    telefone              TEXT,
    email                 TEXT,

    opcao_simples         BOOLEAN,
    opcao_mei             BOOLEAN,
    regime_tributario     JSONB,

    consultado_em         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    fonte                 TEXT        NOT NULL DEFAULT 'brasilapi',

    -- Preenchido = tentamos e falhou. A linha existe para a carga não repetir o
    -- CNPJ a cada rodada, mas `erro IS NOT NULL` é a fila de reprocessamento.
    -- Sem isso, um CNPJ que não existe na Receita seria consultado para sempre.
    erro                  TEXT
);

COMMENT ON TABLE farol.cnpj_receita IS
    'Cadastro da Receita por CNPJ, via BrasilAPI (dump mensal). Separa "cliente perdido" de "empresa que deixou de existir" na lista de clientes sem venda. Sem empresa_id: é dado de referência, compartilhado entre tenants.';
COMMENT ON COLUMN farol.cnpj_receita.situacao_cod IS
    '2=ATIVA 3=SUSPENSA 4=INAPTA 8=BAIXADA. Diferente de ATIVA, o cliente não é oportunidade de venda.';
COMMENT ON COLUMN farol.cnpj_receita.cnpj_raiz IS
    'Oito primeiros dígitos: matriz e filiais compartilham. Rede econômica real, para conferir contra o cod_cliprinc do WinThor.';
COMMENT ON COLUMN farol.cnpj_receita.erro IS
    'Falha da última tentativa. NULL = consultado com sucesso. Fila de reprocessamento é erro IS NOT NULL.';

-- Situação: "quais clientes estão baixados" é a consulta que justifica a tabela.
CREATE INDEX IF NOT EXISTS idx_cnpjrec_situacao ON farol.cnpj_receita (situacao_cod);
-- CNAE: segmentação por atividade real, em vez do ramo digitado no WinThor.
CREATE INDEX IF NOT EXISTS idx_cnpjrec_cnae     ON farol.cnpj_receita (cnae_cod);
-- Raiz: agrupamento de matriz + filiais.
CREATE INDEX IF NOT EXISTS idx_cnpjrec_raiz     ON farol.cnpj_receita (cnpj_raiz);
-- Fila de reprocessamento: parcial, porque o normal é erro NULL.
CREATE INDEX IF NOT EXISTS idx_cnpjrec_erro     ON farol.cnpj_receita (consultado_em) WHERE erro IS NOT NULL;
