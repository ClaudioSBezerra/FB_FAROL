---
title: 'Tipo de Venda — nova coluna de import + filtro no fluxo faturado'
type: 'feature'
created: '2026-07-16'
status: 'in-progress'
context: []
baseline_commit: '84fefc9956527ac67ad52db79c883520fba6c3b7'
branch: 'feat/tipo-venda-filtro-faturado'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** O CSV do ION VENDAS passou a trazer uma nova última coluna `TIPO_VENDA` (11 códigos: 1=Padrão, 4=Simples Fatura, 5=Bonificação, 7=Entrega Futura, 8=Simples Entrega, 9=CFOP específico, 10=Transferência, 11=Venda com Troca, 13=Remessa Manifesto, 14=Venda Manifesto, 20=Consignada). Hoje o import ignora essa coluna e o gestor não consegue separar faturamento efetivo de bonificação/transferência/remessa.

**Approach:** Ler a coluna no import e gravá-la em `vendas_faturadas`; adicionar `tipo_venda` como dimensão de GROUP BY nas 26 MVs do fluxo faturado (`agg_fat_*`) e na tabela de dims; expor "Tipo de Venda" como mais um filtro multi-select nas abas de vendas faturadas, recalculando cards/KPIs. Fluxo transmitido fica intacto.

## Boundaries & Constraints

**Always:**
- A coluna é a ÚLTIMA do CSV; detectar por header `TIPO_VENDA` (com fallback posicional para última coluna) — nunca por índice fixo hardcoded.
- Valor default `''` (vazio) quando ausente — mantém compat com CSVs antigos sem a coluna.
- Só o fluxo **faturado**. `vendas_transmitidas` e as 26 MVs `agg_trans_*` NÃO mudam.
- `tipo_venda` entra na PRIMARY KEY de cada tabela `agg_fat_*_mes` (senão o upsert colapsa linhas de tipos diferentes).
- Filtro é aditivo (AND) aos filtros existentes, igual `cod_fornec`/`uf`.

**Ask First:**
- Se o `upsert_aggs_mes` passar de ~12 min no arquivo cheio (2.5M linhas) por causa da nova cardinalidade — HALT e reavaliar (talvez limitar tipo_venda só aos níveis l0-l2).
- Se algum CSV real trouxer código de tipo_venda fora da lista dos 11 conhecidos — HALT e confirmar rótulo antes de assumir.

**Never:**
- Não filtrar por tipo_venda em telas de detalhe que já leem a base direto sem passar pelo filtro padrão (fora de escopo).
- Não criar tabela nova de aggregates paralela — reusar as `agg_fat_*` existentes.
- Não expor o filtro "Tipo de Venda" no fluxo transmitido na UI (mesmo a coluna existindo lá vazia).

RENEGOCIADO (2026-07-16): a coluna `tipo_venda` será adicionada nas 52 MVs (fat E trans) para preservar a herança `LIKE agg_fat` das agg_trans. As agg_trans terão `tipo_venda=''` sempre (upsert não popula). Só o faturado tem valor real e filtro na UI.

RENEGOCIADO (2026-07-21) — **SUPERSEDE o de 2026-07-16**: `tipo_venda` NÃO entra na PK de nenhuma agg. A leitura do código de agregação revelou que a API lê as agg com `SUM(pvenda)` mas `AVG(base_cli/positivados/mix)`; colocar `tipo_venda` no grão faria a visão SEM filtro exibir positivação/mix ERRADOS (média por tipo). Solução: `tipo_venda` vira **filtro cruzado** — a API já roteia filtros cuja coluna não existe nas agg (hoje uf/empresa) para `queryAggregatedVendas`, que agrega direto de `vendas_faturadas` e calcula todos os indicadores corretamente. As 52 agg ficam INTACTAS; sem reescrever o upsert de 920 linhas; sem risco de upsert >12min. KEEP: filtro exclusivo do faturado; `tipo_venda` só gravado em `vendas_faturadas`.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| CSV com coluna | header tem `TIPO_VENDA`, valor `5` | grava `tipo_venda='5'` em vendas_faturadas | N/A |
| CSV sem coluna | header antigo, sem TIPO_VENDA | `tipo_venda=''`, import segue normal | N/A |
| Valor vazio na linha | célula tipo_venda em branco | grava `''` | N/A |
| Filtro aplicado | `?tipo_venda=1,20` no fluxo faturado | cards recalculados só com tipos 1 e 20 | N/A |
| Filtro no transmitido | `?tipo_venda=1` no fluxo transmitido | ignorado (coluna não existe lá) | silencioso |
| Código desconhecido | tipo_venda=`99` | grava `'99'`, aparece no filtro como código cru | log warn |

