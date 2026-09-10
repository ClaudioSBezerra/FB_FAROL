-- particionar_vendas_2025_2026.sql
-- ════════════════════════════════════════════════════════════════════════════
-- Converte vendas_faturadas/vendas_transmitidas de tabela normal pra
-- particionada por RANGE(data), SEM copiar os dados existentes (~26GB +
-- ~23GB em produção, hoje 10/09/2026) — usa o recurso nativo do Postgres de
-- ANEXAR uma tabela já populada como partição (operação de metadado, quase
-- instantânea, não duplica nada em disco).
--
-- Parte da política de retenção rolante de 2 anos pedida pelo Claudio
-- (10/09/2026): manter ano corrente + ano anterior completos; em fev/2027,
-- apagar 2025 (mantendo 2026+2027). Ver plano completo em
-- /home/claudio/.claude/plans/merry-fluttering-babbage.md (sessão de
-- 10/09/2026) e memória do projeto (farol_retencao_2anos_vendas_10set.md).
--
-- Validado numa cópia descartável do schema (mesmo shape de colunas/índices
-- de produção, populada com 50.000 linhas de teste cobrindo 2025+2026) antes
-- de qualquer execução real — ver seção "VALIDADO" no final deste arquivo.
--
-- ⚠️  NÃO É UMA MIGRATION NUMERADA (não roda sozinha no boot do app).
--     Rodar manualmente, uma vez, contra produção — só depois de
--     confirmação explícita do Claudio (decisão tomada fora do fluxo normal
--     de "aprovar plano", ver o próprio plano). Mesmo padrão já usado antes
--     nesta sessão pra cargas de dado real:
--       ssh -i ~/.ssh/coolify_hostinger root@76.13.171.196 \
--         "docker exec -i <container_db_ativo> psql -U postgres -d fb_farol" \
--         < scripts/particionar_vendas_2025_2026.sql
--     (confirmar o nome do container ATIVO antes — sufixo muda a cada
--     redeploy do Coolify, ver farol_prd_ssh_access.md).
--
-- ⚠️  RISCO DE ESPAÇO: rodar só se houver folga de disco confortável no
--     servidor (checar `df -h /` antes — em 10/09/2026 havia 53GB livres
--     de 193GB, suficiente; ESTE SCRIPT NÃO DUPLICA os ~49GB existentes,
--     mas os passos de VALIDATE CONSTRAINT e ADD CONSTRAINT (FK) fazem
--     scans de leitura que podem levar minutos cada — não são instantâneos
--     como os ALTER/CREATE/ATTACH puramente de metadado).
--
-- ⚠️  Cada bloco abaixo roda como uma transação PRÓPRIA (statements sem
--     BEGIN/COMMIT explícito, cada um autocommita) — isso é DELIBERADO:
--     o VALIDATE CONSTRAINT de uma tabela não precisa segurar nenhum lock
--     que bloqueie a outra tabela ou os passos seguintes.
-- ════════════════════════════════════════════════════════════════════════════


-- ═══ PASSO 1 — vendas_faturadas: CHECK constraint (sem bloquear leitura/escrita) ═══
-- NOT VALID entra instantâneo; VALIDATE faz um scan de leitura (SHARE UPDATE
-- EXCLUSIVE — não bloqueia SELECT/INSERT/UPDATE/DELETE concorrentes, só
-- impede outro DDL simultâneo na mesma tabela). Isto é o que permite o
-- ATTACH do passo 2 pular a revalidação e ser instantâneo.
ALTER TABLE vendas_faturadas
    ADD CONSTRAINT chk_vf_ano_2025_2026
    CHECK (data_faturamento >= '2025-01-01' AND data_faturamento < '2027-01-01')
    NOT VALID;

ALTER TABLE vendas_faturadas VALIDATE CONSTRAINT chk_vf_ano_2025_2026;
-- (validado com 50k linhas: instantâneo; em ~26GB reais, esperar de minutos
--  a baixas dezenas de minutos dependendo de IO do disco — sem bloquear
--  tráfego da aplicação.)


-- ═══ PASSO 2 — vendas_faturadas: rename + cria pai particionado + attach ═══
-- A PRIMARY KEY (id) original NÃO inclui a coluna de partição
-- (data_faturamento) — regra do Postgres pra tabela particionada. Não há
-- nenhuma FK apontando pra vendas_faturadas.id nem uso de RETURNING id no
-- Go (confirmado por grep antes de decidir) — é seguro só dropar a PK em
-- vez de recriá-la composta (evita reconstruir um índice do tamanho da
-- tabela inteira à toa).
ALTER TABLE vendas_faturadas DROP CONSTRAINT vendas_faturadas_pkey;

ALTER TABLE vendas_faturadas RENAME TO vendas_faturadas_2025_2026;

