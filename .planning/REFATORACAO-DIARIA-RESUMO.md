# Refatoração para granularidade diária — Resumo da sessão

**Status:** Concluído no localhost. Nenhum commit feito. Aguardando você acordar.

---

## O insight central que guiou tudo

> **"A venda de hoje pode ser a comparativa de amanhã."**

`tipo_base = ATUAL | COMPARATIVA` era uma propriedade do **dado**, mas conceitualmente é uma propriedade da **consulta**. Uma venda do dia 10/05/26 é "atual" se você está olhando junho/26, e vira "comparativa" automaticamente em junho/27 — sem reclassificação, sem migração.

Tudo o que foi feito decorre disso: **remover `tipo_base` de toda a stack** e substituir por **intervalos de datas livres na consulta**.

---

## Arquitetura final

```
                    ┌──────────────────────────┐
                    │       CSV diário         │
                    │  DATA + PERIODO + ...    │
                    └────────────┬─────────────┘
                                 │ importador rotea linha-por-linha
                ┌────────────────┴────────────────┐
                ▼                                 ▼
        ┌────────────────────┐         ┌─────────────────────┐
        │  vendas_faturadas  │         │ vendas_transmitidas │
        │  data_faturamento  │         │  data_transmissao   │
        └────────┬───────────┘         └──────────┬──────────┘
                 │                                 │
        14 MVs (mv_fat_*)                14 MVs (mv_trans_*)
                 │                                 │
                 └─────────────┬───────────────────┘
                               ▼
                ┌─────────────────────────────────┐
                │   API: range BETWEEN + fluxo    │
                │   2 queries paralelas (atual +  │
                │   comparativo) sobre a MESMA MV │
                └─────────────────────────────────┘
```

**Princípios:**
- Sem `tipo_base`. Sem `estado` na chave de MV. Sem ano/mes como filtro principal.
- `fluxo = faturado | transmitido` define qual tabela/MV consultar (default: `faturado`).
- Ranges de datas livres: `ref_inicio/ref_fim` + `comp_inicio/comp_fim`.
- Retrocompat: UI antiga que manda `ref_ano/ref_mes + comp_mode` continua funcionando (backend converte).

---

## Migrations criadas (já aplicadas no localhost)

| # | Arquivo | O que fez |
|---|---|---|
| 154 | `vendas_data_processo.sql` | Tentativa inicial — adicionou colunas em `vendas_importadas`. **Descontinuada** (sobrescrita por 156). Registrada no `schema_migrations` pra não rerodar. |
| 155 | `mv_data_processo.sql` | Tentativa de MV única com `estado` na chave. **Descontinuada**. Registrada. |
| 156 | `vendas_split.sql` | **DROP** `vendas_importadas` + cria `vendas_faturadas` e `vendas_transmitidas`. Schema definitivo. |
| 157 | `mvs_split.sql` | 28 MVs com `tipo_base` ainda. **Descontinuada** pelo insight do "venda de hoje vs amanhã". Registrada. |
| 158 | `drop_tipo_base.sql` | DROP CASCADE das tabelas (mata MVs antigas) + recria sem `tipo_base`. Drop `tipo_base` em `vendas_import_jobs`. |
| 159 | `mvs_no_tipobase.sql` | **28 MVs finais** sem `tipo_base` na chave. |

**Importante:** se você rodar `migrations/` do zero em produção, as migrations 154/155/157 vão FALHAR (tentam mexer em `vendas_importadas` ou referenciar `tipo_base`). Antes de rodar em PRD:
- **Opção A (recomendada):** apagar fisicamente os arquivos 154, 155, 157. Eles foram superados.
- **Opção B:** Manter os arquivos como histórico mas inserir manualmente no `schema_migrations` antes do startup.

Como você disse "vamos limpar a base em PRD", a opção A é a mais limpa. Posso fazer amanhã se quiser.

---

## Schema final

### `vendas_faturadas`
```
id, empresa_id (uuid), data_faturamento (DATE),
cod_gerente, nome_gerente,
cod_supervisor, nome_supervisor, qtrca_supervisor,
cod_rca, nome_rca, qtcli_rca,
cod_fornec, nome_fornec,
cod_cli, nome_cli, uf, empresa,
cod_prod, nome_prod, ean,
qt, pvenda, plucro,
importado_em
```

