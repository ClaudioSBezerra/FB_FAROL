---
epic: 2
story: 1
story_key: 2-1-vinculo-industria-tipo-metrica
baseline_commit: 1e4255e
---

# Story 2.1: Vínculo Indústria/Fornecedor × Tipo de Métrica

Status: review

## Story

Como admin,
Eu quero vincular uma Indústria — ou um Fornecedor específico dentro dela — a um ou mais Tipos de Métrica já cadastrados,
Para que cada fornecedor pode ter suas próprias metas mesmo dentro da mesma indústria.

Fonte: [`epics.md`](../planning-artifacts/epics.md) (Épico 2, Story 2.1) · FR4 · GitHub issue #10.

## Decisão de design importante

`farol.industrias` já resolve o "Fornecedor específico dentro dela" do FR4 — a seed real (migration 210) já trata UNILEVER HC (cod_fornec 396) e UNILEVER FOOD (cod_fornec 131) como **duas linhas separadas** de indústria, não uma indústria com 2 fornecedores. Então o Vínculo só precisa de `industria_id`, sem conceito extra de "fornecedor dentro da indústria".

O que o vínculo guarda é o **valor concreto de cada parâmetro** que o Tipo de Métrica exige (`parametros_valores JSONB`, validado contra o `parametros_schema` do tipo referenciado) — isso é diferente de "meta por faixa" (Story 2.2): parâmetro calibra COMO calcular (ex: limiar em R$ pra Cobertura), meta é O QUANTO precisa atingir.

## Acceptance Criteria

1. Crio um vínculo para o fornecedor 131-Foods com "Cobertura por Rede" e outro vínculo para o fornecedor 396-HC com o mesmo tipo — os dois existem de forma independente, cada um com seus próprios parâmetros.
2. **Fecha o AC pendente da Story 1.3 (FR3)**: o mesmo Tipo de Métrica reutilizado por 2 indústrias diferentes, cada vínculo com parâmetro independente — testado de ponta a ponta.
3. Todo parâmetro exigido pelo `parametros_schema` do Tipo de Métrica precisa estar preenchido no vínculo — sem isso, 400 com mensagem específica de qual parâmetro falta.
4. Isolamento multi-tenant e auditoria (NFR1), mesmo padrão da Story 1.1.
5. Vínculo duplicado (mesma indústria + mesmo tipo) → 409.

## Tasks / Subtasks

- [x] **Task 1: Migration 216 — tabela `farol.metas_vinculos`** (AC: 1, 2, 3, 5)
  - [x] FK `industria_id` → `farol.industrias` (CASCADE), `tipo_metrica_id` → `farol.tipos_metrica` (RESTRICT — apagar um tipo em uso deve falhar alto)
  - [x] `parametros_valores JSONB`, `UNIQUE (empresa_id, industria_id, tipo_metrica_id)`
- [x] **Task 2: Handler Go — CRUD com validação cruzada de schema** (AC: 1, 2, 3, 4, 5)
  - [x] `backend/handlers/farol_metas_vinculos.go` — segue o molde de `farol_tipos_metrica.go` (transação, `writeAuditLogTx`, isolamento, `gestor_geral`)
  - [x] `validarParametrosValores`: todo `key` do `parametros_schema` do Tipo de Métrica referenciado precisa existir em `parametros_valores`
  - [x] Resposta enriquecida com JOIN (`industria_nome`, `tipo_metrica_nome`, `parametros_schema`) pra o frontend não precisar de 2 requests extras
- [x] **Task 3: Testes Go — incluindo o teste de reuso adiado da Story 1.3** (AC: 1, 2, 3, 4, 5)
  - [x] `TestMetasVinculos_CriarComParametros` — PASS
  - [x] `TestMetasVinculos_ParametroObrigatorioFaltando_400` — PASS
  - [x] `TestMetasVinculos_ReusoDeTipoPorDuasIndustrias` — **fecha o AC pendente do FR3/Story 1.3** — PASS
  - [x] `TestMetasVinculos_MesmaIndustriaMesmoTipo_Conflito409` — PASS
  - [x] `TestMetasVinculos_IsolamentoEntreEmpresas_404` — PASS
- [x] **Task 4: Frontend — tela de Configuração de Metas (vínculos)** (AC: 1, 2, 3)
  - [x] `frontend/src/pages/ConfigMetasVinculos.tsx` — Select de Indústria + Select de Tipo de Métrica; ao escolher o tipo, renderiza dinamicamente um input por parâmetro do `parametros_schema` (mesmo princípio de genericidade da Story 1.1, agora do lado do consumo)
  - [x] Rota `/gestao/metas-vinculos`, menu "Metas por Indústria"
  - [x] `tsc --noEmit` limpo

## Dev Agent Record

### Agent Model Used

Claude Sonnet 5

### Completion Notes List

7 testes Go novos, todos passando (incluindo o de reuso que fecha a Story 1.3). Regressão: mesmas 3 falhas pré-existentes, nada novo quebrado. Ressalva igual à Story 1.1: sem navegador real disponível nesta VM — verificação via type-check + testes de integração do backend.

### File List

- `backend/migrations/216_metas_vinculos.sql` (novo)
- `backend/handlers/farol_metas_vinculos.go` (novo)
- `backend/handlers/farol_metas_vinculos_test.go` (novo)
- `backend/main.go` (modificado)
- `frontend/src/pages/ConfigMetasVinculos.tsx` (novo)
- `frontend/src/App.tsx` (modificado)
- `frontend/src/lib/navigation.ts` (modificado)

### Change Log

- 2026-09-02: Vínculo Indústria × Tipo de Métrica implementado ponta-a-ponta; fecha o AC de reuso adiado da Story 1.3.
