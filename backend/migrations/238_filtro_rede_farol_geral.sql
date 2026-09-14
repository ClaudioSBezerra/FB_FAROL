-- 238_filtro_rede_farol_geral.sql
-- ════════════════════════════════════════════════════════════════════════════
-- Filtro por Rede (cod_cliprinc) no Farol Geral (pedido do Claudio, 14/09/2026):
-- hoje só existe "Por Rede" como VISÃO (agrupamento V06), não como filtro
-- cruzado — pra achar uma rede específica é preciso saber de cor os clientes
-- dela e filtrar um por um em "Cliente".
--
-- Escolha de implementação: em vez de tabelas agg_*_cliprinc novas (que
-- exigiriam ensinar pickAggForCrossFilter/aggServesFilters sobre mais uma
-- coluna, com o mesmo risco de dupla-contagem que fornec/uf/filial tiveram),
-- o filtro de Rede é RESOLVIDO pro conjunto de cod_cli que pertencem a ela
-- (resolveRedeFilter, farol_v2_api.go) — cada cliente pertence a UMA rede só,
-- então isso reaproveita 100% do roteamento agg/scan-ao-vivo que cod_cli já
-- tem, sem risco novo. Esta migration só precisa alimentar o DROPDOWN de
-- opções "Rede" (farol.agg_fat_dims_mes / agg_trans_dims_mes, dim='cliprinc')
-- — o filtro em si não lê essas tabelas.
--
-- Duas partes:
--   1) upsert_aggs_mes_v06 (mig 226) ganha 2 INSERTs novos (dims fat/trans,
--      dim='cliprinc') — cópia integral da função + o bloco novo, mesmo
--      padrão de toda migration que toca essa function (CREATE OR REPLACE
--      substitui o corpo inteiro).
--   2) Backfill pontual: os meses já consolidados não vão reprocessar
--      sozinhos, então copia direto de agg_fat_v06_l0_mes/agg_trans_v06_l0_mes
--      (que já têm cod_cliprinc + nome_cliprinc computados) pra
--      agg_*_dims_mes — sem tocar vendas_faturadas/vendas_transmitidas de
--      novo.
-- ════════════════════════════════════════════════════════════════════════════

CREATE OR REPLACE FUNCTION farol.upsert_aggs_mes_v06(p_empresa_id uuid, p_ano integer, p_mes integer)
 RETURNS void
 LANGUAGE plpgsql
AS $function$
DECLARE
    p_ini DATE := make_date(p_ano, p_mes, 1);
    p_fim DATE := (p_ini + INTERVAL '1 month' - INTERVAL '1 day')::date;
