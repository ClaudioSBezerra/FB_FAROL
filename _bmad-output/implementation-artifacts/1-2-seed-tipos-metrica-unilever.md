---
epic: 1
story: 2
story_key: 1-2-seed-tipos-metrica-unilever
baseline_commit: 229fb0b
---

# Story 1.2: Seed dos Tipos de Métrica de referência Unilever

Status: review

## Story

Como admin,
Eu quero que o sistema já venha com "Cobertura por Rede" e "Sortimento por Rede" pré-cadastrados,
Para que eu não precise recriar manualmente os tipos de métrica do primeiro programa (Unilever) antes de configurar as metas.

Fonte: [`_bmad-output/planning-artifacts/epics.md`](../planning-artifacts/epics.md) (Épico 1, Story 1.2) · FR2 · GitHub issue #7. Depende só da Story 1.1 (tabela `farol.tipos_metrica` + handler CRUD já existem).

## Acceptance Criteria

1. Numa instalação nova (ou existente), o catálogo já contém "Cobertura por Rede" e "Sortimento por Rede" pra toda empresa, sem ação manual do admin.
2. "Cobertura por Rede": `nivel_agregacao = 'rede'`, parâmetro `limiar_valor_medio` (number) — limiar de valor médio de compra por loja, em R$.
3. "Sortimento por Rede": `nivel_agregacao = 'rede'`, parâmetro `qtd_minima_positivacao` (integer) — quantidade mínima de unidades pra um item contar como positivado (não aplicável a itens vendidos em CX/Pacote/Display — essa exceção é regra do motor de apuração, Épico 4, não dado armazenado aqui).
4. Reexecutar a migration (ou rodar num banco onde o admin já tenha criado manualmente um tipo com o mesmo nome) não duplica nem sobrescreve — `ON CONFLICT DO NOTHING`, mesmo padrão da migration 210 (seed de Indústrias).

## Nota de design (decisão tomada nesta story, não estava 100% explícita no PRD)

A "lista de itens válidos" e a "lista de Clientes/Redes válidas" citadas no FR2 **não** são parâmetros do Tipo de Métrica — são dado mensal importado via Épico 3 (FR11/FR12). O `parametros_schema` de Sortimento por Rede aqui é só o parâmetro numérico de regra de cálculo (`qtd_minima_positivacao`), não a lista em si. Isso mantém a separação: Tipo de Métrica = mecânica de cálculo reutilizável; Vínculo (Épico 2) = valores de meta; Listas Mensais (Épico 3) = dado de entrada mensal.

## Tasks / Subtasks

- [x] **Task 1: Migration 215 — seed de Cobertura/Sortimento por Rede** (AC: 1, 2, 3, 4)
  - [x] `backend/migrations/215_tipos_metrica_seed_unilever.sql`, mesmo padrão de `210_industrias_seed_inicial.sql` (`DO $$ ... FOR r IN SELECT id AS empresa_id FROM companies LOOP ... END LOOP; END $$;`, `ON CONFLICT (empresa_id, nome) DO NOTHING`)
  - [x] Rodar localmente e confirmar via `SELECT nome, nivel_agregacao, parametros_schema FROM farol.tipos_metrica WHERE nome IN (...)`

- [x] **Task 2: Confirmar via API (reaproveita o handler da Story 1.1, sem código novo)** (AC: 1)
  - [x] `GET /api/farol/tipos-metrica` retorna os dois tipos seedados junto com quaisquer outros já cadastrados manualmente

## Dev Notes

Sem handler/frontend novo — Story 1.1 já cobre CRUD e exibição. Esta story é só dado.

### References

- [Source: backend/migrations/210_industrias_seed_inicial.sql — molde de seed por empresa]
- [Source: _bmad-output/planning-artifacts/prds/prd-FB_FAROL-2026-09-02/prd.md#FR2]

## Dev Agent Record

### Agent Model Used

Claude Sonnet 5

### Completion Notes List

Migration 215 aplicada e verificada contra o Postgres local: 2 linhas por empresa (ou N empresas × 2), `parametros_schema` com o shape esperado, `ON CONFLICT DO NOTHING` testado rodando a migration novamente sem erro/duplicata.

### File List

- `backend/migrations/215_tipos_metrica_seed_unilever.sql` (novo)

### Change Log

- 2026-09-02: Seed de Cobertura/Sortimento por Rede pra todas as empresas existentes.
