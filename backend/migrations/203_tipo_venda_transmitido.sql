-- 203_tipo_venda_transmitido.sql
-- ════════════════════════════════════════════════════════════════════════════
-- TIPO DE VENDA no fluxo TRANSMITIDO (pedido em reunião, 17/08/2026 — Heverton).
--
-- O painel já filtra por tipo de venda no FATURADO desde a mig 187, mas no
-- transmitido o filtro era removido em silêncio porque a coluna não existia
-- ali. O dado sempre veio da origem: CONDVENDA e DESC_CONDVENDA estão na
-- COMPRAS_FAROL_VW para as duas metades, e o importador as detecta em toda
-- carga — apenas as descartava ao gravar o transmitido.
--
-- Volume que justifica (julho/2026, medido na origem):
--     TRANSMITIDO  1  Venda padrão          1.140.116
--     TRANSMITIDO  5  Bonificação Simples      20.333
--     TRANSMITIDO 10  Transferência             5.529
--     TRANSMITIDO  9  Venda Normal                168
--
-- Espelha exatamente o que as migs 187/188 fizeram para o faturado: coluna,
-- índice parcial de apoio ao filtro cruzado, e a dim que alimenta o dropdown.
--
-- ATENÇÃO — a coluna nasce VAZIA para tudo que já foi importado. Não há como
-- preencher retroativamente a partir de vendas_faturadas: são registros
-- diferentes (pedido transmitido × nota faturada), sem correspondência linha a
-- linha. O filtro só enxerga os períodos recarregados depois desta migration.
-- ════════════════════════════════════════════════════════════════════════════

ALTER TABLE vendas_transmitidas
    ADD COLUMN IF NOT EXISTS tipo_venda     TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS desc_condvenda TEXT NOT NULL DEFAULT '';

-- Mesmo desenho do idx_vf_tipo_venda (mig 187): o filtro cruzado escaneia por
-- empresa_id + data (range) e aplica tipo_venda = ANY(...). Parcial porque a
-- condição só faz sentido onde há tipo — e, enquanto o histórico não for
-- recarregado, isso mantém o índice pequeno em vez de indexar milhões de
-- strings vazias.
CREATE INDEX IF NOT EXISTS idx_vt_tipo_venda
    ON vendas_transmitidas (empresa_id, data_transmissao, tipo_venda)
    WHERE tipo_venda <> '';

-- ── Dim do dropdown no fluxo transmitido ────────────────────────────────────
-- A função da mig 188 grava só em agg_fat_dims_mes lendo de vendas_faturadas.
-- Esta é a gêmea para o transmitido; ficam as duas, cada uma com sua fonte,
-- porque o conjunto de códigos pode divergir entre os fluxos (em julho/2026 a
-- transferência aparece 30× menos no transmitido que no faturado).
CREATE OR REPLACE FUNCTION farol.upsert_tipo_venda_dims_trans(
    p_empresa_id UUID,
    p_ano        INT,
    p_mes        INT
) RETURNS VOID AS $$
DECLARE
    p_ini DATE := make_date(p_ano, p_mes, 1);
    p_fim DATE := (p_ini + INTERVAL '1 month' - INTERVAL '1 day')::date;
BEGIN
    INSERT INTO farol.agg_trans_dims_mes AS t (empresa_id, ano, mes, dim, key, label)
    SELECT p_empresa_id, p_ano, p_mes, 'tipo_venda', v.tipo_venda,
           farol.tipo_venda_label(v.tipo_venda)
      FROM vendas_transmitidas v
     WHERE v.empresa_id = p_empresa_id
       AND v.data_transmissao BETWEEN p_ini AND p_fim
       AND v.tipo_venda <> ''
     GROUP BY v.tipo_venda
    ON CONFLICT (ano, empresa_id, mes, dim, key) DO UPDATE SET label = EXCLUDED.label;
END;
$$ LANGUAGE plpgsql;

-- Backfill do que já estiver preenchido (nada, hoje — mas deixa a migration
-- idempotente e útil caso seja reaplicada após uma recarga).
DO $$
DECLARE r RECORD;
BEGIN
    FOR r IN
        SELECT DISTINCT empresa_id,
               EXTRACT(YEAR  FROM data_transmissao)::int AS ano,
               EXTRACT(MONTH FROM data_transmissao)::int AS mes
          FROM vendas_transmitidas
         WHERE tipo_venda <> ''
    LOOP
        PERFORM farol.upsert_tipo_venda_dims_trans(r.empresa_id, r.ano, r.mes);
    END LOOP;
END $$;

COMMENT ON COLUMN vendas_transmitidas.tipo_venda IS
    'CONDVENDA da origem (1 padrão, 5 bonificação, 10 transferência...). Vazio nos períodos importados antes da mig 203 — recarregar para preencher.';
