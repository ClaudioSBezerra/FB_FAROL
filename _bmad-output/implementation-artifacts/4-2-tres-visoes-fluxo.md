---
epic: 4
story: 2
story_key: 4-2-tres-visoes-fluxo
baseline_commit: 6c9aa9d
---

# Story 4.2: Três visões de fluxo (Faturado / Transmitido / Soma)

Status: review

## Story

Como Supervisor,
Eu quero alternar a visão do Realizado entre Faturado, Transmitido (Emitido) e a Soma dos dois,
Para que eu veja tanto o que já foi vendido quanto o que já foi confirmado, dependendo do que preciso checar.

Fonte: [`epics.md`](../planning-artifacts/epics.md) (Épico 4, Story 4.2) · FR15 · GitHub issue #19.

## Nota

A capacidade já existia desde a Story 4.1 (`CalcularRealizado` recebe `fluxo` como parâmetro, com switch faturado/transmitido/soma) — mas só o caminho "faturado" tinha teste. Esta story fecha a lacuna de cobertura pros outros dois caminhos, que é onde bugs de verdade poderiam se esconder (eram os únicos não exercitados).

## Acceptance Criteria

1. Fluxo "transmitido" lê `vendas_transmitidas`, isolado de `vendas_faturadas`.
2. Fluxo "soma" combina os dois corretamente (3000 faturado + 2000 transmitido = 5000).
3. Trocar de fluxo não tem efeito colateral — o valor de "faturado" isolado continua correto depois de calcular "soma" (sem estado compartilhado indevido).

## Tasks / Subtasks

- [x] **Task 1: Testes fechando a lacuna de cobertura dos 3 fluxos** (AC: 1, 2, 3)
  - [x] `TestCalcularRealizado_FluxoTransmitido` — PASS
  - [x] `TestCalcularRealizado_FluxoSoma` (inclui verificação de que "faturado" isolado não foi contaminado) — PASS

## Dev Agent Record

### Agent Model Used

Claude Sonnet 5

### Completion Notes List

Sem código de produção novo — capacidade já implementada na Story 4.1, esta story fecha lacuna de teste real (os 2 fluxos não testados antes). Regressão: mesmas falhas pré-existentes.

### File List

- `backend/handlers/farol_metas_calculo_test.go` (modificado — 2 testes novos + fixtures de vendas_transmitidas)

### Change Log

- 2026-09-02: Fluxos Transmitido e Soma verificados com teste real.
