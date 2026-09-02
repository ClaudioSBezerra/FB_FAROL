---
epic: 1
story: 1
story_key: 1-1-cadastro-tipo-metrica
baseline_commit: abd6336cd440ac55606b22923a922c0251ccbd41
---

# Story 1.1: Cadastro de Tipo de Métrica com modelo de parâmetros genérico

Status: review

<!-- Note: Validation is optional. Run validate-create-story for quality check before dev-story. -->

## Story

Como admin,
Eu quero cadastrar um novo Tipo de Métrica definindo nome, descrição, nível de agregação (ex: Rede, Cliente/CNPJ) e a lista de parâmetros que uma instância exige preencher,
Para que o framework suporte uma nova forma de calcular atingimento sem exigir alteração de código.

Módulo: **Painel de Gestão de Metas por Indústria** (novo, brownfield sobre o Farol existente). Épico 1 carrega o trabalho de arquitetura embutido — esta story É a decisão de arquitetura (modelo de dados de Tipo de Métrica/parâmetros), não uma fase separada.

Fonte: [`_bmad-output/planning-artifacts/epics.md`](../planning-artifacts/epics.md) (Épico 1, Story 1.1) · PRD: [`_bmad-output/planning-artifacts/prds/prd-FB_FAROL-2026-09-02/prd.md`](../planning-artifacts/prds/prd-FB_FAROL-2026-09-02/prd.md) (FR1, FR2, FR3, NFR1) · GitHub issue: https://github.com/ClaudioSBezerra/FB_FAROL/issues/6

## Acceptance Criteria

1. **Cadastro básico funciona** — Dado que estou logado como admin, quando cadastro um Tipo de Métrica "Cobertura por Rede" com nível de agregação = Rede e um parâmetro "limiar R$" (tipo number), então o tipo é salvo e fica disponível para vínculo futuro (Épico 2). [Fonte: epics.md Story 1.1]
2. **Teste de generalidade do FR1 (critério de aceite da arquitetura)** — Dado o modelo de parâmetros implementado, quando cadastro um Tipo de Métrica hipotético "Frequência de Visita por Cliente" com nível de agregação = Cliente e um parâmetro "nº mínimo de visitas" (tipo integer), então o cadastro é concluído **sem exigir nenhuma coluna nova na tabela `farol.tipos_metrica`** — só um novo conteúdo dentro da coluna `parametros_schema` (JSONB). Este AC é obrigatório e testado explicitamente (não é só documentação). [Fonte: prd.md linha 70, epics.md Story 1.1]
3. **Auditoria** — Toda criação ou edição de Tipo de Métrica grava um evento em `farol.sp_audit_log` (via `writeAuditLogTx`) com `entidade="tipos_metrica"`, `acao` ("criar"/"editar"), e `payload` contendo o valor anterior (só em editar) e o novo valor. [Fonte: epics.md Story 1.1 AC "E", NFR1]
4. **Isolamento multi-tenant** — Um Tipo de Métrica cadastrado pela empresa A não aparece nem é editável/deletável pela empresa B (mesmo padrão de `farol.industrias`: filtro `WHERE empresa_id = $N` em toda query, 404 em vez de 403 pra não vazar existência). [Fonte: farol_industrias.go, seção "Isolamento multi-tenant"]
5. **Nome duplicado é rejeitado** — Cadastrar dois Tipos de Métrica com o mesmo nome na mesma empresa retorna 409 com mensagem clara (mesmo padrão de `uq_farol_industrias_empresa_nome`). [Fonte: farol_industrias.go]
6. **Papel de acesso** — Só usuários com `sp_role >= gestor_geral` conseguem listar/criar/editar/excluir Tipos de Métrica (tela é 100% administrativa; painéis de visualização são de épicos futuros — Épico 5/6). [Fonte: NFR2, precedente `GerarSazonalidadeHandler` que usa `gestor_geral` pro mesmo tipo de tela admin-only]

## Tasks / Subtasks

