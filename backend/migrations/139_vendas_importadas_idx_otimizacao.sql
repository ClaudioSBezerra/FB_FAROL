-- 139_vendas_importadas_idx_otimizacao.sql
-- Remove índice redundante: idx_vi_emp_base_periodo já cobre empresa_id como prefixo,
-- tornando idx_vi_empresa desnecessário. Cada índice a menos = commit mais rápido
-- no COPY de 2M+ rows (350 MB / mês).

DROP INDEX IF EXISTS idx_vi_empresa;

-- Garante que o índice composto de filtro principal existe com a cobertura certa.
-- (Já existe via 136 mas re-cria com IF NOT EXISTS por segurança.)
CREATE INDEX IF NOT EXISTS idx_vi_emp_base_periodo
    ON vendas_importadas(empresa_id, tipo_base, ano, mes);
