---
title: 'Painel BI — endpoint único + dado fresco'
type: 'refactor'
created: '2026-07-22'
status: 'done'
context: []
baseline_commit: 'c72256d'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** Abrir o Painel BI dispara 4 requests paralelas (`/cards` ×3 + `/pulso`) que repetem período, positivação e a lista de períodos — ~27 queries, das quais ~9 são `COUNT(DISTINCT cnpj)` sem cache (`fixOverlappingBaseKPI`) e 3 são inúteis (`fetchPeriodosDisponiveis`, que o BI nem lê). E o React Query segura o payload por 1h sem sinal de frescor: numa TV ligada o painel exibe dado de uma hora atrás com o relógio na hora certa — o gestor não tem como perceber.

**Approach:** Endpoint dedicado `GET /api/v2/farol/bi` resolve período 1×, roda V03/V01/V02 + pulso em paralelo no servidor e devolve só o que a tela consome, com cache em memória por empresa×fluxo×modo invalidado no `RefreshViews`. O payload carrega `atualizado_em` (último import concluído) e o header passa a mostrar a hora do dado em vez de um countdown que não significa nada.

## Boundaries & Constraints

**Always:**
- Layout, cores, gauges, donut e ranking do BI ficam **visualmente idênticos**. A única mudança de tela é a linha sob o relógio: countdown → "dados de DD/MM HH:MM".
- Reusar `fetchCards`, `computeKPI`, `fixOverlappingBaseKPI`, `computePulso` como estão — sem reescrever a lógica de agregação.
- Semântica dos números idêntica à do `/cards` de hoje (mesmo `pr`, mesmo fluxo, mesmo `comp_mode`). O BI não pode divergir do painel Executivo.
- Terminologia de UI: "Objetivo", nunca "Meta".
- Invalidação de cache do BI entra junto de `invalidateBaseCache`/`invalidateVendasPeriodoCache` no `RefreshViewsHandler`.

**Ask First:**
- Qualquer alteração em `fetchCards`, nas tabelas `agg_*` ou em `upsert_aggs_mes` (área de bugs recorrentes).
- Trocar o critério de `atualizado_em` se `MAX(atualizado_em) WHERE status='done'` não refletir a realidade da operação.

**Never:**
- Não tocar em `/api/v2/farol/cards` — o painel Executivo depende dele.
- Não criar migration nem alterar schema.
- Não mexer em layout, rota, AppRail, modo TV/fullscreen.
- Nada de WebSocket/SSE para frescor — polling simples resolve.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Abertura padrão | `GET /api/v2/farol/bi?comp_mode=ytd`, cache frio | 200 com `kpi`, `industrias` (≤9, último = "Outros" se sobrar), `equipes` (≤12), `pulso`, `periodo`, `atualizado_em` | — |
| Segundo acesso | Mesma chave, dentro do TTL (10 min) | Mesma resposta, servida do cache, sem query no Postgres | — |
| Após import | `RefreshViews` roda | Próxima request recomputa (cache invalidado) e `atualizado_em` avança | — |
| Refresh manual | `?nocache=1` | Ignora o cache e recomputa; grava o resultado novo | — |
| Empresa sem dados | `pr.RefInicio` zerado | 200 com listas vazias, `kpi` zerado, `atualizado_em` vazio | Front mostra "Sem dados", não spinner infinito |
| Pulso sem transmissão | `computePulso` → `sem_dado` | Campo `pulso` presente com `sem_dado: true` | Front omite o card (comportamento atual) |
| Sem sessão | Sem `spCtx` | 401 `{"error":"unauthorized"}` | — |
| Falha de uma das 3 views | Query de V01 erra | Demais blocos vêm preenchidos; o que falhou vem vazio; erro logado | Painel degrada por bloco, não em tela cheia |

</frozen-after-approval>

## Code Map

- `backend/handlers/farol_v2_api.go` -- `FarolV2CardsHandler` (modelo a seguir, linha 509), `fetchCards` (1351), `computeKPI` (1639), `fixOverlappingBaseKPI` (1967), `resolvePeriods` (425), `hierarquias` (50), `RefreshViewsHandler` (2148, onde entram as invalidações)
- `backend/handlers/farol_pulso.go` -- `computePulso` (69), reusar direto; `FarolPulsoEmpresaHandler` continua existindo
- `backend/main.go` -- linha 488-492: registro de rotas com `gz(withSP(..., "gestor_filial"))`
- `frontend/src/pages/farol/FarolBI.tsx` -- `useBiData` (410), `PulsoCard` (96), `BiClock` (174), `FarolBI` (427); `IndustryDonut`/`RcaRanking` só perdem o cálculo de top-N/"Outros"

## Tasks & Acceptance

