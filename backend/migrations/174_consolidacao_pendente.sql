-- Migration 174 — Consolidação incremental (P2.1)
--
-- Hoje a RefreshViews re-consolida TODOS os meses presentes em vendas_* (17 meses
-- = ~40 min), mesmo quando o import tocou só 1-2 meses. Esta tabela registra os
-- meses efetivamente tocados por cada import; a RefreshViews passa a consolidar
-- apenas os pendentes e limpá-los — derrubando o tempo do import diário de ~40 min
-- para ~2-4 min.
--
-- Fallback: se não houver pendências (ex: RefreshViews manual após mexer direto no
-- banco), a RefreshViews volta ao comportamento antigo (consolida todos os meses).

CREATE TABLE IF NOT EXISTS farol.consolidacao_pendente (
    empresa_id uuid        NOT NULL,
    ano        int         NOT NULL,
    mes        int         NOT NULL,
    criado_em  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (empresa_id, ano, mes)
);

COMMENT ON TABLE farol.consolidacao_pendente IS
  'Meses tocados por imports aguardando consolidação (upsert_aggs_mes). RefreshViews processa e limpa. Vazio = nada pendente.';
