---
task: adequacao-novo-layout
phase: fase-2-hierarquias-v06-v07
created: 2026-07-01
status: pending-approval
---

# Adequação ao novo layout ION VENDAS — Fase 2 (Hierarquias V06/V07)

## Contexto

A Fase 1 (commit `7dd3e56`) preparou o pipeline de import + schema. As colunas
`cod_cliprinc`, `cod_depto`, `cod_sec`, `cod_categoria` já são gravadas em
`vendas_faturadas` / `vendas_transmitidas` a cada import do novo layout.

Fase 2 expõe essas dimensões no GRID via duas novas hierarquias/visões:

- **V06 "Por Rede"**: `cod_cliprinc → cod_fornec → cod_cli → cod_prod`
- **V07 "Por Departamento"**: `cod_depto → cod_sec → cod_categoria → cod_prod`

Ambas só no Painel Executivo (web autenticado). Sem positivação (só valor).

## Escopo

**INCLUI:**
1. Migration 183: 6 tabelas agg da V06 (fat + trans, L0/L1/L2)
2. Migration 184: 6 tabelas agg da V07 (fat + trans, L0/L1/L2)
3. Migration 185: estende `farol.upsert_aggs_mes` pra popular as novas agg
4. Backend `farol_v2_api.go`:
   - `hierarquias` map ganha `V06` e `V07`
   - `aggTablesFat` / `aggTablesTrans` ganham as tabelas novas
   - `leafServesPositivados` retorna `false` pra V06/V07 (sem positivação)
5. Frontend `FarolExecutivo.tsx`:
   - Toggle ganha "Por Rede" e "Por Departamento"
   - `hidePosit` forçado quando view ∈ {V06, V07}

**NÃO INCLUI:**
- Painel público mobile (SUP/RCA) continua com V02/V05
- Filtros dim novos (`cod_cliprinc`, `cod_depto`) no endpoint `/api/v2/farol/dims`
  (fica pra Fase 3 se o gestor pedir)
- Positivação em V06/V07
- V06 no BI CEO War Room

## Estrutura das novas agg

### V06 (Por Rede)

| Tabela | Nível | Colunas de drill | Métricas |
|---|---|---|---|
| `agg_fat_v06_l0_mes` | L0 = rede | `cod_cliprinc, nome_cliprinc` | `pvenda`, `plucro`, `qt` |
| `agg_fat_v06_l1_mes` | L1 = fornec | + `cod_fornec, nome_fornec` | idem |
| `agg_fat_v06_l2_mes` | L2 = cliente | + `cnpj, cod_cli, nome_cli` | idem |

`nome_cliprinc` = MAX(nome_cli) da mesma rede — como não há tabela de cadastro
de redes, usamos como aproximação a razão social do primeiro cliente da rede.
Pode ser melhorado com uma tabela `redes_cadastro` no futuro.

Como positivação está fora, **não incluem** `base_cli`, `positivados`, `mix`.

### V07 (Por Departamento)

| Tabela | Nível | Colunas de drill |
|---|---|---|
| `agg_fat_v07_l0_mes` | L0 = depto | `cod_depto, depto` |
| `agg_fat_v07_l1_mes` | L1 = seção | + `cod_sec, secao` |
| `agg_fat_v07_l2_mes` | L2 = categoria | + `cod_categoria, categoria` |

Métricas: só `pvenda`, `plucro`, `qt` (sem positivação).

Mesmas 3 tabelas espelhadas em `agg_trans_v07_*_mes`.

## Migration 183 — vendas → agg_v06

Sketch da estrutura (padronizada com `agg_fat_v01_l0_mes`):

```sql
CREATE TABLE IF NOT EXISTS farol.agg_fat_v06_l0_mes (
    empresa_id      UUID    NOT NULL,
    ano             INT     NOT NULL,
    mes             INT     NOT NULL,
    cod_cliprinc    TEXT    NOT NULL,
    nome_cliprinc   TEXT    NOT NULL DEFAULT '',
    pvenda          NUMERIC(18,2) NOT NULL DEFAULT 0,
    plucro          NUMERIC(18,2) NOT NULL DEFAULT 0,
    qt              NUMERIC(18,3) NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, cod_cliprinc)
) PARTITION BY LIST (ano);

-- Idem l1 (+ cod_fornec, nome_fornec)
-- Idem l2 (+ cnpj, cod_cli, nome_cli)
-- Idem agg_trans_v06_*
```

Padrão de particionamento por `ano` segue mig 162 — chamada a
`farol.create_agg_year_partitions(ano)` cria as partições dos anos existentes.

## Migration 184 — agg_v07

Mesmo padrão:

```sql
CREATE TABLE IF NOT EXISTS farol.agg_fat_v07_l0_mes (
    empresa_id      UUID    NOT NULL,
    ano             INT     NOT NULL,
    mes             INT     NOT NULL,
    cod_depto       TEXT    NOT NULL,
    depto           TEXT    NOT NULL DEFAULT '',
    pvenda          NUMERIC(18,2) NOT NULL DEFAULT 0,
    plucro          NUMERIC(18,2) NOT NULL DEFAULT 0,
    qt              NUMERIC(18,3) NOT NULL DEFAULT 0,
    PRIMARY KEY (ano, empresa_id, mes, cod_depto)
) PARTITION BY LIST (ano);

-- l1 (+ cod_sec, secao)
-- l2 (+ cod_categoria, categoria)
-- + versões trans
```

