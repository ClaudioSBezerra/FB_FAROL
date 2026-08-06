-- 200_agg_v11_l5_cnpj.sql
-- ════════════════════════════════════════════════════════════════════════════
-- FOLHA DE CNPJ NA HIERARQUIA DE FILIAL — agg_*_v11_l5_mes
--
-- A mig 199 parou no RCA. Faltava o grão de CLIENTE, e a falta custava DUAS
-- coisas:
--
--   1. Drill até Cliente com filtro de filial caía no scan de vendas_*.
--
--   2. **O denominador da positivação ficava errado na tela.** `fetchCards` só
--      sobrescreve "Clientes Ativos" com COUNT(DISTINCT cnpj) quando
--      leafServesPositivados aceita — e ela recusa se algum filtro citar coluna
--      que a folha não tem. A folha da V01 (agg_fat_v01_l4_mes) não tem
--      `empresa`, então com filtro de filial o override não rodava e o número
--      vinha do base_cli do agregado, que é a CARTEIRA SOMADA (SUM qtcli_rca) —
--      166.572 numa base de 37.719 clientes, porque conta o mesmo cliente uma
--      vez por RCA que o atende. Positivação aparecia 0% em tudo.
--
-- Esta folha tem `empresa` E `cnpj`, então leafServesPositivados passa a aceitar
-- e o COUNT(DISTINCT cnpj) volta. Ver leafForPositivados em farol_v2_api.go.
--
-- GRÃO: (empresa, cod_fornec, cod_gerente, cod_supervisor, cod_rca, cnpj) —
-- é a agg_fat_v01_l4_mes com a filial na frente. base_cli=1 e positivados=0|1
-- por linha, igual à l4.
--
-- mix_total entra INLINE (a v01_l4 não tem, e por isso pickAggForCrossFilter
-- nunca a escolhe; aqui precisamos que escolha).
--
-- TAMANHO: é a maior das aggs de filial. Dividir o grão de CNPJ por filial
-- cresce ~20-30% em linhas — a maioria dos clientes compra de uma filial só
-- (77%), então poucas linhas se dividem.
--
-- ⚠ Função SEPARADA (upsert_aggs_mes_v11_l5) em vez de reescrever
--   upsert_aggs_mes_v10_v11 inteira: a de 199 tem ~350 linhas e reescrevê-la
--   para acrescentar um nível é convite a erro de transcrição. O Go chama as
--   duas em sequência.
--
-- ⚠ BACKFILL: não roda aqui (mesmo motivo da 197/199 — migrations rodam no
--   startup e travariam a subida). Nasce vazia. Se houver recarga da base, ela
--   se popula sozinha; senão, rodar upsert_aggs_mes_v11_l5 pelos meses.
-- ════════════════════════════════════════════════════════════════════════════

-- ── PARTE 1 — DDL ──────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS farol.agg_fat_v11_l5_mes (
    empresa_id      UUID    NOT NULL,
    ano             INT     NOT NULL,
    mes             INT     NOT NULL,
    empresa         TEXT    NOT NULL,
    cod_fornec      TEXT    NOT NULL,
    cod_gerente     TEXT    NOT NULL,
    cod_supervisor  TEXT    NOT NULL,
    cod_rca         TEXT    NOT NULL,
    cnpj            TEXT    NOT NULL,
    cod_cli         TEXT    NOT NULL DEFAULT '',
    nome_cli        TEXT    NOT NULL DEFAULT '',
    base_cli        INT     NOT NULL DEFAULT 1,
    positivados     INT     NOT NULL DEFAULT 0,
    mix             NUMERIC NOT NULL DEFAULT 0,
    mix_total       INT     NOT NULL DEFAULT 0,
    pvenda          NUMERIC NOT NULL DEFAULT 0,
    plucro          NUMERIC NOT NULL DEFAULT 0,
    qt              NUMERIC NOT NULL DEFAULT 0,
    liquido         NUMERIC NOT NULL DEFAULT 0,
    pv_bonif        NUMERIC NOT NULL DEFAULT 0,
    pv_transf       NUMERIC NOT NULL DEFAULT 0,
    pv_remessa      NUMERIC NOT NULL DEFAULT 0,
    pv_devol        NUMERIC NOT NULL DEFAULT 0,
    pv_cancel       NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, empresa, cod_fornec, cod_gerente, cod_supervisor, cod_rca, cnpj)
) PARTITION BY RANGE (ano);

