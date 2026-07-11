---
task: adequacao-novo-layout
phase: fase-1-import-only
created: 2026-07-01
status: pending-approval
---

# Adequação ao novo layout ION VENDAS — Fase 1 (Import)

## Contexto

ION VENDAS reformulou o layout dos CSVs exportados. Legenda oficial em
`Legenda farol.txt` anexada pela gestão. Principais mudanças:

- Nome do arquivo: `MES_ANO_EVENTO.CSV` (ex: `JAN_2025_TF.csv`)
- Eventos: `TF` (Transmitido+Faturado, como hoje) e `CCD` (Cortado, Cancelado, Devolvido — novo)
- Estados novos: `CORTADO`, `CANCELADO`, `DEVOLVIDO` (além de FATURADO/TRANSMITIDO)
- Colunas novas: `CODEPTO`, `DEPARTAMENTO`, `CODSEC`, `SECAO`, `CODCATEGORIA`, `CATEGORIA`, `CODCLIPRINC`, `FANTASIA`, `PVENDA_TOTAL`
- `PVENDA` agora é claramente unitário; `PVENDA_TOTAL` = QT × PVENDA vem no CSV
- `PLUCRO` marcado como "NÃO DEFINIDO"

## Escopo desta Fase 1

**INCLUI:**
1. Migration 181: adiciona colunas novas em `vendas_faturadas` e `vendas_transmitidas`
2. Migration 182: cria tabela `vendas_ccd` com estrutura similar + coluna `evento`
3. Atualiza `farol_v2_import.go`:
   - Detecta as colunas novas por nome (case-insensitive, com aliases)
   - Grava as colunas novas nas tabelas correspondentes
   - Roteia CORTADO/CANCELADO/DEVOLVIDO → `vendas_ccd`
   - Usa `PVENDA_TOTAL` do CSV se presente, fallback para `PVENDA × QT`
   - Grava `PVENDA` unitário na nova coluna `pvenda_unit`
   - Compat total: CSVs no formato antigo continuam funcionando (colunas novas ficam com default vazio/zero)

**NÃO INCLUI (fica pra Fase 2):**
- Novas hierarquias V06 "Por Rede" e V07 "Por Departamento" no GRID
- Novas tabelas `agg_v06_*_mes` e `agg_v07_*_mes`
- Nova função `upsert_aggs_mes` cobrindo V06/V07
- Botões novos no toggle da UI
- Endpoint público do painel mobile precisa lidar com os novos filtros

## Decisão-chave: semântica do `pvenda`

Hoje `vendas_faturadas.pvenda` guarda o **TOTAL** (`pvendaUnit × qt`). Todas as
queries `SUM(pvenda)` e todas as `agg_*_mes` dependem disso. Mudar essa semântica
quebra em cascata.

**Decisão**: `pvenda` continua sendo TOTAL (compat). Nova coluna `pvenda_unit`
guarda o unitário informativo. Import prefere `PVENDA_TOTAL` do CSV pra popular
`pvenda`; se não vier, calcula.

## Migration 181 — colunas novas em vendas_*

```sql
ALTER TABLE vendas_faturadas
  ADD COLUMN IF NOT EXISTS cod_depto      TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS depto          TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS cod_sec        TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS secao          TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS cod_categoria  TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS categoria      TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS cod_cliprinc   TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS fantasia       TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS pvenda_unit    NUMERIC(15,4) NOT NULL DEFAULT 0;

-- Idem vendas_transmitidas
ALTER TABLE vendas_transmitidas
  ADD COLUMN IF NOT EXISTS cod_depto      TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS depto          TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS cod_sec        TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS secao          TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS cod_categoria  TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS categoria      TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS cod_cliprinc   TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS fantasia       TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS pvenda_unit    NUMERIC(15,4) NOT NULL DEFAULT 0;
```

Sem índices novos por enquanto (Fase 2 cuida disso).

## Migration 182 — vendas_ccd

