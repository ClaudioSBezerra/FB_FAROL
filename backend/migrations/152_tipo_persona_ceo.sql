-- 152_tipo_persona_ceo.sql
-- Adiciona 'ceo' aos valores permitidos de tipo_persona.

ALTER TABLE users DROP CONSTRAINT IF EXISTS check_tipo_persona_values;

ALTER TABLE users ADD CONSTRAINT check_tipo_persona_values
  CHECK (tipo_persona IS NULL OR tipo_persona IN (
    'ceo',
    'diretor',
    'gerente_geral',
    'ggv',
    'supervisor',
    'rca',
    'ti',
    'analista_negocios',
    'admin'
  ));
