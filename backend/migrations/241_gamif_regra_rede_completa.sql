-- 241_gamif_regra_rede_completa.sql
-- ════════════════════════════════════════════════════════════════════════════
-- 4º tipo de regra de Gamificação (pedido do Claudio 22/09/2026, pra ficar
-- mais fácil o José Costa entender): "rede_completa_atingida" — bônus só
-- quando TODAS as lojas de uma Rede baterem a meta no mês, não só uma
-- parte (isso já existe via cobertura_atingida/sortimento_atingido, que
-- paga por CADA loja que bate — ver farol_gamificacao.go). É um prêmio
-- maior/extra pra cobertura total da Rede, não substitui o por-loja.
ALTER TABLE farol.gamif_regras DROP CONSTRAINT gamif_regras_tipo_check;
ALTER TABLE farol.gamif_regras ADD CONSTRAINT gamif_regras_tipo_check
    CHECK (tipo IN ('cobertura_atingida', 'sortimento_atingido', 'rede_completa_atingida', 'produto_especifico'));