-- trans não tem composição líquida (Líquido é conceito do faturado, mig 189).
CREATE TABLE IF NOT EXISTS farol.agg_trans_v11_l5_mes (
    empresa_id      UUID    NOT NULL,
    ano             INT     NOT NULL,
    mes             INT     NOT NULL,
    empresa         TEXT    NOT NULL,
    cod_fornec      TEXT    NOT NULL,
    cod_gerente     TEXT    NOT NULL,
    cod_supervisor  TEXT    NOT NULL,
    cod_rca         TEXT    NOT NULL,
    cnpj            TEXT    NOT NULL,
    cod_cli         TEXT    NOT NULL DEFAULT '',
    nome_cli        TEXT    NOT NULL DEFAULT '',
    base_cli        INT     NOT NULL DEFAULT 1,
    positivados     INT     NOT NULL DEFAULT 0,
    mix             NUMERIC NOT NULL DEFAULT 0,
    mix_total       INT     NOT NULL DEFAULT 0,
    pvenda          NUMERIC NOT NULL DEFAULT 0,
    plucro          NUMERIC NOT NULL DEFAULT 0,
    qt              NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, empresa, cod_fornec, cod_gerente, cod_supervisor, cod_rca, cnpj)
) PARTITION BY RANGE (ano);

-- Índice para o COUNT(DISTINCT cnpj) por agrupador (mesmo padrão das mig
-- 176/177 nas folhas v01_l4/v02_l3): parcial em positivados>0, que é o filtro
-- fixo de queryDistinctPositivados.
CREATE INDEX IF NOT EXISTS idx_aggfat_v11l5_fornec
    ON farol.agg_fat_v11_l5_mes (empresa_id, empresa, cod_fornec, ano, mes)
    WHERE positivados > 0;
CREATE INDEX IF NOT EXISTS idx_aggtrans_v11l5_fornec
    ON farol.agg_trans_v11_l5_mes (empresa_id, empresa, cod_fornec, ano, mes)
    WHERE positivados > 0;

-- ── PARTE 2 — agg_table_names() ganha as 2 novas ───────────────────────────
-- Sem isto, create_agg_year_partitions não cria partição de 2028 e o drop de
-- retenção não as alcança.

