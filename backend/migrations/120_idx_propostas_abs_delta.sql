-- 120 — Índice expression em ABS(delta) para acelerar ORDER BY na listagem
--
-- O dashboard ordena por ABS(delta) DESC. O índice idx_sp_propostas_urgencia
-- existente cobre (empresa_id, cd_id, delta, status) mas não permite usar
-- expression scan em ABS(delta), forçando o Postgres a fazer Sort em memória
-- após filtragem. Com este índice expression, o ORDER BY usa Index Scan direto.

CREATE INDEX IF NOT EXISTS idx_sp_propostas_abs_delta_pendente
  ON smartpick.sp_propostas (cd_id, ABS(delta) DESC, id)
  WHERE status = 'pendente';

-- Cobertura adicional para tipos sem filtro de pendente (calibrado/ignorado)
CREATE INDEX IF NOT EXISTS idx_sp_propostas_cd_abs_delta
  ON smartpick.sp_propostas (cd_id, status, ABS(delta) DESC);

ANALYZE smartpick.sp_propostas;