```sql
CREATE TABLE IF NOT EXISTS vendas_ccd (
  id                BIGSERIAL PRIMARY KEY,
  empresa_id        UUID NOT NULL,
  data_evento       DATE NOT NULL,
  evento            TEXT NOT NULL,  -- CORTADO | CANCELADO | DEVOLVIDO
  -- Hierarquia comercial (mesma das outras tabelas)
  cod_gerente       TEXT NOT NULL DEFAULT '',
  nome_gerente      TEXT NOT NULL DEFAULT '',
  cod_supervisor    TEXT NOT NULL DEFAULT '',
  nome_supervisor   TEXT NOT NULL DEFAULT '',
  qtrca_supervisor  INT  NOT NULL DEFAULT 0,
  cod_rca           TEXT NOT NULL DEFAULT '',
  nome_rca          TEXT NOT NULL DEFAULT '',
  qtcli_rca         INT  NOT NULL DEFAULT 0,
  cod_fornec        TEXT NOT NULL DEFAULT '',
  nome_fornec       TEXT NOT NULL DEFAULT '',
  cod_cli           TEXT NOT NULL DEFAULT '',
  nome_cli          TEXT NOT NULL DEFAULT '',
  uf                TEXT NOT NULL DEFAULT '',
  empresa           TEXT NOT NULL DEFAULT '',
  cod_prod          TEXT NOT NULL DEFAULT '',
  nome_prod         TEXT NOT NULL DEFAULT '',
  ean               TEXT NOT NULL DEFAULT '',
  qt                NUMERIC(15,3) NOT NULL DEFAULT 0,
  pvenda            NUMERIC(15,2) NOT NULL DEFAULT 0,  -- total
  pvenda_unit       NUMERIC(15,4) NOT NULL DEFAULT 0,
  plucro            NUMERIC(15,2) NOT NULL DEFAULT 0,
  importado_em      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  cnpj              TEXT NOT NULL DEFAULT '',
  cod_ramo          TEXT NOT NULL DEFAULT '',
  ramo              TEXT NOT NULL DEFAULT '',
  embalagem         TEXT NOT NULL DEFAULT '',
  qt_unit           NUMERIC(15,3) NOT NULL DEFAULT 0,
  qt_unit_cx        NUMERIC(15,3) NOT NULL DEFAULT 0,
  cod_bar           TEXT NOT NULL DEFAULT '',
  -- Novas colunas
  cod_depto         TEXT NOT NULL DEFAULT '',
  depto             TEXT NOT NULL DEFAULT '',
  cod_sec           TEXT NOT NULL DEFAULT '',
  secao             TEXT NOT NULL DEFAULT '',
  cod_categoria     TEXT NOT NULL DEFAULT '',
  categoria         TEXT NOT NULL DEFAULT '',
  cod_cliprinc      TEXT NOT NULL DEFAULT '',
  fantasia          TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_vccd_emp_data
  ON vendas_ccd(empresa_id, data_evento);
```

Mínimo de índices — Fase 2 adiciona os que forem necessários.

## Atualização do `farol_v2_import.go`

**Detecção de colunas (adiciona depois do `iCodBar`):**

```go
iCodDepto     := col(-1, "codepto", "cod_depto", "coddepto")
iDepto        := col(-1, "departamento", "depto")
iCodSec       := col(-1, "codsec", "cod_sec", "codsecao")
iSecao        := col(-1, "secao", "seção")
iCodCategoria := col(-1, "codcategoria", "cod_categoria")
iCategoria    := col(-1, "categoria")
iCodCliPrinc  := col(-1, "codcliprinc", "cod_cliprinc", "codcli_principal")
iFantasia     := col(-1, "fantasia", "nome_fantasia")
iPvendaTotal  := col(-1, "pvenda_total", "pvendatotal", "valor_total", "vl_total")
```

**Detecção do evento** (substitui `detectEstado`):

```go
detectEvento := func(periodo, estadoField string) string {
    e := strings.ToUpper(estadoField)
    switch {
    case strings.Contains(e, "TRANS"):
        return "TRANSMITIDO"
    case strings.Contains(e, "CORT"):
        return "CORTADO"
    case strings.Contains(e, "CANCEL"):
        return "CANCELADO"
    case strings.Contains(e, "DEVOL"):
        return "DEVOLVIDO"
    default:
        return "FATURADO"
    }
}
```

