-- Migration 110: Bloqueio de empresa (sem apagar dados)
--
-- Adiciona colunas para marcar uma empresa como bloqueada.
-- Quando blocked_at IS NOT NULL, usuários não-admin da empresa recebem 403
-- em todas as chamadas de API SmartPick (via SmartPickAuthMiddleware).
--
-- Desbloqueio: UPDATE companies SET blocked_at = NULL, blocked_reason = NULL WHERE id = ?
-- Dados preservados — apenas o acesso fica suspenso.

ALTER TABLE companies
    ADD COLUMN IF NOT EXISTS blocked_at      TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS blocked_reason  TEXT,
    ADD COLUMN IF NOT EXISTS blocked_by      UUID REFERENCES users(id) ON DELETE SET NULL;

COMMENT ON COLUMN companies.blocked_at     IS 'Timestamp do bloqueio; NULL = empresa ativa';
COMMENT ON COLUMN companies.blocked_reason IS 'Motivo do bloqueio (exibido ao usuário no toast)';
COMMENT ON COLUMN companies.blocked_by     IS 'Usuário MASTER que executou o bloqueio';

-- Índice parcial para facilitar queries de empresas bloqueadas (pequeno)
CREATE INDEX IF NOT EXISTS idx_companies_blocked
    ON companies (blocked_at)
    WHERE blocked_at IS NOT NULL;