- [x] **Task 1: Migration 214 — tabela `farol.tipos_metrica`** (AC: 1, 2, 3, 4, 5)
  - [x] Criar `backend/migrations/214_tipos_metrica.sql` com cabeçalho de comentário explicando o "porquê" (padrão do projeto — ver `209_industrias.sql`)
  - [x] `CREATE TABLE IF NOT EXISTS farol.tipos_metrica (id SERIAL PRIMARY KEY, empresa_id UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE, nome TEXT NOT NULL, descricao TEXT, nivel_agregacao TEXT NOT NULL, parametros_schema JSONB NOT NULL DEFAULT '[]', ativo BOOLEAN NOT NULL DEFAULT TRUE, created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(), CONSTRAINT uq_farol_tipos_metrica_empresa_nome UNIQUE (empresa_id, nome))`
  - [x] `CHECK (nivel_agregacao IN ('ggv','crv','rca','rede','cliente'))` — níveis vêm da hierarquia organizacional fixa (Épico 1 Story 1.4), não são parte do "teste de generalidade" (esse é sobre `parametros_schema`, não sobre os níveis existentes)
  - [x] `CREATE INDEX IF NOT EXISTS idx_farol_tipos_metrica_empresa ON farol.tipos_metrica (empresa_id)`
  - [x] Rodar localmente contra o Postgres da VM de dev (`fb_farol`) e confirmar que `schema_migrations` registra `214_tipos_metrica.sql` — confirmado via `go run .` (backend aplicou 211-214 automaticamente) + `\d farol.tipos_metrica` no psql

- [x] **Task 2: Handler Go — CRUD de Tipos de Métrica** (AC: 1, 2, 3, 4, 5, 6)
  - [x] Criar `backend/handlers/farol_tipos_metrica.go`, struct `TipoMetricaRequest {Nome string; Descricao *string; NivelAgregacao string; ParametrosSchema []ParametroSchema; Ativo *bool}` e `ParametroSchema {Key, Label, Type string}` (`Type` livre por ora: `"number"`, `"integer"`, `"text"` — validado só como string não vazia, sem enum rígido, pra não travar tipos futuros)
  - [x] `TiposMetricaHandler` — `GET` (lista, filtra `empresa_id`) e `POST` (cria); seguir exatamente a estrutura de `IndustriasHandler` em `farol_industrias.go` (switch por `r.Method`, `json.NewDecoder(r.Body).Decode`, validação manual com `strings.TrimSpace`)
  - [x] `TipoMetricaItemHandler` — `PUT`/`DELETE` via `/api/farol/tipos-metrica/{id}`, parsing de path com `pathSegment` (mesmo helper de `IndustriaItemHandler`)
  - [x] Validar `ParametrosSchema`: rejeitar array vazio (todo Tipo de Métrica exige ≥1 parâmetro) e `key` duplicada dentro do mesmo tipo
  - [x] POST/PUT dentro de transação (`db.Begin()` + `defer tx.Rollback()` + `tx.Commit()`), chamando `writeAuditLogTx(tx, empresaID, spCtx.UserID, "tipos_metrica", strconv.Itoa(id), "criar"|"editar", payload)` — em "editar", `payload` contém `{"antes": {...}, "depois": {...}}` (registro atual buscado antes do UPDATE)
  - [x] Conflito de nome: `strings.Contains(err.Error(), "uq_farol_tipos_metrica_empresa_nome")` → 409
  - [x] Isolamento: toda query com `WHERE empresa_id = $N`; `RowsAffected() == 0` em PUT/DELETE → 404
  - [x] Registrar rotas em `main.go`, mesmo bloco onde estão as rotas de Indústrias — confirmado com `go build ./...` limpo + `gofmt` aplicado

