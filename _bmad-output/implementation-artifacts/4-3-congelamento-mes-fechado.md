---
epic: 4
story: 3
story_key: 4-3-congelamento-mes-fechado
baseline_commit: 0c88e74
---

# Story 4.3: Congelamento de mês fechado

Status: review

## Story

Como admin (gestor),
Eu quero que o resultado apurado de um mês fechado fique congelado, só recalculável por ação manual explícita,
Para que os números usados pra decisão de bônus nunca mudam de forma silenciosa depois do fechamento.

Fonte: [`epics.md`](../planning-artifacts/epics.md) (Épico 4, Story 4.3) · FR17, NFR3 · GitHub issue #20.

## Decisão de arquitetura desta story

Até a Story 4.2, `CalcularRealizado` era sempre ao vivo — não havia NADA persistido pra "congelar". Esta story introduz `farol.metas_realizados_snapshot` (migration 223): um blob JSONB do `RealizadoResultado` inteiro, por `(vigencia_id, fluxo, nivel)`. A regra:

- **Vigência aberta**: sempre calcula ao vivo, nunca toca o snapshot (é o mês corrente, parcial por natureza — Story 4.1).
- **Vigência fechada, sem snapshot ainda**: calcula uma vez e grava — isso é o "congelamento inicial", não uma violação do FR17 (não existia número anterior pra mudar).
- **Vigência fechada, com snapshot**: serve do snapshot, nunca recalcula sozinho — mesmo que o dado de vendas mude depois.
- **Reprocessamento manual** (`POST .../metas-realizado/reprocessar`, só `gestor_geral`): único jeito de um snapshot já existente mudar — sempre audita (NFR1).

## Acceptance Criteria

1. Mês fechado com Realizado já calculado: alterar o dado de vendas depois não muda o número já apurado.
2. Reprocessamento manual explícito recalcula e atualiza o snapshot, com auditoria.
3. **NFR3, reprodutibilidade**: dado o mesmo snapshot, duas consultas retornam o mesmo resultado — verificado de ponta a ponta, não só "a função não tem estado".

## Tasks / Subtasks

- [x] **Task 1: Migration 223 — `farol.metas_realizados_snapshot`** (AC: 1, 2, 3)
  - [x] JSONB do resultado inteiro, `UNIQUE (vigencia_id, fluxo, nivel)`, `motivo` (congelamento_automatico/reprocessamento_manual)
- [x] **Task 2: `obterOuCongelarRealizado` — decide ao vivo vs. snapshot** (AC: 1, 2, 3)
  - [x] `backend/handlers/farol_metas_congelamento.go`
  - [x] `MetasRealizadoHandler` (Story 4.1) agora chama esta função em vez de `CalcularRealizado` direto
  - [x] `POST /api/farol/metas-realizado/reprocessar` — só `gestor_geral`, sempre audita
- [x] **Task 3: Testes — o fluxo completo é o que prova o FR17 de verdade** (AC: 1, 2, 3)
  - [x] `TestObterOuCongelar_VigenciaAberta_SempreAoVivo` — confirma que vigência aberta nunca grava snapshot — PASS
  - [x] `TestObterOuCongelar_FluxoCompleto` — **o teste mais importante desta story**: calcula aberta → fecha → congela (1000) → MUDA o dado de vendas → calcula de novo (continua 1000, congelado) → reprocessa manualmente → calcula de novo (agora 100999, refletindo o reprocessamento) → confirma auditoria — PASS
  - [x] `TestMetasRealizadoReprocessar_RequerGestorGeral_403` — PASS

## Dev Agent Record

### Agent Model Used

Claude Sonnet 5

### Completion Notes List

3 testes novos, todos passando — o teste de fluxo completo é a prova real do FR17/NFR3 (simula dado de vendas mudando depois do fechamento e confirma que o número congelado não se move sozinho). Regressão: mesmas falhas pré-existentes.

### File List

- `backend/migrations/223_metas_realizados_snapshot.sql` (novo)
- `backend/handlers/farol_metas_congelamento.go` (novo)
- `backend/handlers/farol_metas_congelamento_test.go` (novo)
- `backend/handlers/farol_metas_calculo.go` (modificado — handler usa obterOuCongelarRealizado)
- `backend/main.go` (modificado)

### Change Log

- 2026-09-02: Congelamento de mês fechado implementado com snapshot persistido e reprocessamento manual auditado.
