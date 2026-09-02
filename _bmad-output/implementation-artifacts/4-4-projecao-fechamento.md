---
epic: 4
story: 4
story_key: 4-4-projecao-fechamento
baseline_commit: 98892db
---

# Story 4.4: Projeção de fechamento por nível hierárquico

Status: review

## Story

Como Supervisor/GGV,
Eu quero ver a projeção de fechamento do ano corrente pra cada métrica, com base no ritmo de realização até o momento,
Para que eu saiba hoje se estou no caminho pra bater a meta anual, não só o que já foi realizado até agora.

Fonte: [`epics.md`](../planning-artifacts/epics.md) (Épico 4, Story 4.4) · FR18, FR18a · GitHub issue #21. Fecha o Épico 4 (Motor de Apuração).

## Nota de interpretação do FR18

O texto do FR18 fala em "projeção de fechamento do ano corrente", mas o exemplo numérico que o acompanha usa dias do **período da vigência** ("15 dias de um mês de 30 dias"), não dias do ano. Implementei fiel ao exemplo concreto (mais autoritativo que a frase solta): a projeção usa `data_inicio`/`data_fim` da própria vigência, seja ela mensal, trimestral, etc. — genérico pro tamanho de período que a Story 2.2 já suporta, não hardcoded pra "ano".

## Acceptance Criteria

1. **Verificado com o exemplo numérico exato do FR18**: R$45.000 realizados em 15 dias de um período de 30 → projeção R$90.000.
2. Período já encerrado: projeção = o próprio realizado (nada a extrapolar).
3. **FR18a**: projeção de cada nível (grupo RCA/CRV/GGV) calculada a partir do Realizado PRÓPRIO daquele nível — nunca somando as projeções das Redes que o compõem.

## Tasks / Subtasks

- [x] **Task 1: `projetarFechamento` — método v1 (ritmo linear)** (AC: 1, 2)
  - [x] `RealizadoResultado.Projecao` e `RealizadoGrupo.Projecao` adicionados
  - [x] Ligado em `CalcularRealizado`: projeção do total E de cada grupo, cada uma a partir do `RealizadoTotal` PRÓPRIO (nunca soma de filhos — FR18a por construção)
- [x] **Task 2: Testes** (AC: 1, 2, 3)
  - [x] `TestProjetarFechamento_ExemploDoPRD` — **reproduz o número exato do FR18 (R$90.000)** — PASS
  - [x] `TestProjetarFechamento_PeriodoJaEncerrado_ProjecaoEhORealizado` — PASS
  - [x] `TestCalcularRealizado_ProjecaoPorGrupo_NaoSomaProjecoesFilhas` — confirma que a projeção do grupo vem do realizado próprio, não de somar Redes — PASS

## Dev Agent Record

### Agent Model Used

Claude Sonnet 5

### Completion Notes List

3 testes novos, todos passando — incluindo verificação matemática exata contra o exemplo do FR18. Épico 4 (Motor de Apuração) completo: cálculo do Realizado, 3 fluxos, congelamento com snapshot, e agora projeção de fechamento. Regressão: mesmas falhas pré-existentes.

### File List

- `backend/handlers/farol_metas_calculo.go` (modificado — Projecao em RealizadoResultado/RealizadoGrupo, projetarFechamento)
- `backend/handlers/farol_metas_calculo_test.go` (modificado — 3 testes novos)

### Change Log

- 2026-09-02: Projeção de fechamento (ritmo linear) implementada e verificada contra o exemplo exato do FR18. Épico 4 completo.
