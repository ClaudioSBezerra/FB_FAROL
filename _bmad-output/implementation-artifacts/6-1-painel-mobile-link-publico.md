---
epic: 6
story: 1
story_key: 6-1-painel-mobile-link-publico
baseline_commit: 9162276
---

# Story 6.1: Exposição do painel mobile no padrão de link público existente

Status: review

## Story

Como Supervisor/GGV,
Eu quero acessar o painel de metas por indústria pelo celular, no mesmo padrão de link direto já usado pelo Farol público (sem exigir login completo),
Para que eu consiga acompanhar a meta em campo sem depender de estar logado no sistema web.

Fonte: [`epics.md`](../planning-artifacts/epics.md) (Épico 6, Story 6.1) · FR22 · GitHub issue #26. Primeira story do Épico 6 (Painel Mobile) — reaproveita a infraestrutura pública já existente (`FarolV2PublicCardsHandler`, `resolveEmpresaCNPJ`) em vez de criar um mecanismo novo.

## Decisão de arquitetura mais importante desta story: isolamento de escopo sem login

Como não há login, o **isolamento por Supervisor/RCA é a única proteção** contra um link vazado expor dado de fora do escopo pretendido. `MetasPublicPainelHandler` sempre calcula o Realizado ao nível de Rede (grão atômico) e depois FILTRA (`filtrarRedesPorEscopo`) pra só as Redes do RCA da URL (ou, no caso de Supervisor, as Redes de todos os RCAs sob aquele Supervisor, resolvido via `resolverHierarquiaRCA` — mesma função do Épico 4). Nunca expõe a lista completa do vínculo. Testado explicitamente com 2 RCAs diferentes no mesmo vínculo/vigência, confirmando que o painel de um não mostra a Rede do outro.

## Acceptance Criteria

1. Endpoint público (`resolveEmpresaCNPJ`, sem auth) resolve empresa pelo mesmo CNPJ do padrão `/m/CNPJ/SUP|RCA/cod` já existente.
2. **Isolamento de escopo**: painel do RCA-A nunca mostra Redes do RCA-B, mesmo no mesmo vínculo/vigência.
3. CNPJ inexistente retorna 404 com mensagem clara (mesmo padrão do `FarolV2PublicCardsHandler`).
4. Link "Metas Indústria" acessível a partir do painel público de vendas já existente (`FarolPublicPanel.tsx`) — não é uma rota órfã.

## Tasks / Subtasks

- [x] **Task 1: Handlers públicos** (AC: 1, 2, 3)
  - [x] `backend/handlers/farol_metas_public.go` — `MetasPublicVinculosHandler`, `MetasPublicVigenciasHandler`, `MetasPublicPainelHandler`
  - [x] `filtrarRedesPorEscopo` + `recalcularTotalDeRedes` — isolamento de escopo, reaproveitando `resolverHierarquiaRCA` (Épico 4)
  - [x] Rotas registradas com `publicHandler` (mesmo padrão do painel de vendas), sem auth
- [x] **Task 2: Testes — isolamento de escopo é o mais importante** (AC: 1, 2, 3)
  - [x] `TestMetasPublicVinculos_ResolvePorCNPJ` — PASS
  - [x] `TestMetasPublicVinculos_CNPJInexistente_404` — PASS
  - [x] `TestMetasPublicPainel_EscopoPorRCA_NaoVazaOutrasRedes` — **teste de segurança central**: 2 RCAs, 2 Redes, confirma que cada painel só vê a própria — PASS
  - [x] `TestMetasPublicPainel_ParametrosObrigatorios_400` — PASS
  - [x] Empresa de teste local não tinha CNPJ cadastrado — corrigido (`UPDATE companies SET cnpj=...`) pra não deixar o teste de segurança mais importante permanentemente pulado
- [x] **Task 3: Frontend mobile** (AC: 1, 4)
  - [x] `frontend/src/pages/farol/FarolPublicMetasPanel.tsx` — nova página mobile, mesmo padrão sem-login de `FarolPublicPanel.tsx`
  - [x] Rotas `/m/:cnpj/sup/:cod/metas-industria` e `/m/:cod/rca/:codRca/metas-industria`
  - [x] Link "Metas Indústria" adicionado ao cabeçalho de `FarolPublicPanel.tsx` — acessível a partir do fluxo existente, não uma URL escondida
  - [x] `tsc --noEmit` limpo

## Dev Agent Record

### Agent Model Used

Claude Sonnet 5

### Completion Notes List

4 testes novos, todos passando — incluindo o teste de segurança mais importante do Épico 6 (isolamento de escopo sem login). Regressão: mesmas 3 falhas pré-existentes, suíte estável em 2 rodadas.

### File List

- `backend/handlers/farol_metas_public.go` (novo)
- `backend/handlers/farol_metas_public_test.go` (novo)
- `backend/main.go` (modificado)
- `frontend/src/pages/farol/FarolPublicMetasPanel.tsx` (novo)
- `frontend/src/pages/farol/FarolPublicPanel.tsx` (modificado — link "Metas Indústria")
- `frontend/src/App.tsx` (modificado)

### Change Log

- 2026-09-02: Painel mobile público de Metas por Indústria, com isolamento de escopo verificado por teste.
