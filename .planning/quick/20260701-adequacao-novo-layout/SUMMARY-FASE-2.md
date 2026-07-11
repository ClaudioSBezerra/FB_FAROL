---
task: adequacao-novo-layout
phase: fase-2-hierarquias-v06-v07
status: complete
completed: 2026-07-01
---

# Adequação novo layout ION VENDAS — Fase 2 concluída

## O que foi feito

### Migration 183 — agg tables da V06 "Por Rede"

6 tabelas particionadas por ano criadas:
- `farol.agg_fat_v06_l0_mes` (rede)
- `farol.agg_fat_v06_l1_mes` (rede × fornecedor)
- `farol.agg_fat_v06_l2_mes` (rede × fornecedor × cliente/cnpj)
- Espelhos `agg_trans_v06_*` via `LIKE ... INCLUDING ALL`

Estrutura mínima: `pvenda`, `plucro`, `qt` (sem positivação, decisão da Fase 2).
Partições anuais criadas automaticamente para os anos vistos em `vendas_*`.

### Migration 184 — agg tables da V07 "Por Departamento"

6 tabelas espelhando o padrão da V06:
- L0 (depto), L1 (depto × seção), L2 (depto × seção × categoria)
- Fluxos fat + trans

### Migration 185 — funções `upsert_aggs_mes_v06` e `upsert_aggs_mes_v07`

Padrão auxiliar (mesmo do `upsert_aggs_mes_v05` da mig 167):
cada função cria suas próprias temp tables (`_v06_fat/_v06_trans` e
`_v07_fat/_v07_trans`) e faz `INSERT ... ON CONFLICT DO UPDATE` nas
6 agg tables da sua view.

**Nome da rede** (`nome_cliprinc`):
```sql
COALESCE(NULLIF(MAX(v.fantasia), ''), MAX(v.nome_cli))
```
Prefere `FANTASIA` do cliente; se vazio, cai pra razão social (`nome_cli`).

Vantagem dessa arquitetura: NÃO precisou reescrever a função monstro
`farol.upsert_aggs_mes` (~800 linhas). O backend Go chama as duas novas
funções em sequência após a principal.

### Backend `farol_v2_api.go`

- `hierarquias` map: V06 e V07 adicionadas com labels corretas
- `aggTablesFat` / `aggTablesTrans`: mapeamentos completos
- `allowedCols`: aceita `cod_cliprinc`, `nome_cliprinc`, `cod_depto`,
  `depto`, `cod_sec`, `secao`, `cod_categoria`, `categoria` — sem isso,
  o `safeColName` rejeitava e caía em `cod_fornec` como fallback
- `upsertAggsMesParallel` worker: após `upsert_aggs_mes` principal,
  chama `upsert_aggs_mes_v06` e `upsert_aggs_mes_v07`. Se
  `cod_cliprinc`/`cod_depto` não existem nos dados (CSV formato antigo),
  as temp tables ficam vazias e o custo é ~0

### Frontend `FarolExecutivo.tsx`

- Tipo `View` estendido: `'V01' | 'V02' | 'V03' | 'V06' | 'V07'`
- Toggle ganha 2 botões novos: **"Por Rede"** e **"Por Departamento"**
- `hidePosit` amplia condição: força esconder Positivação quando
  `view === 'V06' || view === 'V07'` (além dos casos existentes de folha
  Cliente/Produto)

## Validações realizadas

| Etapa | Resultado |
|---|---|
| `go build ./...` | ✓ Compila limpo |
| `npx tsc --noEmit` (frontend) | ✓ Type-check limpo |
| `npm run build` (frontend) | ✓ Build produção limpo, 344 KB gzip JS |
| Migrations 183/184/185 aplicadas local | ✓ 12 tabelas + 2 funções |
| Smoke test `upsert_aggs_mes_v06(...)` | ✓ Executa sem erro (temp tables vazias porque base local sem dados novos) |

## Comportamento pós-deploy

Enquanto o gestor **não reimportar** CSVs no novo layout:
- V06/V07 aparecem no toggle mas o GRID mostra vazio (as agg têm 0 rows)
- Todo o resto (V01-V05) continua funcionando exatamente igual

Após reimportar **um único mês** no novo layout:
- Colunas `cod_cliprinc`, `cod_depto`, etc. ficam populadas em `vendas_*`
- No fim do import, `upsertAggsMesParallel` chama as 3 funções seqüencialmente
- V06/V07 daquele mês ficam populadas e visíveis no GRID
- Filtros (dims) daquele mês ficam disponíveis

## O que fica pra futuro (não é bloqueador)

- Filtros dim (`cod_cliprinc`, `cod_depto`) no endpoint
  `/api/v2/farol/dims` — hoje os filtros mostram só V01-V05
- V06/V07 no painel público mobile (SUP/RCA) — decisão foi manter só no
  Executivo web
- Tabela `redes_cadastro` pra ter nome oficial de cada rede em vez de
  usar MAX(fantasia/nome_cli) como aproximação

## Arquivos alterados

**Novos:**
- `backend/migrations/183_agg_v06_por_rede.sql`
- `backend/migrations/184_agg_v07_por_departamento.sql`
- `backend/migrations/185_upsert_v06_v07.sql`
- `.planning/quick/20260701-adequacao-novo-layout/PLAN-FASE-2.md`
- `.planning/quick/20260701-adequacao-novo-layout/SUMMARY-FASE-2.md`

**Modificados:**
- `backend/handlers/farol_v2_api.go` (maps + worker)
- `frontend/src/pages/farol/FarolExecutivo.tsx` (toggle + hidePosit)
