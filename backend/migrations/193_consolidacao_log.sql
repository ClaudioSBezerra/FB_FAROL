-- Migration 193 — Carimbo de "quando a consolidação terminou"
--
-- O Painel BI mostra ao gestor de quando é o dado que está na tela. O primeiro
-- critério usado foi MAX(vendas_import_jobs.atualizado_em) WHERE status='done',
-- que carimba o fim do UPLOAD — não da consolidação.
--
-- Isso mente no caminho multi-arquivo (skip_refresh=true): cada job fecha como
-- 'done' logo após copiar as linhas, e a consolidação (upsert_aggs_mes) só
-- acontece depois, no RefreshViews do fim da fila. Entre um e outro, o painel
-- exibiria "dados de 03:10" mostrando números da carga ANTERIOR.
--
-- Esta tabela registra o instante em que a consolidação de fato terminou, nos
-- dois caminhos que a executam (processImportJob e RefreshViewsHandler).
-- Uma linha por empresa, sobrescrita a cada consolidação — não é histórico.
--
-- Aditiva: não recria função nem reconsolida nada.

CREATE TABLE IF NOT EXISTS farol.consolidacao_log (
    empresa_id  uuid        NOT NULL PRIMARY KEY,
    concluido_em timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE farol.consolidacao_log IS
  'Última consolidação (upsert_aggs_mes) concluída por empresa. Fonte do carimbo "dados de" no Painel BI. Vazia = nunca consolidou desde a mig 193; o BI cai no fallback de vendas_import_jobs.';
