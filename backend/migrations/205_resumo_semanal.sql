-- 205_resumo_semanal.sql
-- ════════════════════════════════════════════════════════════════════════════
-- RESUMO SEMANAL "DINHEIRO NA MESA" — opt-in por usuário.
--
-- Flag por pessoa, e não lista de e-mails em variável de ambiente, porque o
-- destinatário é quem já está cadastrado: o escopo do resumo sai da própria
-- persona do usuário. Gerente geral recebe a empresa inteira, GGV recebe a
-- equipe dele, supervisor recebe a dele — a mesma regra do painel, sem uma
-- segunda lista para manter em sincronia.
--
-- Nasce DESLIGADO para todo mundo. Um relatório semanal que começa ligado
-- chega como spam para quem não pediu, e a primeira reação é criar regra de
-- caixa de entrada — aí ele nunca mais é lido.
--
-- TELEFONE entra agora e fica sem uso. A fase 2 é mandar só o LINK por
-- WhatsApp (decisão de 22/08: e-mail primeiro, WhatsApp depois de conversa com
-- o dono). Guardar o número custa uma coluna; descobrir depois que precisa
-- dela custa outra migration e outro deploy.
-- ════════════════════════════════════════════════════════════════════════════

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS farol_resumo_semanal BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS telefone             TEXT    NOT NULL DEFAULT '';

COMMENT ON COLUMN users.farol_resumo_semanal IS
    'Recebe o resumo semanal do Farol na segunda de manhã. O ESCOPO do resumo vem de tipo_persona/cod_referencia, não daqui.';
COMMENT ON COLUMN users.telefone IS
    'E.164 sem "+" (ex.: 5562999998888). Reservado para o envio do link por WhatsApp; sem uso enquanto a fase 2 não sair.';

-- Log de envio: idempotência e prova.
--
-- A idempotência importa porque o worker acorda de hora em hora e precisa
-- saber que já mandou nesta semana. E a prova importa porque a primeira
-- pergunta quando alguém disser "não recebi" é se o sistema mandou.
CREATE TABLE IF NOT EXISTS farol.resumo_semanal_log (
    id           BIGSERIAL PRIMARY KEY,
    empresa_id   UUID        NOT NULL,
    user_id      UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    semana       DATE        NOT NULL,  -- segunda-feira de referência
    enviado_em   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    destinatario TEXT        NOT NULL,
    rcas         INT         NOT NULL DEFAULT 0,
    total_mesa   NUMERIC(15,2) NOT NULL DEFAULT 0,
    baseline     TEXT        NOT NULL DEFAULT '',
    erro         TEXT        NOT NULL DEFAULT '',
    UNIQUE (user_id, semana)
);

COMMENT ON TABLE farol.resumo_semanal_log IS
    'Um envio por usuário por semana. O UNIQUE é o que impede o worker horário de repetir o e-mail.';

-- Liga para os três gestores que pediram (22/08/2026): Heverton, Edinardo e
-- Greyce. Por e-mail, não por código: eles são os três "Geral", sem
-- cod_referencia, e portanto recebem a empresa inteira.
UPDATE users SET farol_resumo_semanal = TRUE
 WHERE email IN (
     'heverton.sa@jcdistribuicao.com.br',
     'edinardo.magalhaes@jcdistribuicao.com.br',
     'greyce.silva@jcdistribuicao.com.br'
 );