BEGIN
    SET LOCAL work_mem = '256MB';

    -- ─── FATURADO ────────────────────────────────────────────────────────────
    DROP TABLE IF EXISTS _v06_fat;
    CREATE TEMP TABLE _v06_fat ON COMMIT DROP AS
    SELECT
        v.empresa_id,
        v.cod_cliprinc,
        v.cod_fornec, v.nome_fornec,
        v.cnpj, v.cod_cli, v.nome_cli, v.fantasia,
        v.pvenda, v.plucro, v.qt
    FROM vendas_faturadas v
    WHERE v.empresa_id = p_empresa_id
      AND v.data_faturamento BETWEEN p_ini AND p_fim
      AND v.cod_cliprinc <> '';

    -- L0: por rede
    INSERT INTO farol.agg_fat_v06_l0_mes AS t
        (empresa_id, ano, mes, cod_cliprinc, nome_cliprinc, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_cliprinc,
        COALESCE(NULLIF(MAX(v.fantasia), ''), MAX(v.nome_cli)),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v06_fat v
    GROUP BY v.empresa_id, v.cod_cliprinc
    ON CONFLICT (ano, empresa_id, mes, cod_cliprinc) DO UPDATE SET
        nome_cliprinc = EXCLUDED.nome_cliprinc,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- agg_fat_v06_l1_mes / agg_fat_v06_l2_mes REMOVIDAS em 08/09/2026 (mig
    -- 226): desde 21/07/2026 o drill Rede→Cliente/Fornecedor/Produto lê a
    -- base ao vivo (ver hierarquias["V06"] em farol_v2_api.go), então essas
    -- 2 tabelas só recebiam INSERT sem nenhum leitor. Tabelas mantidas (sem
    -- DROP nesta rodada), só paramos de escrever nelas.

    -- DIMS fat (14/09/2026, mig 238) — alimenta o dropdown do filtro cruzado
    -- "Rede" (?dim=cliprinc em /api/v2/farol/dims). Mesmo rótulo de sempre
    -- (fantasia da rede, com fallback pro nome do cliente).
    INSERT INTO farol.agg_fat_dims_mes AS t (empresa_id, ano, mes, dim, key, label)
    SELECT
        v.empresa_id, p_ano, p_mes, 'cliprinc', v.cod_cliprinc,
        COALESCE(NULLIF(MAX(v.fantasia), ''), MAX(v.nome_cli))
    FROM _v06_fat v
    GROUP BY v.empresa_id, v.cod_cliprinc
    ON CONFLICT (ano, empresa_id, mes, dim, key) DO UPDATE SET label = EXCLUDED.label;

    -- ─── TRANSMITIDO ─────────────────────────────────────────────────────────
    DROP TABLE IF EXISTS _v06_trans;
    CREATE TEMP TABLE _v06_trans ON COMMIT DROP AS
    SELECT
        v.empresa_id,
        v.cod_cliprinc,
        v.cod_fornec, v.nome_fornec,
        v.cnpj, v.cod_cli, v.nome_cli, v.fantasia,
        v.pvenda, v.plucro, v.qt
    FROM vendas_transmitidas v
    WHERE v.empresa_id = p_empresa_id
      AND v.data_transmissao BETWEEN p_ini AND p_fim
      AND v.cod_cliprinc <> '';

    INSERT INTO farol.agg_trans_v06_l0_mes AS t
        (empresa_id, ano, mes, cod_cliprinc, nome_cliprinc, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_cliprinc,
        COALESCE(NULLIF(MAX(v.fantasia), ''), MAX(v.nome_cli)),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v06_trans v
    GROUP BY v.empresa_id, v.cod_cliprinc
    ON CONFLICT (ano, empresa_id, mes, cod_cliprinc) DO UPDATE SET
        nome_cliprinc = EXCLUDED.nome_cliprinc,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- agg_trans_v06_l1_mes / agg_trans_v06_l2_mes REMOVIDAS em 08/09/2026 (mig
    -- 226) — mesmo motivo do bloco FATURADO acima.

    -- DIMS trans (14/09/2026, mig 238) — idem ao bloco fat acima.
    INSERT INTO farol.agg_trans_dims_mes AS t (empresa_id, ano, mes, dim, key, label)
    SELECT
        v.empresa_id, p_ano, p_mes, 'cliprinc', v.cod_cliprinc,
        COALESCE(NULLIF(MAX(v.fantasia), ''), MAX(v.nome_cli))
    FROM _v06_trans v
    GROUP BY v.empresa_id, v.cod_cliprinc
    ON CONFLICT (ano, empresa_id, mes, dim, key) DO UPDATE SET label = EXCLUDED.label;

END;
$function$
;

-- ─── Backfill dos meses já consolidados ────────────────────────────────────
-- agg_fat_v06_l0_mes/agg_trans_v06_l0_mes já têm cod_cliprinc + nome_cliprinc
-- prontos pra todo mês já rodado — copia direto, sem reprocessar vendas_*.
INSERT INTO farol.agg_fat_dims_mes (empresa_id, ano, mes, dim, key, label)
SELECT empresa_id, ano, mes, 'cliprinc', cod_cliprinc, nome_cliprinc
  FROM farol.agg_fat_v06_l0_mes
 WHERE cod_cliprinc <> ''
ON CONFLICT (ano, empresa_id, mes, dim, key) DO UPDATE SET label = EXCLUDED.label;

INSERT INTO farol.agg_trans_dims_mes (empresa_id, ano, mes, dim, key, label)
SELECT empresa_id, ano, mes, 'cliprinc', cod_cliprinc, nome_cliprinc
  FROM farol.agg_trans_v06_l0_mes
 WHERE cod_cliprinc <> ''
ON CONFLICT (ano, empresa_id, mes, dim, key) DO UPDATE SET label = EXCLUDED.label;
