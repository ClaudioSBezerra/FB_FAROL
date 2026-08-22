-- 204_filiais_nome.sql
-- ════════════════════════════════════════════════════════════════════════════
-- NOME DAS FILIAIS no filtro (pedido em 22/08/2026).
--
-- Até aqui o filtro "Filial" mostrava só o código do WinThor — 1, 11, 12, 13,
-- 18, 20, 24, 29, 32. Funciona para quem decorou, e mais ninguém: numa reunião
-- com o CEO, "20" não diz que é Conceição do Jacuípe.
--
-- O código continua sendo a chave em tudo (dados, filtro, escopo, agregados).
-- Esta tabela é só CADASTRO DE RÓTULO: quem não tiver nome aqui aparece pelo
-- código, exatamente como antes. Nada quebra por falta de linha.
--
-- Três filiais que EXISTEM nos dados ficaram de fora da lista que a JC mandou —
-- 15, 28 e 33. São as de prestação de serviço de transporte intercompany, as
-- mesmas do produto 429046. Elas seguem aparecendo pelo código até a JC dizer
-- como se chamam.
--
-- E três da lista NÃO têm dado ainda — 14, 21 e 25. São as que aguardam
-- migração no WinThor (os códigos de RCA serão todos novos). Ficam cadastradas
-- aqui, mas não aparecem no filtro até terem venda: o dropdown lista o que tem
-- movimento, não o que está cadastrado.
-- ════════════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS farol.filiais (
    empresa_id  UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    codigo      TEXT NOT NULL,
    nome        TEXT NOT NULL,
    ativa       BOOLEAN NOT NULL DEFAULT TRUE,
    PRIMARY KEY (empresa_id, codigo)
);

COMMENT ON TABLE farol.filiais IS
    'Rótulo das filiais para o filtro. A chave em todo o resto do sistema segue sendo o código do WinThor (coluna `empresa` das tabelas de venda). Filial sem linha aqui aparece pelo código.';

-- Semeia para as empresas que JÁ TÊM esses códigos nos dados, em vez de fixar
-- o UUID da JC no arquivo: migration com tenant hardcoded quebra em qualquer
-- outro ambiente e vira dívida silenciosa.
INSERT INTO farol.filiais (empresa_id, codigo, nome)
SELECT c.id, f.codigo, f.nome
  FROM companies c
  CROSS JOIN (VALUES
      ('1',  'JC ANAPOLIS-GO'),
      ('11', 'JC APARECIDA DE GOIANIA-GO'),
      ('12', 'JC GOIANIA-GO'),
      ('13', 'JC BRASILIA-DF'),
      ('14', 'JC JD GOIANIA-GO'),
      ('18', 'JC V7-GO'),
      ('20', 'JC CONCEICAO DO JACUIPE-BA'),
      ('21', 'JC MARABA-PA'),
      ('24', 'JC PALMAS-TO'),
      ('25', 'JC JD ANAPOLIS-GO'),
      ('29', 'JC IMPERATRIZ-MA'),
      ('32', 'JC CAMPO GRANDE-MS')
  ) AS f(codigo, nome)
 WHERE EXISTS (
     SELECT 1 FROM vendas_faturadas v
      WHERE v.empresa_id = c.id AND v.empresa <> ''
 )
ON CONFLICT (empresa_id, codigo) DO UPDATE SET nome = EXCLUDED.nome;