CREATE OR REPLACE FUNCTION farol.agg_table_names() RETURNS TEXT[] AS $$
BEGIN
    RETURN ARRAY[
        'agg_fat_v01_l0_mes','agg_fat_v01_l1_mes','agg_fat_v01_l2_mes','agg_fat_v01_l3_mes','agg_fat_v01_l4_mes',
        'agg_fat_v02_l0_mes','agg_fat_v02_l1_mes','agg_fat_v02_l2_mes','agg_fat_v02_l3_mes',
        'agg_fat_v03_l0_mes','agg_fat_v03_l1_mes','agg_fat_v03_l2_mes','agg_fat_v03_l3_mes',
        'agg_fat_v04_l0_mes','agg_fat_v04_l1_mes','agg_fat_v04_l2_mes',
        'agg_fat_v05_l0_mes','agg_fat_v05_l1_mes','agg_fat_v05_l2_mes','agg_fat_v05_l3_mes',
        -- V06/V07 (mig 183/184) — hierarquias Por Rede e Por Departamento
        'agg_fat_v06_l0_mes','agg_fat_v06_l1_mes','agg_fat_v06_l2_mes',
        'agg_fat_v07_l0_mes','agg_fat_v07_l1_mes','agg_fat_v07_l2_mes',
        -- V08/V09 (mig 197) — grão com UF
        'agg_fat_v08_l0_mes','agg_fat_v08_l1_mes','agg_fat_v08_l2_mes','agg_fat_v08_l3_mes',
        'agg_fat_v09_l1_mes','agg_fat_v09_l2_mes','agg_fat_v09_l3_mes','agg_fat_v09_l4_mes',
        -- V10/V11 (mig 199) — grão com FILIAL (coluna `empresa`)
        'agg_fat_v10_l0_mes','agg_fat_v10_l1_mes','agg_fat_v10_l2_mes','agg_fat_v10_l3_mes',
        'agg_fat_v11_l1_mes','agg_fat_v11_l2_mes','agg_fat_v11_l3_mes','agg_fat_v11_l4_mes',
        'agg_fat_v11_l5_mes',   -- mig 200: grão FILIAL x CNPJ
        'agg_fat_dims_mes',
        'agg_fat_mkt_cli_mes','agg_fat_mkt_produto_mes',
        'agg_trans_v01_l0_mes','agg_trans_v01_l1_mes','agg_trans_v01_l2_mes','agg_trans_v01_l3_mes','agg_trans_v01_l4_mes',
        'agg_trans_v02_l0_mes','agg_trans_v02_l1_mes','agg_trans_v02_l2_mes','agg_trans_v02_l3_mes',
        'agg_trans_v03_l0_mes','agg_trans_v03_l1_mes','agg_trans_v03_l2_mes','agg_trans_v03_l3_mes',
        'agg_trans_v04_l0_mes','agg_trans_v04_l1_mes','agg_trans_v04_l2_mes',
        'agg_trans_v05_l0_mes','agg_trans_v05_l1_mes','agg_trans_v05_l2_mes','agg_trans_v05_l3_mes',
        'agg_trans_v06_l0_mes','agg_trans_v06_l1_mes','agg_trans_v06_l2_mes',
        'agg_trans_v07_l0_mes','agg_trans_v07_l1_mes','agg_trans_v07_l2_mes',
        'agg_trans_v08_l0_mes','agg_trans_v08_l1_mes','agg_trans_v08_l2_mes','agg_trans_v08_l3_mes',
        'agg_trans_v09_l1_mes','agg_trans_v09_l2_mes','agg_trans_v09_l3_mes','agg_trans_v09_l4_mes',
        'agg_trans_v10_l0_mes','agg_trans_v10_l1_mes','agg_trans_v10_l2_mes','agg_trans_v10_l3_mes',
        'agg_trans_v11_l1_mes','agg_trans_v11_l2_mes','agg_trans_v11_l3_mes','agg_trans_v11_l4_mes',
        'agg_trans_v11_l5_mes',
        'agg_trans_dims_mes',
        'agg_trans_mkt_cli_mes','agg_trans_mkt_produto_mes'
    ];
END;
$$ LANGUAGE plpgsql IMMUTABLE;

-- ── PARTE 3 — partições 2025-2027 ──────────────────────────────────────────
DO $$
DECLARE a INT;
BEGIN
    FOR a IN 2025..2027 LOOP
        PERFORM farol.create_agg_year_partitions(a);
    END LOOP;
END $$;

-- ── PARTE 4 — farol.upsert_aggs_mes_v11_l5 ─────────────────────────────────
-- Chamada pelo Go logo depois de upsert_aggs_mes_v10_v11 e ANTES de
-- upsert_venda_liquida_cols (que preenche liquido/pv_* também nesta folha).

CREATE OR REPLACE FUNCTION farol.upsert_aggs_mes_v11_l5(
    p_empresa_id UUID,
    p_ano        INT,
    p_mes        INT
) RETURNS VOID AS $$
DECLARE
    p_ini DATE := make_date(p_ano, p_mes, 1);
    p_fim DATE := (p_ini + INTERVAL '1 month' - INTERVAL '1 day')::date;
