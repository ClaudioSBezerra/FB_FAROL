# Operação da carga JC — guia de retomada

Escrito em 07/08/2026, antes de formatar a máquina de desenvolvimento. Serve
para retomar a operação sem precisar redescobrir nada.

---

## 1. O desenho em uma tela

Nós **puxamos** dados do Oracle da Jorge Costa. Não há agente rodando lá, não há
VPN, e não há POST de entrada — quem conecta somos nós, e a rede foi liberada por
allowlist do IP do nosso servidor.

```
Oracle JC (26ai)  ──SELECT 40 colunas──►  CSV .gz em disco
   IAUSER.COMPRAS_FAROL_VW                      │
   (sinônimo → IAADMIN.*)                       ▼
                                        processImportJob
                                    (mesmo caminho do upload manual;
                                     apaga as datas antes do COPY)
                                                │
                                                ▼
                              vendas_faturadas / _transmitidas / _ccd
                                                │
                                                ▼
                            farol.upsert_aggs_mes  → V01..V05 + dims
                            farol.upsert_aggs_mes_v06 / _v07
                            farol.upsert_aggs_mes_v08_v09   (grão UF)
                            farol.upsert_aggs_mes_v10_v11   (grão FILIAL)
                            farol.upsert_aggs_mes_v11_l5    (FILIAL × CNPJ)
                            farol.upsert_tipo_venda_dims
                            farol.upsert_venda_liquida_cols  ← SEMPRE por último
                                                │
                                                ▼
                                     e-mail de resumo
```

**Ordem importa:** `upsert_venda_liquida_cols` roda depois de todas as outras,
porque preenche `liquido`/`pv_*` em cima das linhas que elas criaram.

Os painéis **não leem** das tabelas cruas. Leem dos ~50 agregados em `farol`.

## 2. O que roda sozinho

| Hora (Brasília) | O quê |
|---|---|
| 04:30 | carga JC de D-1 (`StartCargaJCDiaria`) |
| 05:30 | prewarm do cache (`StartDailyPrewarm`) |
| domingo 06:00 | reextração de janela móvel — **desligada até `JC_REEXTRACAO_MESES` existir** |

A carga tem que terminar **antes** do prewarm, senão invalida o cache
recém-aquecido.

### Variáveis de ambiente (Coolify)

```
JC_ORACLE_HOST      201.48.119.197      (o de menor jitter dos dois liberados)
JC_ORACLE_PORT      1521
JC_ORACLE_SERVICE   cdb1                (é o CDB root; o PDB1 existe mas não tem o usuário)
JC_ORACLE_USER      IAUSER              MAIÚSCULO
JC_ORACLE_PASS      IAUSER              MAIÚSCULO
JC_ORACLE_OBJETO    IAUSER.COMPRAS_FAROL_VW
JC_EMPRESA_ID       <uuid da empresa no FAROL>
JC_EXTRACAO_HORA    04:30
JC_EXTRACAO_EMAILS  claudiosousadebezerra@gmail.com,keslley.paula@jcdistribuicao.com.br
JC_REEXTRACAO_MESES (vazio = desligado; 3 é o valor sugerido)
JC_REEXTRACAO_DIA   0=domingo (default)
JC_REEXTRACAO_HORA  06:00 (default)
```

Usuário do sistema para disparar carga:
`importacao.dados@jcdistribuicao.com.br` / `123456` — **senha fraca, trocar.**

## 3. Carga manual

```bash
TOKEN=$(curl -s -X POST http://localhost:8087/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"importacao.dados@jcdistribuicao.com.br","password":"123456"}' \
  | grep -o '"token":"[^"]*"' | cut -d'"' -f4)
[ -z "$TOKEN" ] && { echo "LOGIN FALHOU — abortando"; exit 1; }
```

O guard do token vazio não é firula: foi ele que faltou em 29/07 e fez uma
rodada inteira girar em falso.

```bash
# um dia
curl -X POST "…/api/v2/jc/carga?data=2026-07-31" -H "Authorization: Bearer $TOKEN"

# intervalo por dia (teto 92 dias)
curl -X POST "…/api/v2/jc/carga?de=2026-07-31&ate=2026-08-05&passo=dia" -H …

# intervalo por MÊS — para recarga histórica (teto 24 meses)
curl -X POST "…/api/v2/jc/carga?de=2025-01-01&ate=2026-07-31&passo=mes" -H …
```

