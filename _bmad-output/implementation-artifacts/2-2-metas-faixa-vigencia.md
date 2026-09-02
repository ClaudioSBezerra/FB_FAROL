---
epic: 2
story: 2
story_key: 2-2-metas-faixa-vigencia
baseline_commit: 692f6ff
---

# Story 2.2: Metas por faixa e histórico de vigências

Status: review

## Story

Como admin,
Eu quero definir, para cada vínculo, valores de meta por faixa (ex: Faixa 3/2/1), cadastrando quantas vigências diferentes forem necessárias ao longo do ano,
Para que a meta acompanha mudanças de contrato/objetivo mês a mês sem perder o histórico de períodos anteriores.

Fonte: [`epics.md`](../planning-artifacts/epics.md) (Épico 2, Story 2.2) · FR5, FR7 · GitHub issue #11.

## Acceptance Criteria

1. Cadastro a vigência jan-mar/2026 com Faixa 3/2/1, depois cadastro abr-jun/2026 com valores diferentes — as duas ficam registradas como períodos distintos.
2. Vigência tem status aberta/fechada. Fechada, os valores não podem mais ser editados por esta tela — só reprocessamento manual (Épico 4, não implementado ainda; aqui só existe o bloqueio).
3. Toda criação/edição de vigência/faixa é auditada (NFR1).
4. **Duas vigências do mesmo vínculo não podem se sobrepor em data** — garantido no nível do banco (constraint EXCLUDE), não só em Go.

## Decisão de design

`valor_meta` (Story 2.2) é distinto de `parametros_valores` (Story 2.1): parâmetro calibra COMO calcular a métrica (ex: limiar R$), meta é O QUANTO precisa atingir (ex: 78/85/91 Redes cobertas). O AC original citava "meta X automática ao virar o mês" — implementei só o fechamento **manual** (`POST .../fechar`); o fechamento automático mensal é responsabilidade do motor de apuração (Épico 4, job agendado), não desta tela CRUD.

## Tasks / Subtasks

- [x] **Task 1: Migration 217 — `farol.metas_vigencias` + `farol.metas_faixas`** (AC: 1, 2, 4)
  - [x] `CREATE EXTENSION IF NOT EXISTS btree_gist` (precedente: `067_add_trgm_indexes.sql` já usa `CREATE EXTENSION IF NOT EXISTS pg_trgm`, comprovado que funciona neste Postgres)
  - [x] `EXCLUDE USING gist (vinculo_id WITH =, daterange(data_inicio, data_fim, '[]') WITH &&)` — sobreposição impossível mesmo sob concorrência
  - [x] `status CHECK (status IN ('aberta','fechada'))`
- [x] **Task 2: Handler Go — vigência + faixas + fechamento manual** (AC: 1, 2, 3, 4)
  - [x] `backend/handlers/farol_metas_vigencias.go`: `MetasVigenciasHandler` (GET ?vinculo_id=, POST cria vigência+faixas numa transação), `MetaVigenciaItemHandler` (PUT/DELETE só se `status='aberta'`, `POST .../fechar` congela)
  - [x] Conflito de sobreposição: `strings.Contains(err.Error(), "ex_farol_metas_vigencias_sem_overlap")` → 409
- [x] **Task 3: Testes Go** (AC: 1, 2, 3, 4)
  - [x] `TestMetasVigencias_CriarComFaixas` — PASS
  - [x] `TestMetasVigencias_MultiplasVigenciasMesmoVinculo` — **AC central da story** — PASS
  - [x] `TestMetasVigencias_Sobreposicao_Conflito409` — PASS
  - [x] `TestMetasVigencias_FecharBloqueiaEdicao` — PUT e DELETE numa vigência fechada, ambos 403 — PASS
  - [x] `TestMetasVigencias_FaixaVazia_400` — PASS
- [x] **Task 4: Frontend — gestão de vigências dentro do vínculo** (AC: 1, 2)
  - [x] Estendido `ConfigMetasVinculos.tsx` (não criei página nova): botão "Vigências" por linha abre `VigenciasDialog` — lista vigências existentes (com badge aberta/fechada, botão "Fechar"), formulário pra nova vigência com faixas dinâmicas
  - [x] `tsc --noEmit` limpo

## Dev Agent Record

### Agent Model Used

Claude Sonnet 5

### Completion Notes List

5 testes Go novos, todos passando — incluindo verificação real da constraint EXCLUDE (sobreposição barrada pelo banco) e do congelamento (PUT/DELETE bloqueados numa vigência fechada). Regressão: mesmas 3 falhas pré-existentes.

### File List

- `backend/migrations/217_metas_vigencias.sql` (novo)
- `backend/handlers/farol_metas_vigencias.go` (novo)
- `backend/handlers/farol_metas_vigencias_test.go` (novo)
- `backend/main.go` (modificado)
- `frontend/src/pages/ConfigMetasVinculos.tsx` (modificado — `VigenciasDialog` adicionado)

### Change Log

- 2026-09-02: Vigências e faixas implementadas, com sobreposição impossível por constraint de banco e congelamento manual funcional.
