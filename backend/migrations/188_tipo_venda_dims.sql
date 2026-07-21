-- 188_tipo_venda_dims.sql
-- ════════════════════════════════════════════════════════════════════════════
-- Popula as opções do dropdown "Tipo de Venda" (fluxo faturado).
--
-- O endpoint de dims (FarolV2DimsHandler) lê opções de filtro de
-- agg_fat_dims_mes como linhas (dim, key, label). Aqui:
--   1. farol.tipo_venda_label(código) → rótulo de negócio (código cru se
--      desconhecido — honra o edge case "tipo_venda=99 aparece cru").
--   2. farol.upsert_tipo_venda_dims(empresa, ano, mes) → insere as linhas
--      dim='tipo_venda' do mês. Chamada pelo Go logo após upsert_aggs_mes
--      (import e consolidação), então acompanha cada carga.
--   3. Backfill único dos meses já presentes em vendas_faturadas (caso haja
--      dados; após um ciclo limpar+reimportar o passo 2 cuida de tudo).
--
-- NÃO altera schema das agg nem o upsert principal (ver 187 para o porquê da
-- abordagem de filtro cruzado).
-- ════════════════════════════════════════════════════════════════════════════

-- ── 1. Rótulo de negócio dos 11 códigos conhecidos ──────────────────────────
CREATE OR REPLACE FUNCTION farol.tipo_venda_label(p_cod TEXT) RETURNS TEXT AS $$
    SELECT CASE p_cod
        WHEN '1'  THEN 'Padrão'
        WHEN '4'  THEN 'Simples Fatura'
        WHEN '5'  THEN 'Bonificação'
        WHEN '7'  THEN 'Entrega Futura'
        WHEN '8'  THEN 'Simples Entrega'
        WHEN '9'  THEN 'CFOP Específico'
        WHEN '10' THEN 'Transferência'
        WHEN '11' THEN 'Venda com Troca'
        WHEN '13' THEN 'Remessa Manifesto'
        WHEN '14' THEN 'Venda Manifesto'
        WHEN '20' THEN 'Consignada'
        -- Código fora da lista conhecida: mostra o próprio código (cru) para não
        -- sumir do filtro. Prefixa "Tipo N" para ficar legível no dropdown.
        ELSE 'Tipo ' || p_cod
    END;
$$ LANGUAGE sql IMMUTABLE;

-- ── 2. Popula dim='tipo_venda' do mês em agg_fat_dims_mes ────────────────────
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

-- ── 3. Backfill dos meses já presentes ──────────────────────────────────────
DO $$
DECLARE r RECORD;
BEGIN
    FOR r IN
        SELECT DISTINCT empresa_id,
               EXTRACT(YEAR  FROM data_faturamento)::int AS ano,
               EXTRACT(MONTH FROM data_faturamento)::int AS mes
          FROM vendas_faturadas
         WHERE tipo_venda <> ''
    LOOP
        PERFORM farol.upsert_tipo_venda_dims(r.empresa_id, r.ano, r.mes);
    END LOOP;
END $$;
