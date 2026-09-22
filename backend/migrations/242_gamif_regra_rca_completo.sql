-- 242_gamif_regra_rca_completo.sql
-- ════════════════════════════════════════════════════════════════════════════
-- 5º tipo de regra de Gamificação (pedido do Claudio 22/09/2026): "bater
-- 100% de TODAS as Redes do RCA" — diferente de rede_completa_atingida
-- (100% de UMA Rede específica), aqui é o portfólio inteiro do RCA junto.
-- Paga uma vez quando completo; mostra PROGRESSO ("faltam N lojas de M")
-- mesmo antes de completar — ver CalcularPontuacaoCampanha.
ALTER TABLE farol.gamif_regras DROP CONSTRAINT gamif_regras_tipo_check;
ALTER TABLE farol.gamif_regras ADD CONSTRAINT gamif_regras_tipo_check
    CHECK (tipo IN ('cobertura_atingida', 'sortimento_atingido', 'rede_completa_atingida', 'rca_completo', 'produto_especifico'));
