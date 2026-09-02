---
epic: 5
story: 3
story_key: 5-3-recortes-tempo-projecao
baseline_commit: 0c3a79b
---

# Story 5.3: Recortes de tempo e projeção de fechamento (aba separada)

Status: review

## Story

Como Supervisor/GGV,
Eu quero comparar o Realizado em diferentes recortes de tempo (dia anterior, semana, mês, ano corrente) e, numa aba/tela separada dos indicadores oficiais, visualizar a projeção de fechamento do ano,
Para que eu acompanhe a evolução recente sem misturar o que o fornecedor cobra oficialmente com uma capacidade nova do Farol.

Fonte: [`epics.md`](../planning-artifacts/epics.md) (Épico 5, Story 5.3) · FR21, FR18 · GitHub issue #24.

## Decisão de arquitetura desta story

Até aqui, `CalcularRealizado` sempre usava o período INTEIRO da vigência. Refatorei em `CalcularRealizadoComPeriodo` (nova função, `CalcularRealizado` virou um wrapper fino) que aceita uma janela de datas opcional pra sobrescrever — usada pelos 4 recortes (`dia_anterior`/`semana`/`mes`/`ano_corrente`, resolvidos em `calcularRecorteDatas`).

**Decisão importante**: a PROJEÇÃO de fechamento continua sempre calculada com base nos dias da VIGÊNCIA inteira, nunca do recorte — projetar o fechamento do mês com base só em "ontem" não faz sentido matemático. Recortes afetam o Realizado exibido; a base de dias da projeção nunca muda. Testado explicitamente (`TestCalcularRealizadoComPeriodo_RecorteNaoAfetaProjecao`).

**Recortes são sempre ao vivo** — não passam pelo congelamento da Story 4.3 (são leitura de momentum recente, não o número oficial mensal que precisa ficar estável).

## Acceptance Criteria

1. `?recortes=1` no `/api/farol/metas-painel` retorna os 4 recortes, cada um com seu próprio Realizado.
2. Aba "Projeção" separada dos indicadores oficiais, com aviso visual de que é estimativa.
3. Projeção não muda com o recorte — sempre baseada na vigência inteira.

## Tasks / Subtasks

- [x] **Task 1: `CalcularRealizadoComPeriodo` + `calcularRecorteDatas`** (AC: 1, 3)
  - [x] Refatoração preserva `CalcularRealizado` como wrapper (nenhuma story anterior quebrada — confirmado com a suíte completa)
- [x] **Task 2: Endpoint `/api/farol/metas-painel?recortes=1`** (AC: 1)
  - [x] `PainelResponse.Recortes` — mapa dos 4 recortes, calculados ao vivo
- [x] **Task 3: Testes** (AC: 1, 2, 3)
  - [x] `TestCalcularRecorteDatas` — janelas de data corretas pros 4 recortes — PASS
  - [x] `TestCalcularRealizadoComPeriodo_RecorteNaoAfetaProjecao` — **prova a decisão de arquitetura**: mesmo Realizado (venda incluída nas duas janelas) → mesma projeção, confirmando que a base de dias vem da vigência — PASS
  - [x] `TestMetasPainel_Recortes` — ponta a ponta pelo endpoint, com `dia_anterior`=0 e `semana`=1 pra uma venda de hoje — PASS
- [x] **Task 4: Frontend — abas "Indicadores oficiais" / "Projeção"** (AC: 2)
  - [x] `FarolPainelMetas.tsx` — aba Projeção com aviso âmbar de "estimativa" + tabela dos 4 recortes
  - [x] `tsc --noEmit` limpo

## Dev Agent Record

### Agent Model Used

Claude Sonnet 5

### Completion Notes List

3 testes novos, todos passando — incluindo um que pegou um erro de premissa MEU antes de eu marcar a story como pronta (achei que a projeção "não devia mudar com o recorte" sem perceber que ela depende do Realizado, que legitimamente muda; corrigi o teste pra provar a invariante real: mesma base de dias da vigência). Suíte completa estável em 3 rodadas consecutivas (mesmas 3 falhas pré-existentes).

### File List

- `backend/handlers/farol_metas_calculo.go` (modificado — CalcularRealizadoComPeriodo, calcularRecorteDatas)
- `backend/handlers/farol_metas_calculo_test.go` (modificado — 2 testes novos)
- `backend/handlers/farol_metas_painel.go` (modificado — recortes=1)
- `backend/handlers/farol_metas_painel_test.go` (modificado — 1 teste novo)
- `frontend/src/pages/FarolPainelMetas.tsx` (modificado — abas + recortes)

### Change Log

- 2026-09-02: Recortes de tempo e aba de Projeção separada implementados.
