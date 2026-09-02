---
epic: 5
story: 1
story_key: 5-1-painel-indicadores-oficiais
baseline_commit: b2eea74
---

# Story 5.1: Painel de indicadores oficiais (Meta × Realizado × delta)

Status: review

## Story

Como Supervisor/GGV,
Eu quero ver, para cada Indústria/Fornecedor configurado, um painel de Meta × Realizado por Tipo de Métrica, navegável pela hierarquia GGV → CRV → RCA → Rede → Cliente,
Para que eu identifique rapidamente onde estou abaixo da meta e aja em campo.

Fonte: [`epics.md`](../planning-artifacts/epics.md) (Épico 5, Story 5.1) · FR19, FR19a · GitHub issue #22. Primeira tela de VISUALIZAÇÃO real do módulo (Épicos 1-4 eram só admin/motor).

## Decisões desta story

**Endpoint novo (`/api/farol/metas-painel`)** combina o Realizado (Épico 4) com as Faixas de meta (Story 2.2) — não existia antes um lugar que juntasse os dois. Calcula `faixa_atual` (maior faixa já batida) e `proxima_faixa` (menor faixa ainda não batida), e o `delta` (FR19a) é sempre contra a PRÓXIMA faixa — "faltam X pra bater a Faixa 2", não um número genérico contra a meta mais alta.

**Navegação por hierarquia = troca de nível de agregação, não drill-down aninhado.** O seletor de Nível (Rede/RCA/CRV/GGV) troca qual agregação o painel mostra, mas não implementa "clicar num GGV e ver só os CRVs dele" — isso exigiria filtro adicional no backend que não foi pedido explicitamente no AC desta story. Registrado como possível refinamento futuro, não como lacuna escondida.

## Achado importante: corrigi uma fragilidade real na infraestrutura de testes

Ao rodar a suíte completa depois de escrever os testes desta story, `TestObterOuCongelar_FluxoCompleto` (Story 4.3) começou a falhar de forma intermitente — não por bug de lógica, mas porque `biTestDB()` abria uma conexão Postgres NOVA a cada chamada (nunca fechada), e com o volume de fixtures das stories anteriores, o Postgres local batia em `max_connections`, fazendo `DELETE`s de limpeza falharem silenciosamente e vazarem dado de um teste pro outro. Corrigido: `biTestDB()` agora usa `sync.Once` pra compartilhar uma única conexão real (pool) entre todos os testes do pacote — e removi os 7 `defer db.Close()` que assumiam (incorretamente, a partir de agora) que cada teste possuía sua própria conexão exclusiva. Rodei a suíte completa 7 vezes seguidas depois da correção: sempre as mesmas 3 falhas pré-existentes, nada mais.

## Acceptance Criteria

1. Painel mostra Meta, Realizado e o delta explícito por Tipo de Métrica, navegável por nível hierárquico.
2. Delta calculado contra a próxima faixa não atingida — testado com o padrão real Unilever (múltiplas faixas).
3. Painel mostra só indicadores oficiais — projeção fica pra Story 5.3 (endpoint já retorna `projecao`, mas esta tela não exibe ainda).

## Tasks / Subtasks

- [x] **Task 1: Endpoint `/api/farol/metas-painel`** (AC: 1, 2)
  - [x] `backend/handlers/farol_metas_painel.go` — combina `obterOuCongelarRealizado` (Épico 4) com `metas_faixas` (Story 2.2)
- [x] **Task 2: Testes** (AC: 1, 2)
  - [x] `TestMetasPainel_DeltaExplicito` — PASS
  - [x] `TestMetasPainel_TodasFaixasAtingidas_DeltaZero` — PASS
  - [x] **Correção de infraestrutura de testes**: `biTestDB` agora compartilha conexão (`sync.Once`), removidos 7 `defer db.Close()` incompatíveis — suíte completa estável em 7 rodadas consecutivas
- [x] **Task 3: Frontend — `FarolPainelMetas.tsx`** (AC: 1, 2, 3)
  - [x] Seletores de Vínculo/Vigência/Nível/Fluxo, cards de Realizado/Meta/Delta, tabela de Redes ou Grupos conforme o nível
  - [x] Rota `/farol/metas-industria`, módulo próprio no menu ("Metas Indústria")
  - [x] `tsc --noEmit` limpo

## Dev Agent Record

### Agent Model Used

Claude Sonnet 5

### Completion Notes List

2 testes novos de painel, todos passando. Correção de infraestrutura de testes (conexões Postgres) é o achado mais valioso desta story além do próprio endpoint — deixa toda a suíte mais confiável daqui pra frente, não só os testes desta story.

### File List

- `backend/handlers/farol_metas_painel.go` (novo)
- `backend/handlers/farol_metas_painel_test.go` (novo)
- `backend/handlers/farol_bi_api_test.go` (modificado — correção de infraestrutura de testes)
- `backend/main.go` (modificado)
- `frontend/src/pages/FarolPainelMetas.tsx` (novo)
- `frontend/src/App.tsx` (modificado)
- `frontend/src/lib/navigation.ts` (modificado)

### Change Log

- 2026-09-02: Painel de indicadores oficiais implementado. Corrigida fragilidade real de conexões na infraestrutura de testes de integração.
