-- 198_idx_cliprinc_drill_rede.sql
-- ════════════════════════════════════════════════════════════════════════════
-- ÍNDICE DE APOIO — drill da visão V06 "Por Rede" (Rede → Cliente → ...).
--
-- Só o nível Rede (l0) usa agg; Cliente/Fornecedor/Produto SOB a rede leem
-- vendas_* escopados pelo drill (decisão de 2026-07-21, ver aggTablesFat no
-- farol_v2_api.go). Esse scan filtra por cod_cliprinc — coluna adicionada na
-- mig 181 e que NUNCA ganhou índice.
--
-- Efeito medido em produção 27/07/2026: abrir uma rede para ver seus clientes
-- levou 31,7s para devolver 9 clientes (Parallel Seq Scan em 18M linhas). O
-- nível seguinte (Rede+Cliente → Fornecedor) já era rápido, 1,7s, porque
-- aproveita idx_v[ft]_emp_cli_data.
--
-- Ordem (empresa_id, cod_cliprinc, data): igualdades primeiro, range de data
-- por último — a lição da mig 196, onde a data no meio impedia o planner de
-- usar o índice.
--
-- Parcial (cod_cliprinc <> '') porque só clientes de rede têm o campo
-- preenchido; indexar o resto seria volume sem ganho. Mesmo padrão das
-- migs 187/196.
-- ════════════════════════════════════════════════════════════════════════════

CREATE INDEX IF NOT EXISTS idx_vf_cliprinc
    ON vendas_faturadas (empresa_id, cod_cliprinc, data_faturamento)
    WHERE cod_cliprinc <> '';

CREATE INDEX IF NOT EXISTS idx_vt_cliprinc
    ON vendas_transmitidas (empresa_id, cod_cliprinc, data_transmissao)
    WHERE cod_cliprinc <> '';
