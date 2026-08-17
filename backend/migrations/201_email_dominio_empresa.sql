-- 201_farol_slug_empresa.sql
-- ════════════════════════════════════════════════════════════════════════════
-- Domínio de e-mail por empresa, usado nas contas que o SyncUsuarios cria
-- automaticamente para GGVs, supervisores e RCAs.
--
-- ANTES:  ggv.347@ac91bee4.farol.local
-- DEPOIS: ggv.347@jcdistribuicao.com.br
--
-- O "ac91bee4" eram os 8 primeiros caracteres do empresa_id. Garantia
-- unicidade, mas ninguém consegue ditar isso por telefone para um gerente em
-- campo — que é exatamente o que precisa acontecer.
--
-- POR QUE O ÍNDICE ÚNICO IMPORTA: users.email é único GLOBAL, entre todos os
-- tenants. Era o hash do empresa_id que impedia duas empresas de gerarem o
-- mesmo e-mail para códigos iguais (o gerente 347 de duas distribuidoras
-- diferentes). Com o domínio no lugar do hash, a unicidade passa a depender
-- deste índice — sem ele, o SyncUsuarios da segunda empresa bateria em
-- conflito e simplesmente não criaria o usuário, em silêncio.
--
-- ATENÇÃO — o domínio agora é REAL: ggv.347@jcdistribuicao.com.br é um
-- endereço que não existe no servidor de e-mail da JC. Isso não afeta o
-- login (a senha é validada no banco), mas qualquer fluxo que ENVIE mensagem
-- para o usuário — recuperação de senha, por exemplo — vai gerar bounce.
-- ════════════════════════════════════════════════════════════════════════════

ALTER TABLE companies ADD COLUMN IF NOT EXISTS farol_email_dominio VARCHAR(120);

-- Deriva do domínio do dono da empresa, que é um humano com e-mail corporativo
-- de verdade (claudio@jcdistribuicao.com.br → jcdistribuicao.com.br).
UPDATE companies c
   SET farol_email_dominio = lower(split_part(u.email, '@', 2))
  FROM users u
 WHERE u.id = c.owner_id
   AND c.farol_email_dominio IS NULL
   AND u.email LIKE '%@%'
   AND split_part(u.email, '@', 2) NOT LIKE '%.farol%';  -- não herdar conta sintética

-- Sem dono identificável, mantém o comportamento antigo — o código faz o
-- mesmo fallback quando a coluna vem vazia, então os dois lados combinam.
UPDATE companies
   SET farol_email_dominio = left(id::text, 8) || '.farol.local'
 WHERE farol_email_dominio IS NULL OR farol_email_dominio = '';

CREATE UNIQUE INDEX IF NOT EXISTS idx_companies_farol_email_dominio
    ON companies(farol_email_dominio) WHERE farol_email_dominio IS NOT NULL;

-- ─── Renomeia os usuários já criados ────────────────────────────────────────
-- Sem isto ficaríamos com DUAS contas por pessoa: o SyncUsuarios não acharia o
-- e-mail antigo (a chave do ON CONFLICT é o e-mail), criaria a versão nova, e a
-- antiga continuaria ativa — com senha válida e acesso ao painel.
--
-- O casamento usa o próprio hash embutido no e-mail antigo, que é a forma mais
-- direta de ligar usuário → empresa sem depender de outra tabela.
UPDATE users u
   SET email = split_part(u.email, '@', 1) || '@' || c.farol_email_dominio
  FROM companies c
 WHERE u.email LIKE '%@' || left(c.id::text, 8) || '.farol.local'
   AND c.farol_email_dominio IS NOT NULL
   AND c.farol_email_dominio NOT LIKE '%.farol.local'   -- nada a fazer no fallback
   AND NOT EXISTS (                                     -- respeita e-mail já existente
        SELECT 1 FROM users u2
         WHERE u2.email = split_part(u.email, '@', 1) || '@' || c.farol_email_dominio
       );

-- ─── Destrava o painel para quem já existe ──────────────────────────────────
-- O SyncUsuarios passou a criar GGV/supervisor como gestor_filial, mas ele usa
-- ON CONFLICT (email) DO NOTHING: quem já estava cadastrado nunca seria
-- atualizado e continuaria tomando 403 na porta, exatamente como o GGV 347 na
-- verificação de 14/08/2026.
--
-- Elevar o papel aqui NÃO amplia o que essas pessoas enxergam: desde o mesmo
-- dia o recorte por persona (farol_escopo.go) limita cada uma ao próprio
-- cod_referencia, e nega o acesso de quem estiver sem código. É por isso que a
-- condição abaixo exige cod_referencia preenchido — elevar alguém sem código
-- criaria uma conta que entra e é barrada adiante, sem utilidade nenhuma.
--
-- RCA fica de fora de propósito: ele usa o link público do ION VENDAS.
UPDATE users
   SET sp_role = 'gestor_filial'
 WHERE tipo_persona IN ('ggv', 'supervisor')
   AND sp_role = 'somente_leitura'
   AND COALESCE(cod_referencia, '') <> '';

COMMENT ON COLUMN companies.farol_email_dominio IS
    'Domínio dos e-mails das contas auto-criadas pelo SyncUsuarios (ggv.347@<dominio>). Único entre tenants. Ajustável: UPDATE companies SET farol_email_dominio = ''jcdistribuicao.com.br'' WHERE id = ...';
