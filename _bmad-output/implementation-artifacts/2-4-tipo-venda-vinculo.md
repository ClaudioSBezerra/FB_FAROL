---
epic: 2
story: 4
story_key: 2-4-tipo-venda-vinculo
baseline_commit: 60b2529
---

# Story 2.4: Tipo de venda configurável por vínculo

Status: review

## Story

Como admin,
Eu quero configurar quais tipos de venda (ex: só Tipo 1 e 9) contam para a apuração de um vínculo específico,
Para que o cálculo desse fornecedor não usa o "Líquido" padrão do Farol quando o contrato exige uma regra diferente.

Fonte: [`epics.md`](../planning-artifacts/epics.md) (Épico 2, Story 2.4) · FR6 · GitHub issue #13. Fecha o Épico 2 (última story de Configuração de Metas).

## Acceptance Criteria

1. Configuro `tipos_venda_validos = ["1", "9"]` no vínculo — persiste e volta corretamente.
2. Vazio (default) = motor de apuração (Épico 4, ainda não implementado) usa "Líquido" padrão — aqui só é o dado configurado, a aplicação real é Épico 4.
3. NFR1: alteração é auditada.

## Tasks / Subtasks

- [x] **Task 1: Migration 219 — ALTER `tipos_venda_validos TEXT[]`** (AC: 1, 2)
  - [x] `tipo_venda` já existe como TEXT em `vendas_faturadas`/`vendas_transmitidas` (migration 187/203) — aqui é só a lista de códigos válidos por vínculo, mesmo tipo de dado
- [x] **Task 2: Handler — persistir e retornar** (AC: 1, 2, 3)
  - [x] Mesmo padrão de `recorte_ggvs` (Story 2.3), já com a correção de nil-slice aplicada preventivamente (sem bug desta vez)
- [x] **Task 3: Teste Go** (AC: 1, 2, 3)
  - [x] `TestMetasVinculos_TipoVendaValido` — PASS
- [x] **Task 4: Frontend** (AC: 1, 2)
  - [x] Campo de texto (códigos separados por vírgula) no form de vínculo, mesmo padrão de GGVs
  - [x] `tsc --noEmit` limpo

## Dev Agent Record

### Agent Model Used

Claude Sonnet 5

### Completion Notes List

Última story do Épico 2 — fecha Configuração de Metas por Indústria. Regressão: mesmas 3 falhas pré-existentes, nada novo quebrado.

### File List

- `backend/migrations/219_metas_vinculos_tipo_venda.sql` (novo)
- `backend/handlers/farol_metas_vinculos.go` (modificado)
- `backend/handlers/farol_metas_vinculos_test.go` (modificado)
- `frontend/src/pages/ConfigMetasVinculos.tsx` (modificado)

### Change Log

- 2026-09-02: Tipo de venda configurável por vínculo. Épico 2 completo.