- [x] **Task 3: Teste Go de integração — teste de generalidade é o AC mais importante** (AC: 2, 4, 5)
  - [x] Criar `backend/handlers/farol_tipos_metrica_test.go` seguindo o padrão de `farol_industrias_test.go` (teste de integração contra `DATABASE_URL`, skip se não setada, helper local `tipoMetricaReq` — com `sp_role=gestor_geral`, diferente de `industriaReq` — reaproveitando `biTestDB(t)`)
  - [x] `TestTiposMetrica_CriarCoberturaPorRede` — cria com nível `rede` + parâmetro `limiar_valor` (number) — PASS
  - [x] `TestTiposMetrica_CriarTipoHipotetico_SemAlterarSchema` — **teste do critério de aceite do FR1**: cria "Frequência de Visita por Cliente" com nível `cliente` + parâmetro `min_visitas` (integer), sem qualquer migration adicional — PASS
  - [x] `TestTiposMetrica_NomeDuplicado_Conflito409` — PASS
  - [x] `TestTiposMetrica_ParametrosSchemaVazio_400` — PASS (adicionado além do previsto, cobre validação do AC1/AC2 mais a fundo)
  - [x] `TestTiposMetrica_RequerGestorGeral_403` — PASS (cobre AC6 diretamente)
  - [x] `TestTiposMetrica_IsolamentoEntreEmpresas_404` — PASS
  - [x] `TestTiposMetrica_Editar_GravaAuditLog` — após um PUT, consulta `farol.sp_audit_log` e confirma `entidade='tipos_metrica'`, `acao='editar'`, `payload` contendo `antes`/`depois` — PASS
  - [x] **Descoberta durante o teste**: `writeAuditLogTx` faz `$2::uuid` contra `sp_audit_log.user_id` (FK pra `users`), então o literal `UserID: "teste"` usado em `industriaReq`/`biReq` não serve pra handlers que chamam auditoria — precisei de um helper novo (`tipoMetricaTestUserID`) que busca um `user_id` real na base. Vale documentar isso pra quem for escrever teste de outro handler que use `writeAuditLogTx`.
  - [x] Regressão: `go test ./...` roda com as mesmas 3 falhas pré-existentes (`TestBIParidadeComCards`, `TestBICache`, `TestFarolV2Cards_FiltroIndustria_MesCompletoIgnoraAggCorrompida`) — confirmado via `git stash` que elas já falhavam **antes** desta story (dados incompletos no Postgres local + teste de timing de cache sensível ao ambiente). Nenhuma regressão nova introduzida.

- [x] **Task 4: Frontend — tela admin CRUD de Tipos de Métrica** (AC: 1, 2, 6)
  - [x] Criar `frontend/src/pages/ConfigTiposMetrica.tsx`, seguindo a estrutura de `GestaoIndustrias.tsx` (TanStack Query `useQuery`/`useMutation` + `fetch` cru com header `Authorization: Bearer <token>` de `useAuth()`, `Table`/`Dialog`/`AlertDialog` do design system `@/components/ui/*`, toasts via `sonner`)
  - [x] Listagem em `Table`: Nome, Nível de Agregação, Ativo, parâmetros (chips)
  - [x] Formulário de criar/editar em `Dialog`: campos Nome, Descrição, Nível de Agregação (`Select` com as 5 opções fixas: GGV/CRV/RCA/Rede/Cliente), e um **editor dinâmico de parâmetros** (lista repetível de linhas Key/Label/Type, com botão "+ Adicionar parâmetro" e remover linha)
  - [x] Exclusão via `AlertDialog` de confirmação (mesmo padrão de Indústrias)
  - [x] Registrar rota em `frontend/src/App.tsx`: `<Route path="/gestao/tipos-metrica" element={<ProtectedRoute><ConfigTiposMetrica /></ProtectedRoute>} />` — **corrigido durante a implementação**: ao ler `navigation.ts` por completo, `AdminFbtaxRoute` (usado por Sazonalidade) exige `admin_fbtax`, mais restrito que o `gestor_geral` que o backend desta story exige; o precedente certo é `Indústrias` (`ProtectedRoute` + RBAC real no backend), não Sazonalidade
  - [x] Registrar no menu em `frontend/src/lib/navigation.ts`, dentro de `modules.config.tabs`: `{ label: 'Tipos de Métrica', path: '/gestao/tipos-metrica' }`
  - [x] `npx tsc --noEmit` limpo e `npm run build` (vite) concluído sem erro
  - [x] **Ressalva honesta**: não consegui abrir num navegador de verdade — esta VM de dev não tem Chromium/Playwright/Puppeteer instalado, só shell. A verificação foi: type-check limpo, build de produção limpo, e a lógica do handler já validada ponta-a-ponta pelos testes Go de integração (Task 3). **Falta um clique manual real do Claudio na tela antes de considerar 100% verificado visualmente.**

## Dev Notes

### Decisões de arquitetura tomadas nesta story

