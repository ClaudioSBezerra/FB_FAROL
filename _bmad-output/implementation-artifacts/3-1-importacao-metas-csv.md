---
epic: 3
story: 1
story_key: 3-1-importacao-metas-csv
baseline_commit: 882230d
---

# Story 3.1: Importação de metas via CSV

Status: review

## Story

Como admin,
Eu quero fazer upload de um CSV com valores de meta por vínculo/faixa/vigência,
Para que eu não precise digitar cada valor manualmente na UI (Épico 2) quando recebo a planilha do fornecedor.

Fonte: [`epics.md`](../planning-artifacts/epics.md) (Épico 3, Story 3.1) · FR8, FR9 · GitHub issue #14. Primeira story do Épico 3.

## Decisão de design

`ObjetivosImportHandler` (import de vendas, alto volume) usa streaming SSE com upsert em lote — **não serve de molde aqui**. FR9 exige atomicidade estrita ("sem aplicar parcialmente um lote com erro"), e o volume de metas é baixo (dezenas de linhas). Escrevi um handler síncrono: valida o arquivo INTEIRO primeiro (formato de cada linha, depois existência de cada `vinculo_id`, depois regras de negócio por vigência agrupada), e só aplica tudo numa única transação se **zero** erros. Reaproveitei o tratamento de encoding (BOM, Latin-1→UTF-8) do `ObjetivosImportHandler` — isso é boa prática de robustez, não acoplamento indevido.

Refatorei `MetasVigenciasHandler`: extraí `inserirVigenciaTx` (insert de vigência + faixas dentro de uma tx) como função compartilhada entre o POST normal (Story 2.2) e esta importação — mesma regra de negócio, dois pontos de entrada, sem duplicar lógica.

## Acceptance Criteria

1. CSV com colunas `vinculo_id;data_inicio;data_fim;faixa;valor_meta` — linhas com o mesmo vínculo+período agrupam numa única vigência (mesmo modelo da Story 2.2).
2. **FR9, o AC mais importante**: se qualquer linha tiver erro, NENHUMA linha do lote é aplicada — nem as que estariam corretas sozinhas.
3. Erros reportados linha a linha, de forma clara (`{"erros":[{"linha":N,"erro":"..."}]}`).
4. Sobreposição com vigência já existente (mesma regra da Story 2.2, constraint de banco) também barra o lote inteiro.

## Tasks / Subtasks

- [x] **Task 1: Handler de importação síncrono e atômico** (AC: 1, 2, 3, 4)
  - [x] `backend/handlers/farol_metas_import_csv.go` — parse completo, 3 fases de validação (formato → existência de vínculo → regra de negócio por vigência) antes de qualquer escrita
  - [x] Refatoração: `inserirVigenciaTx` extraído de `farol_metas_vigencias.go`, reaproveitado aqui
- [x] **Task 2: Testes Go — FR9 é o teste mais importante desta story** (AC: 1, 2, 3, 4)
  - [x] `TestMetasImportarCSV_LoteValido` — PASS
  - [x] `TestMetasImportarCSV_LinhaComErro_NadaAplicado` — **verifica o banco depois**, não só o código HTTP: confirma que vínculos com linha válida também ficam com ZERO vigências quando outra linha do mesmo arquivo falha — PASS
  - [x] `TestMetasImportarCSV_VinculoInexistente_400` — PASS
  - [x] `TestMetasImportarCSV_ColunaObrigatoriaAusente_400` — PASS
  - [x] `TestMetasImportarCSV_SobreposicaoComVigenciaExistente_409` — PASS
- [x] **Task 3: Frontend — botão de upload** (AC: 1, 2, 3)
  - [x] Botão "Importar Metas (CSV)" em `ConfigMetasVinculos.tsx`, input de arquivo escondido, erros exibidos linha a linha no toast
  - [x] `tsc --noEmit` limpo

## Dev Agent Record

### Agent Model Used

Claude Sonnet 5

### Completion Notes List

5 testes Go, todos passando — o mais importante (`LinhaComErro_NadaAplicado`) verifica o estado real do banco depois da tentativa, não só o status HTTP, confirmando atomicidade de verdade. Regressão: mesmas 3 falhas pré-existentes.

### File List

- `backend/handlers/farol_metas_import_csv.go` (novo)
- `backend/handlers/farol_metas_import_csv_test.go` (novo)
- `backend/handlers/farol_metas_vigencias.go` (modificado — `inserirVigenciaTx` extraído)
- `backend/main.go` (modificado)
- `frontend/src/pages/ConfigMetasVinculos.tsx` (modificado)

### Change Log

- 2026-09-02: Importação de metas via CSV, estritamente atômica (FR9).