### `vendas_transmitidas`
Estrutura idêntica, exceto `data_transmissao` no lugar de `data_faturamento`.

### MVs (28 no total)

| Fluxo | Hierarquia | MVs |
|---|---|---|
| Faturado | base (cliente) | `mv_fat_cli` |
| Faturado | V01 (indústria→...) | `mv_fat_v01_l0..l3` |
| Faturado | V02 (equipe→...) | `mv_fat_v02_l0..l2` |
| Faturado | V03 (gerência→...) | `mv_fat_v03_l0..l3` |
| Faturado | Marketing | `mv_fat_mkt_produto`, `mv_fat_mkt_prod_pen` |
| Transmitido | (idem) | `mv_trans_*` (14 MVs simétricas) |

Cada MV tem chave `(empresa_id, <data>, <hierarquia>...)` + colunas calculadas `ano`/`mes` via `EXTRACT` + índice secundário em `(empresa_id, ano, mes)` para retrocompat de queries antigas.

---

## API — novo contrato

### `GET /api/v2/farol/cards`

**Novos parâmetros:**
| Param | Tipo | Default | Descrição |
|---|---|---|---|
| `view` | `V01\|V02\|V03` | `V01` | Hierarquia |
| `fluxo` | `faturado\|transmitido` | `faturado` | Qual base usar |
| `ref_inicio` | `YYYY-MM-DD` | (último mês com dados) | Início do período principal |
| `ref_fim` | `YYYY-MM-DD` | idem | Fim do período principal |
| `comp_inicio` | `YYYY-MM-DD` | (vazio) | Início do comparativo |
| `comp_fim` | `YYYY-MM-DD` | idem | Fim do comparativo |
| `drill` | JSON | `[]` | Drill-path |

**Retrocompat (UI antiga continua funcionando):**
- `ref_ano + ref_mes` → backend converte para `ref_inicio/ref_fim` cobrindo o mês inteiro.
- `comp_mode = yoy|mom|ytd` → backend deriva `comp_inicio/comp_fim` automaticamente.
  - `yoy`: subtrai 1 ano do range de ref
  - `mom`: range contíguo imediatamente anterior (mesmo número de dias)
  - `ytd`: 1º jan do ano anterior até a mesma data
- `comp_ano + comp_mes` → override de mês exato (vira `comp_mode=mom`)

**Sem nenhum desses parâmetros**, o backend usa o último mês com dados como `ref` e nenhum comparativo (UI mostra cards sem comparação).

### `GET /api/v2/farol/public/cards` (ION VENDAS — sem auth)

Mesmos parâmetros + os já existentes (`cnpj`, `scope=sup|rca`, `cod`). URLs do ION continuam batendo no mesmo lugar.

### `GET /api/v2/marketing/cards`, `/produto-detalhe`, `/cliente-detalhe`

Mesmo padrão: `fluxo`, ranges com retrocompat ano/mes/comp_mode.

### `DELETE /api/v2/vendas/clear`

Agora aceita:
- `?data_inicio=YYYY-MM-DD&data_fim=YYYY-MM-DD` → apaga o intervalo em ambas as tabelas
- (sem params) → apaga toda a base da empresa

**Removido:** `?tipo_base=`. Antes existiam variantes "limpar só ATUAL" — não fazem mais sentido.

### `POST /api/v2/vendas/import`

Continua aceitando `?ano=X&mes=Y` como **fallback** (linhas do CSV sem coluna `DATA` válida usam dia 1 desse mês). Mas a fonte de verdade agora é a coluna `DATA` do CSV + `PERIODO` (que define a tabela destino).

Header do CSV aceito: `DATA`, `data_processo`, `dataprocesso`, `dt`, `data_movimento`.

---

## Smoke tests rodados (todos OK)

Inserí 4 linhas sintéticas (2 fat maio/26, 1 fat maio/25 "histórico", 1 trans maio/26) e validei:

| Cenário | Resultado |
|---|---|
| Cards por fornecedor — fluxo=faturado, ref=maio/26 | F01: R$1000 \| F02: R$2000 ✓ |
| Comparativo YoY (maio/26 vs maio/25) | F01: atual 1000, comp 800, +25% ✓ |
| Clientes inativos (transmitiu mas não faturou) | C03 (PAO_DE_ACUCAR) R$500 ✓ |
| Marketing produto — penetração | LEITE NINHO, OMO com EAN+plucro ✓ |
| `vendas_clear` por range de datas | OK, apaga só os dias do range em ambas tabelas ✓ |

