---
epic: 2
story: 3
story_key: 2-3-recorte-organizacional
baseline_commit: 1fc6ef0
---

# Story 2.3: Recorte organizacional do vínculo

Status: review

## Story

Como admin,
Eu quero restringir um vínculo a um recorte organizacional (UF específica, GGVs específicos, ou empresa toda),
Para que consigo configurar programas regionais/parciais — como o da Unilever, restrito a UF=GO e GGVs específicos — sem afetar o resto da operação.

Fonte: [`epics.md`](../planning-artifacts/epics.md) (Épico 2, Story 2.3) · FR5 · GitHub issue #12.

## Decisão de design

`recorte_uf`/`recorte_ggvs` são colunas **adicionadas** em `farol.metas_vinculos` (ALTER, não tabela nova) — são atributo do vínculo, mesma linha. `recorte_ggvs` fica como `TEXT[]` livre (não FK pra uma tabela de GGVs) porque não existe hoje no Farol um cadastro formal de GGV pra referenciar — os nomes usados no programa real (GO, GO FOOD, V7, DF, TELEVENDAS, SITE, BALCAO ANAPOLIS, BALCAO JH) são rótulos operacionais, não códigos de uma tabela mestra. **Nota pra épicos futuros**: se aparecer um cadastro formal de GGV, migrar pra FK é um ajuste pequeno; por ora, texto livre é a decisão certa (evita inventar um cadastro que não foi pedido).

A aplicação REAL do recorte (filtrar quem entra na apuração/painel) é do Épico 4/5 — esta story só garante que o dado é persistido e volta corretamente.

## Acceptance Criteria

1. Configuro o recorte de um vínculo como UF=GO + GGVs [GO, GO FOOD, V7] — persiste e volta corretamente na consulta.
2. Vazio em ambos os eixos = "empresa toda" (sem restrição) — comportamento padrão pra vínculos existentes.
3. NFR1: alteração de recorte é auditada.

## Tasks / Subtasks

- [x] **Task 1: Migration 218 — ALTER TABLE com as 2 colunas** (AC: 1, 2)
  - [x] `recorte_uf TEXT` (nullable), `recorte_ggvs TEXT[] NOT NULL DEFAULT '{}'`
- [x] **Task 2: Handler — persistir e retornar o recorte** (AC: 1, 2, 3)
  - [x] `MetaVinculoRequest`/`Response` estendidos; INSERT/UPDATE gravam `recorte_uf`/`recorte_ggvs` (via `pq.Array`)
  - [x] **Bug real pego pelo teste**: `pq.Array(nil []string)` gera SQL `NULL`, não array vazio — violava o `NOT NULL DEFAULT '{}'` da coluna. Corrigido inicializando `req.RecorteGGVs = []string{}` quando vier nil do JSON.
- [x] **Task 3: Teste Go** (AC: 1, 2, 3)
  - [x] `TestMetasVinculos_RecorteOrganizacional` — cria vínculo com UF+GGVs, confirma persistência — PASS (e pegou o bug acima antes de eu marcar como pronto)
- [x] **Task 4: Frontend — campos de recorte no form de vínculo** (AC: 1, 2)
  - [x] `ConfigMetasVinculos.tsx`: campos UF (input curto) + GGVs (texto separado por vírgula, convertido pra array no submit); coluna "Recorte" na tabela mostrando "Empresa toda" quando vazio
  - [x] `tsc --noEmit` limpo

## Dev Agent Record

### Agent Model Used

Claude Sonnet 5

### Completion Notes List

O teste pegou um bug real antes do código ser considerado pronto (nil slice → NULL em vez de array vazio, violando NOT NULL) — corrigido e reverificado. Regressão: mesmas 3 falhas pré-existentes.

### File List

- `backend/migrations/218_metas_vinculos_recorte.sql` (novo)
- `backend/handlers/farol_metas_vinculos.go` (modificado)
- `backend/handlers/farol_metas_vinculos_test.go` (modificado)
- `frontend/src/pages/ConfigMetasVinculos.tsx` (modificado)

### Change Log

- 2026-09-02: Recorte organizacional (UF + GGVs) adicionado ao vínculo, com bug de array nulo corrigido durante o teste.
