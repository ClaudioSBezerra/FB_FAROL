-- 207_quadro_token.sql
-- ════════════════════════════════════════════════════════════════════════════
-- LINK DO QUADRO SEM LOGIN (decisão de 22/08/2026).
--
-- O link vai por WhatsApp. Abrir o navegador do celular e cair na tela de login
-- mata a conveniência que justifica mandar por lá, então o quadro passa a
-- aceitar um token na URL — mesmo padrão das rotas /m/CNPJ/... que o ION VENDAS
-- já usa com os RCAs.
--
-- O RISCO É REAL E FOI ACEITO: mensagem de WhatsApp se reencaminha sozinha, e
-- quem receber o link vê o quadro sem precisar de senha. As mitigações abaixo
-- limitam o estrago, não o eliminam.
--
--   1. UM TOKEN POR PESSOA. Link vazado é rastreável até quem o recebeu, e
--      revogável sem afetar os outros. Token único compartilhado seria
--      impossível de investigar e de cortar.
--
--   2. SÓ O QUADRO. O token abre a tela de resumo e nada mais. Os links de
--      "abrir o painel" continuam exigindo login, então o que vaza é uma tela
--      de números agregados, não o sistema.
--
--   3. ESCOPO DE QUEM RECEBEU. O token resolve para a persona do dono; um GGV
--      com token vê a equipe dele, igual ao que veria logado.
--
--   4. REVOGÁVEL E CONTADO. `revogado` corta na hora; `acessos` e
--      `ultimo_acesso` mostram se um link está sendo aberto além do esperado —
--      é o sinal de que circulou.
-- ════════════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS farol.quadro_token (
    user_id       UUID        PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    token         TEXT        NOT NULL UNIQUE,
    criado_em     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revogado      BOOLEAN     NOT NULL DEFAULT FALSE,
    acessos       INT         NOT NULL DEFAULT 0,
    ultimo_acesso TIMESTAMPTZ
);

COMMENT ON TABLE farol.quadro_token IS
    'Token do link público do quadro "dinheiro na mesa". É credencial portadora: quem tem o link vê o quadro daquela pessoa. Revogue com revogado=TRUE.';
COMMENT ON COLUMN farol.quadro_token.acessos IS
    'Contador de aberturas. Muito acima do esperado para uma pessoa é o sinal de que o link circulou.';

CREATE INDEX IF NOT EXISTS idx_quadro_token_ativo
    ON farol.quadro_token (token) WHERE NOT revogado;
