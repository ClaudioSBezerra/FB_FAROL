# FB_FAROL — Modelo de importação de vendas

Arquivo de referência: `MODELO_IMPORT.csv` (5 linhas exemplo cobrindo FATURADO + TRANSMITIDO).

## Regras gerais

| Item | Valor |
|------|-------|
| **Separador de coluna** | `;` (ponto-e-vírgula) |
| **Encoding** | UTF-8 (com ou sem BOM) |
| **Decimal** | Aceita `.` (`57.63`) ou `,` (`115,26`) — detecta pelo último separador da linha |
| **Cabeçalho** | Obrigatório na 1ª linha. Nomes case-insensitive (ignora maiúscula/minúscula, espaços e underscore) |
| **Aspas** | Permitidas para campos que contenham `;` ou quebra de linha |
| **Linha vazia** | Ignorada |
| **Coluna ausente** | Tratada como vazio. Apenas `ESTADO` + `DATA` + `PVENDA` são realmente críticos |

## Aliases aceitos no cabeçalho

O importador faz `lowercase + remove(espaços, underscore)`. Então `Cod_Cli`, `CODCLI`, `cod cli` são equivalentes.

| Campo lógico | Aliases aceitos |
|--------------|------------------|
| Data | `DATA`, `DATA_PROCESSO`, `DATAPROCESSO`, `DT`, `DATA_MOVIMENTO` |
| Estado | `ESTADO` |
| Período (rótulo) | `PERIODO` |
| Cód. Gerente | `COD_GERENTE`, `CODGERENTE` |
| Nome Gerente | `GERENTE`, `NOME_GERENTE` |
| Cód. Supervisor | `COD_SUPERVISOR`, `CODSUPERVISOR` |
| Nome Supervisor | `SUPERVISOR`, `NOME_SUPERVISOR` |
| Qtd. RCAs do Supervisor | `QTRCA_SUPERVISOR`, `QTRCASUPERVISOR` |
| Cód. RCA | `COD_RCA`, `CODRCA`, `COD_USUR`, `CODUSUR` |
| Nome RCA | `RCA`, `NOME_RCA` |
| Carteira do RCA (qtd clientes) | `QTCLI_RCA`, `QTCLIRCA` |
| Cód. Fornecedor | `COD_FORNEC`, `CODFORNEC` |
| Nome Fornecedor | `FORNECEDOR`, `NOME_FORNEC` |
| Cód. Cliente | `COD_CLI`, `CODCLI` |
| Nome Cliente | `CLIENTE`, `NOME_CLI` |
| CNPJ Cliente | `CNPJ`, `CNPJ_CLI`, `CNPJ_CLIENTE` |
| UF | `UF` |
| Filial / unidade | `EMPRESA` |
| **Cód. Ramo** (visual) | `COD_RAMO`, `CODRAMO` |
| **Ramo de atividade** (visual) | `RAMO`, `NOME_RAMO` |
| Cód. Produto | `COD_PROD`, `CODPROD` |
| Nome Produto | `PRODUTO`, `NOME_PROD` |
| EAN | `EAN`, `CODEAN`, `COD_EAN` |
| **Cód. Barras** (visual, segundo cód.) | `COD_BAR`, `CODBAR`, `CODIGOBAR` |
| **Embalagem** (visual) | `EMBALAGEM` |
| **Qt. Unidades** (visual) | `QT_UNIT`, `QTUNIT` |
| **Qt. Unidades por Caixa** (visual) | `QT_UNIT_CX`, `QTUNITCX`, `QTUNITCAIXA` |
| Quantidade vendida | `QT`, `QUANTIDADE` |
| Valor de venda | `PVENDA`, `VALOR`, `VL_VENDA` |
| Lucro | `PLUCRO`, `LUCRO`, `VL_LUCRO` |

## Significado de cada coluna

### `DATA` (obrigatória)
Data do evento. Formatos aceitos:
- `2026-06-01` (ISO)
- `01/06/2026` (BR)

O ano da `DATA` decide a partição da tabela (`agg_*_mes_2026`).

### `ESTADO` (obrigatória)
Define em qual tabela a linha vai cair:
- `FATURADO` → `vendas_faturadas` (gera receita, conta no fechamento)
- `TRANSMITIDO` → `vendas_transmitidas` (pedido digitado pelo RCA, ainda pendente)

Qualquer outro valor é ignorado.

### `PERIODO`
Rótulo livre (ex: `Junho/2026`). Apenas para auditoria — não afeta cálculo.

