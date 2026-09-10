-- 230_drop_dead_aggs_e_para_v04.sql
-- ════════════════════════════════════════════════════════════════════════════
-- Continuação da limpeza começada na migration 226 (pedido do Claudio,
-- 10/09/2026: "pense profundamente na estrutura... o que podemos excluir de
-- VIEWS agregadas não utilizadas e deixarmos principalmente o farol
-- indústria voando"). Investigação confirmou que o Painel de Objetivos por
-- Indústria (farol_metas_*.go) NÃO lê nenhuma tabela agg_* — consulta
-- vendas_faturadas/vendas_transmitidas diretamente. Não há risco de dado
-- aqui pra esse painel; o ganho é reduzir o volume de escrita da
-- consolidação diária (upsertAggsMesParallel), que hoje grava em ~104
-- tabelas agg_* e ainda roda UPDATE dinâmico sobre 43 delas — tudo
-- disputando o mesmo pool de conexões Postgres e as mesmas tabelas brutas
-- que o Painel de Objetivos consulta.
--
-- Duas ações, cada uma seguindo um padrão diferente de risco:
--
-- 1) DROP FÍSICO das 7 tabelas que a migration 226 (08/09/2026) já tinha
--    parado de escrever: agg_fat_v06_l1_mes, agg_fat_v06_l2_mes,
--    agg_trans_v06_l1_mes, agg_trans_v06_l2_mes, agg_fat_mkt_produto_mes,
--    agg_trans_mkt_cli_mes, agg_trans_mkt_produto_mes. Zero escrita desde
--    então, zero leitura desde sempre (confirmado por grep em handlers/*.go
--    e services/*.go), sem FK/trigger/view apontando pra elas. Antes do
--    DROP, agg_table_names() precisa parar de listá-las — essa função
--    alimenta farol.create_agg_year_partitions(), chamada em
--    handlers/admin.go e handlers/farol_v2_import.go; se a tabela for
--    apagada e continuar na lista, a próxima chamada de
--    create_agg_year_partitions() quebra em runtime com
--    "relation ... does not exist" e aborta a criação de partição de TODAS
--    as tabelas daquele ano, inclusive as que devem continuar vivas.
--
-- 2) PARAR DE ESCREVER (sem apagar ainda) na view V04 ("Por RCA"):
--    roteável via ?view=V04, mas zero consumidor em todo o frontend
--    (grep -rn "V04" frontend/src não retorna nada). Segue o mesmo padrão
--    de 2 etapas que a própria 226 usou: parar de escrever agora, avaliar
--    fisicamente apagar numa migration futura depois de um ciclo de
--    produção sem incidente. As tabelas agg_fat_v04_*/agg_trans_v04_*
--    continuam existindo e continuam em agg_table_names() (ainda recebem
--    partição anual) — só não recebem INSERT novo.
--
-- NÃO tocado (fora de escopo, decisão explícita): V08/V09/V10/V11 (grão
-- UF/Filial) são lidas de verdade pelo roteamento de filtro cruzado
-- (pickAggForCrossFilter em farol_v2_api.go) — removê-las jogaria a carga
-- de volta pro scan ao vivo em vendas_faturadas/vendas_transmitidas, contra
-- o objetivo. V12 alimenta a stored procedure de Sazonalidade (feature
-- real via botão de admin) — não é código morto.
-- ════════════════════════════════════════════════════════════════════════════

-- ─── 1) Tira as 7 tabelas órfãs de agg_table_names() ───────────────────────
-- Corpo idêntico ao da migration 229, só removendo as 7 entradas confirmadas
-- órfãs desde a 226.
CREATE OR REPLACE FUNCTION farol.agg_table_names() RETURNS TEXT[] AS $$
BEGIN
    RETURN ARRAY[
        'agg_fat_v01_l0_mes','agg_fat_v01_l1_mes','agg_fat_v01_l2_mes','agg_fat_v01_l3_mes','agg_fat_v01_l4_mes',
        'agg_fat_v02_l0_mes','agg_fat_v02_l1_mes','agg_fat_v02_l2_mes','agg_fat_v02_l3_mes',
        'agg_fat_v03_l0_mes','agg_fat_v03_l1_mes','agg_fat_v03_l2_mes','agg_fat_v03_l3_mes',
        'agg_fat_v04_l0_mes','agg_fat_v04_l1_mes','agg_fat_v04_l2_mes',
        'agg_fat_v05_l0_mes','agg_fat_v05_l1_mes','agg_fat_v05_l2_mes','agg_fat_v05_l3_mes',
        'agg_fat_v06_l0_mes',
        'agg_fat_v07_l0_mes','agg_fat_v07_l1_mes','agg_fat_v07_l2_mes',
        'agg_fat_v08_l0_mes','agg_fat_v08_l1_mes','agg_fat_v08_l2_mes','agg_fat_v08_l3_mes',
        'agg_fat_v09_l1_mes','agg_fat_v09_l2_mes','agg_fat_v09_l3_mes','agg_fat_v09_l4_mes',
        'agg_fat_v10_l0_mes','agg_fat_v10_l1_mes','agg_fat_v10_l2_mes','agg_fat_v10_l3_mes',
        'agg_fat_v11_l1_mes','agg_fat_v11_l2_mes','agg_fat_v11_l3_mes','agg_fat_v11_l4_mes','agg_fat_v11_l5_mes',
        'agg_fat_dims_mes',
        'agg_fat_mkt_cli_mes',
        'agg_fat_v12_l1_mes',
        'agg_sazonalidade_produto_ano',
        'agg_fat_v03_ind_l0_mes','agg_fat_v03_ind_l1_mes','agg_fat_v03_ind_l2_mes','agg_fat_v03_ind_l3_mes',
        'agg_fat_v02_ind_l0_mes','agg_fat_v02_ind_l1_mes',
        -- Somente Indústrias — V06/V07 (mig 229)
        'agg_fat_v06_ind_l0_mes',
        'agg_fat_v07_ind_l0_mes','agg_fat_v07_ind_l1_mes','agg_fat_v07_ind_l2_mes',
        'agg_trans_v01_l0_mes','agg_trans_v01_l1_mes','agg_trans_v01_l2_mes','agg_trans_v01_l3_mes','agg_trans_v01_l4_mes',
        'agg_trans_v02_l0_mes','agg_trans_v02_l1_mes','agg_trans_v02_l2_mes','agg_trans_v02_l3_mes',
        'agg_trans_v03_l0_mes','agg_trans_v03_l1_mes','agg_trans_v03_l2_mes','agg_trans_v03_l3_mes',
        'agg_trans_v04_l0_mes','agg_trans_v04_l1_mes','agg_trans_v04_l2_mes',
        'agg_trans_v05_l0_mes','agg_trans_v05_l1_mes','agg_trans_v05_l2_mes','agg_trans_v05_l3_mes',
        'agg_trans_v06_l0_mes',
        'agg_trans_v07_l0_mes','agg_trans_v07_l1_mes','agg_trans_v07_l2_mes',
        'agg_trans_v08_l0_mes','agg_trans_v08_l1_mes','agg_trans_v08_l2_mes','agg_trans_v08_l3_mes',
        'agg_trans_v09_l1_mes','agg_trans_v09_l2_mes','agg_trans_v09_l3_mes','agg_trans_v09_l4_mes',
        'agg_trans_v10_l0_mes','agg_trans_v10_l1_mes','agg_trans_v10_l2_mes','agg_trans_v10_l3_mes',
        'agg_trans_v11_l1_mes','agg_trans_v11_l2_mes','agg_trans_v11_l3_mes','agg_trans_v11_l4_mes','agg_trans_v11_l5_mes',
        'agg_trans_dims_mes',
        'agg_trans_v03_ind_l0_mes','agg_trans_v03_ind_l1_mes','agg_trans_v03_ind_l2_mes','agg_trans_v03_ind_l3_mes',
        'agg_trans_v02_ind_l0_mes','agg_trans_v02_ind_l1_mes',
        'agg_trans_v06_ind_l0_mes',
        'agg_trans_v07_ind_l0_mes','agg_trans_v07_ind_l1_mes','agg_trans_v07_ind_l2_mes'
    ];