BEGIN
    SET LOCAL work_mem = '256MB';

    -- ── FATURADO ───────────────────────────────────────────────────────────
    INSERT INTO farol.agg_fat_v11_l5_mes AS t
        (empresa_id, ano, mes, empresa, cod_fornec, cod_gerente, cod_supervisor,
         cod_rca, cnpj, cod_cli, nome_cli, base_cli, positivados, mix, mix_total,
         pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.empresa, v.cod_fornec, v.cod_gerente,
           v.cod_supervisor, v.cod_rca, v.cnpj, MAX(v.cod_cli), MAX(v.nome_cli),
           1,
           (CASE WHEN SUM(v.qt) > 0 THEN 1 ELSE 0 END)::INT,
           COUNT(DISTINCT v.cod_prod) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC,
           COUNT(DISTINCT v.cod_prod) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::INT,
           SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
      FROM vendas_faturadas v
     WHERE v.empresa_id = p_empresa_id
       AND v.data_faturamento BETWEEN p_ini AND p_fim
       AND v.empresa <> '' AND v.cod_fornec <> '' AND v.cod_gerente <> ''
       AND v.cod_supervisor <> '' AND v.cod_rca <> '' AND v.cnpj <> ''
     GROUP BY v.empresa_id, v.empresa, v.cod_fornec, v.cod_gerente,
              v.cod_supervisor, v.cod_rca, v.cnpj
    ON CONFLICT (ano, empresa_id, mes, empresa, cod_fornec, cod_gerente,
                 cod_supervisor, cod_rca, cnpj) DO UPDATE SET
        cod_cli = EXCLUDED.cod_cli, nome_cli = EXCLUDED.nome_cli,
        base_cli = EXCLUDED.base_cli, positivados = EXCLUDED.positivados,
        mix = EXCLUDED.mix, mix_total = EXCLUDED.mix_total,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- ── TRANSMITIDO ────────────────────────────────────────────────────────
    INSERT INTO farol.agg_trans_v11_l5_mes AS t
        (empresa_id, ano, mes, empresa, cod_fornec, cod_gerente, cod_supervisor,
         cod_rca, cnpj, cod_cli, nome_cli, base_cli, positivados, mix, mix_total,
         pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.empresa, v.cod_fornec, v.cod_gerente,
           v.cod_supervisor, v.cod_rca, v.cnpj, MAX(v.cod_cli), MAX(v.nome_cli),
           1,
           (CASE WHEN SUM(v.qt) > 0 THEN 1 ELSE 0 END)::INT,
           COUNT(DISTINCT v.cod_prod) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC,
           COUNT(DISTINCT v.cod_prod) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::INT,
           SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
      FROM vendas_transmitidas v
     WHERE v.empresa_id = p_empresa_id
       AND v.data_transmissao BETWEEN p_ini AND p_fim
       AND v.empresa <> '' AND v.cod_fornec <> '' AND v.cod_gerente <> ''
       AND v.cod_supervisor <> '' AND v.cod_rca <> '' AND v.cnpj <> ''
     GROUP BY v.empresa_id, v.empresa, v.cod_fornec, v.cod_gerente,
              v.cod_supervisor, v.cod_rca, v.cnpj
    ON CONFLICT (ano, empresa_id, mes, empresa, cod_fornec, cod_gerente,
                 cod_supervisor, cod_rca, cnpj) DO UPDATE SET
        cod_cli = EXCLUDED.cod_cli, nome_cli = EXCLUDED.nome_cli,
        base_cli = EXCLUDED.base_cli, positivados = EXCLUDED.positivados,
        mix = EXCLUDED.mix, mix_total = EXCLUDED.mix_total,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION farol.upsert_aggs_mes_v11_l5(UUID, INT, INT) IS
  'Popula agg_fat/trans_v11_l5_mes (grão FILIAL x CNPJ). Chamada após upsert_aggs_mes_v10_v11 e antes de upsert_venda_liquida_cols.';

-- ── PARTE 5 — upsert_venda_liquida_cols com a l5 na lista ──────────────────
CREATE OR REPLACE FUNCTION farol.upsert_venda_liquida_cols(
    p_empresa_id UUID,
    p_ano        INT,
    p_mes        INT
) RETURNS VOID AS $$
DECLARE
    p_ini     DATE := make_date(p_ano, p_mes, 1);
    p_fim     DATE := (p_ini + INTERVAL '1 month' - INTERVAL '1 day')::date;
    lvl       RECORD;
    k         TEXT;
    keycols   TEXT;
    wherecond TEXT;
    joincond  TEXT;
BEGIN
    SET LOCAL work_mem = '256MB';

    -- Temp unificado: faturado (evento='') + CCD (devol/cancel). tipo_venda só
    -- é relevante nas linhas faturadas; nas de CCD o discriminador é `evento`.
    -- empresa incluída (mig 197) para os níveis V10/V11 — vendas_ccd também tem empresa.
    DROP TABLE IF EXISTS _liq_all;
    CREATE TEMP TABLE _liq_all ON COMMIT DROP AS
    SELECT empresa_id, cod_fornec, cod_gerente, cod_supervisor, cod_rca, cnpj,
           cod_cliprinc, cod_depto, cod_sec, cod_categoria, uf, empresa,
           tipo_venda, ''::text AS evento, pvenda
      FROM vendas_faturadas
     WHERE empresa_id = p_empresa_id AND data_faturamento BETWEEN p_ini AND p_fim
    UNION ALL
    SELECT empresa_id, cod_fornec, cod_gerente, cod_supervisor, cod_rca, cnpj,
           cod_cliprinc, cod_depto, cod_sec, cod_categoria, uf, empresa,
           ''::text AS tipo_venda, evento, pvenda
      FROM vendas_ccd
     WHERE empresa_id = p_empresa_id AND data_evento BETWEEN p_ini AND p_fim
       AND evento IN ('DEVOLVIDO','CANCELADO');

    CREATE INDEX ON _liq_all (cod_fornec);
    CREATE INDEX ON _liq_all (cod_supervisor);
    CREATE INDEX ON _liq_all (cod_gerente);
    CREATE INDEX ON _liq_all (cod_rca);
    CREATE INDEX ON _liq_all (cod_cliprinc);
    CREATE INDEX ON _liq_all (cod_depto);
    CREATE INDEX ON _liq_all (uf);
    CREATE INDEX ON _liq_all (empresa);
    ANALYZE _liq_all;

    -- (tabela, chaves) de cada um dos níveis faturados.
    FOR lvl IN
        SELECT tbl, keys FROM (VALUES
            ('agg_fat_v01_l0_mes', ARRAY['cod_fornec']),
            ('agg_fat_v01_l1_mes', ARRAY['cod_fornec','cod_gerente']),
            ('agg_fat_v01_l2_mes', ARRAY['cod_fornec','cod_gerente','cod_supervisor']),
            ('agg_fat_v01_l3_mes', ARRAY['cod_fornec','cod_gerente','cod_supervisor','cod_rca']),
            ('agg_fat_v01_l4_mes', ARRAY['cod_fornec','cod_gerente','cod_supervisor','cod_rca','cnpj']),
            ('agg_fat_v02_l0_mes', ARRAY['cod_supervisor']),
            ('agg_fat_v02_l1_mes', ARRAY['cod_supervisor','cod_rca']),
            ('agg_fat_v02_l2_mes', ARRAY['cod_supervisor','cod_rca','cod_fornec']),
            ('agg_fat_v02_l3_mes', ARRAY['cod_supervisor','cod_rca','cod_fornec','cnpj']),
            ('agg_fat_v03_l0_mes', ARRAY['cod_gerente']),
            ('agg_fat_v03_l1_mes', ARRAY['cod_gerente','cod_supervisor']),
            ('agg_fat_v03_l2_mes', ARRAY['cod_gerente','cod_supervisor','cod_rca']),
            ('agg_fat_v03_l3_mes', ARRAY['cod_gerente','cod_supervisor','cod_rca','cnpj']),
            ('agg_fat_v04_l0_mes', ARRAY['cod_rca']),
            ('agg_fat_v04_l1_mes', ARRAY['cod_rca','cod_fornec']),
            ('agg_fat_v04_l2_mes', ARRAY['cod_rca','cod_fornec','cnpj']),
            ('agg_fat_v05_l0_mes', ARRAY['cod_supervisor']),
            ('agg_fat_v05_l1_mes', ARRAY['cod_supervisor','cod_fornec']),
            ('agg_fat_v05_l2_mes', ARRAY['cod_supervisor','cod_fornec','cod_rca']),
            ('agg_fat_v05_l3_mes', ARRAY['cod_supervisor','cod_fornec','cod_rca','cnpj']),
            ('agg_fat_v06_l0_mes', ARRAY['cod_cliprinc']),
            ('agg_fat_v06_l1_mes', ARRAY['cod_cliprinc','cod_fornec']),
            ('agg_fat_v06_l2_mes', ARRAY['cod_cliprinc','cod_fornec','cnpj']),
            ('agg_fat_v07_l0_mes', ARRAY['cod_depto']),
            ('agg_fat_v07_l1_mes', ARRAY['cod_depto','cod_sec']),
            ('agg_fat_v07_l2_mes', ARRAY['cod_depto','cod_sec','cod_categoria']),
            -- V08/V09 (mig 197) — grão com UF
            ('agg_fat_v08_l0_mes', ARRAY['uf']),
            ('agg_fat_v08_l1_mes', ARRAY['uf','cod_gerente']),
            ('agg_fat_v08_l2_mes', ARRAY['uf','cod_gerente','cod_supervisor']),
            ('agg_fat_v08_l3_mes', ARRAY['uf','cod_gerente','cod_supervisor','cod_rca']),
            ('agg_fat_v09_l1_mes', ARRAY['uf','cod_fornec']),
            ('agg_fat_v09_l2_mes', ARRAY['uf','cod_fornec','cod_gerente']),
            ('agg_fat_v09_l3_mes', ARRAY['uf','cod_fornec','cod_gerente','cod_supervisor']),
            ('agg_fat_v09_l4_mes', ARRAY['uf','cod_fornec','cod_gerente','cod_supervisor','cod_rca']),
            -- V10/V11 (mig 199) — grão com FILIAL (coluna `empresa`)
            ('agg_fat_v10_l0_mes', ARRAY['empresa']),
            ('agg_fat_v10_l1_mes', ARRAY['empresa','cod_gerente']),
            ('agg_fat_v10_l2_mes', ARRAY['empresa','cod_gerente','cod_supervisor']),
            ('agg_fat_v10_l3_mes', ARRAY['empresa','cod_gerente','cod_supervisor','cod_rca']),
            ('agg_fat_v11_l1_mes', ARRAY['empresa','cod_fornec']),
            ('agg_fat_v11_l2_mes', ARRAY['empresa','cod_fornec','cod_gerente']),
            ('agg_fat_v11_l3_mes', ARRAY['empresa','cod_fornec','cod_gerente','cod_supervisor']),
            ('agg_fat_v11_l4_mes', ARRAY['empresa','cod_fornec','cod_gerente','cod_supervisor','cod_rca']),
            -- l5 (mig 200) — grão CNPJ: é ela que devolve o COUNT(DISTINCT cnpj)
            ('agg_fat_v11_l5_mes', ARRAY['empresa','cod_fornec','cod_gerente','cod_supervisor','cod_rca','cnpj'])
        ) AS t(tbl, keys)
    LOOP
        -- tabela ainda não existe nesse ambiente? pula.
        IF to_regclass('farol.' || lvl.tbl) IS NULL THEN
            CONTINUE;
        END IF;

        keycols := ''; wherecond := ''; joincond := '';
        FOREACH k IN ARRAY lvl.keys LOOP
            keycols   := keycols   || CASE WHEN keycols=''   THEN '' ELSE ', '   END || quote_ident(k);
            wherecond := wherecond || CASE WHEN wherecond='' THEN '' ELSE ' AND ' END || quote_ident(k) || ' <> ' || quote_literal('');
            joincond  := joincond  || CASE WHEN joincond=''  THEN '' ELSE ' AND ' END || 'a.' || quote_ident(k) || ' = s.' || quote_ident(k);
        END LOOP;

        EXECUTE format($f$
            UPDATE farol.%I a SET
                pv_bonif   = s.bonif,
                pv_transf  = s.transf,
                pv_remessa = s.remessa,
                pv_devol   = s.devol,
                pv_cancel  = s.cancel,
                liquido    = s.venda_real - s.devol - s.cancel
            FROM (
                SELECT %s,
                    COALESCE(SUM(pvenda) FILTER (WHERE evento = '' AND tipo_venda IN ('1','4','7','8','9','11','14','20')), 0) AS venda_real,
                    COALESCE(SUM(pvenda) FILTER (WHERE evento = '' AND tipo_venda = '5'),  0) AS bonif,
                    COALESCE(SUM(pvenda) FILTER (WHERE evento = '' AND tipo_venda = '10'), 0) AS transf,
                    COALESCE(SUM(pvenda) FILTER (WHERE evento = '' AND tipo_venda = '13'), 0) AS remessa,
                    COALESCE(SUM(pvenda) FILTER (WHERE evento = 'DEVOLVIDO'), 0) AS devol,
                    COALESCE(SUM(pvenda) FILTER (WHERE evento = 'CANCELADO'), 0) AS cancel
                FROM _liq_all
                WHERE %s
                GROUP BY %s
            ) s
            WHERE a.empresa_id = %L AND a.ano = %s AND a.mes = %s AND %s
        $f$, lvl.tbl, keycols, wherecond, keycols, p_empresa_id, p_ano, p_mes, joincond);
    END LOOP;
END;
$$ LANGUAGE plpgsql;
