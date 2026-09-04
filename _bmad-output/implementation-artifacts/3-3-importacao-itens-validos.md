---
epic: 3
story: 3
story_key: 3-3-importacao-itens-validos
baseline_commit: 8ae4c4d
---

# Story 3.3: Importação de Itens Válidos (EAN + embalagem)

Status: review

## Story

Como admin,
Eu quero fazer upload de um CSV com os EANs válidos de um vínculo/vigência, mapeados para o(s) cod_prod interno(s) e com o tipo de embalagem,
Para que o motor de apuração (Épico 4) saiba quais produtos contam para "Sortimento" e se a regra de quantidade mínima se aplica a cada um.

Fonte: [`epics.md`](../planning-artifacts/epics.md) (Épico 3, Story 3.3) · FR12 · GitHub issue #16. Mesmo padrão estrutural da Story 3.2, adaptado pro shape de dado desta lista.

## Decisão de design

`tipo_embalagem` é `CHECK` constraint (`UN`/`CX`/`PACOTE`/`DISPLAY`), diferente de `rede_nome` (Story 3.2, texto livre) — embalagem é conceito fixo do catálogo de produtos do distribuidor, não varia por fornecedor, então travar o domínio é a decisão certa aqui (ao contrário de Rede). Unicidade é `(vigencia_id, ean, cod_prod)`, não `(vigencia_id, ean)`, porque um EAN pode ter várias variantes de embalagem mapeadas (AC explícito do FR12).

## Acceptance Criteria

1. CSV `ean;cod_prod;tipo_embalagem` — um EAN pode aparecer em várias linhas (cod_prod diferentes).
2. **FR12, regra central**: tipo de embalagem é obrigatório e restrito a UN/CX/PACOTE/DISPLAY (case-insensitive na entrada, normalizado). Linha inválida rejeita o lote inteiro (FR9).
3. Combinação EAN+cod_prod duplicada no mesmo arquivo é erro (mesmo princípio de qualidade de dado da Story 3.2).
4. Reimportação substitui a lista; vigência fechada bloqueia.

## Tasks / Subtasks

- [x] **Task 1: Migration 221 — `farol.metas_itens_validos`** (AC: 1, 2, 3)
  - [x] `CHECK (tipo_embalagem IN ('UN','CX','PACOTE','DISPLAY'))`, `UNIQUE (vigencia_id, ean, cod_prod)`
- [x] **Task 2: Handler de importação CSV atômico** (AC: 1, 2, 3, 4)
  - [x] `backend/handlers/farol_metas_itens_validos_csv.go` — mesmo padrão de 3.1/3.2, normaliza `tipo_embalagem` pra maiúsculo antes de validar
- [x] **Task 3: Testes Go** (AC: 1, 2, 3, 4)
  - [x] `TestMetasItensValidos_ImportarLoteValido` (inclui EAN com 2 variantes de embalagem) — PASS
  - [x] `TestMetasItensValidos_EmbalagemInvalida_FR12` — verifica banco depois (0 itens) — PASS
  - [x] `TestMetasItensValidos_EmbalagemCaseInsensitive` — PASS
  - [x] `TestMetasItensValidos_CombinacaoDuplicada_400` — PASS
  - [x] `TestMetasItensValidos_VigenciaFechada_403` — PASS
- [x] **Task 4: Frontend — botão "Itens" ao lado de "Clientes"** (AC: 1, 2, 3, 4)
  - [x] Estendido `VigenciasDialog` — mesmo padrão do botão de Clientes (Story 3.2)
  - [x] `tsc --noEmit` limpo

## Dev Agent Record

### Agent Model Used

Claude Sonnet 5

### Completion Notes List

5 testes Go, todos passando. Regressão: mesmas 3 falhas pré-existentes.

### File List

- `backend/migrations/221_metas_itens_validos.sql` (novo)
- `backend/handlers/farol_metas_itens_validos_csv.go` (novo)
- `backend/handlers/farol_metas_itens_validos_csv_test.go` (novo)
- `backend/main.go` (modificado)
- `frontend/src/pages/ConfigMetasVinculos.tsx` (modificado — upload de Itens por vigência)

### Change Log

- 2026-09-02: Importação de Itens Válidos (EAN + embalagem).
