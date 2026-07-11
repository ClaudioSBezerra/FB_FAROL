---
task: adequacao-novo-layout
phase: fase-1-import-only
status: complete
completed: 2026-07-01
---

# Adequação ao novo layout ION VENDAS — Fase 1 concluída

## O que foi feito

### Migration 181 — colunas novas em `vendas_faturadas` e `vendas_transmitidas`

Adicionadas 9 colunas em cada tabela (todas com default seguro para preservar
compatibilidade com CSVs antigos):

- `cod_depto`, `depto` — departamento do produto
- `cod_sec`, `secao` — seção do produto
- `cod_categoria`, `categoria` — categoria do produto
- `cod_cliprinc` — cliente principal (rede)
- `fantasia` — nome fantasia do cliente
- `pvenda_unit NUMERIC(15,4)` — preço unitário informativo (o CSV manda)

**Semântica preservada**: `pvenda` continua sendo TOTAL da linha. Todas as
`agg_*_mes` e queries `SUM(pvenda)` seguem funcionando sem mudança.

### Migration 182 — nova tabela `vendas_ccd`

Estrutura completa espelha `vendas_faturadas` (33 colunas comuns) + coluna
`evento` (`CORTADO` | `CANCELADO` | `DEVOLVIDO`) + 8 colunas do novo layout.

Índices mínimos criados:
- PK (`id`)
- `idx_vccd_emp_data(empresa_id, data_evento)` — para range queries
- `idx_vccd_emp_evento_data(empresa_id, evento, data_evento)` — filtro por tipo

Fase 2 adicionará mais índices quando precisar expor CCD no GRID.

### `farol_v2_import.go` — importer estendido

- Detecção das 9 colunas novas via `col()` com aliases (case-insensitive).
- `detectEvento` substitui `detectEstado`: reconhece TRANSMITIDO, FATURADO,
  CORTADO, CANCELADO, DEVOLVIDO. Fallback é FATURADO (compat).
- Struct `vendaRaw` estendida de `[29]any` para `[38]any` + campo `evento`
  para uso exclusivo do fluxo CCD.
- `pvenda_total`: usa `PVENDA_TOTAL` do CSV se presente; fallback
  `PVENDA × QT` (compat CSV antigo).
- `plucro`: gravado como zero quando CSV manda `0` ou vazio (comportamento
  do novo layout). Se vier valor `%`, aplica a fórmula legada
  (`pvenda_total × pct / 100`).
- Terceiro `processFlowCcd` faz `DELETE + COPY` em `vendas_ccd` com o campo
  `evento` anexado como último argumento no `COPY IN`.
- Log de diagnóstico expandido: nova linha `[import:diag] colunas do novo
  layout — depto/secao/categoria/cliprinc/fantasia`.
- Log de roteamento inclui contagem CCD.

## Validações realizadas

| Etapa | Resultado |
|---|---|
| `go build ./...` | ✓ Compila limpo (exit 0) |
| Migration 181 aplicada no Postgres local | ✓ 9 colunas em cada tabela |
| Migration 182 aplicada no Postgres local | ✓ Tabela criada com 41 colunas + 3 índices |
| Idempotência (rodar de novo) | ✓ Emite apenas `NOTICE ... already exists, skipping` |
| Estrutura `vendas_ccd` | ✓ Espelha vendas_faturadas + `evento` |

## Compat garantida

- CSV do formato antigo: continua funcionando exatamente como antes
  (colunas novas ficam vazias/zero, todas as queries preservadas).
- Nova coluna `pvenda_unit` foi criada mas não é usada em nenhuma agg — só
  gravamos o valor informativo.

## O que NÃO foi feito (Fase 2)

- Hierarquias `V06` "Por Rede" e `V07` "Por Departamento" no GRID.
- `agg_v06_*_mes` e `agg_v07_*_mes` para as novas visões.
- Função `farol.upsert_aggs_mes` estendida.
- Botões novos no toggle da UI (`FarolExecutivo`, `FarolV2Dashboard`,
  `FarolPublicPanel`).
- Novos filtros dim (`cod_depto`, `cod_cliprinc`) no endpoint de dims.

## Próximo passo do gestor

Após deploy dessa Fase 1, o gestor pode reimportar um CSV no novo layout
(qualquer mês) e conferir:

1. As linhas TF vão pra `vendas_faturadas` / `vendas_transmitidas` com as
   colunas novas populadas.
2. As linhas CCD (arquivo `_CCD.csv`) vão pra `vendas_ccd` com o campo
   `evento` correto.
3. GRID atual continua funcionando exatamente como antes.

Depois disso pode-se planejar a Fase 2 (~1-2 dias) com as hierarquias novas.

## Arquivos alterados

- `backend/migrations/181_novo_layout_ion_vendas.sql` (novo)
- `backend/migrations/182_tabela_vendas_ccd.sql` (novo)
- `backend/handlers/farol_v2_import.go` (modificado)
