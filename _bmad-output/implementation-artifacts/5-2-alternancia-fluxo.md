---
epic: 5
story: 2
story_key: 5-2-alternancia-fluxo
baseline_commit: a72b8c8
---

# Story 5.2: Alternância de fluxo (Faturado / Transmitido / Soma)

Status: review

## Story

Como Supervisor,
Eu quero alternar o painel entre Faturado, Transmitido e Soma,
Para que eu veja o indicador na visão que preciso checar, igual já faço hoje no restante do Farol.

Fonte: [`epics.md`](../planning-artifacts/epics.md) (Épico 5, Story 5.2) · FR20 · GitHub issue #23.

## Nota

Capacidade já implementada na Story 5.1: `FarolPainelMetas.tsx` já tinha o seletor de Fluxo (estado independente do Nível), e `/api/farol/metas-painel` já repassa `fluxo` pro motor (Épico 4, Story 4.2). Esta story fecha a lacuna de teste específica do painel combinado (Meta+Realizado+delta), que não tinha sido testada trocando fluxo — só o motor isolado tinha (Story 4.2).

## Acceptance Criteria

1. Trocar fluxo recalcula Realizado/delta corretamente no painel combinado.
2. Nível de drill-down (rede/rca/crv/ggv) não se perde ao trocar fluxo — são estados independentes.
3. Fluxo sem dado (ex: transmitido sem nenhuma venda transmitida) mostra 0, não herda o valor de outro fluxo.

## Tasks / Subtasks

- [x] **Task 1: Teste do painel combinado com troca de fluxo** (AC: 1, 2, 3)
  - [x] `TestMetasPainel_AlternanciaFluxo` — confirma nível preservado entre as duas chamadas e que fluxo=transmitido não vaza o valor do faturado — PASS

## Dev Agent Record

### Agent Model Used

Claude Sonnet 5

### Completion Notes List

Sem código de produção novo — capacidade já existia desde a Story 5.1 (UI) e Story 4.2 (motor). Fecha lacuna de teste no nível do painel combinado. Regressão: mesmas 3 falhas pré-existentes, suíte estável.

### File List

- `backend/handlers/farol_metas_painel_test.go` (modificado — 1 teste novo)

### Change Log

- 2026-09-02: Alternância de fluxo verificada no painel combinado (Meta+Realizado+delta).
