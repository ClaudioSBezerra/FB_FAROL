-- Migration 116: tabela de tipos de ignorado + FK em sp_ignorados
--
-- sp_tipo_ignorado: motivos estruturados pelos quais um produto é ignorado
--   na calibragem (equivalente a sp_tipo_rejeicao para rejeições).
-- sp_ignorados.tipo_ignorado_id: referência ao tipo selecionado pelo gestor.

CREATE TABLE IF NOT EXISTS smartpick.sp_tipo_ignorado (
    id        SERIAL   PRIMARY KEY,
    codigo    INTEGER  NOT NULL UNIQUE,
    descricao TEXT     NOT NULL,
    ativo     BOOLEAN  NOT NULL DEFAULT TRUE
);

INSERT INTO smartpick.sp_tipo_ignorado (codigo, descricao) VALUES
    (1, 'Volume X Capacidade Rua'),
    (2, 'Alto Giro Sazonalidade')
ON CONFLICT (codigo) DO NOTHING;

ALTER TABLE smartpick.sp_ignorados
    ADD COLUMN IF NOT EXISTS tipo_ignorado_id INTEGER
        REFERENCES smartpick.sp_tipo_ignorado(id) ON DELETE SET NULL;
