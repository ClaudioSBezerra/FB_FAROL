---
title: 'Painel BI CEO — KPIs de Indústria e UF'
type: 'feature'
created: '2026-07-22'
status: 'done'
context: []
baseline_commit: '44bbb01'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** O painel BI é o War Room do CEO, mas os dois blocos de baixo hoje são o donut de Indústria (só mostra fatia, ignora atingimento) e o ranking de Equipes (nível supervisor — granular demais para o CEO). Falta a leitura geográfica (UF) e o sinal de quais indústrias estão batendo o objetivo — exatamente o que um CEO olha primeiro.

**Approach:** Reorganizar a faixa inferior do painel para a visão do CEO: (1) **Faturado + atingimento por UF** — novo bloco, faturado por estado com % vs período anterior e cor verde/vermelho, lido de `vendas_faturadas` (UF do cliente) e cacheado junto com o resto do payload; (2) **Indústria com atingimento** — o donut/lista passa a marcar verde/vermelho por fornecedor, reusando `pct`/`cor` que a V01 já calcula; (3) **Concentração (Pareto)** — número "top 5 indústrias = X% do faturado". O ranking de Equipes sai. Os 3 gauges do topo ficam intocados.

## Boundaries & Constraints

**Always:**
- "Atingimento" = período atual vs período de comparação (YoY), a MESMA semântica dos gauges e do `/cards`. Verde se atual ≥ anterior, vermelho caso contrário.
- UF = do cliente (coluna `uf` de `vendas_faturadas`), lida no mesmo `pr` (RefInicio/RefFim e CompInicio/CompFim) já resolvido pelo endpoint.
- O bloco UF entra no cache `biCache` existente; sem custo extra de banco no segundo acesso, invalidado no import como o resto.
- A query de UF roda como goroutine adicional em paralelo com as 4 existentes — não pode aumentar o tempo de parede em série.
- Terminologia de UI: "Objetivo", nunca "Meta".
- 3 gauges do topo (Objetivo, Positivação, Mix) e o card Pulso ficam idênticos.

**Ask First:**
- Se o scan de UF do período de comparação (ano anterior inteiro no modo YTD) passar de ~3s por chamada mesmo em paralelo — avaliar reduzir o comp a mesmos-meses antes de materializar uma agg.
- Qualquer uso da tabela `objetivos_importados` (fora de escopo: não tem UF, é RCA×Produto×Fornecedor, spec incompleta).

**Never:**
- Não criar tabela `agg` de UF nem migration (o scan cacheado resolve; materializar UF é decisão futura).
- Não tocar em `fetchCards`, `upsert_aggs_mes`, nem no `/api/v2/farol/cards`.
- Não mexer nos 3 gauges, no Pulso, no header, na rota ou no AppRail.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Payload padrão | `GET /api/v2/farol/bi?comp_mode=ytd` | Resposta ganha `ufs[]` (estado, faturado, faturado_ant, pct, cor) e `industrias[]` ganha `pct`+`cor`; `concentracao_top5` (0-100) | — |
| UF sem comparativo | Empresa só com 1 ano de dado | `ufs[]` com `faturado_ant=0`, `pct=0`, `cor="verde"` (sem alerta, igual ao `pickCor` do /cards) | — |
| Segundo acesso | Dentro do TTL | Bloco UF vem do `biCache`, sem novo scan | — |
| Empresa sem dados | `pr.RefInicio` zerado | `ufs: []`, `industrias: []`, `concentracao_top5: 0` | Front mostra "Sem dados" |
| UF nula/vazia numa linha | `uf=''` em `vendas_faturadas` | Agrupada sob rótulo "—"; não quebra ordenação | — |
| Todas as indústrias com share 0 | faturado total 0 | `concentracao_top5=0`; donut mostra "Sem dados" | Sem divisão por zero |
| Falha do scan de UF | Query de UF erra | `ufs: []`, demais blocos preenchidos, erro logado; resposta NÃO cacheada (guarda `degradado` já existente estende-se a esse caso) | Painel degrada por bloco |

</frozen-after-approval>

## Code Map

- `backend/handlers/farol_bi_api.go` -- `biResponse`/`biIndustria` (linha 35), `biTopIndustriasComOutros` (292), `FarolV2BIHandler` (goroutines ~156), guarda `degradado` (~215); aqui entram o tipo `biUF`, a query `biFaturadoPorUF`, os campos novos e a 5ª goroutine
- `backend/handlers/farol_v2_api.go` -- `buildRangeCond` (651, reusar p/ o WHERE do scan), `cardItem.Pct`/`.Cor` (já preenchidos na V01), `pr`/`periodResolution` (comp já resolvido)
- `frontend/src/pages/farol/FarolBI.tsx` -- `IndustryDonut` (~300, ganha cor de atingimento + Pareto), `RcaRanking` (~350, **removido** do render), grid inferior (~590), interfaces `BiIndustria`/`BiResponse` (~10-40)

## Tasks & Acceptance

