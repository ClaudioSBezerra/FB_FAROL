# Plano de Performance — FB_FAROL (2026-06-12, atualizado 2026-06-15)

> Elaborado após a sessão de correções do incidente DNS + reimport + migrations 169-173.

---

## ⭐ PLANO DE IMPLEMENTAÇÃO P1+P2 (2026-06-15) — fonte de verdade atual

> As seções P1/P2 mais abaixo ficaram **parcialmente desatualizadas**: a migration
> 173 JÁ removeu as temp tables `_v_fat_12m`. Mesmo assim a consolidação continua
> ~8 min/mês — provando que o gargalo NUNCA foi as temp tables, e sim as ~38
> agregações com `COUNT(DISTINCT)`. Esta seção é o diagnóstico correto.

### São DOIS problemas distintos (não confundir)

| Sintoma | Onde | Natureza | Item |
|---|---|---|---|
| Painel mix "X de Y" demora 6-13s no 1º acesso (YTD) | LEITURA (`queryMixTotal` em vendas_*) | scan de 9M linhas no request | **P1** |
| Consolidação após import leva ~40 min (17 meses) | ESCRITA (`upsert_aggs_mes`) | 38 INSERTs × COUNT(DISTINCT) | **P2** |

⚠️ Atenção: **P1 deixa a ESCRITA um pouco MAIS lenta** (adiciona 1 COUNT DISTINCT
por INSERT) para deixar a LEITURA instantânea. Se a dor principal é a consolidação,
P2 é o que importa. Os dois são independentes.

### P1 — Materializar `mix_total` (painel: 13s → ~150ms)
1. **Migration 174**: `ALTER TABLE farol.agg_(fat|trans)_v0X_lY_mes ADD COLUMN mix_total INT DEFAULT 0` (todas as ~30 tabelas que hoje têm a coluna `mix`).
2. **upsert_aggs_mes**: cada INSERT calcula `COUNT(DISTINCT v.cod_prod) FILTER (WHERE v.qt>0 AND v.cod_prod<>'')` como `mix_total` (a temp `_v_fat`/`_v_trans` já tem cod_prod → custo marginal pequeno).
3. **queryAggregatedMes**: adiciona `<AGG>(v.mix_total)` ao SELECT e ao scan; popula `card.MixTotal`.
4. **fetchCards**: remove as chamadas a `queryMixTotal` (atual+ant) → some o scan de 9M linhas. Remove `queryMixTotal`, cache e singleflight (código morto).
5. **Repopular** uma vez (custo da consolidação, ~40 min; ou backfill por UPDATE).
- **Decisão necessária:** agregação multi-mês de `mix_total`. Mês único (YoY, M-1) é exato. Para YTD (vários meses):
  - **MAX** = "maior portfólio mensal" (recomendado: estável, intuitivo, subestima leve)
  - **AVG** = "portfólio médio mensal"
  - (SUM seria errado — conta o mesmo SKU N vezes)
- **Risco:** baixo. É a 5ª mexida no upsert → regra do `grep "INSERT INTO farol\."` antes do push.

### P2 — Consolidação mais rápida (o que você realmente sente: 40 min)
Causa real (pós-173): 38 INSERTs/mês, cada um GROUP BY + `COUNT(DISTINCT cnpj)` e
`COUNT(DISTINCT (cnpj,cod_prod))` sobre ~1,2M linhas. 17 meses × 38 = 646 agregações.

**P2.1 — Consolidação INCREMENTAL (maior ganho no dia a dia, baixo risco):**
A `RefreshViews` hoje re-consolida TODOS os meses. O import já sabe quais meses
foram tocados (`mesesTocados`). Mudar para consolidar **só os meses do import**:
- Import diário (1-2 meses) → ~2-4 min em vez de 40 min.
- Carga total (17 meses) continua ~40 min (inevitável de uma vez), mas é evento raro.
- Esforço: pequeno (passar mesesTocados ao consolidador). Risco: baixo.

**P2.2 — Reduzir índices das temp tables:** hoje cria 6 índices em `_v_fat` + 6 em
`_v_trans` por mês. Medir com EXPLAIN quais o GROUP BY usa; remover os inúteis
(cada build custa). Risco: baixo.

**P2.3 — Tunar paralelismo:** testar 2 workers × work_mem maior vs 4 × 256MB
(4 scans concorrentes brigam por I/O no container de 2G). Risco: baixo, experimental.

**P2.4 — Pré-agregação base (rolar de baixo p/ cima):** computar 1× a granularidade
fina (cnpj×cod_prod×grupo) e derivar os níveis por roll-up, em vez de 38 scans
independentes. Maior ganho potencial, mas **alto risco/esforço** (reescrita grande).

