---
epic: 3
story: 4
story_key: 3-4-snapshot-mensal-congelado
baseline_commit: 61182bc
---

# Story 3.4: Snapshot mensal congelado

Status: review

## Story

Como admin,
Eu quero que listas (clientes/itens) e metas de um período já fechado não possam ser alteradas de forma a mudar retroativamente um cálculo já apurado,
Para que decisões de bônus já fechadas permanecem estáveis e auditáveis mesmo se a planilha do fornecedor mudar depois.

Fonte: [`epics.md`](../planning-artifacts/epics.md) (Épico 3, Story 3.4) · FR13 · GitHub issue #17. Última story do Épico 3.

## ⚠️ Achado: o mecanismo já existia — esta story é verificação + 1 lacuna fechada, não construção do zero

Ao chegar nesta story, percebi que o comportamento pedido **já estava implementado** desde as Stories 2.2/3.2/3.3, porque fazia sentido construir a checagem `status == 'fechada'` no momento exato em que cada handler de escrita foi criado, em vez de adiar pra uma story "de congelamento" genérica:

- `MetaVigenciaItemHandler` PUT/DELETE (Story 2.2) — já bloqueia (403) vigência fechada.
- `MetasClientesValidosImportarCSVHandler` (Story 3.2) — já bloqueia (403) reimportação numa vigência fechada.
- `MetasItensValidosImportarCSVHandler` (Story 3.3) — já bloqueia (403) reimportação numa vigência fechada.

**A lacuna real que faltava**: a importação de METAS via CSV (Story 3.1) cria vigências NOVAS — nunca edita uma existente — então o bloqueio `status == 'fechada'` das outras stories não se aplicava a ela da mesma forma. Mas a constraint `EXCLUDE` (migration 217) já impede qualquer vigência nova que se SOBREPONHA a uma existente, fechada ou não — então o efeito protetor já existia, só não tinha teste explícito provando isso pro caso "fechada" especificamente (só pro caso "aberta"). Adicionei esse teste.

**Conclusão**: nenhum código de produção novo nesta story — é consolidação e prova de que o congelamento é real em todos os pontos de escrita relevantes ao FR13, com um teste a mais fechando a única lacuna de cobertura (não de comportamento) que existia.

## Acceptance Criteria

1. Vigência fechada bloqueia edição/exclusão da própria vigência (PUT/DELETE) — **já coberto**, Story 2.2.
2. Vigência fechada bloqueia reimportação de Clientes Válidos — **já coberto**, Story 3.2.
3. Vigência fechada bloqueia reimportação de Itens Válidos — **já coberto**, Story 3.3.
4. Nenhum caminho de escrita cria dado novo que se sobreponha (mesmo período, outro vínculo/vigência) a uma vigência fechada — **lacuna de teste fechada nesta story**.

## Tasks / Subtasks

- [x] **Task 1: Auditoria dos pontos de escrita relevantes ao FR13** (AC: 1, 2, 3, 4)
  - [x] Revisado `farol_metas_vigencias.go`, `farol_metas_clientes_validos_csv.go`, `farol_metas_itens_validos_csv.go`, `farol_metas_import_csv.go` — todo caminho de escrita que toca uma vigência (direta ou indiretamente via `vigencia_id`) tem proteção
- [x] **Task 2: Fechar a lacuna de cobertura em Story 3.1** (AC: 4)
  - [x] `TestMetasImportarCSV_SobreposicaoComVigenciaFechada_409` — confirma que a EXCLUDE constraint protege mesmo o caminho de criação de vigência nova (Story 3.1) contra sobreposição com uma vigência fechada
- [x] **Task 3: Rodar todos os testes de "Fechada" juntos** (AC: 1, 2, 3, 4)
  - [x] `go test -run Fechada` — 3 testes (Clientes, Itens, ImportarCSV-sobreposição) — PASS, provando o congelamento de ponta a ponta

## Dev Agent Record

### Agent Model Used

Claude Sonnet 5

### Completion Notes List

Nenhum código de produção novo — story de verificação/consolidação. 1 teste novo fechando a única lacuna de cobertura real encontrada. Regressão: mesmas 3 falhas pré-existentes.

### File List

- `backend/handlers/farol_metas_import_csv_test.go` (modificado — 1 teste novo)

### Change Log

- 2026-09-02: Verificado que o congelamento (FR13) já estava implementado em todos os pontos de escrita relevantes desde as Stories 2.2/3.2/3.3; fechada a única lacuna de teste encontrada (sobreposição via importação de metas contra vigência fechada). Épico 3 completo.