**Execution:**
- [x] `backend/handlers/farol_bi_api.go` -- add `biUF{Estado, Faturado, FaturadoAnt, Pct, Cor}` e `biFaturadoPorUF(db, empresaID, fluxo, pr)`: dois `SUM(pvenda) GROUP BY uf` (atual + comp) sobre `vendas_faturadas` com `buildRangeCond`, junta por UF, calcula pct/cor com a mesma regra do `pickCor` (verde se ant≤0 OU atual≥ant); UF vazia vira "—"; ordena por faturado desc -- traz a visão geográfica sem agg nova
- [x] `backend/handlers/farol_bi_api.go` -- `biIndustria` ganha `Pct float64` e `Cor string`; `biTopIndustriasComOutros` copia `c.Pct`/`c.Cor` (a fatia "Outros" fica `cor=""`); `biResponse` ganha `UFs []biUF`, `ConcentracaoTop5 float64`; nova 5ª goroutine (com o mesmo `recover` das outras) roda `biFaturadoPorUF`; `degradado` passa a exigir também `len(UFs)==0` -- expõe atingimento por indústria e Pareto
- [x] `backend/handlers/farol_bi_api.go` -- calcular `ConcentracaoTop5` = soma dos 5 maiores faturados de indústria / total, na montagem do payload -- número de risco de concentração
- [x] `frontend/src/pages/farol/FarolBI.tsx` -- `BiIndustria` ganha `pct`/`cor`; `BiResponse` ganha `ufs`, `concentracao_top5`; `IndustryDonut` marca verde/vermelho por item (ponto/legenda) e exibe "Top 5 = X%" no cabeçalho; remover `RcaRanking` do render -- bloco de indústria vira leitura de CEO
- [x] `frontend/src/pages/farol/FarolBI.tsx` -- novo componente `UFRanking` no lugar do bloco de Equipes: barras horizontais por estado, largura proporcional ao faturado (clamp 0-100, piso maxFat=1), cor por atingimento, % e valor -- o bloco geográfico

**Acceptance Criteria:**
- Given o painel aberto, when a faixa inferior é lida, then há Indústria (com marca de atingimento) e UF — não há mais ranking de Equipes.
- Given os mesmos período/fluxo, when comparo o faturado por indústria do BI com a V01 do `/cards`, then os valores e a cor batem.
- Given o segundo acesso dentro de 10 min, when olho os logs, then não há novo scan de `vendas_faturadas` para UF (veio do cache).
- Given uma UF sem dado no período anterior, when o card renderiza, then aparece verde sem alerta, sem `NaN` na barra.
- Given o Pareto, when top 5 indústrias somam 82% do faturado, then o card mostra "Top 5 = 82%".

## Spec Change Log

### 2026-07-22 — UF em líquido via MV (renegociação do "Never: migration")

Durante a implementação, o self-review pegou uma divergência: o painel exibe
**líquido** (gauges/donut usam `agg.liquido`), mas o scan de UF somava `pvenda`
**bruto** — o total por UF não fecharia com o headline. Como o líquido depende
de `tipo_venda` + `vendas_ccd` (devol/cancel), não dá para reproduzi-lo num scan
simples sem re-derivar a fórmula da mig 190.

**O gestor autorizou explicitamente criar uma MV** (quebra do "Never: não criar
migration nem agg de UF" do bloco congelado). Solução:
- `migrations/194_mv_fat_uf_mes.sql` — MV `farol.mv_fat_uf_mes` (empresa, uf,
  ano, mês, líquido, bruto) com a **mesma fórmula da mig 190**; índice único
  para `REFRESH CONCURRENTLY`. Aditiva, não toca o upsert existente.
- `refreshUFMV` chamado junto do upsert nas duas consolidações
  (`processImportJob` e `RefreshViewsHandler`).
- `biFaturadoPorUF`: faturado lê a MV (líquido, grão mensal); transmitido segue
  scan de `vendas_transmitidas` (bruto, semântica do transmitido).

**Validação (banco local recriado com dado sintético):** MV = GO 1350 / SP 750 /
— 100; total 2200 = total de líquido das aggs = faturado do KPI. Fecha com o
painel. `mig 194` aplica limpa em Postgres real.

Também corrigido no caminho: `buildRangeCond` referencia alias `v` (a query da
UF precisou aliasar a tabela); e o gatilho de não-cachear passou a incluir
`!ufOK` (falha do scan de UF, o único bloco cujo erro é distinguível de
"sem dado").

## Design Notes

`biFaturadoPorUF` espelha o padrão de `queryAggregatedVendas` (scan de `vendas_*` com `buildRangeCond`), mas agrupa por `uf` — dimensão sem agg. Duas queries (atual/comp) em paralelo dentro da própria goroutine; cada uma ~1,6s/mês medido, absorvido pelo `biCache` (TTL 10 min) e sobreposto às outras 4 goroutines.

Atingimento reusa a regra exata do `/cards` (`farol_v2_api.go` `pickCor`): sem comparativo → neutro/verde; com comparativo → verde se atual ≥ ant. Não introduzir outra régua de cor.

`ConcentracaoTop5` sai dos cards da V01 (todos, antes do corte top-8), então o denominador é o faturado total de indústria, não o dos 8 exibidos.

## Verification

**Commands:**
- `cd backend && go build ./...` -- expected: compila
- `cd frontend && npx tsc --noEmit` -- expected: sem erro de tipo
- `cd backend && set -a && source .env && set +a && go test ./handlers -run TestBI -vet=off` -- expected: paridade indústria BI × /cards segue válida; exige banco com migrations em dia

**Manual checks:** no painel, faixa inferior = Indústria (com cor de atingimento + "Top 5 = X%") e UF (barras por estado); recarregar em <10 min não gera scan de `vendas_faturadas` no log.
