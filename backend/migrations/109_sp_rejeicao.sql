-- 109_sp_rejeicao.sql
-- Tipos de rejeição de propostas de calibragem (Item 1.2 do backlog pós-reunião)
--
-- Cria sp_tipo_rejeicao com os 3 tipos iniciais e adiciona motivo_rejeicao_id
-- em sp_propostas para rastrear o motivo quando o gestor rejeita uma sugestão.

CREATE TABLE IF NOT EXISTS smartpick.sp_tipo_rejeicao (
    id         SERIAL PRIMARY KEY,
    codigo     INTEGER  NOT NULL UNIQUE,
    descricao  TEXT     NOT NULL,
    ativo      BOOLEAN  NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO smartpick.sp_tipo_rejeicao (codigo, descricao) VALUES
    (1, 'Estratégia de alocação'),
    (2, 'Sazonalidade'),
    (3, 'Opção do gestor')
ON CONFLICT (codigo) DO NOTHING;

ALTER TABLE smartpick.sp_propostas
    ADD COLUMN IF NOT EXISTS motivo_rejeicao_id INTEGER
        REFERENCES smartpick.sp_tipo_rejeicao(id);
