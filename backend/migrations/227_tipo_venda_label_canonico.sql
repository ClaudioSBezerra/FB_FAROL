-- 227_tipo_venda_label_canonico.sql
-- ════════════════════════════════════════════════════════════════════════════
-- Padroniza o rótulo do filtro "Tipo de Venda" entre Faturado e Transmitido.
--
-- ACHADO (08/09/2026, dado real de produção — JC Distribuição): o mesmo código
-- mostrava rótulo DIFERENTE dependendo do fluxo, porque só o Faturado
-- (mig 192) priorizava o texto cru do ERP (`desc_condvenda`) como label; o
-- Transmitido (mig 203) sempre usou o nome de negócio fixo
-- (`farol.tipo_venda_label`). Resultado visto no dropdown real:
--   código 1  → "Venda padrão" (Faturado) vs "Normal"           (Transmitido)
--   código 5  → "Bonificação Simples" (Fat) vs "Bonificação"    (Trans)
--   código 9  → "Venda Normal" (Fat) vs "CFOP Específico"       (Trans)  ← pior caso,
--                parecem categorias diferentes sendo o MESMO código
--   código 10 → "Transferência:" (Fat, com ":" sobrando) vs "Transferência" (Trans)
-- Também havia mojibake na entrada crua do Faturado ("BonificaÃ§Ã£o Simples"
-- ao lado de "Bonificação Simples" pro mesmo código/período) — o dropdown só
-- não mostrava o texto quebrado por sorte de ordenação alfabética no MAX().
--
-- DECISÃO (Claudio, 08/09/2026): usar sempre o nome de negócio fixo
-- (farol.tipo_venda_label) nos dois fluxos — é o que já rodava certo no
-- Transmitido. NÃO removemos a coluna `desc_condvenda` (import continua
-- gravando o texto cru do ERP; só paramos de usá-lo como rótulo do dropdown).
-- ════════════════════════════════════════════════════════════════════════════

CREATE OR REPLACE FUNCTION farol.upsert_tipo_venda_dims(
    p_empresa_id UUID,
    p_ano        INT,
    p_mes        INT
) RETURNS VOID AS $$
DECLARE
    p_ini DATE := make_date(p_ano, p_mes, 1);
    p_fim DATE := (p_ini + INTERVAL '1 month' - INTERVAL '1 day')::date;
BEGIN
    INSERT INTO farol.agg_fat_dims_mes AS t (empresa_id, ano, mes, dim, key, label)
    SELECT p_empresa_id, p_ano, p_mes, 'tipo_venda', v.tipo_venda,
           farol.tipo_venda_label(v.tipo_venda)
      FROM vendas_faturadas v
     WHERE v.empresa_id = p_empresa_id
       AND v.data_faturamento BETWEEN p_ini AND p_fim
       AND v.tipo_venda <> ''
     GROUP BY v.tipo_venda
    ON CONFLICT (ano, empresa_id, mes, dim, key) DO UPDATE SET label = EXCLUDED.label;
END;
$$ LANGUAGE plpgsql;

-- Backfill: relabela AGORA os meses já importados (21 meses em produção),
-- sem esperar a próxima carga/reimportação pra refletir o nome certo.
UPDATE farol.agg_fat_dims_mes
   SET label = farol.tipo_venda_label(key)
 WHERE dim = 'tipo_venda'
   AND label <> farol.tipo_venda_label(key);