END;
$$ LANGUAGE plpgsql IMMUTABLE;


-- ─── 2) Apaga fisicamente as 7 tabelas (partições-filha caem junto) ────────
DROP TABLE IF EXISTS farol.agg_fat_v06_l1_mes;
DROP TABLE IF EXISTS farol.agg_fat_v06_l2_mes;
DROP TABLE IF EXISTS farol.agg_trans_v06_l1_mes;
DROP TABLE IF EXISTS farol.agg_trans_v06_l2_mes;
DROP TABLE IF EXISTS farol.agg_fat_mkt_produto_mes;
DROP TABLE IF EXISTS farol.agg_trans_mkt_cli_mes;
DROP TABLE IF EXISTS farol.agg_trans_mkt_produto_mes;

-- ─── 3) Para de escrever em V04 (mantém tabelas e partição, só corta INSERT) ──
-- Corpo idêntico ao da migration 226 (upsert_aggs_mes), removendo só os 6
-- blocos de INSERT de V04 (l0/l1/l2 × fat/trans).

CREATE OR REPLACE FUNCTION farol.upsert_aggs_mes(p_empresa_id uuid, p_ano integer, p_mes integer)
 RETURNS void
 LANGUAGE plpgsql
AS $function$
DECLARE
    p_ini    DATE := make_date(p_ano, p_mes, 1);
    p_fim    DATE := (p_ini + INTERVAL '1 month' - INTERVAL '1 day')::date;