- **Genericidade fica 100% em `parametros_schema JSONB`**, não em colunas da tabela. A tabela `farol.tipos_metrica` nunca ganha coluna nova por causa de um Tipo de Métrica diferente — é isso que o teste de generalidade do FR1 valida (Task 3). Se uma futura necessidade exigir mudar a tabela pra suportar um novo Tipo de Métrica, isso é um sinal de que o modelo falhou o teste — reportar ao invés de contornar silenciosamente.
- **`nivel_agregacao` é a única coisa fixa/CHECK-constrained** (GGV/CRV/RCA/Rede/Cliente) porque vem da hierarquia organizacional real do Farol (Story 1.4), não é parâmetro livre do Tipo de Métrica.
- **Papel de acesso: `gestor_geral`**, não `gestor_filial` (diferente do precedente de Indústrias). Justificativa: Tipo de Métrica é catálogo transversal (não é por filial) e alimenta programas com impacto financeiro (bônus de fornecedor) — mesmo nível de acesso usado em `GerarSazonalidadeHandler`, outra tela admin-only recente. Se o Claudio preferir `gestor_filial` (mais permissivo, como Indústrias), é uma troca de uma linha (`requiredSpRole` no `main.go`) — não vale bloquear a story por isso, mas vale confirmar na review.
- **Auditoria usa a infra existente (`writeAuditLogTx`/`farol.sp_audit_log`)**, não uma tabela nova. É append-only, `payload JSONB` livre — o "valor anterior" do NFR1 fica dentro do payload como `{"antes": {...}, "depois": {...}}`, não como colunas dedicadas.

### Padrões existentes a seguir (não inventar novo estilo)

- **Migrations**: `backend/migrations/NNN_descricao.sql`, 3 dígitos, próximo número livre = **214**. Sem lib de migration (runner caseiro em `main.go`, `onDBConnected()`), sem DOWN, DDL sempre com `IF NOT EXISTS`. [Fonte: investigação de `backend/migrations/`, `209_industrias.sql`, `212_sazonalidade_produto_ano.sql`, `213_sazonalidade_produto_filial_param.sql`]
- **Handler Go**: `net/http` puro (sem chi/gorilla), `database/sql` + `github.com/lib/pq` puro (sem ORM), SQL raw parametrizado, transação explícita com `defer tx.Rollback()` logo após `db.Begin()`. Erros de constraint única via `strings.Contains(err.Error(), "uq_...")` → 409 (não usa `pq.Error`/`errors.As`). Resposta de erro em texto puro via `http.Error` (não JSON) — seguir o padrão de `farol_industrias.go`, não o de outros handlers que usam `{"error":"..."}`. [Fonte: `backend/handlers/farol_industrias.go`, lido inteiro]
- **Auth/RBAC**: gate de rota via `withSP(handlerFactory, "gestor_geral")` no `main.go` (que encadeia `FarolAuthMiddleware`); dentro do handler, `spCtx := GetSpContext(r)` + checar nil. Hierarquia: `somente_leitura(1) < gestor_filial(2) < gestor_geral(3) < admin_fbtax(4)`, checagem via `hasSpRole(role, required)`. [Fonte: `backend/handlers/smartpick_auth.go`]
- **Frontend**: TanStack Query (`useQuery`/`useMutation`) + `fetch` cru (sem axios/client gerado), token de `useAuth()` injetado manualmente no header, componentes shadcn (`@/components/ui/*`: Table, Dialog, AlertDialog, Select, Button, Input, Label), toasts via `sonner`, ícones `lucide-react`. [Fonte: `frontend/src/pages/ConfigSazonalidade.tsx` e `frontend/src/pages/GestaoIndustrias.tsx`, lidos inteiros]
- **Guard de rota frontend**: `AdminFbtaxRoute` (mesmo usado por `ConfigSazonalidade`) — o backend é a fonte de verdade, o guard do frontend é só conveniência de UX. [Fonte: comentário explícito em `App.tsx`]

### Testing Standards

