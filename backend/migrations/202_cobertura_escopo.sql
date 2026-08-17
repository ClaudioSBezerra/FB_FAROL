-- 202_cobertura_escopo.sql
-- ════════════════════════════════════════════════════════════════════════════
-- Cobertura temporária de escopo — "fulano saiu de férias, sicrano enxerga a
-- equipe dele até tal dia".
--
-- Criada agora, antes da tela de cadastro, por um motivo prático: o código de
-- escopo já consulta esta tabela a cada request, e sem ela o log de produção
-- de 17/08/2026 encheu de
--     [farol:escopo] coberturasVigentes ERRO: relation "farol.cobertura_escopo"
--     does not exist
-- quatro vezes por carregamento de página. O acesso seguia funcionando (a
-- função trata o erro e devolve "sem cobertura"), mas erro recorrente em log
-- treina todo mundo a ignorar log.
--
-- A vigência é por DATA e não por flag: quando o período passa, a linha deixa
-- de valer sozinha. Ninguém precisa lembrar de revogar quando o titular volta
-- — e esquecer de revogar é exatamente como um acesso temporário vira
-- permanente.
-- ════════════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS farol.cobertura_escopo (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id  UUID NOT NULL,
    -- Quem RECEBE a permissão (o substituto), não quem saiu de férias.
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    -- Nível e código que o substituto passa a enxergar. Mesmo vocabulário do
    -- escopo (farol_escopo.go): cod_gerente | cod_supervisor | cod_rca.
    nivel       TEXT NOT NULL CHECK (nivel IN ('cod_gerente', 'cod_supervisor', 'cod_rca')),
    cod         TEXT NOT NULL,
    inicio      DATE NOT NULL,
    fim         DATE NOT NULL,
    motivo      TEXT NOT NULL DEFAULT 'ferias',
    criado_por  UUID REFERENCES users(id) ON DELETE SET NULL,
    criado_em   TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT cobertura_periodo_valido CHECK (fim >= inicio)
);

-- A consulta do escopo é sempre (empresa, usuário, nível) filtrando por data —
-- roda a cada request de quem tem persona restrita.
CREATE INDEX IF NOT EXISTS idx_cobertura_escopo_lookup
    ON farol.cobertura_escopo (empresa_id, user_id, nivel, inicio, fim);

-- Impede cadastrar duas vezes a mesma cobertura para o mesmo período.
CREATE UNIQUE INDEX IF NOT EXISTS idx_cobertura_escopo_unica
    ON farol.cobertura_escopo (empresa_id, user_id, nivel, cod, inicio, fim);

COMMENT ON TABLE farol.cobertura_escopo IS
    'Cobertura temporária de escopo (férias). user_id = quem RECEBE a visão; cod = a equipe que ele passa a enxergar. Somente leitura, e só enquanto CURRENT_DATE estiver entre inicio e fim.';