**Execution:**
- [x] `backend/handlers/farol_bi_api.go` (novo) -- criar `FarolV2BIHandler`: resolve `pr` 1×, dispara V03/V01/V02 (`fetchCards`+`computeKPI`+`fixOverlappingBaseKPI` só onde já se aplica hoje) e `computePulso` em goroutines com `sync.WaitGroup`, monta o payload enxuto (KPI de V03, top 8 indústrias + "Outros", top 12 equipes por faturado), lê `atualizado_em` de `vendas_import_jobs` -- elimina as 3 chamadas redundantes de `resolvePeriods`/`fetchPeriodosDisponiveis` e o fan-out de 4 requests
- [x] `backend/handlers/farol_bi_api.go` -- cache em memória `biCache` (chave `empresa|fluxo|comp_mode`, TTL 10 min, `sync.RWMutex`) + `invalidateBICache(empresaID)` + bypass por `?nocache=1` -- painel na TV recarrega sem custo de banco
- [x] `backend/handlers/farol_v2_api.go` -- chamar `invalidateBICache(spCtx.EmpresaID)` junto das invalidações existentes no `RefreshViewsHandler` -- dado novo aparece no BI logo após o import
- [x] `backend/main.go` -- registrar `GET /api/v2/farol/bi` com `gz(withSP(handlers.FarolV2BIHandler, "gestor_filial"))` -- mesma proteção do `/cards`
- [x] `frontend/src/pages/farol/FarolBI.tsx` -- trocar os 3 `useBiData` + a query do `PulsoCard` por um único `useQuery` do novo endpoint (`refetchInterval` 5 min); `PulsoCard` passa a receber `data` por prop; `handleRefresh` chama o endpoint com `nocache=1` -- 4 requests → 1
- [x] `frontend/src/pages/farol/FarolBI.tsx` -- `BiClock` exibe "dados de DD/MM HH:MM" no lugar do countdown, em âmbar se `atualizado_em` tiver mais de 24h -- o gestor enxerga na hora se o painel está defasado

**Acceptance Criteria:**
- Given o painel BI aberto, when a aba Network é inspecionada, then há **uma** request de dados (`/api/v2/farol/bi`) — nenhuma a `/cards` ou `/pulso`.
- Given o painel já carregado, when o gestor alterna "Acumulado Ano" ↔ "Mês Atual" e volta, then a volta é servida do cache do React Query, sem nova request.
- Given dois acessos ao BI dentro de 10 min, when os logs do backend são lidos, then o segundo não emite `[farol:agg] fetchCards`.
- Given um `RefreshViews` executado, when o BI recarrega, then os números refletem o import e `atualizado_em` mostra o horário novo.
- Given os mesmos período e fluxo, when BI e painel Executivo são comparados, then objetivo %, positivação, mix e faturado por indústria batem exatamente.

## Spec Change Log

### 2026-07-22 — revisão adversarial (Blind Hunter + Edge Case Hunter + Acceptance Auditor)

Nenhum achado tocou o bloco congelado. Todos viraram correção de código (commit `c293f9b`):

1. Panic em goroutina derrubava o **processo inteiro** → `recover()` por bloco.
2. Resposta degradada (falha de query → 200 zerado) era cacheada 10 min → guarda `degradado` recusa cachear payload vazio.
3. Cálculo iniciado antes de um import gravava resultado pré-import no cache → contador de geração (`biGen`).
4. `?fluxo=cancdev` caía em scan de `vendas_ccd` → fluxo validado.
5. `cardItem.Faturado` é zero no fluxo transmitido → passou a usar `ValorAtual`.
6. `sort.Slice` instável fazia o ranking mudar sozinho → `SliceStable`.
7. `invalidateBICache` estava antes de `status='done'` e dentro do `if` → caminho `skip_refresh` nunca invalidava; movido para depois e para fora.
8. `VendasClear` invalidava só o cache do BI → derruba os três juntos.
9. Front: "Atualizar" podia ser deduplicado numa request já em voo → busca com `nocache=1` de forma determinística.
10. Barra do ranking com `maxFat` 0/negativo → piso e clamp.
11. Falha de rede apagava o painel inteiro → mantém último dado válido com aviso.

**Estado conhecido-ruim evitado:** um blip no Postgres congelaria painel zerado na TV por 10 min **depois** do banco voltar — pior que o problema original.

**KEEP (deve sobreviver a qualquer re-derivação):**
- A paridade `biKPI` × `FarolV2CardsHandler` foi conferida parâmetro a parâmetro e está correta. Não reescrever `biKPI` sem refazer essa conferência.
- `biFetchL0`/`biKPI` apenas **chamam** `fetchCards`/`computeKPI` — nunca reimplementar agregação.
- O guard de `fixOverlappingBaseKPI` tem de continuar idêntico ao do `/cards`.

### 2026-07-22 — renegociação do bloco congelado (autorizada pelo gestor)