⚠ A resposta diz `"estimativa": "~190 min"` para 19 meses. **Está errada** — ela
assume 10 min/mês, chute de julho que a medição desmentiu. O real é **~40
min/mês, perto de 13 horas**. Constante não corrigida no código.

## 4. Recarga completa do zero

Roteiro versionado: **`scripts/reset_base_completo.sh`**. Use o script; ele tem
as quatro coisas que já deram errado embutidas.

Sequência:

1. Garantir que o código desejado está **deployado** (ex.: a remoção do dedup
   precisava estar no ar ANTES da recarga, senão a recarga deduplicaria)
2. `bash scripts/reset_base_completo.sh` — pede `APAGAR TUDO`
3. Disparar a carga com `passo=mes`
4. Conferir na primeira fatia: tem que aparecer `[import] sem dedup`

### Sondar o Oracle sem passar pelo sistema

O binário `/tmp/jc-probe` aceita SQL arbitrário read-only:

```bash
JC_USER=IAUSER JC_PASS=IAUSER JC_SQL="
SELECT TO_CHAR(DATA,'YYYY-MM') mes, ESTADO, COUNT(*) linhas
  FROM IAUSER.COMPRAS_FAROL_VW
 WHERE DATA >= DATE '2026-01-01'
 GROUP BY TO_CHAR(DATA,'YYYY-MM'), ESTADO ORDER BY 1,2" /tmp/jc-probe
```

Fonte em `backend/cmd/jc-probe/`. Compilar com `go build ./cmd/jc-probe`.
**Usa go-ora v2** — a v3 não negocia o protocolo TCP contra o Oracle 26ai deles.

## 5. As armadilhas — todas já custaram tempo

**Nunca dar `git push` com carga rodando.** O redeploy do Coolify recria os
containers e mata o processo. Aconteceu em 05/08 e apagou o log do backfill.

**O Coolify usa `docker-compose.yml`, não `.prod.yml`.** Fix no arquivo errado
vira commit decorativo.

**`consolidacao_pendente` e `consolidacao_log` estão no schema `farol`**, não em
`public`. Sem o prefixo o psql erra; **sem `-v ON_ERROR_STOP=1` ele segue e o
`COMMIT` vira ROLLBACK silencioso**, com a saída ainda mostrando zeros que
parecem sucesso.

**Os gates `aggUFReady`/`aggFilialReady` guardam o resultado positivo para
sempre** (`if val { return true }`). Depois de um TRUNCATE eles continuam
abertos e os filtros de UF e filial roteiam para tabelas VAZIAS → cards zerados,
**sem erro nenhum no log**. O `docker restart` da API não é opcional.

**`upsert_aggs_mes` é UPSERT, não rebuild.** Não apaga linha agregada cuja
origem sumiu. Por isso o reset trunca os agregados explicitamente, e os descobre
pelo **catálogo** — lista escrita à mão esquece alguma (já são V01 a V11).

**O CSV usa `;`, não `,`.** Com vírgula, cada linha vira um campo só, o
cabeçalho não casa e as linhas são rejeitadas com "nenhuma linha válida" — falha
silenciosa que parece extração perfeita nos logs.

**`Format("02/01/2026")` em Go produz lixo.** O ano é `2006`. Já gerou
`15/07/15156`.

**`ss -tnp` no host não enxerga o namespace de rede do container.** Use
`nsenter -t <PID> -n ss -tnp`.

**Os IDs de container mudam a cada redeploy.** Refazer `source /root/farol-env.sh`.

## 6. Verificações úteis

```bash
source /root/farol-env.sh

# meses carregados
docker exec -i "$DB" psql -U postgres -d fb_farol -c \
"SELECT to_char(data_faturamento,'YYYY-MM') mes, COUNT(*) FROM vendas_faturadas
  WHERE empresa_id='$EMP' GROUP BY 1 ORDER BY 1;"

# agregados populados (todas as famílias)
docker exec -i "$DB" psql -U postgres -d fb_farol -c \
"SELECT (SELECT COUNT(*) FROM farol.agg_fat_v01_l0_mes) v01,
        (SELECT COUNT(*) FROM farol.agg_fat_v08_l0_mes) uf,
        (SELECT COUNT(*) FROM farol.agg_fat_v10_l0_mes) filial,
        (SELECT COUNT(*) FROM farol.agg_fat_v11_l5_mes) filial_cnpj;"
```