Testes Go são **de integração contra banco real** (`DATABASE_URL` do ambiente), não table-driven/mock. Se `DATABASE_URL` não estiver setada, o teste dá skip (não falha) — normal em CI mínima, mas rodar com banco real na VM de dev antes de considerar a story pronta pra review. Seguir `farol_industrias_test.go` como molde exato: helper `biTestDB(t)` reaproveitado, requests via `httptest.NewRequest` + `FarolContext` injetado direto no contexto (bypass de JWT), assert manual em `w.Code`/`w.Body`, limpeza de fixtures com prefixo único + `t.Cleanup`.

### Ambiente de execução (não é sobre o código, é sobre onde rodar)

Implementar e testar **só na VM de dev** (`2.25.119.46`, backend Farol na porta 8084, Postgres local `fb_farol`). **Nenhum deploy em produção** (`76.13.171.196`/Coolify) sem aprovação explícita do Claudio. Commitar em branch de feature (`feature/painel-metas-industria`), nunca push direto pra `main`.

### Project Structure Notes

Nenhum conflito com a estrutura existente — a story só adiciona arquivos novos (`214_tipos_metrica.sql`, `farol_tipos_metrica.go`, `farol_tipos_metrica_test.go`, `ConfigTiposMetrica.tsx`) e duas linhas de registro em arquivos já existentes (`main.go`, `App.tsx`, `navigation.ts`). Nenhum arquivo existente precisa ser reescrito, só ter linhas adicionadas.

### References

- [Source: _bmad-output/planning-artifacts/epics.md#Epic-1-Story-1.1]
- [Source: _bmad-output/planning-artifacts/prds/prd-FB_FAROL-2026-09-02/prd.md#FR1-FR3, linha 70 (teste de generalidade)]
- [Source: backend/handlers/farol_industrias.go — molde do handler CRUD]
- [Source: backend/handlers/farol_industrias_test.go — molde do teste de integração]
- [Source: backend/handlers/smartpick_auth.go — RBAC (spRoleLevel, hasSpRole, FarolAuthMiddleware)]
- [Source: backend/handlers/sp_audit.go — writeAuditLogTx / farol.sp_audit_log]
- [Source: backend/migrations/209_industrias.sql, 212_sazonalidade_produto_ano.sql, 213_sazonalidade_produto_filial_param.sql — convenção de migration]
- [Source: frontend/src/pages/ConfigSazonalidade.tsx, GestaoIndustrias.tsx — molde de tela admin]
- [Source: frontend/src/App.tsx, frontend/src/lib/navigation.ts — registro de rota/menu]

## Dev Agent Record

### Agent Model Used

Claude Sonnet 5

### Debug Log References

### Completion Notes List

Ultimate context engine analysis completed - comprehensive developer guide created (investigação exaustiva via subagente Explore contra o código real do backend/frontend, sem Architecture.md — padrões extraídos diretamente de `farol_industrias.go`, `smartpick_auth.go`, `sp_audit.go`, `ConfigSazonalidade.tsx`, `GestaoIndustrias.tsx`).

### File List

- `backend/migrations/214_tipos_metrica.sql` (novo)
- `backend/handlers/farol_tipos_metrica.go` (novo)
- `backend/handlers/farol_tipos_metrica_test.go` (novo)
- `backend/main.go` (modificado — 2 rotas novas registradas)
- `frontend/src/pages/ConfigTiposMetrica.tsx` (novo)
- `frontend/src/App.tsx` (modificado — import + rota nova)
- `frontend/src/lib/navigation.ts` (modificado — item de menu novo)

### Change Log

- 2026-09-02: Implementação completa da Story 1.1 — migration 214, handler CRUD (`farol_tipos_metrica.go`) com auditoria via `writeAuditLogTx`, 7 testes de integração Go (incluindo o teste de generalidade do FR1), tela admin `ConfigTiposMetrica.tsx` registrada em `/gestao/tipos-metrica`. Nenhuma regressão introduzida (confirmado via `git stash` + `go test ./...`, mesmas 3 falhas pré-existentes em `TestBIParidadeComCards`/`TestBICache`/`TestFarolV2Cards_FiltroIndustria_MesCompletoIgnoraAggCorrompida`). Desvio documentado do Dev Notes original: rota frontend movida de `/config/tipos-metrica` (guard `AdminFbtaxRoute`) para `/gestao/tipos-metrica` (guard `ProtectedRoute`), pra bater com o nível de RBAC real do backend (`gestor_geral`, não `admin_fbtax`).