O item "Ask First" sobre o critério de `atualizado_em` foi resolvido: o carimbo
passa a vir do fim da **consolidação**, não do fim do upload.

Isso exigiu quebrar um "Never" do bloco congelado — *"não criar migration nem
alterar schema"* — porque não existe nenhum lugar durável para o timestamp: as
`agg_*` não têm coluna de tempo e as tabelas de config existentes são de outro
domínio. **O gestor autorizou explicitamente a migration 193.** O texto do
"Never" fica como está para preservar o registro do que foi acordado
originalmente; esta entrada é a autorização da exceção.

- `backend/migrations/193_consolidacao_log.sql` — tabela aditiva, 1 linha por
  empresa, sem reconsolidação.
- Gravada em `processImportJob` (após `upsertAggsMesParallel`) e em
  `RefreshViewsHandler` (onde a carga multi-arquivo de fato consolida).
- `biUltimoImport` lê dela; só cai no critério antigo enquanto a empresa não
  passar por uma consolidação após a 193.

## Design Notes

`atualizado_em` = `SELECT MAX(atualizado_em) FROM vendas_import_jobs WHERE empresa_id=$1 AND status='done'` — é o carimbo do último import que efetivamente entrou. Devolver em RFC3339 e formatar no front (evita divergência de fuso no servidor).

Forma do payload (só o que a tela lê):

```json
{
  "kpi": { "...": "kpiSummary de V03, sem alteração" },
  "industrias": [{ "label": "IND X", "faturado": 123.4 }],
  "equipes":    [{ "label": "SUP Y", "faturado": 99.9, "pct": 87.3 }],
  "pulso":      { "...": "pulsoResp de computePulso" },
  "periodo":    { "cur_label": "...", "ant_label": "..." },
  "atualizado_em": "2026-07-22T03:14:00Z"
}
```

O "Outros" do donut passa a vir pronto do backend (soma da cauda a partir do 9º), então `IndustryDonut` só pinta e formata. `RcaRanking` continua calculando `maxFat` e a cor localmente.

Se uma das goroutines falhar, o bloco correspondente sai vazio e o painel renderiza o resto — hoje um erro em qualquer das 3 requests apaga a tela inteira (`isError` global).

## Verification

**Commands:**
- `cd backend && go build ./...` -- expected: compila sem erro
- `cd frontend && npx tsc --noEmit` -- expected: sem erro de tipo
- `cd backend && set -a && source .env && set +a && go test ./handlers -run TestBI -v -vet=off` -- expected: paridade BI × /cards em ytd e mtd. Precisa de banco com migrations em dia (a coluna `mix_total` da 175); sem ela, `/cards` e `/bi` falham igual e o teste aborta em vez de passar vazio. O `-vet=off` é obrigatório por causa de 2 erros de vet pré-existentes em `objetivos.go`.

**Manual checks:** com o painel aberto, a aba Network deve mostrar uma única request de dados; o header deve trazer "dados de DD/MM/AA HH:MM" coerente com o último import.

## Suggested Review Order

**Endpoint consolidado (entrada)**

- Handler: período resolvido 1×, 4 goroutines, payload enxuto.
  [`farol_bi_api.go:99`](../../backend/handlers/farol_bi_api.go#L99)

- KPI replica exatamente o que o /cards faz para V03 — é aqui que BI e Executivo podem divergir.
  [`farol_bi_api.go:204`](../../backend/handlers/farol_bi_api.go#L204)

- Rota com a mesma proteção do /cards.
  [`main.go:492`](../../backend/main.go#L492)

**Cache e frescor do dado**

- Cache por empresa|fluxo|modo, TTL 10 min como rede de segurança.
  [`farol_bi_api.go:116`](../../backend/handlers/farol_bi_api.go#L116)

- Invalidação no caminho REAL do dado novo: fim do import (achado da revisão).
  [`farol_v2_import.go:1023`](../../backend/handlers/farol_v2_import.go#L1023)

- Invalidação no RefreshViews manual.
  [`farol_v2_api.go:2243`](../../backend/handlers/farol_v2_api.go#L2243)

- Limpeza de vendas não pode deixar o BI exibindo o que foi apagado.
  [`farol_v2_import.go:1371`](../../backend/handlers/farol_v2_import.go#L1371)

**Consumo na tela**

- 4 queries viram 1; "Atualizar" fura o cache do servidor via nocache=1.
  [`FarolBI.tsx:416`](../../frontend/src/pages/farol/FarolBI.tsx#L416)

- Countdown → hora do dado, em âmbar quando passa de 24h.
  [`FarolBI.tsx:169`](../../frontend/src/pages/farol/FarolBI.tsx#L169)

- Pulso deixa de ter query própria e passa a receber prop.
  [`FarolBI.tsx:99`](../../frontend/src/pages/farol/FarolBI.tsx#L99)