-- LIKE ... INCLUDING DEFAULTS captura o schema ATUAL (todas as colunas
-- acrescentadas ao longo de ~15 migrations desde a 156) sem arrastar PK/
-- índices/FK — esses são recriados abaixo, deliberadamente.
CREATE TABLE vendas_faturadas (LIKE vendas_faturadas_2025_2026 INCLUDING DEFAULTS)
    PARTITION BY RANGE (data_faturamento);

-- Instantâneo — o CHECK do passo 1 já provou que os dados cabem no range,
-- Postgres pula a revalidação.
ALTER TABLE vendas_faturadas
    ATTACH PARTITION vendas_faturadas_2025_2026
    FOR VALUES FROM ('2025-01-01') TO ('2027-01-01');

-- FK de saída (empresa_id → companies) — Postgres 15 NÃO aceita NOT VALID
-- pra FK numa tabela particionada referenciando outra tabela ("not yet
-- supported"), então este ADD já valida na hora. Validado rápido com poucas
-- empresas distintas (sistema multi-tenant, mas poucas empresas ativas) —
-- ainda assim, diferente do CHECK do passo 1, este passo específico PODE
-- segurar lock breve.
ALTER TABLE vendas_faturadas
    ADD CONSTRAINT vendas_faturadas_empresa_id_fkey
    FOREIGN KEY (empresa_id) REFERENCES companies(id) ON DELETE CASCADE;


-- ═══ PASSO 3 — vendas_faturadas: renomeia os 17 índices existentes pra
-- "_legado" e recria no pai com o nome original ═══
-- IMPORTANTE: nomes de índice são globais no schema `public`, não por
-- tabela — por isso não dá pra criar o índice do pai com o MESMO nome que
-- já existe na partição sem antes liberar o nome. O Postgres reconhece a
-- definição idêntica (mesmas colunas, mesmo WHERE parcial) e ANEXA o índice
-- físico já existente da partição ao índice novo do pai automaticamente —
-- SEM reconstruir nada (validado: <0.2s mesmo pra reconectar, no teste com
-- 50k linhas; o custo real em produção é o tempo de VERIFICAR a definição,
-- não de reconstruir ~26GB de índice).
--
-- ⚠️ NUNCA rode um DROP INDEX num índice do PAI depois de criado — dropar o
-- índice particionado do pai DERRUBA (cascata) o índice físico da partição
-- também, mesmo que ele já existisse antes e só tivesse sido "anexado".
ALTER INDEX idx_vf_cliprinc              RENAME TO idx_vf_cliprinc_legado;
ALTER INDEX idx_vf_data_fornec           RENAME TO idx_vf_data_fornec_legado;
ALTER INDEX idx_vf_data_ger              RENAME TO idx_vf_data_ger_legado;
ALTER INDEX idx_vf_data_rca              RENAME TO idx_vf_data_rca_legado;
ALTER INDEX idx_vf_data_sup              RENAME TO idx_vf_data_sup_legado;
ALTER INDEX idx_vf_emp_cli_data          RENAME TO idx_vf_emp_cli_data_legado;
ALTER INDEX idx_vf_emp_cnpj              RENAME TO idx_vf_emp_cnpj_legado;
ALTER INDEX idx_vf_emp_data              RENAME TO idx_vf_emp_data_legado;
ALTER INDEX idx_vf_emp_data_produto      RENAME TO idx_vf_emp_data_produto_legado;
ALTER INDEX idx_vf_emp_fornec            RENAME TO idx_vf_emp_fornec_legado;
ALTER INDEX idx_vf_emp_rca               RENAME TO idx_vf_emp_rca_legado;
ALTER INDEX idx_vf_emp_supervisor        RENAME TO idx_vf_emp_supervisor_legado;
ALTER INDEX idx_vf_mixtotal_fornec       RENAME TO idx_vf_mixtotal_fornec_legado;
ALTER INDEX idx_vf_mixtotal_gerente      RENAME TO idx_vf_mixtotal_gerente_legado;
ALTER INDEX idx_vf_mixtotal_rca          RENAME TO idx_vf_mixtotal_rca_legado;
ALTER INDEX idx_vf_mixtotal_supervisor   RENAME TO idx_vf_mixtotal_supervisor_legado;
ALTER INDEX idx_vf_tipo_venda            RENAME TO idx_vf_tipo_venda_legado;
ALTER INDEX idx_vf_uf                    RENAME TO idx_vf_uf_legado;

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


-- ═══ PASSO 4 — vendas_transmitidas: mesma sequência (CHECK) ═══
ALTER TABLE vendas_transmitidas
    ADD CONSTRAINT chk_vt_ano_2025_2026
    CHECK (data_transmissao >= '2025-01-01' AND data_transmissao < '2027-01-01')
    NOT VALID;

ALTER TABLE vendas_transmitidas VALIDATE CONSTRAINT chk_vt_ano_2025_2026;


-- ═══ PASSO 5 — vendas_transmitidas: rename + pai particionado + attach + FK ═══
ALTER TABLE vendas_transmitidas DROP CONSTRAINT vendas_transmitidas_pkey;

ALTER TABLE vendas_transmitidas RENAME TO vendas_transmitidas_2025_2026;

CREATE TABLE vendas_transmitidas (LIKE vendas_transmitidas_2025_2026 INCLUDING DEFAULTS)
    PARTITION BY RANGE (data_transmissao);

ALTER TABLE vendas_transmitidas
    ATTACH PARTITION vendas_transmitidas_2025_2026
    FOR VALUES FROM ('2025-01-01') TO ('2027-01-01');

ALTER TABLE vendas_transmitidas
    ADD CONSTRAINT vendas_transmitidas_empresa_id_fkey
    FOREIGN KEY (empresa_id) REFERENCES companies(id) ON DELETE CASCADE;


-- ═══ PASSO 6 — vendas_transmitidas: índices (mesmo padrão do passo 3) ═══
ALTER INDEX idx_vt_cliprinc              RENAME TO idx_vt_cliprinc_legado;
ALTER INDEX idx_vt_data_fornec           RENAME TO idx_vt_data_fornec_legado;
ALTER INDEX idx_vt_data_ger              RENAME TO idx_vt_data_ger_legado;
ALTER INDEX idx_vt_data_rca              RENAME TO idx_vt_data_rca_legado;
ALTER INDEX idx_vt_data_sup              RENAME TO idx_vt_data_sup_legado;
ALTER INDEX idx_vt_emp_cli_data          RENAME TO idx_vt_emp_cli_data_legado;
ALTER INDEX idx_vt_emp_cnpj              RENAME TO idx_vt_emp_cnpj_legado;
ALTER INDEX idx_vt_emp_data              RENAME TO idx_vt_emp_data_legado;
ALTER INDEX idx_vt_emp_data_produto      RENAME TO idx_vt_emp_data_produto_legado;
ALTER INDEX idx_vt_emp_fornec            RENAME TO idx_vt_emp_fornec_legado;
ALTER INDEX idx_vt_emp_rca               RENAME TO idx_vt_emp_rca_legado;
ALTER INDEX idx_vt_emp_supervisor        RENAME TO idx_vt_emp_supervisor_legado;
ALTER INDEX idx_vt_mixtotal_fornec       RENAME TO idx_vt_mixtotal_fornec_legado;
ALTER INDEX idx_vt_mixtotal_gerente      RENAME TO idx_vt_mixtotal_gerente_legado;
ALTER INDEX idx_vt_mixtotal_rca          RENAME TO idx_vt_mixtotal_rca_legado;
ALTER INDEX idx_vt_mixtotal_supervisor   RENAME TO idx_vt_mixtotal_supervisor_legado;
ALTER INDEX idx_vt_tipo_venda            RENAME TO idx_vt_tipo_venda_legado;
ALTER INDEX idx_vt_uf                    RENAME TO idx_vt_uf_legado;

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


-- ═══ PASSO 7 — cria partições vazias pro próximo ano (folga de segurança) ═══
-- A migration 231 (farol.ensure_vendas_ano_partition, roda automática no
-- boot) cuida disso daqui pra frente — este passo aqui é só pra já deixar
-- 2027 pronto no exato momento em que este script rodar, sem depender de
-- esperar o próximo boot do app.
SELECT farol.ensure_vendas_ano_partition(2027);
SELECT farol.ensure_vendas_ano_partition(2028);


-- ════════════════════════════════════════════════════════════════════════════
-- VALIDADO em 10/09/2026, Postgres 17 descartável, schema idêntico ao de
-- produção (todas as 231 migrations aplicadas), populado com 50.000 linhas
-- de teste cobrindo 2025-2026:
--   • ATTACH PARTITION: 0.096s (sem re-scan, graças ao CHECK já validado)
--   • Todos os 18 índices de cada tabela reconectados automaticamente aos
--     físicos já existentes na partição (confirmado via pg_inherits,
--     indisvalid=true em todos) — sem reconstruir nenhum
--   • FK de empresa_id → companies: 0.092s
--   • EXPLAIN de uma query real do Painel de Objetivos
--     (farol_metas_calculo.go, somaPvendaClientes) confirma Index Scan,
--     não Seq Scan
--   • INSERT sem partição do ano cai em erro claro ("no partition of
--     relation found for row"); com farol.ensure_vendas_ano_partition
--     chamada antes, funciona
--   • Suite inteira `go test ./...` rodou sem nenhuma falha NOVA (as únicas
--     6 falhas são pré-existentes e confirmadas independentes desta
--     mudança — 4 de outra feature + 2 do Painel BI que falham igual num
--     banco vazio sem particionamento nenhum)
-- NÃO validado ainda: volume real de produção (26GB/23GB) — os tempos acima
-- são com 50k linhas; o ATTACH/índices continuam instantâneos independente
-- do volume (são operações de metadado), mas o VALIDATE CONSTRAINT e o ADD
-- CONSTRAINT da FK escalam com o tamanho real da tabela.
-- ════════════════════════════════════════════════════════════════════════════
