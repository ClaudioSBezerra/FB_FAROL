-- Migration 111: Rastreamento de uso por módulo
-- Registra tempo de permanência de cada usuário em cada módulo do sistema.

CREATE TABLE IF NOT EXISTS smartpick.sp_usage_log (
    id          BIGSERIAL   PRIMARY KEY,
    empresa_id  UUID        NOT NULL,
    user_id     UUID        NOT NULL,
    modulo      TEXT        NOT NULL,
    caminho     TEXT        NOT NULL,
    duracao_seg INT         NOT NULL DEFAULT 0,
    sessao_id   TEXT        NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS sp_usage_log_empresa_at
    ON smartpick.sp_usage_log (empresa_id, created_at DESC);

CREATE INDEX IF NOT EXISTS sp_usage_log_user_at
    ON smartpick.sp_usage_log (user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS sp_usage_log_modulo
    ON smartpick.sp_usage_log (modulo, created_at DESC);