</frozen-after-approval>

## Code Map (implementado 2026-07-21 — abordagem cross-filter)

- `backend/handlers/farol_v2_import.go` -- detecta `TIPO_VENDA` (`col(-1,"tipovenda","tipo_venda","tipodevenda")` + fallback posicional guardado p/ última coluna); campo `tipoVenda` em `vendaRaw` (fora de `vals`, igual `evento`); `processFlow` ganhou `extraCol`/`extraVal` — faturado passa `("tipo_venda", r.tipoVenda)`, transmitido passa `("", nil)`
- `backend/migrations/187_tipo_venda_faturado.sql` -- `ALTER vendas_faturadas ADD COLUMN tipo_venda` + índice parcial `idx_vf_tipo_venda (empresa_id, data_faturamento, tipo_venda) WHERE tipo_venda<>''`. **NÃO** mexe nas agg
- `backend/migrations/188_tipo_venda_dims.sql` -- `farol.tipo_venda_label(cod)` (rótulos dos 11 códigos, cru se desconhecido) + `farol.upsert_tipo_venda_dims(emp,ano,mes)` (popula `dim='tipo_venda'` em `agg_fat_dims_mes`) + backfill dos meses existentes. **NÃO** reescreve `upsert_aggs_mes`
- `backend/handlers/farol_v2_api.go` -- `allowedCols["tipo_venda"]=true`; `parseMultiFilters` add `tipo_venda`; guarda `if fluxo!=faturado { delete(filters,"tipo_venda") }` nos 2 handlers de cards; `upsertAggsMesParallel` chama `upsert_tipo_venda_dims` após cada mês; `FarolV2DimsHandler` expõe `tipo_venda` só no faturado. Roteamento p/ `queryAggregatedVendas` é automático (aggServesFilters→false; pickAggForCrossFilter→sem match)
- `frontend/src/pages/farol/FarolExecutivo.tsx` -- `tipo_venda?` em `DimsResponse`; entry condicional a `fluxo==='faturado'` em `FILTER_DIMS`; limpa filtro tipo_venda ao trocar p/ transmitido

## Tasks & Acceptance

**Execution:** (implementado como cross-filter — ver Change Log 2026-07-21)
- [x] `backend/handlers/farol_v2_import.go` -- lê TIPO_VENDA (header/última), grava só no faturado via `processFlow` extraCol
- [x] `backend/migrations/187_tipo_venda_faturado.sql` -- ADD COLUMN em vendas_faturadas + índice parcial (agg INTACTAS)
- [x] `backend/migrations/188_tipo_venda_dims.sql` -- funções de rótulo + populate de `dim='tipo_venda'` + backfill (upsert principal NÃO tocado)
- [x] `backend/handlers/farol_v2_api.go` -- whitelist + parse + guarda transmitido + dims endpoint + call de dims no runner
- [x] `frontend/src/pages/farol/FarolExecutivo.tsx` -- filtro "Tipo de Venda" faturado-only + limpa ao trocar fluxo
- [x] `go build ./...` e `npx tsc --noEmit` -- OK
- [ ] **PENDENTE (ambiente do usuário)** aplicar migs 187/188 + import de teste com/sem coluna; validar filtro no faturado e ausência no transmitido

