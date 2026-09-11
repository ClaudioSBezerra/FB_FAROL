-- Migration 234: Snapshot também para vigência ABERTA + recortes de tempo
--
-- Até aqui (migration 223, Story 4.3) só vigência FECHADA gravava snapshot —
-- vigência aberta e os 4 recortes (FR21: dia_anterior/semana/mes/ano_corrente)
-- eram SEMPRE recalculados ao vivo, a cada request, em cima de
-- vendas_faturadas/vendas_transmitidas. Isso virou o gargalo real do Painel
-- de Objetivos por Indústria: web E mobile (ION VENDAS) recalculam o mesmo
-- resultado pra TODOS os clientes válidos a cada abertura de tela, mesmo o
-- dado-fonte só mudando 1x/dia (carga JC).
--
-- Decisão do Claudio (11/09/2026): trocar "sempre ao vivo" por "snapshot
-- gravado no banco, renovado 1x/dia" (prewarm, depois da carga diária) —
-- mesmo princípio de baseCache/aggMesCache já usado no Painel Geral, só que
-- persistido em vez de em memória (sobrevive a deploy, comum neste projeto).
-- Isso relaxa deliberadamente a garantia "vigência aberta reflete dado novo
-- na hora" (nunca foi FR/NFR numerado — só um efeito colateral de não existir
-- cache nenhum até aqui) em troca de não recalcular por request. Ver
-- farol_metas_congelamento.go e farol_metas_prewarm.go.
--
-- `recorte`: '' = resultado da vigência inteira (mesma semântica de antes);
-- 'dia_anterior'/'semana'/'mes'/'ano_corrente' = um snapshot por recorte,
-- porque a janela de datas de cada um desliza com "hoje" (calcularRecorteDatas)
-- e precisa da própria linha pra não colidir com a vigência inteira.

ALTER TABLE farol.metas_realizados_snapshot
    ADD COLUMN IF NOT EXISTS recorte TEXT NOT NULL DEFAULT '';

ALTER TABLE farol.metas_realizados_snapshot
    DROP CONSTRAINT IF EXISTS uq_farol_metas_realizados_snapshot;
ALTER TABLE farol.metas_realizados_snapshot
    ADD CONSTRAINT uq_farol_metas_realizados_snapshot UNIQUE (vigencia_id, fluxo, nivel, recorte);

ALTER TABLE farol.metas_realizados_snapshot
    DROP CONSTRAINT IF EXISTS ck_farol_metas_realizados_snapshot_motivo;
ALTER TABLE farol.metas_realizados_snapshot
    ADD CONSTRAINT ck_farol_metas_realizados_snapshot_motivo
    CHECK (motivo IN ('congelamento_automatico', 'reprocessamento_manual', 'snapshot_diario'));
