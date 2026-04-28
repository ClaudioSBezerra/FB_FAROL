-- Módulo Cadastros: Gestores, RCAs e relacionamento

CREATE TABLE IF NOT EXISTS gestores (
    cod_supervisor  INTEGER PRIMARY KEY,
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
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS rcas (
    cod_rca     INTEGER PRIMARY KEY,
    nome        TEXT    NOT NULL,
    cod_filial  TEXT,
    tipo        TEXT    NOT NULL DEFAULT 'RCA',
    ativo       BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS gestor_rca (
    cod_supervisor  INTEGER NOT NULL REFERENCES gestores(cod_supervisor) ON DELETE RESTRICT,
    cod_rca         INTEGER NOT NULL REFERENCES rcas(cod_rca)            ON DELETE CASCADE,
    PRIMARY KEY (cod_supervisor, cod_rca)
);

CREATE INDEX IF NOT EXISTS idx_gestor_rca_rca      ON gestor_rca(cod_rca);
CREATE INDEX IF NOT EXISTS idx_gestores_uf         ON gestores(uf);
CREATE INDEX IF NOT EXISTS idx_rcas_tipo           ON rcas(tipo);
CREATE INDEX IF NOT EXISTS idx_rcas_ativo          ON rcas(ativo);
