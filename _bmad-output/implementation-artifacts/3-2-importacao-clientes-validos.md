---
epic: 3
story: 2
story_key: 3-2-importacao-clientes-validos
baseline_commit: c78b487
---

# Story 3.2: Importação de Clientes Válidos (Redes + RCA responsável)

Status: review

## Story

Como admin,
Eu quero fazer upload de um CSV com as Redes válidas de um vínculo/vigência, cada CNPJ com o RCA responsável,
Para que a hierarquia de drill-down (GGV→CRV→RCA→Rede→Cliente) tem a atribuição correta de responsável para aquele período.

Fonte: [`epics.md`](../planning-artifacts/epics.md) (Épico 3, Story 3.2) · FR11 · GitHub issue #15. **Executa a decisão de arquitetura da Story 1.4.**

## Decisões de design

Migration 220 aplica exatamente o que a Story 1.4 decidiu: `vinculo_id`/`vigencia_id` como FK (não hierarquia global), `rede_nome TEXT` livre (não FK — Rede varia por fornecedor), sem duplicar RCA→CRV→GGV (só `cod_rca` guardado; resolver o resto é JOIN de quem consome, Épico 4/5).

Decisões novas desta story:
- **Escopo por query string, não por linha do CSV**: `vinculo_id`/`vigencia_id` vêm de `?vinculo_id=&vigencia_id=` (o admin já escolheu a vigência ao clicar "Importar Clientes" na tela), não repetidos em toda linha do arquivo — menos redundância, menos chance de erro de digitação linha a linha.
- **Reimportação SUBSTITUI a lista da vigência** (`DELETE` + `INSERT` na mesma transação) — cada import é "isto é a lista válida agora", não um acréscimo.
- **Vigência fechada bloqueia reimportação** (403) — aplica a mesma regra de congelamento da Story 2.2/3.4 aqui também, mesmo essa story sendo anterior à 3.4 na sequência (a regra já existe desde a Story 2.2, só sendo reaproveitada).
- **CNPJ duplicado dentro do MESMO arquivo é erro** (não só duplicata contra o banco) — decisão nova, não estava no AC original, mas é claramente parte do "regra de qualidade de dado" do FR11 (2 RCAs diferentes pro mesmo CNPJ no mesmo arquivo é ambíguo, não dá pra silenciosamente pegar o último).

## Acceptance Criteria

1. CSV `rede_nome;cnpj;cod_rca` — cada CNPJ importado fica vinculado a exatamente um RCA.
2. **FR11, regra central**: linha com CNPJ sem RCA é rejeitada — e rejeita o LOTE INTEIRO (FR9), não só a linha.
3. Reimportar substitui a lista anterior da mesma vigência.
4. Vigência fechada bloqueia importação (congelamento).
5. CNPJ duplicado no mesmo arquivo é erro (ambiguidade de RCA).

## Tasks / Subtasks

- [x] **Task 1: Migration 220 — `farol.metas_clientes_validos`** (AC: 1, 2, 3, 4, 5)
  - [x] Executa a decisão da Story 1.4 (ver comentário da migration citando o arquivo da story)
- [x] **Task 2: Handler de importação CSV atômico** (AC: 1, 2, 3, 4, 5)
  - [x] `backend/handlers/farol_metas_clientes_validos_csv.go` — mesmo padrão de 3 fases da Story 3.1 (formato → regra de negócio → aplicação atômica)
  - [x] Checagem de vigência fechada ANTES de processar o arquivo (falha rápido)
  - [x] `GET .../metas-clientes-validos?vigencia_id=` pra listar o que já foi importado
- [x] **Task 3: Testes Go** (AC: 1, 2, 3, 4, 5)
  - [x] `TestMetasClientesValidos_ImportarLoteValido` — PASS
  - [x] `TestMetasClientesValidos_CNPJSemRCA_FR11` — verifica banco depois (0 clientes), não só HTTP — PASS
  - [x] `TestMetasClientesValidos_CNPJInvalido_400` — PASS
  - [x] `TestMetasClientesValidos_ReimportacaoSubstituiLista` — PASS
  - [x] `TestMetasClientesValidos_VigenciaFechada_403` — PASS
  - [x] `TestMetasClientesValidos_CNPJDuplicadoNoArquivo_400` — PASS
- [x] **Task 4: Frontend — botão de importar dentro do card de vigência** (AC: 1, 2, 3, 4)
  - [x] Estendido `VigenciasDialog` (dentro de `ConfigMetasVinculos.tsx`): botão "Clientes" por vigência aberta, upload direto pro `vigencia_id` daquele card
  - [x] `tsc --noEmit` limpo

## Dev Agent Record

### Agent Model Used

Claude Sonnet 5

### Completion Notes List

6 testes Go, todos passando. Regressão: mesmas 3 falhas pré-existentes. Esta story valida na prática que a decisão de arquitetura da Story 1.4 (documentada sem código na época) era executável sem ajuste — bom sinal de que a análise antecipada foi correta.

### File List

- `backend/migrations/220_metas_clientes_validos.sql` (novo)
- `backend/handlers/farol_metas_clientes_validos_csv.go` (novo)
- `backend/handlers/farol_metas_clientes_validos_csv_test.go` (novo)
- `backend/main.go` (modificado)
- `frontend/src/pages/ConfigMetasVinculos.tsx` (modificado — upload de Clientes por vigência)

### Change Log

- 2026-09-02: Importação de Clientes Válidos, executando a decisão de arquitetura da Story 1.4.