### Ordem recomendada
1. **P2.1 (incremental)** — resolve a dor diária com baixo risco. Fazer primeiro.
2. **P1 (mix_total)** — painel instantâneo; aceitar leve aumento na escrita.
3. P2.2 / P2.3 — afinar. 4. P2.4 só se ainda doer.

---


## Status (2026-06-13)

- ✅ **Migration 172 validada**: filtros listam opções (gerente/rca/sup/fornec/cli/uf), Marketing com dados, mobile V05 OK.
- ✅ **BUG do filtro cruzado corrigido** (commit 26d2104) — NÃO estava no plano original, mas era regressão funcional urgente. Filtrar por dimensão ausente na tabela agg da view (ex: fornecedor em "Por Gerência", ou UF/Filial em qualquer view) deixava a tela vazia. Agora cai para `vendas_*` via `queryAggregatedVendas`. Tradeoff aceito pelo usuário: base_cli sob filtro cruzado = compradores-12M-do-recorte.
- ✅ **P3 (tuning Postgres) aplicado** (commit 868a3c3) — shared_buffers 512MB, effective_cache_size 1536MB, maintenance_work_mem 256MB, work_mem 24MB. Dentro do limite 2G do container.
- ⏳ **P1 e P2 PENDENTES** — exigem usuário presente (mexem em base_cli/mix, números sensíveis; P2 é a 5ª/6ª reescrita de upsert_aggs_mes). Ver abaixo.

### Opcional de baixo custo já identificado (P3+)
Subir `deploy.resources.limits.memory` de 2G → 3-4G e `shared_buffers` → 1GB
renderia ganho maior (banco tem ~13GB). **Decidir após `free -h`** no host
(há 5 Postgres rodando — confirmar folga de RAM antes).

## Números observados hoje (baseline)

| Operação | Tempo medido | Status |
|---|---|---|
| `queryAggregatedMes` (todas as views) | 5-200 ms | ✓ saudável |
| `queryMixTotal` 1ª vez, mês único, fornec/sup | 0.6-2 s | ✓ aceitável (pós-índice 171) |
| `queryMixTotal` 1ª vez, YTD (12 meses) | 3-14 s | ⚠ P1 resolve |
| `queryMixTotal` 1ª vez, por gerência | 4-6 s | ✓ índice na 172 |
| `queryMixTotal` repetida (cache TTL 10min) | <1 ms (CACHE HIT) | ✓ funcionando |
| `upsert_aggs_mes` POR MÊS | **6-15 min** | ✗ P2 (pior ofensor da operação) |
| RefreshViews completo (11 meses, 4 workers) | **34 min** | ✗ P2 |
| Login (bcrypt) | 1.1-1.5 s | ok, custo do hash |

## P1 — Materializar `mix_total` nas agg_*_mes (elimina queryMixTotal do caminho crítico)

**Problema:** o "Mix X de Y" consulta `vendas_faturadas` (5.8M linhas) a cada request.
Cache de 10 min mitiga, mas toda primeira request por (janela × nível × drill) paga 0.6-14 s.

**Solução:** coluna `mix_total INT DEFAULT 0` em todas as agg_(fat|trans)_v0X_lY_mes
(exceto L4/folha-cliente, onde mix já é por cliente), populada no upsert com
`COUNT(DISTINCT v.cod_prod) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')` —
a temp table `_v_fat` já tem cod_prod, custo marginal ~zero no upsert.

Backend: `queryAggregatedMes` ganha `SUM/AVG(mix_total)` no SELECT (avaliar
semântica de agregação multi-mês: para "universo de SKUs no período" o correto
seria COUNT DISTINCT global — aproximação aceitável: MAX(mix_total) do maior
mês, documentar a escolha). Remove `queryMixTotal`, o cache e o singleflight
(código morto após isso).

- **Impacto:** painel 100% servido pelas aggs; pior request cai de 14s → ~200ms.
- **Esforço:** migration ALTER + função (1 arquivo, mesmo padrão da 172) + ~40 linhas Go + repopulação.
- **Risco:** baixo. Atenção à agregação multi-mês (decidir SUM vs MAX e documentar).

## P2 — Otimizar `upsert_aggs_mes` (consolidação 34 min → alvo <5 min)

**Problema:** para CADA mês upsertado, a função cria `_v_fat_12m` copiando as
**linhas brutas** de 12 meses de vendas (~3M linhas) + 4 índices na temp table.
Upsertar 11 meses = ~36M linhas copiadas + 44 índices temporários. É o grosso
dos 6-15 min/mês.

**Solução:** substituir `_v_fat_12m` (linhas brutas) por 4 temp tables **já agregadas**:

