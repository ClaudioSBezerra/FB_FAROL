-- 251_recria_gestores_rcas.sql — recria gestores/rcas/gestor_rca.
--
-- Achado real 2026-10-07: essas tabelas (migration 121 + 122) não existem
-- em produção. schema_migrations já marca 121/122 como executadas — o
-- "schema limpo" da Reescrita 2026 (ver CLAUDE.md) aparentemente as
-- descartou sem reexecutar as migrations, e como o runner pula migration
-- já marcada, elas nunca voltaram sozinhas.
--
-- Consequência real: nomeSupervisorPorEmpresa/resolveSupervisor
-- (farol_mobile.go) e a busca de nome de RCA (farol_web.go:267) leem
-- dessas tabelas pra exibir o nome no link do ION VENDAS
-- (/m/CNPJ/SUP/cod, /m/CNPJ/RCA/cod). Sem elas, o supervisor/RCA via
-- "Supervisor 92" em vez do próprio nome (o `_ = ` ali engole o erro
-- "relation does not exist" e cai no fallback genérico).
--
-- Mesma forma final de 121+122 combinadas (empresa_id já na PK direto,
-- sem o TRUNCATE+ALTER que 122 fez porque aqui não há dado prévio pra
-- migrar). O sync diário do CADRCA_JC (jc_organograma.go, migration 250)
-- passa a popular o nome automaticamente a partir daqui.
CREATE TABLE IF NOT EXISTS gestores (
    empresa_id      UUID    NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    cod_supervisor  INTEGER NOT NULL,
    nome            TEXT    NOT NULL,
    uf              CHAR(2),
    regiao          TEXT,
    atuacao         TEXT GENERATED ALWAYS AS (
                        CASE WHEN uf IS NOT NULL AND regiao IS NOT NULL
                             THEN uf || ' - ' || regiao
                             ELSE NULL
                        END
                    ) STORED,
    ativo           BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (empresa_id, cod_supervisor)
);

CREATE TABLE IF NOT EXISTS rcas (
    empresa_id  UUID    NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    cod_rca     INTEGER NOT NULL,
    nome        TEXT    NOT NULL,
    cod_filial  TEXT,
    tipo        TEXT    NOT NULL DEFAULT 'RCA',
    ativo       BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (empresa_id, cod_rca)
);

CREATE TABLE IF NOT EXISTS gestor_rca (
    empresa_id      UUID    NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    cod_supervisor  INTEGER NOT NULL,
    cod_rca         INTEGER NOT NULL,
    PRIMARY KEY (empresa_id, cod_supervisor, cod_rca),
    CONSTRAINT fk_gestor_rca_gestor
        FOREIGN KEY (empresa_id, cod_supervisor) REFERENCES gestores(empresa_id, cod_supervisor) ON DELETE RESTRICT,
    CONSTRAINT fk_gestor_rca_rca
        FOREIGN KEY (empresa_id, cod_rca) REFERENCES rcas(empresa_id, cod_rca) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_gestores_empresa    ON gestores(empresa_id);
CREATE INDEX IF NOT EXISTS idx_gestores_emp_uf     ON gestores(empresa_id, uf);
CREATE INDEX IF NOT EXISTS idx_rcas_empresa        ON rcas(empresa_id);
CREATE INDEX IF NOT EXISTS idx_rcas_emp_tipo       ON rcas(empresa_id, tipo);
CREATE INDEX IF NOT EXISTS idx_rcas_emp_ativo      ON rcas(empresa_id, ativo);
CREATE INDEX IF NOT EXISTS idx_gestor_rca_emp_rca  ON gestor_rca(empresa_id, cod_rca);