Backend rodando em http://localhost:8087 ainda. PID atual provavelmente diferente (rebuildei algumas vezes). Pra confirmar:
```bash
ss -tlnp | grep 8087
tail -50 /tmp/fb_farol_backend.log
```

---

## O que NÃO foi feito (pendências p/ próxima sessão)

### 1. Frontend — UI de upload (`FarolV2Import.tsx`)
Ainda tem dropdown "ATUAL / COMPARATIVA". Backend ignora — não dá erro — mas é **lixo visual** que confunde o usuário. Precisa:
- Remover o dropdown de tipo_base
- Talvez adicionar um date-picker se ele quiser corrigir `?ano=&mes=` para o mês real do CSV (ou só esconder esses campos — o importador lê do CSV agora)

### 2. Frontend — painéis (Vendas / Marketing / BI)
Hoje a UI manda `?ref_ano=&ref_mes=&comp_mode=yoy`. Funciona via retrocompat. Pra aproveitar a granularidade nova precisa:
- **Date-range pickers** (em vez de dropdown de mês)
- **Toggle de fluxo** — botão pra alternar "Vendas (Faturado) ↔ Pedidos (Transmitido)"
- **Comparativo livre** — preset YoY/MoM ou range custom

### 3. Esboço da tela (`Esboço.csv` que você mandou)
O design implícito é: cards por fornecedor com 3 colunas (Vendas, Positivação, Mix). Posso transformar isso em componente React amanhã se quiser.

### 4. Migrations 154/155/157 — limpar
Sugerido apagar fisicamente, já que foram descontinuadas.

### 5. Frontend de "Limpar dados"
Pode ter dropdown ATUAL/COMP no clear que hoje quebraria. Verificar.

### 6. CompMode `mom`, `ytd` derivados — testar
O `deriveCompRange` foi implementado mas não foi validado com smoke test. Provavelmente funciona, mas vale conferir amanhã.

---

## Estado de commits

**Zero commits.** Tudo nos arquivos de trabalho do localhost.

Arquivos modificados:
```
backend/migrations/154_vendas_data_processo.sql     (criado — descontinuado)
backend/migrations/155_mv_data_processo.sql         (criado — descontinuado)
backend/migrations/156_vendas_split.sql             (criado — vigente)
backend/migrations/157_mvs_split.sql                (criado — descontinuado)
backend/migrations/158_drop_tipo_base.sql           (criado — vigente)
backend/migrations/159_mvs_no_tipobase.sql          (criado — vigente)
backend/handlers/farol_v2_api.go                    (reescrito)
backend/handlers/marketing_api.go                   (reescrito)
backend/handlers/farol_v2_import.go                 (refatorado fortemente)
backend/handlers/farol_v2_cleanup.go                (ajuste)
```

Quando autorizar o commit, sugiro **um único commit grande** com mensagem clara:
> `refactor(data): granularidade diária + remoção de tipo_base + fluxo fat/trans separados`

Ou dois commits:
1. Schema + migrations (158, 159)
2. Backend handlers (farol_v2_api, marketing_api, importer, cleanup)

---

## 📄 CSV de teste pronto para importar

**Arquivo:** `/home/claudio/projetos/FB_FAROL/test-data/vendas-teste.csv`

**Conteúdo:** 102 linhas cobrindo:

| Cenário | Linhas | Por que |
|---|---|---|
| FATURADO maio/2026 | 48 | Período "atual" — alimenta painel principal |
| TRANSMITIDO maio/2026 | 27 | Painel de transmitidos + identifica cliente inativo |
| FATURADO maio/2025 | 21 | Comparativa YoY (cresceu / decresceu vs ano anterior) |
| FATURADO abril/2026 | 6 | Comparativa MoM (mês anterior contíguo) |