BEGIN
    SET LOCAL work_mem = '256MB';

    -- ════════════════════ TEMP TABLE FATURADO (mês corrente) ═════════════════
    DROP TABLE IF EXISTS _v_fat;
    CREATE TEMP TABLE _v_fat ON COMMIT DROP AS
    SELECT
        v.empresa_id, v.cod_fornec, v.nome_fornec,
        v.cod_gerente, v.nome_gerente,
        v.cod_supervisor, v.nome_supervisor,
        v.cod_rca, v.nome_rca, v.qtcli_rca,
        v.cnpj, v.cod_cli, v.nome_cli, v.uf, v.empresa,
        v.cod_prod, v.nome_prod, v.ean,
        v.pvenda, v.plucro, v.qt
    FROM vendas_faturadas v
    WHERE v.empresa_id = p_empresa_id
      AND v.data_faturamento BETWEEN p_ini AND p_fim;

    CREATE INDEX ON _v_fat (cod_fornec);
    CREATE INDEX ON _v_fat (cod_gerente);
    CREATE INDEX ON _v_fat (cod_supervisor);
    CREATE INDEX ON _v_fat (cod_rca);
    CREATE INDEX ON _v_fat (cod_cli);
    CREATE INDEX ON _v_fat (cnpj);
    ANALYZE _v_fat;

    -- ════════════════════ FATURADO INSERTs ═══════════════════════════════════

    -- V01_l0 (Fornec) — base_cli = carteira total da empresa (penetração)
    INSERT INTO farol.agg_fat_v01_l0_mes AS t
        (empresa_id, ano, mes, cod_fornec, nome_fornec, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_fornec, MAX(v.nome_fornec),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_fat_carteira_rca c WHERE c.empresa_id = v.empresa_id),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v_fat v
    WHERE v.cod_fornec <> ''
    GROUP BY v.empresa_id, v.cod_fornec
    ON CONFLICT (ano, empresa_id, mes, cod_fornec) DO UPDATE SET
        nome_fornec = EXCLUDED.nome_fornec, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V01_l1 (+Gerente) — base_cli = carteira (Rotina 302) desta gerência (penetração)
    INSERT INTO farol.agg_fat_v01_l1_mes AS t
        (empresa_id, ano, mes, cod_fornec, cod_gerente, nome_gerente, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_fornec, v.cod_gerente, MAX(v.nome_gerente),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_fat_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_gerente = v.cod_gerente),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v_fat v
    WHERE v.cod_fornec <> '' AND v.cod_gerente <> ''
    GROUP BY v.empresa_id, v.cod_fornec, v.cod_gerente
    ON CONFLICT (ano, empresa_id, mes, cod_fornec, cod_gerente) DO UPDATE SET
        nome_gerente = EXCLUDED.nome_gerente, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V01_l2 (+Supervisor) — base_cli = carteira (Rotina 302) deste supervisor (penetração)
    INSERT INTO farol.agg_fat_v01_l2_mes AS t
        (empresa_id, ano, mes, cod_fornec, cod_gerente, cod_supervisor, nome_supervisor, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_fornec, v.cod_gerente, v.cod_supervisor, MAX(v.nome_supervisor),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_fat_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_supervisor = v.cod_supervisor),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v_fat v
    WHERE v.cod_fornec <> '' AND v.cod_gerente <> '' AND v.cod_supervisor <> ''
    GROUP BY v.empresa_id, v.cod_fornec, v.cod_gerente, v.cod_supervisor
    ON CONFLICT (ano, empresa_id, mes, cod_fornec, cod_gerente, cod_supervisor) DO UPDATE SET
        nome_supervisor = EXCLUDED.nome_supervisor, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V01_l3 (+RCA) — base_cli = carteira (Rotina 302) deste RCA (MAX qtcli_rca)
    INSERT INTO farol.agg_fat_v01_l3_mes AS t
        (empresa_id, ano, mes, cod_fornec, cod_gerente, cod_supervisor, cod_rca, nome_rca,
         base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_fornec, v.cod_gerente, v.cod_supervisor, v.cod_rca, MAX(v.nome_rca),
        MAX(v.qtcli_rca)::INT,
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v_fat v
    WHERE v.cod_fornec <> '' AND v.cod_gerente <> '' AND v.cod_supervisor <> '' AND v.cod_rca <> ''
    GROUP BY v.empresa_id, v.cod_fornec, v.cod_gerente, v.cod_supervisor, v.cod_rca
    ON CONFLICT (ano, empresa_id, mes, cod_fornec, cod_gerente, cod_supervisor, cod_rca) DO UPDATE SET
        nome_rca = EXCLUDED.nome_rca, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V01_l4 (+Cliente) — sem alteração: base_cli=1 por linha de cliente
    INSERT INTO farol.agg_fat_v01_l4_mes AS t
        (empresa_id, ano, mes, cod_fornec, cod_gerente, cod_supervisor, cod_rca, cnpj, cod_cli, nome_cli,
         base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cod_fornec, v.cod_gerente, v.cod_supervisor, v.cod_rca,
        v.cnpj, MAX(v.cod_cli), MAX(v.nome_cli),
        1,
        (CASE WHEN SUM(v.qt) > 0 THEN 1 ELSE 0 END)::INT,
        COUNT(DISTINCT v.cod_prod) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC,
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v_fat v
    WHERE v.cod_fornec <> '' AND v.cod_gerente <> '' AND v.cod_supervisor <> ''
      AND v.cod_rca <> '' AND v.cnpj <> ''
    GROUP BY v.empresa_id, v.cod_fornec, v.cod_gerente, v.cod_supervisor, v.cod_rca, v.cnpj
    ON CONFLICT (ano, empresa_id, mes, cod_fornec, cod_gerente, cod_supervisor, cod_rca, cnpj) DO UPDATE SET
        cod_cli = EXCLUDED.cod_cli, nome_cli = EXCLUDED.nome_cli,
        base_cli = EXCLUDED.base_cli, positivados = EXCLUDED.positivados,
        mix = EXCLUDED.mix, pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V02-V05 e restante: copiados de migration 166 sem alteração
    INSERT INTO farol.agg_fat_v02_l0_mes AS t
        (empresa_id, ano, mes, cod_supervisor, nome_supervisor, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_supervisor, MAX(v.nome_supervisor),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_fat_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_supervisor = v.cod_supervisor),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v_fat v WHERE v.cod_supervisor <> ''
    GROUP BY v.empresa_id, v.cod_supervisor
    ON CONFLICT (ano, empresa_id, mes, cod_supervisor) DO UPDATE SET
        nome_supervisor = EXCLUDED.nome_supervisor, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_fat_v02_l1_mes AS t
        (empresa_id, ano, mes, cod_supervisor, cod_rca, nome_rca, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_supervisor, v.cod_rca, MAX(v.nome_rca),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_fat_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_rca = v.cod_rca),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v_fat v WHERE v.cod_supervisor <> '' AND v.cod_rca <> ''
    GROUP BY v.empresa_id, v.cod_supervisor, v.cod_rca
    ON CONFLICT (ano, empresa_id, mes, cod_supervisor, cod_rca) DO UPDATE SET
        nome_rca = EXCLUDED.nome_rca, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_fat_v02_l2_mes AS t
        (empresa_id, ano, mes, cod_supervisor, cod_rca, cod_fornec, nome_fornec, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_supervisor, v.cod_rca, v.cod_fornec, MAX(v.nome_fornec),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_fat_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_rca = v.cod_rca),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v_fat v WHERE v.cod_supervisor <> '' AND v.cod_rca <> '' AND v.cod_fornec <> ''
    GROUP BY v.empresa_id, v.cod_supervisor, v.cod_rca, v.cod_fornec
    ON CONFLICT (ano, empresa_id, mes, cod_supervisor, cod_rca, cod_fornec) DO UPDATE SET
        nome_fornec = EXCLUDED.nome_fornec, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_fat_v02_l3_mes AS t
        (empresa_id, ano, mes, cod_supervisor, cod_rca, cod_fornec, cnpj, cod_cli, nome_cli,
         base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_supervisor, v.cod_rca, v.cod_fornec,
        v.cnpj, MAX(v.cod_cli), MAX(v.nome_cli),
        1, (CASE WHEN SUM(v.qt) > 0 THEN 1 ELSE 0 END)::INT,
        COUNT(DISTINCT v.cod_prod) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC,
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v_fat v WHERE v.cod_supervisor <> '' AND v.cod_rca <> '' AND v.cod_fornec <> '' AND v.cnpj <> ''
    GROUP BY v.empresa_id, v.cod_supervisor, v.cod_rca, v.cod_fornec, v.cnpj
    ON CONFLICT (ano, empresa_id, mes, cod_supervisor, cod_rca, cod_fornec, cnpj) DO UPDATE SET
        cod_cli = EXCLUDED.cod_cli, nome_cli = EXCLUDED.nome_cli,
        base_cli = EXCLUDED.base_cli, positivados = EXCLUDED.positivados,
        mix = EXCLUDED.mix, pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_fat_v03_l0_mes AS t
        (empresa_id, ano, mes, cod_gerente, nome_gerente, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_gerente, MAX(v.nome_gerente),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_fat_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_gerente = v.cod_gerente),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v_fat v WHERE v.cod_gerente <> ''
    GROUP BY v.empresa_id, v.cod_gerente
    ON CONFLICT (ano, empresa_id, mes, cod_gerente) DO UPDATE SET
        nome_gerente = EXCLUDED.nome_gerente, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_fat_v03_l1_mes AS t
        (empresa_id, ano, mes, cod_gerente, cod_supervisor, nome_supervisor, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_gerente, v.cod_supervisor, MAX(v.nome_supervisor),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_fat_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_supervisor = v.cod_supervisor),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v_fat v WHERE v.cod_gerente <> '' AND v.cod_supervisor <> ''
    GROUP BY v.empresa_id, v.cod_gerente, v.cod_supervisor
    ON CONFLICT (ano, empresa_id, mes, cod_gerente, cod_supervisor) DO UPDATE SET
        nome_supervisor = EXCLUDED.nome_supervisor, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_fat_v03_l2_mes AS t
        (empresa_id, ano, mes, cod_gerente, cod_supervisor, cod_rca, nome_rca, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_gerente, v.cod_supervisor, v.cod_rca, MAX(v.nome_rca),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_fat_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_rca = v.cod_rca),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v_fat v WHERE v.cod_gerente <> '' AND v.cod_supervisor <> '' AND v.cod_rca <> ''
    GROUP BY v.empresa_id, v.cod_gerente, v.cod_supervisor, v.cod_rca
    ON CONFLICT (ano, empresa_id, mes, cod_gerente, cod_supervisor, cod_rca) DO UPDATE SET
        nome_rca = EXCLUDED.nome_rca, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_fat_v03_l3_mes AS t
        (empresa_id, ano, mes, cod_gerente, cod_supervisor, cod_rca, cnpj, cod_cli, nome_cli,
         base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_gerente, v.cod_supervisor, v.cod_rca,
        v.cnpj, MAX(v.cod_cli), MAX(v.nome_cli),
        1, (CASE WHEN SUM(v.qt) > 0 THEN 1 ELSE 0 END)::INT,
        COUNT(DISTINCT v.cod_prod) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC,
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v_fat v WHERE v.cod_gerente <> '' AND v.cod_supervisor <> '' AND v.cod_rca <> '' AND v.cnpj <> ''
    GROUP BY v.empresa_id, v.cod_gerente, v.cod_supervisor, v.cod_rca, v.cnpj
    ON CONFLICT (ano, empresa_id, mes, cod_gerente, cod_supervisor, cod_rca, cnpj) DO UPDATE SET
        cod_cli = EXCLUDED.cod_cli, nome_cli = EXCLUDED.nome_cli,
        base_cli = EXCLUDED.base_cli, positivados = EXCLUDED.positivados,
        mix = EXCLUDED.mix, pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- agg_fat_v04_l0_mes / agg_fat_v04_l1_mes / agg_fat_v04_l2_mes REMOVIDAS
    -- em 10/09/2026 (mig 230): view V04 ("Por RCA") e roteavel via
    -- ?view=V04, mas nenhuma tela do frontend jamais oferece essa opcao
    -- (confirmado por grep em todo frontend/src) -- so recebia INSERT sem
    -- nenhum leitor. Tabelas mantidas (sem DROP nesta rodada, ver 230), so
    -- paramos de escrever nelas -- mesmo padrao ja usado na mig 226.

    -- DIMS fat
    INSERT INTO farol.agg_fat_dims_mes AS t (empresa_id, ano, mes, dim, key, label)
    SELECT empresa_id, p_ano, p_mes, 'fornec', cod_fornec, COALESCE(MAX(nome_fornec),'')
        FROM _v_fat WHERE cod_fornec <> '' GROUP BY empresa_id, cod_fornec
    UNION ALL
    SELECT empresa_id, p_ano, p_mes, 'gerente', cod_gerente, COALESCE(MAX(nome_gerente),'')
        FROM _v_fat WHERE cod_gerente <> '' GROUP BY empresa_id, cod_gerente
    UNION ALL
    SELECT empresa_id, p_ano, p_mes, 'supervisor', cod_supervisor, COALESCE(MAX(nome_supervisor),'')
        FROM _v_fat WHERE cod_supervisor <> '' GROUP BY empresa_id, cod_supervisor
    UNION ALL
    SELECT empresa_id, p_ano, p_mes, 'rca', cod_rca, COALESCE(MAX(nome_rca),'')
        FROM _v_fat WHERE cod_rca <> '' GROUP BY empresa_id, cod_rca
    UNION ALL
    SELECT empresa_id, p_ano, p_mes, 'cli', cod_cli, COALESCE(MAX(nome_cli),'')
        FROM _v_fat WHERE cod_cli <> '' GROUP BY empresa_id, cod_cli
    UNION ALL
    SELECT empresa_id, p_ano, p_mes, 'uf', uf, uf
        FROM _v_fat WHERE uf <> '' GROUP BY empresa_id, uf
    UNION ALL
    SELECT empresa_id, p_ano, p_mes, 'empresa', empresa, empresa
        FROM _v_fat WHERE empresa <> '' GROUP BY empresa_id, empresa
    ON CONFLICT (ano, empresa_id, mes, dim, key) DO UPDATE SET label = EXCLUDED.label;

    -- Marketing Cliente×mês
    INSERT INTO farol.agg_fat_mkt_cli_mes AS t
        (empresa_id, ano, mes, cnpj, cod_cli, nome_cli, positivados, mix, pvenda, plucro, qt)
    SELECT
        v.empresa_id, p_ano, p_mes, v.cnpj,
        MAX(v.cod_cli), MAX(v.nome_cli),
        (CASE WHEN SUM(v.qt) > 0 THEN 1 ELSE 0 END)::INT,
        COUNT(DISTINCT v.cod_prod) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC,
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v_fat v
    WHERE v.cnpj <> ''
    GROUP BY v.empresa_id, v.cnpj
    ON CONFLICT (ano, empresa_id, mes, cnpj) DO UPDATE SET
        cod_cli = EXCLUDED.cod_cli, nome_cli = EXCLUDED.nome_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- agg_fat_mkt_produto_mes REMOVIDA em 08/09/2026 (mig 226): nenhum handler
    -- lê essa tabela (fetchMktProduto nunca existiu / foi removido) — a tabela
    -- em si é mantida (sem DROP nesta rodada), só paramos de escrever nela.

    -- ════════════════════ TEMP TABLE TRANSMITIDO (mês corrente) ══════════════
    DROP TABLE IF EXISTS _v_trans;
    CREATE TEMP TABLE _v_trans ON COMMIT DROP AS
    SELECT
        v.empresa_id, v.cod_fornec, v.nome_fornec,
        v.cod_gerente, v.nome_gerente,
        v.cod_supervisor, v.nome_supervisor,
        v.cod_rca, v.nome_rca, v.qtcli_rca,
        v.cnpj, v.cod_cli, v.nome_cli, v.uf, v.empresa,
        v.cod_prod, v.nome_prod, v.ean,
        v.pvenda, v.plucro, v.qt
    FROM vendas_transmitidas v
    WHERE v.empresa_id = p_empresa_id
      AND v.data_transmissao BETWEEN p_ini AND p_fim;

    CREATE INDEX ON _v_trans (cod_fornec);
    CREATE INDEX ON _v_trans (cod_gerente);
    CREATE INDEX ON _v_trans (cod_supervisor);
    CREATE INDEX ON _v_trans (cod_rca);
    CREATE INDEX ON _v_trans (cod_cli);
    CREATE INDEX ON _v_trans (cnpj);
    ANALYZE _v_trans;

    -- ════════════════════ TRANSMITIDO INSERTs ════════════════════════════════

    -- V01_l0 trans
    INSERT INTO farol.agg_trans_v01_l0_mes AS t
        (empresa_id, ano, mes, cod_fornec, nome_fornec, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_fornec, MAX(v.nome_fornec),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_trans_carteira_rca c WHERE c.empresa_id = v.empresa_id),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v_trans v WHERE v.cod_fornec <> ''
    GROUP BY v.empresa_id, v.cod_fornec
    ON CONFLICT (ano, empresa_id, mes, cod_fornec) DO UPDATE SET
        nome_fornec = EXCLUDED.nome_fornec, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V01_l1 trans
    INSERT INTO farol.agg_trans_v01_l1_mes AS t
        (empresa_id, ano, mes, cod_fornec, cod_gerente, nome_gerente, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_fornec, v.cod_gerente, MAX(v.nome_gerente),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_trans_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_gerente = v.cod_gerente),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v_trans v WHERE v.cod_fornec <> '' AND v.cod_gerente <> ''
    GROUP BY v.empresa_id, v.cod_fornec, v.cod_gerente
    ON CONFLICT (ano, empresa_id, mes, cod_fornec, cod_gerente) DO UPDATE SET
        nome_gerente = EXCLUDED.nome_gerente, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V01_l2 trans
    INSERT INTO farol.agg_trans_v01_l2_mes AS t
        (empresa_id, ano, mes, cod_fornec, cod_gerente, cod_supervisor, nome_supervisor, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_fornec, v.cod_gerente, v.cod_supervisor, MAX(v.nome_supervisor),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_trans_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_supervisor = v.cod_supervisor),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v_trans v WHERE v.cod_fornec <> '' AND v.cod_gerente <> '' AND v.cod_supervisor <> ''
    GROUP BY v.empresa_id, v.cod_fornec, v.cod_gerente, v.cod_supervisor
    ON CONFLICT (ano, empresa_id, mes, cod_fornec, cod_gerente, cod_supervisor) DO UPDATE SET
        nome_supervisor = EXCLUDED.nome_supervisor, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V01_l3 trans
    INSERT INTO farol.agg_trans_v01_l3_mes AS t
        (empresa_id, ano, mes, cod_fornec, cod_gerente, cod_supervisor, cod_rca, nome_rca,
         base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_fornec, v.cod_gerente, v.cod_supervisor, v.cod_rca, MAX(v.nome_rca),
        MAX(v.qtcli_rca)::INT,
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v_trans v WHERE v.cod_fornec <> '' AND v.cod_gerente <> '' AND v.cod_supervisor <> '' AND v.cod_rca <> ''
    GROUP BY v.empresa_id, v.cod_fornec, v.cod_gerente, v.cod_supervisor, v.cod_rca
    ON CONFLICT (ano, empresa_id, mes, cod_fornec, cod_gerente, cod_supervisor, cod_rca) DO UPDATE SET
        nome_rca = EXCLUDED.nome_rca, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- V01_l4 trans
    INSERT INTO farol.agg_trans_v01_l4_mes AS t
        (empresa_id, ano, mes, cod_fornec, cod_gerente, cod_supervisor, cod_rca, cnpj, cod_cli, nome_cli,
         base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_fornec, v.cod_gerente, v.cod_supervisor, v.cod_rca,
        v.cnpj, MAX(v.cod_cli), MAX(v.nome_cli),
        1, (CASE WHEN SUM(v.qt) > 0 THEN 1 ELSE 0 END)::INT,
        COUNT(DISTINCT v.cod_prod) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC,
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v_trans v WHERE v.cod_fornec <> '' AND v.cod_gerente <> '' AND v.cod_supervisor <> ''
      AND v.cod_rca <> '' AND v.cnpj <> ''
    GROUP BY v.empresa_id, v.cod_fornec, v.cod_gerente, v.cod_supervisor, v.cod_rca, v.cnpj
    ON CONFLICT (ano, empresa_id, mes, cod_fornec, cod_gerente, cod_supervisor, cod_rca, cnpj) DO UPDATE SET
        cod_cli = EXCLUDED.cod_cli, nome_cli = EXCLUDED.nome_cli,
        base_cli = EXCLUDED.base_cli, positivados = EXCLUDED.positivados,
        mix = EXCLUDED.mix, pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_trans_v02_l0_mes AS t
        (empresa_id, ano, mes, cod_supervisor, nome_supervisor, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_supervisor, MAX(v.nome_supervisor),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_trans_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_supervisor = v.cod_supervisor),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v_trans v WHERE v.cod_supervisor <> ''
    GROUP BY v.empresa_id, v.cod_supervisor
    ON CONFLICT (ano, empresa_id, mes, cod_supervisor) DO UPDATE SET
        nome_supervisor = EXCLUDED.nome_supervisor, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_trans_v02_l1_mes AS t
        (empresa_id, ano, mes, cod_supervisor, cod_rca, nome_rca, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_supervisor, v.cod_rca, MAX(v.nome_rca),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_trans_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_rca = v.cod_rca),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v_trans v WHERE v.cod_supervisor <> '' AND v.cod_rca <> ''
    GROUP BY v.empresa_id, v.cod_supervisor, v.cod_rca
    ON CONFLICT (ano, empresa_id, mes, cod_supervisor, cod_rca) DO UPDATE SET
        nome_rca = EXCLUDED.nome_rca, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_trans_v02_l2_mes AS t
        (empresa_id, ano, mes, cod_supervisor, cod_rca, cod_fornec, nome_fornec, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_supervisor, v.cod_rca, v.cod_fornec, MAX(v.nome_fornec),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_trans_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_rca = v.cod_rca),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v_trans v WHERE v.cod_supervisor <> '' AND v.cod_rca <> '' AND v.cod_fornec <> ''
    GROUP BY v.empresa_id, v.cod_supervisor, v.cod_rca, v.cod_fornec
    ON CONFLICT (ano, empresa_id, mes, cod_supervisor, cod_rca, cod_fornec) DO UPDATE SET
        nome_fornec = EXCLUDED.nome_fornec, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_trans_v02_l3_mes AS t
        (empresa_id, ano, mes, cod_supervisor, cod_rca, cod_fornec, cnpj, cod_cli, nome_cli,
         base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_supervisor, v.cod_rca, v.cod_fornec,
        v.cnpj, MAX(v.cod_cli), MAX(v.nome_cli),
        1, (CASE WHEN SUM(v.qt) > 0 THEN 1 ELSE 0 END)::INT,
        COUNT(DISTINCT v.cod_prod) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC,
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v_trans v WHERE v.cod_supervisor <> '' AND v.cod_rca <> '' AND v.cod_fornec <> '' AND v.cnpj <> ''
    GROUP BY v.empresa_id, v.cod_supervisor, v.cod_rca, v.cod_fornec, v.cnpj
    ON CONFLICT (ano, empresa_id, mes, cod_supervisor, cod_rca, cod_fornec, cnpj) DO UPDATE SET
        cod_cli = EXCLUDED.cod_cli, nome_cli = EXCLUDED.nome_cli,
        base_cli = EXCLUDED.base_cli, positivados = EXCLUDED.positivados,
        mix = EXCLUDED.mix, pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_trans_v03_l0_mes AS t
        (empresa_id, ano, mes, cod_gerente, nome_gerente, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_gerente, MAX(v.nome_gerente),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_trans_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_gerente = v.cod_gerente),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v_trans v WHERE v.cod_gerente <> ''
    GROUP BY v.empresa_id, v.cod_gerente
    ON CONFLICT (ano, empresa_id, mes, cod_gerente) DO UPDATE SET
        nome_gerente = EXCLUDED.nome_gerente, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_trans_v03_l1_mes AS t
        (empresa_id, ano, mes, cod_gerente, cod_supervisor, nome_supervisor, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_gerente, v.cod_supervisor, MAX(v.nome_supervisor),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_trans_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_supervisor = v.cod_supervisor),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v_trans v WHERE v.cod_gerente <> '' AND v.cod_supervisor <> ''
    GROUP BY v.empresa_id, v.cod_gerente, v.cod_supervisor
    ON CONFLICT (ano, empresa_id, mes, cod_gerente, cod_supervisor) DO UPDATE SET
        nome_supervisor = EXCLUDED.nome_supervisor, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_trans_v03_l2_mes AS t
        (empresa_id, ano, mes, cod_gerente, cod_supervisor, cod_rca, nome_rca, base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_gerente, v.cod_supervisor, v.cod_rca, MAX(v.nome_rca),
        (SELECT COALESCE(SUM(c.qtcli_rca),0)::INT FROM farol.mv_trans_carteira_rca c
          WHERE c.empresa_id = v.empresa_id AND c.cod_rca = v.cod_rca),
        COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0)::INT,
        COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC
            / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0), 0)::NUMERIC, 0),
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v_trans v WHERE v.cod_gerente <> '' AND v.cod_supervisor <> '' AND v.cod_rca <> ''
    GROUP BY v.empresa_id, v.cod_gerente, v.cod_supervisor, v.cod_rca
    ON CONFLICT (ano, empresa_id, mes, cod_gerente, cod_supervisor, cod_rca) DO UPDATE SET
        nome_rca = EXCLUDED.nome_rca, base_cli = EXCLUDED.base_cli,
        positivados = EXCLUDED.positivados, mix = EXCLUDED.mix,
        pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    INSERT INTO farol.agg_trans_v03_l3_mes AS t
        (empresa_id, ano, mes, cod_gerente, cod_supervisor, cod_rca, cnpj, cod_cli, nome_cli,
         base_cli, positivados, mix, pvenda, plucro, qt)
    SELECT v.empresa_id, p_ano, p_mes, v.cod_gerente, v.cod_supervisor, v.cod_rca,
        v.cnpj, MAX(v.cod_cli), MAX(v.nome_cli),
        1, (CASE WHEN SUM(v.qt) > 0 THEN 1 ELSE 0 END)::INT,
        COUNT(DISTINCT v.cod_prod) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::NUMERIC,
        SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
    FROM _v_trans v WHERE v.cod_gerente <> '' AND v.cod_supervisor <> '' AND v.cod_rca <> '' AND v.cnpj <> ''
    GROUP BY v.empresa_id, v.cod_gerente, v.cod_supervisor, v.cod_rca, v.cnpj
    ON CONFLICT (ano, empresa_id, mes, cod_gerente, cod_supervisor, cod_rca, cnpj) DO UPDATE SET
        cod_cli = EXCLUDED.cod_cli, nome_cli = EXCLUDED.nome_cli,
        base_cli = EXCLUDED.base_cli, positivados = EXCLUDED.positivados,
        mix = EXCLUDED.mix, pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;

    -- agg_trans_v04_l0_mes / agg_trans_v04_l1_mes / agg_trans_v04_l2_mes
    -- REMOVIDAS em 10/09/2026 (mig 230) -- mesmo motivo do bloco FATURADO
    -- acima.

    -- DIMS trans
    INSERT INTO farol.agg_trans_dims_mes AS t (empresa_id, ano, mes, dim, key, label)
    SELECT empresa_id, p_ano, p_mes, 'fornec', cod_fornec, COALESCE(MAX(nome_fornec),'')
        FROM _v_trans WHERE cod_fornec <> '' GROUP BY empresa_id, cod_fornec
    UNION ALL
    SELECT empresa_id, p_ano, p_mes, 'gerente', cod_gerente, COALESCE(MAX(nome_gerente),'')
        FROM _v_trans WHERE cod_gerente <> '' GROUP BY empresa_id, cod_gerente
    UNION ALL
    SELECT empresa_id, p_ano, p_mes, 'supervisor', cod_supervisor, COALESCE(MAX(nome_supervisor),'')
        FROM _v_trans WHERE cod_supervisor <> '' GROUP BY empresa_id, cod_supervisor
    UNION ALL
    SELECT empresa_id, p_ano, p_mes, 'rca', cod_rca, COALESCE(MAX(nome_rca),'')
        FROM _v_trans WHERE cod_rca <> '' GROUP BY empresa_id, cod_rca
    UNION ALL
    SELECT empresa_id, p_ano, p_mes, 'cli', cod_cli, COALESCE(MAX(nome_cli),'')
        FROM _v_trans WHERE cod_cli <> '' GROUP BY empresa_id, cod_cli
    UNION ALL
    SELECT empresa_id, p_ano, p_mes, 'uf', uf, uf
        FROM _v_trans WHERE uf <> '' GROUP BY empresa_id, uf
    UNION ALL
    SELECT empresa_id, p_ano, p_mes, 'empresa', empresa, empresa
        FROM _v_trans WHERE empresa <> '' GROUP BY empresa_id, empresa
    ON CONFLICT (ano, empresa_id, mes, dim, key) DO UPDATE SET label = EXCLUDED.label;

    -- agg_trans_mkt_cli_mes / agg_trans_mkt_produto_mes REMOVIDAS em 08/09/2026
    -- (mig 226): nenhum handler lê essas tabelas — só a versão FAT
    -- (agg_fat_mkt_cli_mes) tem consumidor real (farol_cnpj_receita.go). As
    -- tabelas em si são mantidas (sem DROP nesta rodada), só paramos de
    -- escrever nelas.

    -- ════════════════════════════════════════════════════════════════════════
    -- V05 (Por Fornecedor sob supervisor) — função auxiliar separada.
    -- Restaurando chamada perdida na 169 (vide cabeçalho desta migration).
    -- ════════════════════════════════════════════════════════════════════════
    PERFORM farol.upsert_aggs_mes_v05(p_empresa_id, p_ano, p_mes);

END;
$function$

;

