-- 206_email_case_insensitive.sql
-- ════════════════════════════════════════════════════════════════════════════
-- E-MAIL SEM DIFERENCIAR MAIÚSCULA.
--
-- Descoberto em 22/08/2026: o diretor foi cadastrado como
-- "Edinardo.magalhaes@jcdistribuicao.com.br", com E maiúsculo. O login
-- comparava `email = $1` — comparação exata —, então ele só entraria digitando
-- o E maiúsculo. Tudo minúsculo, como qualquer pessoa digita, devolveria
-- "e-mail ou senha inválidos" na conta dele mesmo.
--
-- Pior no "esqueci minha senha": aquele fluxo responde sucesso vago de
-- propósito, para não revelar quais e-mails existem. Ele ficaria esperando um
-- link que nunca sairia, sem nenhuma mensagem de erro para reclamar.
--
-- Isso apareceu porque a migration 205 não ligou a flag do resumo semanal para
-- ele — o UPDATE procurava minúsculo. O sintoma era pequeno; a causa não.
--
-- Três partes: normalizar o que já existe, impedir que volte a acontecer, e
-- ligar a flag que ficou faltando.
-- ════════════════════════════════════════════════════════════════════════════

-- ── 1. Normaliza, mas só onde é seguro ──────────────────────────────────────
-- Se existirem "joao@x.com" e "Joao@x.com" como contas DIFERENTES, baixar a
-- caixa da segunda colide com a UNIQUE(email) e derruba a migration inteira.
-- O NOT EXISTS pula esses casos: eles ficam como estão e aparecem no aviso
-- abaixo, para alguém decidir qual conta vale.
UPDATE users u
   SET email = lower(u.email)
 WHERE u.email <> lower(u.email)
   AND NOT EXISTS (
       SELECT 1 FROM users o
        WHERE o.id <> u.id AND lower(o.email) = lower(u.email)
   );

-- ── 2. Impede que volte ─────────────────────────────────────────────────────
-- Índice único sobre lower(email). Criado condicionalmente: com duplicata por
-- caixa ainda pendente, o CREATE falharia e quebraria o deploy — trocando um
-- problema de cadastro por uma indisponibilidade.
DO $$
DECLARE dups INT;
BEGIN
    SELECT COUNT(*) INTO dups FROM (
        SELECT lower(email) FROM users GROUP BY 1 HAVING COUNT(*) > 1
    ) d;

    IF dups > 0 THEN
        RAISE WARNING 'users: % e-mail(s) duplicados ignorando maiúsculas — índice único NÃO criado. Resolva os duplicados e reaplique esta migration.', dups;
    ELSE
        CREATE UNIQUE INDEX IF NOT EXISTS idx_users_email_lower ON users (lower(email));
        RAISE NOTICE 'users: índice único sobre lower(email) criado';
    END IF;
END $$;

-- ── 3. A flag que a mig 205 não pegou ───────────────────────────────────────
-- Agora por lower() dos dois lados, para não repetir o mesmo erro.
UPDATE users SET farol_resumo_semanal = TRUE
 WHERE lower(email) IN (
     'heverton.sa@jcdistribuicao.com.br',
     'edinardo.magalhaes@jcdistribuicao.com.br',
     'greyce.silva@jcdistribuicao.com.br'
 );