**Variedade:**
- **5 fornecedores**: NESTLE, UNILEVER, P&G, AMBEV, COCA-COLA
- **8 clientes** em 6 estados (SP/RJ/MG/CE/PR/SC), 3 filiais (NORDESTE/SUDESTE/SUL)
- **15 produtos** com EAN (LEITE NINHO, NESCAU, KITKAT, OMO, REXONA, HELLMANNS, ARIEL, GILLETTE, PAMPERS, SKOL, BRAHMA, GUARANÁ, COCA-COLA, FANTA, SPRITE)
- **9 RCAs** distribuídos em 4 supervisores e 2 gerentes
- **56 dias distintos** de data — testa granularidade diária end-to-end

**Cenários expostos pelo CSV:**
1. **Cliente totalmente inativo no faturamento**: `C08 GIASSI SUPERMERCADOS` (RCA Fernanda) — transmitiu pedidos em maio/26 mas não faturou nenhum. Aparece no card "Clientes Inativos" do painel Marketing.
2. **Comparativo YoY rico**: F01 NESTLE em CARREFOUR/JOAO cresceu (1400 → 1800 = +28%), AMBEV em EXTRA cresceu (760 → 960 = +26%), etc.
3. **Margens variadas** (plucro): cerveja 15%, leite/produtos higiene 20%, KitKat 25%, Gillette/Mach3 30%.
4. **MoM disponível**: abril/26 tem 6 linhas para testar comparativa "Maio vs Abril".

### ⚠️ Plucro vem como **percentual** no CSV

Última hora mudei isso: o CSV traz `PLUCRO` como **% (15-30)**, não como valor absoluto.

**O que mudou no backend** (uma linha em [`farol_v2_import.go`](backend/handlers/farol_v2_import.go)):
```go
// plucro vem no CSV como PERCENTUAL. Converte para valor absoluto na inserção:
plucroValor = pvenda * (plucroPct / 100)
```

Vantagem: **nada mais precisou mudar**. MVs e handlers continuam guardando/somando lucro absoluto (R$). Só a entrada vira mais natural.

### Como subir pela UI

Backend (8087) e frontend (3087) já estão rodando. Para testar:

1. Acesse http://localhost:3087/
2. Login: `claudio_bezerra@hotmail.com` / `123456`
3. Menu lateral → **Importar dados** (módulo Painel Vendas)
4. **Selecione `ATUAL`** no dropdown (o backend ignora mas a UI ainda pede — vamos remover esse dropdown amanhã)
5. **Ano: 2026 / Mês: 5** (idem, ignorado — só usado como fallback se alguma linha do CSV não tiver coluna DATA)
6. Selecione o arquivo `test-data/vendas-teste.csv`
7. Sobe → backend processa, distribui em `vendas_faturadas` (75 linhas — 48 fat/26 + 21 fat/25 + 6 fat/04) e `vendas_transmitidas` (27 linhas), refresca as 28 MVs

**Após importar, valide:**
- Painel **Vendas** com `?fluxo=faturado&ref_inicio=2026-05-01&ref_fim=2026-05-31&comp_inicio=2025-05-01&comp_fim=2025-05-31` deve mostrar comparativo YoY com cards coloridos
- Mesma chamada com `?fluxo=transmitido` mostra os pedidos transmitidos
- Painel Marketing → "Clientes Inativos" deve listar GIASSI

### Conhecidos / observações

- **Migrations antigas falhando no startup** (127, 128) — não-relacionadas, lixo herdado do SMARTPICK. Não bloqueiam nada.
- **Frontend ainda manda `tipo_base`** no upload — backend ignora, não dá erro. Limpar amanhã.
- **CSV usa `;` separador** e vírgula decimal (formato pt-BR padrão do WinThor). O parser detecta ambos.

---

## Próximos passos sugeridos (amanhã)

1. **Apagar migrations 154/155/157** (são lixo agora).
2. **Refatorar `FarolV2Import.tsx`** (remover dropdown tipo_base).
3. **Adicionar date-range pickers** nos painéis (Vendas/Marketing/BI).
4. **Toggle de fluxo** (faturado/transmitido) nos painéis.
5. **Validar `comp_mode=mom` e `ytd`** com dados reais.
6. **Decidir UX dos 2 ranges independentes** (era a discussão de "Vendas" e "Transmitido" com pickers separados — vale rediscutir agora que cada painel é só de um fluxo).
7. **Commitar e fazer push** quando estiver redondo.

Bom dia! ☀️