**Acceptance Criteria:**
- Given um CSV com coluna TIPO_VENDA, when importado, then `SELECT DISTINCT tipo_venda FROM vendas_faturadas` retorna os códigos do arquivo.
- Given um CSV antigo sem a coluna, when importado, then import conclui com `tipo_venda=''` e cards não quebram.
- Given fluxo faturado com filtro `tipo_venda=1`, when carrego cards, then o valor total é menor ou igual ao total sem filtro e exclui bonificação/transferência.
- Given fluxo transmitido, when abro filtros, then "Tipo de Venda" não aparece.
- Given `upsert_aggs_mes` rodando no arquivo cheio, when concluído, then tempo ≤ ~12 min (senão HALT conforme Ask First).

## Spec Change Log

- **2026-07-21 — cross-filter substitui grão-na-PK (correção de correção):** ao ler o código de agregação (`queryAggregatedMes`) descobriu-se que a API soma `pvenda`/`plucro` mas faz `AVG` de `base_cli`/`positivados`/`mix`. A abordagem congelada (tipo_venda na PK das 26 agg) faria a visão SEM filtro exibir positivação e mix incorretos (média por tipo_venda). Humano aprovou trocar para **filtro cruzado**: tipo_venda existe só em `vendas_faturadas`; nenhuma agg tem a coluna → `aggServesFilters` retorna false → `fetchCards` roteia para `queryAggregatedVendas` (scan da base, todos os indicadores corretos), exatamente como uf/empresa hoje. Impacto: descarta a recriação das 52 agg e a reescrita do upsert (188 virou só populate do dropdown); tradeoff = query com filtro de tipo escaneia a base (escopada por período+drill) em vez de acertar a agg. SUPERSEDE o RENEGOCIADO 2026-07-16.
- **2026-07-16 — herança LIKE das agg_trans:** investigação revelou que `agg_trans_*` são criadas com `LIKE farol.agg_fat_* INCLUDING ALL`. Adicionar tipo_venda só nas agg_fat quebraria a herança. Decisão do humano: adicionar a coluna nas 52 MVs (simetria preservada); agg_trans ficam com `tipo_venda=''`; upsert só popula no faturado; filtro na UI só no faturado. KEEP: filtro exclusivo do faturado.

## Design Notes (revisado 2026-07-21)

**Abordagem ABANDONADA (grão na PK):** ~~adicionar tipo_venda à PK das 26 agg~~. Descartada porque quebrava positivação/mix na visão sem filtro (API faz AVG desses indicadores — média por tipo_venda ≠ total do mês). Ver Change Log 2026-07-21.

**Abordagem ADOTADA (filtro cruzado):** tipo_venda é só uma coluna de `vendas_faturadas`. O motor de filtros da API classifica qualquer coluna ausente das agg como "cruzada" e cai em `queryAggregatedVendas`, que recomputa valor/plucro/positivados/mix/base_cli direto da base — corretos por construção. Fluxo:

- `parseMultiFilters` capta `tipo_venda`; handler remove-o se `fluxo != faturado`.
- `aggServesFilters(view, drillIdx, {tipo_venda})` → false (nenhuma `colsInAggTable` tem tipo_venda).
- `pickAggForCrossFilter` → sem match (nenhuma agg tem a coluna) → `queryAggregatedVendas`.
- `buildMultiFilterCond` emite `AND v.tipo_venda = ANY($n::text[])` contra `vendas_faturadas v`.

Dropdown: `farol.upsert_tipo_venda_dims` insere `dim='tipo_venda'` em `agg_fat_dims_mes` (rótulos via `tipo_venda_label`); o `FarolV2DimsHandler` devolve essas opções só no faturado. Comportamento do totalizador de positivação sob filtro de tipo_venda = idêntico ao já aceito para uf/empresa (`leafServesPositivados` retorna false → sem recount no leaf).

## Verification

**Commands:**
- `cd backend && go build ./...` -- expected: sem erros
- `cd frontend && npx tsc --noEmit` -- expected: sem erros de tipo
- import de teste via localhost + `SELECT tipo_venda, count(*) FROM vendas_faturadas GROUP BY 1` -- expected: distribuição dos 11 tipos

**Manual checks:**
- Na aba faturado, aplicar filtro Tipo de Venda=1 e confirmar que o total cai vs sem filtro; conferir que transmitido não mostra o filtro.
