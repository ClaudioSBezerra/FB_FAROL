---
title: 'Filtro cruzado "Indústria" nas visões principais + rename V01 para FORN.GERAL'
type: 'feature'
created: '2026-08-28'
status: 'done'
review_loop_iteration: 0
context: []
baseline_commit: '1db7f5a014661e1f3f934cf8410136037f30738f'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** Goal 2 do pedido original de cadastro de indústrias (ver `spec-cadastro-industria.md`, deferido em `deferred-work.md` entrada 2026-08-28): o cadastro (`farol.industrias`/`farol.industria_fornecedores`) existia, mas não filtrava nada — nenhuma visão do Farol sabia que 2+ `cod_fornec` eram o mesmo fabricante. O rótulo "Por Indústria" (V01) continuava significando `cod_fornec` cru, confundindo com o conceito canônico novo.

**Approach:** Trabalho executado sem supervisão direta (o Claudio foi dormir, pediu commit sem push) — por isso a decisão de escopo foi deliberadamente conservadora: filtro cruzado "Indústria" plugado em Por Gerência (V03)/Por Equipe (V02)/Por Rede (V06)/Por Departamento (V07), resolvido pro filtro `cod_fornec` já existente (`resolveIndustriaFilter`, farol_v2_api.go) — NENHUMA tabela nova, NENHUMA migration nova além do que já existia. V01 renomeado de "Por Indústria" para "Por FORN.GERAL" nos 2 lugares onde aparecia (FarolExecutivo.tsx, FarolV2Dashboard.tsx).

**Achado durante a implementação (não estava no escopo original, mas bloqueava a segurança do que foi pedido):** `pickAggForCrossFilter` não tinha proteção contra selecionar uma tabela agg pré-computada (ex.: `agg_fat_v01_l1_mes`, grão cod_fornec×cod_gerente) quando 2+ `cod_fornec` estão filtrados — a mesma classe de bug que Filial (migration 199) e UF (migration 197) já tiveram e corrigiram, só que cod_fornec nunca ganhou o guard equivalente. Como o filtro "Indústria" quase sempre resolve pra 2+ cod_fornec (é o motivo dele existir), esse bug LATENTE virava a norma, não a exceção. Corrigido com o mesmo padrão de `filialReady`: bloqueado.

## Boundaries & Constraints

**Always:**
- `resolveIndustriaFilter(db, empresaID, raw, filters)` traduz `?cod_industria=1,2` (IDs de `farol.industrias`) pros `cod_fornec` mapeados, e funde (união) em `filters["cod_fornec"]` — reaproveita 100% do mecanismo de cross-filter já existente pra cod_fornec, sem código novo em `queryAggregatedVendas`.
- Falha fechado: indústria(s) sem nenhum `cod_fornec` mapeado, ID inválido, ou erro de consulta → `filters["cod_fornec"]` recebe um sentinela que não bate com nenhum código real (nunca "sem filtro", que mostraria a empresa inteira).
- `pickAggForCrossFilter` ganha o guard `fornecMultiValor` (2+ `cod_fornec` filtrados + a tabela candidata tem `cod_fornec` no grão + não é o próprio `groupCol`) → descarta a tabela, força o scan ao vivo (`queryAggregatedVendas`) — mesmo padrão de `filialReady`. Esse guard também corrige o filtro cru de `cod_fornec` (rótulo "FORN.GERAL") com 2+ valores manuais, não só o novo filtro de Indústria.
- V01 renomeado "Por Indústria" → "Por FORN.GERAL" (FarolExecutivo.tsx, FarolV2Dashboard.tsx) e o chip de filtro cruzado `cod_fornec` (mesmos arquivos) renomeado "Indústria" → "FORN.GERAL", liberando "Indústria" pro chip novo.
- Filtro "Indústria" só aparece nas views V02/V03/V06/V07 (`view !== 'V01'` em FarolExecutivo.tsx) — em V01 seria redundante com a própria hierarquia.
- Opções do filtro vêm de `/api/farol/industrias` (a mesma API do cadastro CRUD), não do `/api/v2/farol/dims` — lista fixa por empresa, sem depender de período/fluxo.

**Never:**
- Não cria tabelas `agg_*` novas nem migrations novas — decisão explícita de manter o caminho ao vivo (`queryAggregatedVendas`) para o cross-filter, em vez de replicar o padrão V10/V11 de Filial (~30-40 tabelas, projeto de vários dias — ver pesquisa que embasou essa escolha).
- Não mexe na rota pública `/api/v2/farol/public/cards` (mobile ION VENDAS) — o filtro "Indústria" é só do painel web autenticado.
- Não corrige a mesma classe de bug (agg somando/mediando métrica pré-computada com 2+ valores filtrados) pra OUTRAS dimensões que possam ter o mesmo problema (ex.: `tipo_venda`, se um cliente compra sob mais de um tipo) — fora do escopo desta sessão, fica pra quando o Claudio revisar.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Indústria com 2+ cod_fornec, cliente comum, range de mês cheio | `?view=V03&cod_industria=N&ref_inicio=<1º dia>&ref_fim=<último dia>` | `positivados` deduplicado (1, não 2) — scan ao vivo, NUNCA a tabela agg | N/A |
| Indústria sem fornecedor mapeado | `?cod_industria=<id vazio>` | Zero resultados (sentinela), nunca a empresa inteira | N/A |
| ID de indústria inválido/não-numérico | `?cod_industria=abc` | Zero resultados (sentinela) | N/A |
| Filtro cru de cod_fornec com 2+ valores manuais (chip "FORN.GERAL") | `?cod_fornec=A,B` | Mesmo guard `fornecMultiValor` se aplica — scan ao vivo | N/A |

</frozen-after-approval>

## Code Map

- `backend/handlers/farol_v2_api.go` — `resolveIndustriaFilter` (novo), chamada em `FarolV2CardsHandler` logo após `parseMultiFilters`; guard `fornecMultiValor` em `pickAggForCrossFilter`
- `backend/handlers/farol_v2_api_industria_test.go` (novo) — `resolveIndustriaFilter` isolado + 2 testes ponta a ponta pelo handler HTTP real (dedup básico + mês cheio com agg corrompida de propósito)
- `frontend/src/pages/farol/FarolExecutivo.tsx` — rótulos renomeados, `useIndustrias()`, chip "Indústria" novo (V02/V03/V06/V07)
- `frontend/src/pages/farol/FarolV2Dashboard.tsx` — rótulo V01 renomeado

## Verification

**Commands:**
- `cd backend && go build ./...` — compila sem erro
- `cd backend && go test ./handlers/... -run "Industria|ResolveIndustriaFilter"` — inclui os 2 testes ponta a ponta
- `cd frontend && npx tsc --noEmit` — sem erro

**Pendências pro Claudio revisar de manhã (não commitado como "resolvido", só investigado até onde deu sem supervisão):**
- O guard `fornecMultiValor` conserta cod_fornec especificamente. Se `tipo_venda` (ou outra dimensão onde um cliente pode ter múltiplos valores) tiver uma tabela agg com essa coluna no grão, o mesmo bug pode existir lá — não auditado nesta sessão.
- Cross-filter "Indústria" nas outras views permanece SEM tabela pré-agregada (sempre scan ao vivo) — mais lento que os demais filtros em períodos longos. Se isso incomodar no uso real, a Fase 2 seria replicar o padrão V10/V11 (pesquisa já feita, ver histórico da conversa — não commitada em nenhum arquivo).
