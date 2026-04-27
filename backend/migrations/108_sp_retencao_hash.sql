-- Migration 108: sp_retencao_hash
-- Story 8.2 — Hash de arquivo para detecção de duplicatas e parâmetro de retenção
--
-- sp_csv_jobs.file_hash:
--   SHA-256 do conteúdo do arquivo. Usado para detectar re-importação do mesmo
--   arquivo e bloquear com 409, orientando o gestor a exportar nova carga do Winthor.
--
-- sp_motor_params.retencao_csv_meses:
--   Período de retenção dos dados brutos de importação (sp_csv_jobs + sp_enderecos).
--   Após esse período, o endpoint /api/sp/admin/purgar-csv-antigos pode limpar os dados.
--   sp_propostas e sp_historico NÃO são afetados pela purga (são auditoria permanente).

-- ─── file_hash em sp_csv_jobs ────────────────────────────────────────────────
ALTER TABLE smartpick.sp_csv_jobs
  ADD COLUMN IF NOT EXISTS file_hash TEXT;

-- Índice para lookup rápido de duplicatas por empresa+CD+hash
CREATE INDEX IF NOT EXISTS idx_sp_csv_jobs_hash
  ON smartpick.sp_csv_jobs (empresa_id, cd_id, file_hash)
  WHERE file_hash IS NOT NULL;

-- ─── retencao_csv_meses em sp_motor_params ───────────────────────────────────
ALTER TABLE smartpick.sp_motor_params
  ADD COLUMN IF NOT EXISTS retencao_csv_meses INTEGER NOT NULL DEFAULT 6
    CHECK (retencao_csv_meses BETWEEN 1 AND 60);

COMMENT ON COLUMN smartpick.sp_motor_params.retencao_csv_meses IS
  'Meses de retenção dos dados brutos de importação CSV (sp_csv_jobs + sp_enderecos). Mín: 1 mês, Máx: 60 meses.';
