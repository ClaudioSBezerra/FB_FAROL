-- 243_gamif_pontuacao_volume_desempate.sql
-- ════════════════════════════════════════════════════════════════════════════
-- Pedido do Claudio 22/09/2026 ("o ranking ficou estranho... curva ABC de
-- vendas"): hoje quem bate o mínimo de uma regra tipo produto_especifico
-- ganha pontos/bônus FIXOS — quem vendeu 10 garrafas e quem vendeu 188
-- empatavam em 1º lugar. O prêmio continua o mesmo (não é proporcional),
-- mas a POSIÇÃO no ranking passa a desempatar por volume real vendido —
-- estilo curva ABC, não só passou/não passou.
ALTER TABLE farol.gamif_pontuacao
    ADD COLUMN volume_desempate NUMERIC NOT NULL DEFAULT 0;

COMMENT ON COLUMN farol.gamif_pontuacao.volume_desempate IS
    'Soma da grandeza real por trás de cada regra (qtd vendida pra produto_especifico, lojas/Redes atingidas pras demais) — usado só pra ORDER BY, não afeta pontos_total nem bonus_total.';
