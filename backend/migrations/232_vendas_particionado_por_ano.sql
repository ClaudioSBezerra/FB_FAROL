-- 232_vendas_particionado_por_ano.sql
-- ════════════════════════════════════════════════════════════════════════════
-- Torna "particionada por RANGE(data), uma partição por ANO" a estrutura
-- DEFINITIVA de vendas_faturadas / vendas_transmitidas — não só o resultado de
-- um script manual rodado uma vez. A partir daqui, qualquer banco novo (clone
-- de dev, Postgres descartável de teste, rebuild do zero em DR) nasce com a
-- mesma estrutura que produção.
--
-- Contexto (pedido do Claudio, 10/09/2026): política de retenção rolante de 2
-- anos — manter ano corrente + ano anterior; em fevereiro de cada ano, apagar
-- o ano mais antigo. Com partição por ano, esse purge vira
--   ALTER TABLE ... DETACH PARTITION vendas_faturadas_2025; DROP TABLE ...;
-- — instantâneo, sem bloat, sem VACUUM gigante.
--
-- ─── Idempotência (a migration roda no boot de TODO deploy) ───
--   • já particionada           → NO-OP (produção, depois do reset manual)
--   • vazia e não particionada   → converte (banco novo / disposable / rebuild)
--   • COM linhas e não part.     → NÃO faz nada, só emite WARNING dizendo pra
--     rodar scripts/reset_base_completo.sh manualmente. NÃO lança exceção: um
--     RAISE aqui derrubaria o boot da aplicação em produção no primeiro deploy
--     que trouxesse esta migration (quando as tabelas ainda têm os ~49GB).
--
-- A função farol._rebuild_vendas_particionado() abaixo é a ÚNICA fonte da
-- verdade da estrutura — scripts/reset_base_completo.sh também a chama (depois
-- do operador confirmar "APAGAR TUDO"), em vez de duplicar 200 linhas de DDL.
--
-- Schema conferido em 10/09/2026 contra o banco real (42 colunas, 18 índices
-- por tabela, FK empresa_id→companies, PK só (id)). A PK passa a ser
-- (id, data_*) — regra do Postgres: a coluna de partição TEM que estar em
-- qualquer UNIQUE/PK de tabela particionada. Nada no Go faz RETURNING id nem
-- JOIN por vendas_*.id (conferido por grep), então incluir a data na PK é
-- inócuo.
--
-- As 3 MATERIALIZED VIEWs que dependem dessas tabelas (mv_fat_carteira_rca,
-- mv_trans_carteira_rca, mv_fat_uf_mes) são dropadas e recriadas junto —
-- definição idêntica às migrations 161 e 194, nunca alteradas desde então.
-- ════════════════════════════════════════════════════════════════════════════