## Migration 185 — estende `farol.upsert_aggs_mes`

Adiciona 12 novos `INSERT ... ON CONFLICT` no corpo da função (6 pra V06 fat/trans
+ 6 pra V07), lendo das temp tables `_v_fat` / `_v_trans` que já são
criadas pela mig 169.

Cada bloco segue o padrão:

```sql
INSERT INTO farol.agg_fat_v06_l0_mes
    (empresa_id, ano, mes, cod_cliprinc, nome_cliprinc, pvenda, plucro, qt)
SELECT v.empresa_id, p_ano, p_mes, v.cod_cliprinc, MAX(v.nome_cli),
    SUM(v.pvenda), SUM(v.plucro), SUM(v.qt)
FROM _v_fat v WHERE v.cod_cliprinc <> ''
GROUP BY v.empresa_id, v.cod_cliprinc
ON CONFLICT (ano, empresa_id, mes, cod_cliprinc) DO UPDATE SET
    nome_cliprinc = EXCLUDED.nome_cliprinc,
    pvenda = EXCLUDED.pvenda, plucro = EXCLUDED.plucro, qt = EXCLUDED.qt;
```

Ao final, adiciona chamada `farol.create_agg_year_partitions(p_ano)` pra
garantir que os anos das novas tabelas particionadas estão criados.

## Backend (`farol_v2_api.go`)

```go
var hierarquias = map[string][]hierLevel{
    // ... V01-V05 existentes ...
    "V06": {
        {Level: "cod_cliprinc", NameField: "nome_cliprinc", Label: "Rede"},
        {Level: "cod_fornec",   NameField: "nome_fornec",   Label: "Fornecedor"},
        {Level: "cod_cli",      NameField: "nome_cli",      Label: "Cliente"},
        {Level: "cod_prod",     NameField: "nome_prod",     Label: "Produto"},
    },
    "V07": {
        {Level: "cod_depto",     NameField: "depto",     Label: "Departamento"},
        {Level: "cod_sec",       NameField: "secao",     Label: "Seção"},
        {Level: "cod_categoria", NameField: "categoria", Label: "Categoria"},
        {Level: "cod_prod",      NameField: "nome_prod", Label: "Produto"},
    },
}

var aggTablesFat = map[string][]string{
    // ... V01-V05 ...
    "V06": {"agg_fat_v06_l0_mes", "agg_fat_v06_l1_mes", "agg_fat_v06_l2_mes"},
    "V07": {"agg_fat_v07_l0_mes", "agg_fat_v07_l1_mes", "agg_fat_v07_l2_mes"},
}
```

Além de `allowedCols` que precisa aceitar `cod_cliprinc`, `nome_cliprinc`,
`cod_depto`, `depto`, `cod_sec`, `secao`, `cod_categoria`, `categoria`.

`leafServesPositivados` retorna `false` sempre pra V06 e V07 (não há
positivação nas agg dessas views).

## Frontend (`FarolExecutivo.tsx`)

Toggle atual:
```
[Por Indústria] [Por Gerência] [Por Equipe]
```

Vira:
```
[Por Indústria] [Por Gerência] [Por Equipe] [Por Rede] [Por Departamento]
```

`hidePosit` já é usado hoje — precisa ampliar a condição:
```ts
const hidePosit = view === 'V06' || view === 'V07' ||
                  curLevel === 'cod_cli' || curLevel === 'cod_prod'
```

Tipo `View` estendido:
```ts
type View = 'V01' | 'V02' | 'V03' | 'V06' | 'V07'
```

## Riscos

| Risco | Mitigação |
|---|---|
| Base local sem `cod_cliprinc`/`cod_depto` populados | Agg V06/V07 ficam vazias (funcional mas GRID mostra "sem dados") — resolvido quando gestor reimportar CSVs novos |
| `nome_cliprinc` = MAX(nome_cli) fica ruim quando redes têm nomes muito diferentes | Aceito como aproximação inicial. Fase 3: criar tabela `redes_cadastro` |
| Frontend do painel público (mobile) referencia tipo `View` | Confinar tipos V06/V07 só no Executivo — painel público continua `V02|V05` |
| Performance: 12 tabelas novas particionadas por ano | Volume estimado similar a V04 (que já existe). Sem preocupação |

## Rollback

Migrations idempotentes (`CREATE TABLE IF NOT EXISTS`, `CREATE OR REPLACE FUNCTION`).
Se algo quebrar: `git revert` do commit; agg tables ficam órfãs mas não atrapalham.
`DROP TABLE farol.agg_*_v06_*_mes CASCADE` deixa limpo.

## Commits previstos

1. `feat(schema): migration 183 cria agg tables da V06 Por Rede`
2. `feat(schema): migration 184 cria agg tables da V07 Por Departamento`
3. `feat(schema): migration 185 estende upsert_aggs_mes com V06/V07`
4. `feat(api): registra V06/V07 nos maps de hierarquias e agg tables`
5. `feat(farol): botões Por Rede e Por Departamento no Painel Executivo`

Ou consolida em 2-3 commits temáticos. Decidido no fim.

## Estimativa

~4-6h de trabalho + testes locais.

## Ready to proceed?

Se aprovado:
1. Migrations 183 + 184 (~30min cada)
2. Migration 185 (mais complexa, ~1-2h de SQL)
3. Backend Go (~30min)
4. Frontend (~45min)
5. Aplica migrations local + build backend + build frontend
6. Commits + push