### Hierarquia comercial (campos para drill no Farol)
- `COD_GERENTE` / `GERENTE` — gerência regional
- `COD_SUPERVISOR` / `SUPERVISOR` — supervisor de equipe
- `QTRCA_SUPERVISOR` — quantidade de RCAs sob esse supervisor (usado pra cálculo de base hierárquica)
- `COD_RCA` / `RCA` — vendedor (Representante Comercial Autônomo)
- `QTCLI_RCA` — **carteira do RCA** (qtd de clientes cadastrados). **É o denominador da positivação** no nível RCA. Vem do WinThor; mesmo que o RCA não tenha vendido para todos no mês, o número é o total da carteira.

Se algum desses campos for vazio, a linha não aparece em drills que dependam dele (ex: linha sem `COD_RCA` não vai para `agg_*_v04_*`).

### Cliente
- `COD_CLI` — código interno do cliente (WinThor)
- `CLIENTE` — razão social
- `CNPJ` — CNPJ com ou sem máscara (`12.345.678/0001-90` ou `12345678000190`). **CNPJ é a chave de positivação** — clientes com o mesmo CNPJ contam como um só ativo no mês.
- `UF` — sigla do estado
- `EMPRESA` — filial / unidade de negócio (não confundir com a empresa-tenant do sistema)
- **`COD_RAMO` / `RAMO`** — ramo de atividade do cliente (ex: `5 / SUPERMERCADO`, `7 / HORTIFRUTI`). **Apenas visual** — aparece no detalhe do cliente no painel Marketing. Não é usado em agregados nem filtros.

### Produto
- `COD_PROD` — código do produto
- `PRODUTO` — descrição
- `EAN` — código de barras EAN-13 padrão
- **`COD_BAR`** — código de barras alternativo (ex: DUN-14 da caixa). **Apenas visual** — aparece no detalhe do produto. Se igual ao EAN, a UI esconde a redundância.
- **`EMBALAGEM`** — descrição da embalagem (ex: `UN/0001/UN`, `CX/0048/UN`). **Apenas visual**.
- **`QT_UNIT`** — quantidade de unidades por embalagem de venda. **Apenas visual**.
- **`QT_UNIT_CX`** — quantidade de unidades por caixa master. **Apenas visual**.

### Métricas (todas numéricas, agregadas)
- `QT` — quantidade vendida (aceita decimais; ex: `1,5` kg)
- `PVENDA` — valor da venda em R$ (líquido). Decimal pode ser `,` ou `.`
- `PLUCRO` — **percentual** de lucro (ex: `20` = 20%). O sistema converte internamente para valor absoluto: `lucro_R$ = pvenda × plucro / 100`

### Campos puramente visuais (mig 168)
Os 6 campos marcados como **"Apenas visual"** acima (`COD_RAMO`, `RAMO`, `COD_BAR`, `EMBALAGEM`, `QT_UNIT`, `QT_UNIT_CX`) são gravados na tabela base mas **não entram em nenhum `agg_*_mes` nem totalizador**. Aparecem apenas no detalhe do cliente (Ramo) e detalhe do produto (Embalagem, Qt/Un, Qt/Cx, Cód. Barras) no painel Marketing.

Decisão de design: economizar espaço/tempo no upsert dos agregados, já que esses campos não são usados em drill ou filtragem — só como contexto descritivo do item clicado.

## Comportamentos importantes

### Idempotência por dia
Antes de importar, o sistema executa um `DELETE` das linhas de cada **dia** presente no CSV (não do mês inteiro). Logo:
- Pode reimportar o mesmo arquivo várias vezes — não duplica
- Pode importar correções pontuais (ex: só o dia 15 reimportado) sem afetar os outros dias

### Pós-import
Para cada (empresa, ano, mês) tocado pelo arquivo, o sistema chama `farol.upsert_aggs_mes(...)` automaticamente após o `COPY` terminar. Isso popula as 30+ tabelas agregadas em ~8 minutos por mês.

### Tamanho máximo
Não há limite hard — testado com 1.143.844 linhas (~365MB) em um único arquivo. Acima disso, sugerimos dividir em arquivos mensais.

## Mínimo absoluto pra uma linha válida

Se o programador quiser gerar o CSV mais enxuto possível, **estas são as colunas que de fato participam dos cálculos**:

```
DATA;ESTADO;COD_SUPERVISOR;COD_RCA;QTCLI_RCA;COD_FORNEC;COD_CLI;CNPJ;COD_PROD;QT;PVENDA;PLUCRO
```

Os campos `NOME_*` são populados via lookup nas dimensões; se vierem vazios, o painel exibe o código no lugar do nome. Os outros (UF, EMPRESA, EAN) e os **6 campos visuais** (`COD_RAMO`, `RAMO`, `COD_BAR`, `EMBALAGEM`, `QT_UNIT`, `QT_UNIT_CX`) são opcionais — apenas enriquecem o detalhe no painel.