CREATE OR REPLACE FUNCTION farol._rebuild_vendas_particionado() RETURNS void AS $rebuild$
BEGIN
    -- ── 1. Derruba dependentes e as tabelas cruas ────────────────────────
    -- As MVs dependem das tabelas por OID — têm que sair ANTES do DROP TABLE.
    DROP MATERIALIZED VIEW IF EXISTS farol.mv_fat_uf_mes;
    DROP MATERIALIZED VIEW IF EXISTS farol.mv_fat_carteira_rca;
    DROP MATERIALIZED VIEW IF EXISTS farol.mv_trans_carteira_rca;

    DROP TABLE IF EXISTS vendas_faturadas;
    DROP TABLE IF EXISTS vendas_transmitidas;

    -- ── 2. vendas_faturadas particionada por data_faturamento ────────────
    CREATE TABLE vendas_faturadas (
        id               bigserial,
        empresa_id       uuid NOT NULL,
        data_faturamento date NOT NULL,
        cod_gerente      text NOT NULL DEFAULT '',
        nome_gerente     text NOT NULL DEFAULT '',
        cod_supervisor   text NOT NULL DEFAULT '',
        nome_supervisor  text NOT NULL DEFAULT '',
        qtrca_supervisor integer NOT NULL DEFAULT 0,
        cod_rca          text NOT NULL DEFAULT '',
        nome_rca         text NOT NULL DEFAULT '',
        qtcli_rca        integer NOT NULL DEFAULT 0,
        cod_fornec       text NOT NULL DEFAULT '',
        nome_fornec      text NOT NULL DEFAULT '',
        cod_cli          text NOT NULL DEFAULT '',
        nome_cli         text NOT NULL DEFAULT '',
        uf               text NOT NULL DEFAULT '',
        empresa          text NOT NULL DEFAULT '',
        cod_prod         text NOT NULL DEFAULT '',
        nome_prod        text NOT NULL DEFAULT '',
        ean              text NOT NULL DEFAULT '',
        qt               numeric NOT NULL DEFAULT 0,
        pvenda           numeric NOT NULL DEFAULT 0,
        plucro           numeric NOT NULL DEFAULT 0,
        importado_em     timestamptz NOT NULL DEFAULT now(),
        cnpj             text NOT NULL DEFAULT '',
        cod_ramo         text NOT NULL DEFAULT '',
        ramo             text NOT NULL DEFAULT '',
        embalagem        text NOT NULL DEFAULT '',
        qt_unit          numeric NOT NULL DEFAULT 0,
        qt_unit_cx       numeric NOT NULL DEFAULT 0,
        cod_bar          text NOT NULL DEFAULT '',
        cod_depto        text NOT NULL DEFAULT '',
        depto            text NOT NULL DEFAULT '',
        cod_sec          text NOT NULL DEFAULT '',
        secao            text NOT NULL DEFAULT '',
        cod_categoria    text NOT NULL DEFAULT '',
        categoria        text NOT NULL DEFAULT '',
        cod_cliprinc     text NOT NULL DEFAULT '',
        fantasia         text NOT NULL DEFAULT '',
        pvenda_unit      numeric NOT NULL DEFAULT 0,
        tipo_venda       text NOT NULL DEFAULT '',
        desc_condvenda   text NOT NULL DEFAULT '',
        PRIMARY KEY (id, data_faturamento)
    ) PARTITION BY RANGE (data_faturamento);

    ALTER TABLE vendas_faturadas
        ADD CONSTRAINT vendas_faturadas_empresa_id_fkey
        FOREIGN KEY (empresa_id) REFERENCES companies(id) ON DELETE CASCADE;

    -- 18 índices — cópia fiel de produção (10/09/2026). Os 2 de filial
    -- (idx_vf_filial/idx_vt_filial) NÃO entram aqui de propósito: em produção
    -- são criados CONCURRENTLY só depois da recarga inteira (CREATE INDEX
    -- durante o COPY encarece a importação) — ver reset_base_completo.sh.
    CREATE INDEX idx_vf_cliprinc ON vendas_faturadas (empresa_id, cod_cliprinc, data_faturamento) WHERE cod_cliprinc <> '';
    CREATE INDEX idx_vf_data_fornec ON vendas_faturadas (empresa_id, data_faturamento, cod_fornec) WHERE cod_fornec <> '';
    CREATE INDEX idx_vf_data_ger ON vendas_faturadas (empresa_id, data_faturamento, cod_gerente) WHERE cod_gerente <> '';
    CREATE INDEX idx_vf_data_rca ON vendas_faturadas (empresa_id, data_faturamento, cod_rca) WHERE cod_rca <> '';
    CREATE INDEX idx_vf_data_sup ON vendas_faturadas (empresa_id, data_faturamento, cod_supervisor) WHERE cod_supervisor <> '';
    CREATE INDEX idx_vf_emp_cli_data ON vendas_faturadas (empresa_id, cod_cli, data_faturamento);
    CREATE INDEX idx_vf_emp_cnpj ON vendas_faturadas (empresa_id, cnpj);
    CREATE INDEX idx_vf_emp_data ON vendas_faturadas (empresa_id, data_faturamento);
    CREATE INDEX idx_vf_emp_data_produto ON vendas_faturadas (empresa_id, data_faturamento, cod_prod) WHERE cod_prod <> '';
    CREATE INDEX idx_vf_emp_fornec ON vendas_faturadas (empresa_id, cod_fornec);
    CREATE INDEX idx_vf_emp_rca ON vendas_faturadas (empresa_id, cod_rca);
    CREATE INDEX idx_vf_emp_supervisor ON vendas_faturadas (empresa_id, cod_supervisor);
    CREATE INDEX idx_vf_mixtotal_fornec ON vendas_faturadas (empresa_id, data_faturamento, cod_fornec, cod_prod) WHERE qt > 0 AND cod_prod <> '' AND cod_fornec <> '';
    CREATE INDEX idx_vf_mixtotal_gerente ON vendas_faturadas (empresa_id, data_faturamento, cod_gerente, cod_prod) WHERE qt > 0 AND cod_prod <> '' AND cod_gerente <> '';
    CREATE INDEX idx_vf_mixtotal_rca ON vendas_faturadas (empresa_id, data_faturamento, cod_rca, cod_prod) WHERE qt > 0 AND cod_prod <> '' AND cod_rca <> '';
    CREATE INDEX idx_vf_mixtotal_supervisor ON vendas_faturadas (empresa_id, data_faturamento, cod_supervisor, cod_prod) WHERE qt > 0 AND cod_prod <> '' AND cod_supervisor <> '';
    CREATE INDEX idx_vf_tipo_venda ON vendas_faturadas (empresa_id, data_faturamento, tipo_venda) WHERE tipo_venda <> '';
    CREATE INDEX idx_vf_uf ON vendas_faturadas (empresa_id, uf, data_faturamento) WHERE uf <> '';

    -- ── 3. vendas_transmitidas — mesma estrutura, coluna de data diferente ──
    CREATE TABLE vendas_transmitidas (
        id               bigserial,
        empresa_id       uuid NOT NULL,
        data_transmissao date NOT NULL,
        cod_gerente      text NOT NULL DEFAULT '',
        nome_gerente     text NOT NULL DEFAULT '',
        cod_supervisor   text NOT NULL DEFAULT '',
        nome_supervisor  text NOT NULL DEFAULT '',
        qtrca_supervisor integer NOT NULL DEFAULT 0,
        cod_rca          text NOT NULL DEFAULT '',
        nome_rca         text NOT NULL DEFAULT '',
        qtcli_rca        integer NOT NULL DEFAULT 0,
        cod_fornec       text NOT NULL DEFAULT '',
        nome_fornec      text NOT NULL DEFAULT '',
        cod_cli          text NOT NULL DEFAULT '',
        nome_cli         text NOT NULL DEFAULT '',
        uf               text NOT NULL DEFAULT '',
        empresa          text NOT NULL DEFAULT '',
        cod_prod         text NOT NULL DEFAULT '',
        nome_prod        text NOT NULL DEFAULT '',
        ean              text NOT NULL DEFAULT '',
        qt               numeric NOT NULL DEFAULT 0,
        pvenda           numeric NOT NULL DEFAULT 0,
        plucro           numeric NOT NULL DEFAULT 0,
        importado_em     timestamptz NOT NULL DEFAULT now(),
        cnpj             text NOT NULL DEFAULT '',
        cod_ramo         text NOT NULL DEFAULT '',
        ramo             text NOT NULL DEFAULT '',
        embalagem        text NOT NULL DEFAULT '',
        qt_unit          numeric NOT NULL DEFAULT 0,
        qt_unit_cx       numeric NOT NULL DEFAULT 0,
        cod_bar          text NOT NULL DEFAULT '',
        cod_depto        text NOT NULL DEFAULT '',
        depto            text NOT NULL DEFAULT '',
        cod_sec          text NOT NULL DEFAULT '',
        secao            text NOT NULL DEFAULT '',
        cod_categoria    text NOT NULL DEFAULT '',
        categoria        text NOT NULL DEFAULT '',
        cod_cliprinc     text NOT NULL DEFAULT '',
        fantasia         text NOT NULL DEFAULT '',
        pvenda_unit      numeric NOT NULL DEFAULT 0,
        tipo_venda       text NOT NULL DEFAULT '',
        desc_condvenda   text NOT NULL DEFAULT '',
        PRIMARY KEY (id, data_transmissao)
    ) PARTITION BY RANGE (data_transmissao);

    ALTER TABLE vendas_transmitidas
        ADD CONSTRAINT vendas_transmitidas_empresa_id_fkey
        FOREIGN KEY (empresa_id) REFERENCES companies(id) ON DELETE CASCADE;

    CREATE INDEX idx_vt_cliprinc ON vendas_transmitidas (empresa_id, cod_cliprinc, data_transmissao) WHERE cod_cliprinc <> '';
    CREATE INDEX idx_vt_data_fornec ON vendas_transmitidas (empresa_id, data_transmissao, cod_fornec) WHERE cod_fornec <> '';
    CREATE INDEX idx_vt_data_ger ON vendas_transmitidas (empresa_id, data_transmissao, cod_gerente) WHERE cod_gerente <> '';
    CREATE INDEX idx_vt_data_rca ON vendas_transmitidas (empresa_id, data_transmissao, cod_rca) WHERE cod_rca <> '';
    CREATE INDEX idx_vt_data_sup ON vendas_transmitidas (empresa_id, data_transmissao, cod_supervisor) WHERE cod_supervisor <> '';
    CREATE INDEX idx_vt_emp_cli_data ON vendas_transmitidas (empresa_id, cod_cli, data_transmissao);
    CREATE INDEX idx_vt_emp_cnpj ON vendas_transmitidas (empresa_id, cnpj);
    CREATE INDEX idx_vt_emp_data ON vendas_transmitidas (empresa_id, data_transmissao);
    CREATE INDEX idx_vt_emp_data_produto ON vendas_transmitidas (empresa_id, data_transmissao, cod_prod) WHERE cod_prod <> '';
    CREATE INDEX idx_vt_emp_fornec ON vendas_transmitidas (empresa_id, cod_fornec);
    CREATE INDEX idx_vt_emp_rca ON vendas_transmitidas (empresa_id, cod_rca);
    CREATE INDEX idx_vt_emp_supervisor ON vendas_transmitidas (empresa_id, cod_supervisor);
    CREATE INDEX idx_vt_mixtotal_fornec ON vendas_transmitidas (empresa_id, data_transmissao, cod_fornec, cod_prod) WHERE qt > 0 AND cod_prod <> '' AND cod_fornec <> '';
    CREATE INDEX idx_vt_mixtotal_gerente ON vendas_transmitidas (empresa_id, data_transmissao, cod_gerente, cod_prod) WHERE qt > 0 AND cod_prod <> '' AND cod_gerente <> '';
    CREATE INDEX idx_vt_mixtotal_rca ON vendas_transmitidas (empresa_id, data_transmissao, cod_rca, cod_prod) WHERE qt > 0 AND cod_prod <> '' AND cod_rca <> '';
    CREATE INDEX idx_vt_mixtotal_supervisor ON vendas_transmitidas (empresa_id, data_transmissao, cod_supervisor, cod_prod) WHERE qt > 0 AND cod_prod <> '' AND cod_supervisor <> '';
    CREATE INDEX idx_vt_tipo_venda ON vendas_transmitidas (empresa_id, data_transmissao, tipo_venda) WHERE tipo_venda <> '';
    CREATE INDEX idx_vt_uf ON vendas_transmitidas (empresa_id, uf, data_transmissao) WHERE uf <> '';

    -- ── 4. Partições de ano-1 até ano+2 (não hardcoda ano nenhum) ────────
    PERFORM farol.ensure_vendas_ano_partition(g)
    FROM generate_series(
        EXTRACT(YEAR FROM CURRENT_DATE)::int - 1,
        EXTRACT(YEAR FROM CURRENT_DATE)::int + 2
    ) AS g;

    -- ── 5. Recria as 3 MVs (idêntico às migrations 161 e 194) ────────────
    CREATE MATERIALIZED VIEW farol.mv_fat_carteira_rca AS
    SELECT empresa_id, cod_rca,
           MAX(cod_gerente)    AS cod_gerente,
           MAX(cod_supervisor) AS cod_supervisor,
           MAX(qtcli_rca)      AS qtcli_rca
    FROM vendas_faturadas
    WHERE cod_rca <> ''
    GROUP BY empresa_id, cod_rca;
    CREATE UNIQUE INDEX idx_mvfatcart_pk  ON farol.mv_fat_carteira_rca (empresa_id, cod_rca);
    CREATE INDEX        idx_mvfatcart_ger ON farol.mv_fat_carteira_rca (empresa_id, cod_gerente);
    CREATE INDEX        idx_mvfatcart_sup ON farol.mv_fat_carteira_rca (empresa_id, cod_supervisor);

    CREATE MATERIALIZED VIEW farol.mv_trans_carteira_rca AS
    SELECT empresa_id, cod_rca,
           MAX(cod_gerente)    AS cod_gerente,
           MAX(cod_supervisor) AS cod_supervisor,
           MAX(qtcli_rca)      AS qtcli_rca
    FROM vendas_transmitidas
    WHERE cod_rca <> ''
    GROUP BY empresa_id, cod_rca;
    CREATE UNIQUE INDEX idx_mvtranscart_pk  ON farol.mv_trans_carteira_rca (empresa_id, cod_rca);
    CREATE INDEX        idx_mvtranscart_ger ON farol.mv_trans_carteira_rca (empresa_id, cod_gerente);
    CREATE INDEX        idx_mvtranscart_sup ON farol.mv_trans_carteira_rca (empresa_id, cod_supervisor);

    CREATE MATERIALIZED VIEW farol.mv_fat_uf_mes AS
    WITH un AS (
        SELECT empresa_id,
               COALESCE(NULLIF(uf, ''), '—')            AS uf,
               EXTRACT(YEAR  FROM data_faturamento)::int AS ano,
               EXTRACT(MONTH FROM data_faturamento)::int AS mes,
               tipo_venda,
               ''::text                                 AS evento,
               pvenda
          FROM vendas_faturadas
        UNION ALL
        SELECT empresa_id,
               COALESCE(NULLIF(uf, ''), '—'),
               EXTRACT(YEAR  FROM data_evento)::int,
               EXTRACT(MONTH FROM data_evento)::int,
               ''::text                                 AS tipo_venda,
               evento,
               pvenda
          FROM vendas_ccd
         WHERE evento IN ('DEVOLVIDO', 'CANCELADO')
    )
    SELECT empresa_id, uf, ano, mes,
        COALESCE(SUM(pvenda) FILTER (WHERE evento = '' AND tipo_venda IN ('1','4','7','8','9','11','14','20')), 0)
          - COALESCE(SUM(pvenda) FILTER (WHERE evento = 'DEVOLVIDO'), 0)
          - COALESCE(SUM(pvenda) FILTER (WHERE evento = 'CANCELADO'), 0) AS liquido,
        COALESCE(SUM(pvenda) FILTER (WHERE evento = ''), 0)              AS bruto
    FROM un
    GROUP BY empresa_id, uf, ano, mes;
    CREATE UNIQUE INDEX mv_fat_uf_mes_pk ON farol.mv_fat_uf_mes (empresa_id, uf, ano, mes);
    COMMENT ON MATERIALIZED VIEW farol.mv_fat_uf_mes IS
      'Faturado líquido por UF (do cliente) × mês, mesma fórmula da mig 190. Fonte do bloco "Faturado por UF" do Painel BI. Refresh junto do upsert das agg.';

    -- ── 6. Grants de leitura — as tabelas antigas os tinham; o DROP+recria
    -- os perdeu. Guardados por pg_roles: em banco novo/disposable esses roles
    -- não existem e o GRANT direto abortaria a migration. Nenhuma migration
    -- anterior mexe em grant (é config de infra), então re-conceder aqui é o
    -- ponto certo pra não deixar farol_ro/cerebro_readonly sem acesso depois
    -- de um rebuild.
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'farol_ro') THEN
        EXECUTE 'GRANT SELECT ON vendas_faturadas, vendas_transmitidas TO farol_ro';
    END IF;
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'cerebro_readonly') THEN
        EXECUTE 'GRANT SELECT ON vendas_faturadas, vendas_transmitidas TO cerebro_readonly';
    END IF;
END;
$rebuild$ LANGUAGE plpgsql;


-- ─── Chamada guardada: só converte quando é seguro ───────────────────────────
DO $guard$
DECLARE
    v_particionada boolean;
    v_linhas       bigint;
BEGIN
    SELECT EXISTS (
        SELECT 1 FROM pg_partitioned_table pt
        JOIN pg_class c ON c.oid = pt.partrelid
        WHERE c.relname = 'vendas_faturadas'
    ) INTO v_particionada;

    IF v_particionada THEN
        RAISE NOTICE '232: vendas_faturadas já é particionada — nada a fazer.';
        RETURN;
    END IF;

    EXECUTE 'SELECT count(*) FROM vendas_faturadas' INTO v_linhas;
    IF v_linhas > 0 THEN
        RAISE WARNING '232: vendas_faturadas tem % linhas e ainda NÃO é particionada. '
            'Esta migration NÃO converte tabela com dado (evita destruição automática). '
            'Rode scripts/reset_base_completo.sh manualmente para migrar a estrutura.', v_linhas;
        RETURN;
    END IF;

    PERFORM farol._rebuild_vendas_particionado();
    RAISE NOTICE '232: vendas_faturadas / vendas_transmitidas convertidas para particionadas por ano.';
END;
$guard$;
