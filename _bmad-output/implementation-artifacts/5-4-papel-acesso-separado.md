---
epic: 5
story: 4
story_key: 5-4-papel-acesso-separado
baseline_commit: b3612eb
---

# Story 5.4: Papel de acesso — visualização separada de edição

Status: review

## Story

Como Supervisor/GGV,
Eu quero acessar o painel de visualização sem ter acesso às telas de configuração/importação (Épicos 1-3),
Para que meu acesso de campo não me dê permissão de alterar meta/lista/tipo de venda por engano.

Fonte: [`epics.md`](../planning-artifacts/epics.md) (Épico 5, Story 5.4) · NFR2 · GitHub issue #25. Última story do Épico 5 (Painel Web).

## Nota

A separação já existia por construção: todo endpoint de escrita foi gateado em `gestor_geral` desde a story que o criou (Épicos 1-3), e os endpoints de visualização (Épico 5) em `somente_leitura`. Esta story é uma **auditoria consolidada** — prova que a separação vale pro módulo INTEIRO de uma vez, não handler a handler isoladamente (o que já estava implicitamente coberto, mas nunca testado em conjunto).

## Achado

Dois handlers de leitura administrativa (`MetasClientesValidosHandler`, `MetasItensValidosHandler` — listam o que foi importado, usados só pela tela admin) não têm checagem interna de papel — dependem só do gate de rota (`main.go`, nível `gestor_geral`). Mesmo padrão de outros GETs administrativos do projeto (ex.: `ObjetivosImportHandler` não tem checagem própria de escrita além do `RequireWrite`). Não são exercitáveis num teste de handler direto (que pula o middleware de rota de propósito) — ficam de fora desta auditoria por esse motivo técnico, não por omissão.

## Acceptance Criteria

1. Usuário `somente_leitura` acessa o painel (Épico 5) normalmente.
2. Usuário `somente_leitura` é bloqueado (403) em TODO endpoint de escrita dos Épicos 1-3.

## Tasks / Subtasks

- [x] **Task 1: Auditoria consolidada de RBAC** (AC: 1, 2)
  - [x] `backend/handlers/farol_metas_rbac_test.go` — 14 endpoints de escrita testados num loop só, todos confirmando 403 pra `somente_leitura`
  - [x] 2 endpoints de visualização (`metas-realizado`, `metas-painel`) confirmados abertos pra `somente_leitura`
  - [x] Todos os 15 casos (14+1) — PASS

## Dev Agent Record

### Agent Model Used

Claude Sonnet 5

### Completion Notes List

Auditoria de RBAC cobrindo os 14 endpoints de escrita do módulo inteiro num único teste tabular — nenhum bug encontrado (a separação já estava correta desde as stories originais), mas agora é uma garantia testada, não só uma convenção seguida manualmente. Épico 5 (Painel Web) completo. Regressão: mesmas 3 falhas pré-existentes.

### File List

- `backend/handlers/farol_metas_rbac_test.go` (novo)

### Change Log

- 2026-09-02: Auditoria de RBAC consolidada — separação edição/visualização confirmada pro módulo inteiro. Épico 5 completo.