### Conferir valor contra a origem

O confronto que fecha (ALPARGATAS, `CODFORNEC=19263`, exclui transferência):

```sql
-- no Oracle
SELECT ESTADO, COUNT(*), ROUND(SUM(PVENDA_TOTAL),2)
  FROM IAUSER.COMPRAS_FAROL_VW
 WHERE DATA >= DATE '2025-01-01' AND DATA < DATE '2026-01-01'
   AND CODFORNEC = 19263 AND CONDVENDA <> 10
 GROUP BY ESTADO;
```

```sql
-- no Postgres
SELECT COUNT(*), ROUND(SUM(pvenda)::numeric,2) FROM vendas_faturadas
 WHERE empresa_id='…' AND cod_fornec='19263'
   AND data_faturamento BETWEEN DATE '2025-01-01' AND DATE '2025-12-31'
   AND tipo_venda <> '10';
```

Em 07/08 a origem passou a bater **ao centavo** com a Rotina 1464 do WinThor:
**R$ 137.842.047,45**.

## 7. Estado em 07/08/2026

**Feito**

- Dedup **removido** (`45ddec9`). A JC corrigiu a duplicação na própria view; o
  que sobrava era venda legítima que o relatório 1464 conta. Não há `NUMNOTA` no
  layout e a JC não pode acrescentá-lo.
- Filtro de **Filial** (a coluna `empresa`) restaurado e servido por agg —
  migrations 199 e 200. Antes caía no scan e, pior, trocava o significado de dois
  números: o valor virava bruto e o denominador virava rolling-12M.
- Reextração de janela móvel implementada, **desligada** por padrão.
- `random_page_cost=1.1` + `effective_io_concurrency=200` (o host é SSD; o
  default 4,0 descreve HD com prato girando e fazia o planejador preterir índice).
- Recarga completa disparada 07/08 10:10, 19 fatias, 2025-01 a 2026-07.

**Aberto**

- **Positivação tem TRÊS denominadores vivos** e o exibido **cresce com o
  histórico carregado** ("quem já comprou alguma vez"). Carregar mais meses faz a
  positivação cair sozinha, sem nada ter piorado. Precisa de decisão do gestor.
  Ver `farol_v2_api.go` em `queryBasePositivados` e o comentário de `fetchCards`.
- **Bonificação, transferência e remessa positivam o cliente** — `_v_fat` não
  carrega `tipo_venda`. O card pode dizer "positivou" exibindo R$ 0,00.
- **`qtcli_rca` é o máximo histórico**, não a carteira atual (a MV não filtra
  data). E **RCA que não vendeu nada some do denominador**, inflando o número do
  supervisor.
- **Transmitido virou bruto na origem** em 07/08 (`PVENDA_TOTAL` deixou de ser
  líquido de desconto). A apresentação deve virar `transmitido_bruto − cortado`
  quando a origem passar a mandar o estado CORTADO — o importador já o reconhece
  e roteia para `vendas_ccd`.
- **Agosto/2026 ainda não liberado** na origem. Até liberar, a carga diária manda
  e-mail `SEM DADOS` (não é FALHOU — o código já distingue).
- Senha `123456` do usuário de importação.

## 8. Ambiente novo (Windows)

Depois do format, para voltar a trabalhar:

```
git clone https://github.com/ClaudioSBezerra/FB_FAROL.git
```

Repositórios do grupo, todos em `github.com/ClaudioSBezerra`:
`FB_FAROL`, `FB_SMARTPICK`, `FB_APU01`, `FB_CONTROLADORIA`, `FB_EVENTOS`,
`FB_FBTAX_CLOUD`, `FB_FBTECHIA`, `viagem` (pasta local `FB_VIAGEM`).

O que **não** está no git e precisa vir do backup (`.tar.gz` no HD externo ou o
OneDrive): `~/.claude` (memória e histórico das sessões), `~/.config`, `~/.ssh`,
`~/.bmad`, e as pastas pessoais.

A operação em si não depende da máquina local — tudo roda no servidor Hostinger
via SSH. O notebook só edita código e dá push.
