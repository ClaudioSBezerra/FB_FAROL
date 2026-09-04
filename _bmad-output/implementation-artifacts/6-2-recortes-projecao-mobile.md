---
epic: 6
story: 2
story_key: 6-2-recortes-projecao-mobile
baseline_commit: b3fa8f8
---

# Story 6.2: Recortes de tempo e projeção no mobile

Status: review

## Story

Como Supervisor/GGV,
Eu quero ver os mesmos recortes de tempo e a projeção de fechamento no mobile, adaptados pra tela pequena,
Para que eu tenha a mesma informação que teria no painel web, mesmo estando em campo.

Fonte: [`epics.md`](../planning-artifacts/epics.md) (Épico 6, Story 6.2) · FR21↔FR23, FR18 · GitHub issue #27. **Última story do PRD inteiro** — fecha o Épico 6 e o módulo Painel de Gestão de Metas por Indústria por completo.

## Decisão de arquitetura

Extraí `calcularRealizadoEscopoPublico` (novo helper em `farol_metas_public.go`) unificando o caminho "período inteiro da vigência" (uso normal) e "recorte" (Story 6.2) — o handler público agora usa a mesma função pros dois casos, só variando se passa um override de data. Isso evitou duplicar a lógica de filtro-por-escopo (Story 6.1) + recálculo de total quatro vezes (uma por recorte).

**Isolamento de escopo continua valendo em TODOS os recortes** — testado explicitamente: nenhum recorte do painel do RCA-A mostra a Rede do RCA-B.

## Acceptance Criteria

1. Painel mobile mostra os mesmos 4 recortes de tempo do painel web (Story 5.3).
2. Projeção de fechamento aparece numa aba separada dos indicadores oficiais, com aviso de estimativa — mesma regra do Épico 5.
3. Isolamento de escopo (Story 6.1) continua valendo em todos os recortes.

## Tasks / Subtasks

- [x] **Task 1: `calcularRealizadoEscopoPublico` + `?recortes=1` no endpoint público** (AC: 1, 3)
  - [x] Refatoração de `MetasPublicPainelHandler` pra usar o novo helper — testes existentes da Story 6.1 confirmaram que nada quebrou
- [x] **Task 2: Testes** (AC: 1, 2, 3)
  - [x] `TestMetasPublicPainel_RecortesRespeitaEscopo` — 4 recortes, 2 RCAs, confirma que NENHUM recorte vaza a Rede do outro RCA — PASS
  - [x] Suíte completa dos testes de Story 6.1 (`TestMetasPublic*`) re-executada e confirmada intacta após a refatoração
- [x] **Task 3: Frontend mobile — abas + recortes** (AC: 1, 2)
  - [x] `FarolPublicMetasPanel.tsx` — abas "Oficial"/"Projeção" (mesmo padrão da Story 5.3, adaptado pra mobile), tabela de recortes na aba Projeção
  - [x] `tsc --noEmit` limpo

## Dev Agent Record

### Agent Model Used

Claude Sonnet 5

### Completion Notes List

1 teste novo (recortes + isolamento de escopo combinados), suíte completa estável em 3 rodadas consecutivas (mesmas 3 falhas pré-existentes). **Módulo Painel de Gestão de Metas por Indústria completo: 22/22 stories, 6 épicos, todos os cards do board em In Review.**

### File List

- `backend/handlers/farol_metas_public.go` (modificado — calcularRealizadoEscopoPublico, recortes=1)
- `backend/handlers/farol_metas_public_test.go` (modificado — 1 teste novo)
- `frontend/src/pages/farol/FarolPublicMetasPanel.tsx` (modificado — abas + recortes)

### Change Log

- 2026-09-02: Recortes de tempo e projeção de fechamento no painel mobile. Módulo completo.