```sql
CREATE TEMP TABLE _b12_fornec     AS SELECT cod_fornec, COUNT(DISTINCT cnpj) base FROM vendas_faturadas WHERE <12m> GROUP BY 1;
CREATE TEMP TABLE _b12_fornec_ger AS SELECT cod_fornec, cod_gerente, COUNT(DISTINCT cnpj) base ... GROUP BY 1,2;
CREATE TEMP TABLE _b12_fornec_sup AS ... ;
CREATE TEMP TABLE _b12_fornec_rca AS ... ;
```

Os sub-selects correlacionados do V01 L0-L3 viram LEFT JOIN nas tabelinhas
(centenas de linhas em vez de milhões). Mesmo resultado, ~10-20× menos I/O.

Extras do P2:
- Índice de apoio ao scan 12M: `(empresa_id, data_faturamento) INCLUDE (cod_fornec, cod_gerente, cod_supervisor, cod_rca, cnpj) WHERE qt>0 AND cod_fornec<>''` — permite index-only scan (validar com EXPLAIN antes; pode ser desnecessário após a agregação).
- Reavaliar `workers=4` em `upsertAggsMesParallel`: 4 workers × scan 12M concorrente = briga de I/O. Testar 2.

- **Impacto:** consolidação pós-import diário de ~10 min/mês → ~1-2 min/mês.
- **Esforço:** médio. Migration 17X reescrevendo só o trecho `_v_fat_12m`/`_v_trans_12m` + os 8 INSERTs V01 L0-L3 (fat+trans). Manter o resto intocado (lição das 169/170/172: **diff sistemático contra a versão anterior antes do push**).
- **Risco:** médio — é a 4ª reescrita da função. Obrigatório: `grep -oE "INSERT INTO farol\.[a-z_0-9]+" | sort | uniq -c` comparando com a 172 antes de commitar.

## P3 — Tuning do Postgres (baixo esforço, ganho geral)

Compose atual: `shared_buffers=256MB`, `effective_cache_size=1GB` — para um banco
que já tem ~13GB. Todo scan grande vai a disco.

Proposta (validar RAM total do host antes — `free -h`; há 5 postgres no servidor):
- `shared_buffers=1GB` (se houver folga de RAM)
- `effective_cache_size=3GB`
- `random_page_cost=1.1` (SSD)
- `maintenance_work_mem=256MB` (acelera CREATE INDEX e VACUUM)

Mudança no `docker-compose.prod.yml` (seção command do db) + redeploy.
- **Impacto:** todos os scans/agregações; beneficia P1/P2 também.
- **Esforço:** 30 min. **Risco:** baixo (validar que não tem OOM com 5 bancos no host).

## P4 — Eliminar trabalho duplicado no caminho do dashboard

1. **Prefetch desalinhado (verificar):** o prefetch pós-login usa preset YTD
   calculado com a data de HOJE, mas o componente calcula YTD a partir de
   `periodos[0]` (último mês importado). Se as queryKeys diferirem em 1 dia,
   o prefetch é desperdiçado. Validar comparando a queryKey do prefetch com a
   primeira query real no React Query DevTools; alinhar a derivação das datas
   (extrair helper único compartilhado).
2. **Cache backend de cards (opção B da discussão anterior):** generalizar o
   padrão cache+singleflight (hoje só no mix) para `fetchCards` inteiro,
   keyed por (empresa, view, fluxo, janela, drill, filtros), TTL 10 min,
   invalidado no RefreshViews. Vale a pena se P1 não bastar.
3. **Logs:** `[dims]`/`[farol:agg]` logam toda request (7+ linhas cada) — passar
   para nível debug ou logar só >100ms quando a operação estabilizar.

## P5 — Housekeeping (quando sobrar tempo)

- `VACUUM ANALYZE` nas partições agg após repopulações grandes (172 fez ANALYZE; vacuum fica para o autovacuum — ok).
- Dropar índices duplicados detectados: `idx_companies_group_id`, `idx_companies_owner_id` (apontados em sessão anterior, nunca executado).
- Migrations 121-137 são legado de objetivos dropado pela 138 — candidatas a no-op em batch para acelerar bootstrap de banco novo (cosmético).
- `docker-compose.prod.yml` tem `sslmode=require` mas runtime usa `sslmode=disable` — inconsistência a investigar (segurança, não performance).

## Ordem recomendada de execução

1. **P3** (30 min, destrava tudo) → 2. **P1** (elimina pior latência de UI) → 3. **P2** (operação diária de import) → 4. P4 conforme necessidade → 5. P5.

## Pendências de validação (antes de qualquer item)

- [ ] Migration 172 rodou OK? Filtros com opções? Marketing com dados?
- [ ] `queryMixTotal nível=cod_gerente` < 1s pós-172?
- [ ] Mobile /m/.../sup/701 → "Por Fornecedor" com cards?