**Cálculo pvenda_total** (substitui a fórmula atual):

```go
// pvenda_total preferencial: CSV se presente, fallback QT × PVENDA
qtVal := parseNum(rawQt)
pvendaUnit := parseNum(rawPvenda)
var pvendaTotal float64
if rawPvendaTotal := getField(csvRow, iPvendaTotal); rawPvendaTotal != "" {
    pvendaTotal = parseNum(rawPvendaTotal)
} else {
    pvendaTotal = pvendaUnit * qtVal
}
// PLUCRO: campo perdeu significado no novo layout — grava 0
// (compat: se CSV mandar %, converte pra valor)
plucroValor := 0.0
if rawPlucro != "" {
    plucroPct := parseNum(rawPlucro)
    if plucroPct != 0 {
        plucroValor = pvendaTotal * plucroPct / 100.0
    }
}
```

**Rota de linhas por evento:**

```go
switch evento {
case "TRANSMITIDO":
    allTrans = append(allTrans, r)
case "FATURADO":
    allFat = append(allFat, r)
case "CORTADO", "CANCELADO", "DEVOLVIDO":
    allCCD = append(allCCD, r)
}
```

**Layout `vals` estendido** (de 29 pra 38 campos):

Adiciona nos índices `[29]-[37]`:
- 29: cod_depto, 30: depto
- 31: cod_sec, 32: secao
- 33: cod_categoria, 34: categoria
- 35: cod_cliprinc, 36: fantasia
- 37: pvenda_unit

**Terceiro `processFlow`** pra vendas_ccd (mesma estrutura de fat/trans + coluna `evento`).

## Dedup no CCD

O dedup defensivo continua com a mesma chave `(data, cnpj, prod, qt, pvenda)`,
aplicado separadamente por tabela. Se um CSV `_CCD.csv` também vier triplicado
(mesmo bug do ION VENDAS que já corrigimos pro TF), o dedup já protege.

## Testes

**Local:**
1. Compila `go build ./...` — deve passar sem erros
2. Roda um CSV de teste do novo layout (small sample) via UI local
3. Confere:
   - Linhas TF vão pra vendas_faturadas/transmitidas
   - Linhas CCD vão pra vendas_ccd
   - Colunas novas populadas
   - `pvenda` continua sendo total (SUM funciona)
   - `pvenda_unit` populado

**Compat (crítico):**
Roda um CSV do formato antigo — deve continuar funcionando exatamente como
antes (colunas novas ficam com default vazio/zero).

## Riscos e mitigações

| Risco | Mitigação |
|---|---|
| CSVs antigos param de funcionar | Colunas novas são NULLABLE/default '' — não obrigatórias |
| `pvenda_total` vem 0 no CSV novo | Fallback `PVENDA × QT` cobre isso |
| Migration 182 falha (vendas_ccd) | `CREATE TABLE IF NOT EXISTS` — idempotente |
| Backup do banco fica maior | ~1-2 GB adicionais só se CCD volumar. Prod tem 43 GB livres |
| Layout tem colunas em MAIÚSCULAS diferentes | `col()` normaliza (lowercase + strip); aliases cobrem variantes |

## Rollback

Se algo quebrar depois do deploy:
1. `git revert HEAD` — desfaz o commit
2. Migrations 181 e 182 são idempotentes (`ADD COLUMN IF NOT EXISTS`, `CREATE TABLE IF NOT EXISTS`) — não precisam de rollback SQL
3. Colunas novas ficam órfãs mas não atrapalham queries existentes

## Commits previstos

1. `feat(schema): migration 181 adiciona colunas do novo layout ION VENDAS`
2. `feat(schema): migration 182 cria tabela vendas_ccd`
3. `feat(import): suporte ao novo layout ION VENDAS (TF + CCD)`

Pode ser 3 commits separados ou 1 único — decidir na hora.

## Estimativa

~4-6h de trabalho + tempo de teste com CSV real do novo layout.

## Ready to proceed?

Se aprovado, executo na sequência:
1. Escreve migration 181
2. Escreve migration 182
3. Modifica `farol_v2_import.go`
4. Compila local
5. Commit + push
