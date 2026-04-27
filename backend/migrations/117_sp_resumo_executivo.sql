-- 117 — Resumo Executivo Semanal por CD
-- Cria duas tabelas:
--   sp_destinatarios_resumo: gestores que recebem o resumo por email
--   sp_relatorios_semanais:  histórico de resumos gerados (KPIs + narrativa IA)

CREATE TABLE IF NOT EXISTS smartpick.sp_destinatarios_resumo (
    id              SERIAL      PRIMARY KEY,
    cd_id           INTEGER     NOT NULL REFERENCES smartpick.sp_centros_dist(id) ON DELETE CASCADE,
    nome_completo   TEXT        NOT NULL,
    cargo           TEXT,
    email           TEXT        NOT NULL,
    ativo           BOOLEAN     NOT NULL DEFAULT TRUE,
    criado_em       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    atualizado_em   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_sp_destinatarios_cd      ON smartpick.sp_destinatarios_resumo(cd_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_sp_destinatarios_unico
  ON smartpick.sp_destinatarios_resumo(cd_id, lower(email))
  WHERE ativo = TRUE;

CREATE TABLE IF NOT EXISTS smartpick.sp_relatorios_semanais (
    id              SERIAL      PRIMARY KEY,
    cd_id           INTEGER     NOT NULL REFERENCES smartpick.sp_centros_dist(id) ON DELETE CASCADE,
    periodo_inicio  DATE        NOT NULL,
    periodo_fim     DATE        NOT NULL,
    dados_json      JSONB       NOT NULL,
    narrativa_md    TEXT        NOT NULL,
    enviado_em      TIMESTAMPTZ,
    enviado_para    TEXT[],
    erro_envio      TEXT,
    criado_em       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    criado_por      TEXT
);

CREATE INDEX IF NOT EXISTS idx_sp_relatorios_cd_data ON smartpick.sp_relatorios_semanais(cd_id, periodo_fim DESC);

COMMENT ON TABLE  smartpick.sp_destinatarios_resumo IS 'Gestores que recebem o resumo executivo semanal por CD';
COMMENT ON TABLE  smartpick.sp_relatorios_semanais  IS 'Histórico de resumos executivos gerados pela IA';
COMMENT ON COLUMN smartpick.sp_relatorios_semanais.dados_json   IS 'KPIs estruturados do período (JSON)';
COMMENT ON COLUMN smartpick.sp_relatorios_semanais.narrativa_md IS 'Narrativa em markdown gerada pela IA';
