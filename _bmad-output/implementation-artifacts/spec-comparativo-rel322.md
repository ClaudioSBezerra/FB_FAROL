---
title: 'Comparativo REL 322 (WinThor) x Farol'
type: 'feature'
created: '2026-08-27'
status: 'done'
review_loop_iteration: 0
context: []
baseline_commit: '5e4675127b76a654697e8b7531a100df980d9a99'
---

<!-- Escopo restrito ao núcleo pós-split (2026-08-27): histórico persistido foi
     adiado — ver deferred-work.md, entrada "spec-comparativo-rel322". -->

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** O gestor precisa validar se os números do Farol batem com o relatório oficial do WinThor ("322 — Venda Por Departamento", por supervisor), mas nem o Oracle de origem (BD VM) nem o Postgres de produção do Farol são alcançáveis a partir do ambiente onde a comparação seria feita — só o PDF que o WinThor já exporta está disponível.

**Approach:** Nova aba "Comparativo REL 322" no menu Relatórios. O usuário sobe o PDF; o backend extrai as linhas por supervisor, lê o período do próprio cabeçalho, busca no Farol (tabelas de vendas por dia, não os agregados mensais — o período pode ser parcial) o Bruto e o Líquido para o mesmo `cod_supervisor` no mesmo intervalo, e devolve os três valores lado a lado com status por linha. Processamento em memória, sem persistir — cada upload é independente.

## Boundaries & Constraints

**Always:**
- O período da consulta ao Farol vem da linha "Periodo :" do cabeçalho do PDF (`DD/MM/AAAA a DD/MM/AAAA`), usado como range de datas — nunca mês fechado, pois os PDFs de exemplo já incluem período parcial (ex.: 01/08/2026 a 26/08/2026).
- Farol calcula Bruto = `SUM(pvenda)` de `vendas_faturadas` e Líquido = venda real (`tipo_venda` em 1,4,7,8,9,11,14,20) menos DEVOLVIDO/CANCELADO de `vendas_ccd`, ambos por `cod_supervisor` no range — mesma classificação do painel faturado (spec-venda-liquida-composicao.md).
- Match é por `cod_supervisor` (código do PDF == `cod_supervisor` do Farol).
- Divergência: linha é "OK" se Bruto OU Líquido do Farol estiver a até 0,5% do Vl.Vendido do PDF; senão "divergência".
- Supervisor sem correspondente de um dos lados vira linha órfã, destacada, sem travar o restante do comparativo.
- Autenticação/multiempresa via `GetSpContext(r)`, mesmo padrão dos demais handlers do Farol. Endpoint é leitura (não persiste nada) — permissão `somente_leitura`, mesmo fazendo upload via POST.

**Ask First:**
- Se o parser não reconhecer o layout de alguma linha do PDF (relatório mudou de formato), aborta com erro claro em vez de gerar comparativo parcial silencioso.

**Never:**
- Não tenta conectar direto no Oracle/BD-VM nem no Postgres de produção — o caminho é sempre via upload do PDF.
- Não compara Qt.Cli.Ativos/Qt.Cli.Pos (positivação) nesta versão — fica fora de escopo, é métrica com regra própria (mix).
- Não faz OCR — só extração de texto de PDF gerado digitalmente (o REL 322 exporta texto real).
- Não persiste upload nem resultado — ver deferred-work.md para o histórico consultável.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| PDF válido, tudo bate | um dos 4 PDFs de exemplo | linhas "OK", totais do PDF batendo com a soma do Farol | N/A |
| Supervisor só de um lado | código existe no PDF, sem venda no Farol no período (ou vice-versa) | linha órfã destacada, resto do comparativo segue | N/A |
| Divergência real | Bruto e Líquido do Farol >0,5% distantes do Vl.Vendido do PDF | linha marcada divergência, com % calculado | N/A |
| PDF de layout inesperado | upload de PDF que não é o 322-Por Supervisor | erro claro, nada é processado | HTTP 422 com mensagem |
| Período sem dado no Farol | range futuro ou ainda não importado | Farol = 0 em Bruto/Líquido, tudo divergência (reflete a realidade, não é bug) | N/A |

</frozen-after-approval>

## Code Map

- `backend/handlers/farol_cnpj_receita.go` -- referência de handler multi-formato, leitura de `spCtx`, estilo de comentários do projeto
- `backend/handlers/farol_v2_import.go:52-90` -- padrão de upload multipart (`ParseMultipartForm`/`FormFile`)
- `backend/migrations/158_drop_tipo_base.sql` -- schema `vendas_faturadas` (`data_faturamento`, `cod_supervisor`, `tipo_venda`, `pvenda`) — fonte do Bruto e da venda real
- `backend/migrations/182_tabela_vendas_ccd.sql` -- schema `vendas_ccd` (`data_evento`, `evento`, `cod_supervisor`, `pvenda`) — devolvido/cancelado, usado no Líquido
- `backend/migrations/187_tipo_venda_faturado.sql` -- códigos de `tipo_venda`: venda real 1,4,7,8,9,11,14,20 vs bonif/transf/remessa 5,10,13
- `backend/main.go:560-566` -- bloco de rotas `/api/v2/farol/relatorio/*` (adicionar a nova aqui)
- `backend/go.mod` -- adicionar `github.com/ledongthuc/pdf` (extração de texto; `pdfcpu`, já indireto, não tem extração de texto limpa)
- `frontend/src/pages/farol/FarolRelatorios.tsx:78-182` -- padrão de abas (array `aba` + render condicional) para acrescentar 'comparativo'
- `frontend/src/pages/farol/FarolRelatorioReceita.tsx` -- página irmã de referência de estilo/estrutura

## Tasks & Acceptance

**Execution:**
- [x] `backend/go.mod` -- `go get github.com/ledongthuc/pdf` -- lib de extração de texto
- [x] `backend/handlers/farol_comparativo_rel322.go` -- parser (regex por linha: código + descrição + 11 números; extrai "Periodo :"); query Bruto/Líquido por `cod_supervisor` no range; monta o comparativo em memória; handler `ComparativoRel322Handler` (POST multipart, `somente_leitura`) -- núcleo da feature
- [x] `backend/main.go` -- registrar `/api/v2/farol/relatorio/comparativo-rel322` (POST) -- expõe o handler
- [x] `frontend/src/pages/farol/FarolComparativoRel322.tsx` -- tela de upload + tabela (código, supervisor, Vl.Vendido PDF, Bruto Farol, Líquido Farol, % diferença, status OK/divergência/órfã) -- UI nova
- [x] `frontend/src/pages/farol/FarolRelatorios.tsx` -- acrescentar aba 'Comparativo REL 322' -- integra ao menu Relatórios
- [x] Teste unitário do parser (linhas normais, linha "Total Geral", múltiplas páginas com cabeçalho repetido, descrição de supervisor com hífen) cobrindo a I/O Matrix

**Acceptance Criteria:**
- Given um PDF REL 322 válido com supervisores existentes no Farol no mesmo período, when o usuário faz upload, then a resposta mostra Vl.Vendido do PDF, Bruto e Líquido do Farol e o status OK/divergência por linha (tolerância 0,5%).
- Given um supervisor presente só de um lado, when o comparativo é montado, then a linha aparece órfã e destacada, sem interromper as demais.
- Given um PDF que não é o layout 322-Por Supervisor, when o usuário faz upload, then a API responde 422 com mensagem clara e nada é processado.

## Design Notes

Parser: como a descrição do supervisor tem espaços e hífens ("GO - VALE SAO PATRICIO - LUCAS"), não dá para dividir a linha por espaço. Extrair de trás para frente: os últimos 11 tokens numéricos (Qt.Cli.Ativos, Qt.Cli.Pos., %Pos., Qt.Peso, Qt.Meta, Vl.Meta, %, Qt.Vendida, Vl.Vendido, %, Volume) via regex `(?:[\d.,]+\s+){10}[\d.,]+$`; o que sobra entre o código inicial e essa cauda é a descrição. A linha "N Supervisores Listados — Total Geral:" tem formato diferente e vira o total de conferência, não uma linha de supervisor.

Por que Bruto E Líquido, não só um: o "Vl.Vendido" do WinThor é o relatório nativo do ERP — não se sabe de antemão se ele já exclui bonificação/transferência/remessa como o Líquido do Farol faz. Devolver os dois deixa o usuário confirmar contra o valor real do PDF, sem eu supor.

## Verification

**Commands:**
- `cd backend && go build ./...` -- compila sem erro
- `cd backend && go test ./handlers/... -run ComparativoRel322` -- parser cobre a I/O Matrix

**Manual checks:**
- Upload dos 4 PDFs de exemplo em `/home/claudio/uploads/*.pdf` e conferir que a soma das linhas do comparativo bate com a linha "Total Geral" de cada PDF.

## Suggested Review Order

**Parser do REL 322 (a parte nova e mais arriscada)**

- Entrada: como o texto vem em token-por-linha (não linha visual) e por que o primeiro dos 11 números é sempre o Vl.Vendido — evidência empírica documentada aqui.
  [`farol_comparativo_rel322.go:14`](../../backend/handlers/farol_comparativo_rel322.go#L14)

- `parseRel322Texto` — separa cabeçalho/dado por regex, extrai o período, e agora rejeita código de supervisor duplicado (patch da revisão).
  [`farol_comparativo_rel322.go:149`](../../backend/handlers/farol_comparativo_rel322.go#L149)

- `extrairTextoPDF` — envolve a lib de terceiros com `recover()`: PDF corrompido vira erro 422, não panic (patch da revisão).
  [`farol_comparativo_rel322.go:105`](../../backend/handlers/farol_comparativo_rel322.go#L105)

**Recorte por persona e cruzamento com o Farol (segurança — achado mais sério da revisão)**

- `escopoCondRel322` — aplica a mesma regra de `farol_escopo.go` que a rota irmã já usa; falha fechado (`1=0`) se chamada indevidamente com `Negar`.
  [`farol_comparativo_rel322.go:311`](../../backend/handlers/farol_comparativo_rel322.go#L311)

- `farolBrutoLiquidoPorSupervisor` — Bruto/Líquido por supervisor já recortados pelo escopo, direto das tabelas diárias (não os agregados mensais, por causa de período parcial).
  [`farol_comparativo_rel322.go:332`](../../backend/handlers/farol_comparativo_rel322.go#L332)

- `cruzarComRel322` — a regra de status (tolerância 0,5%, órfãos, período sem dado); `DiferencaPct` vira `nil` em vez de `+Inf` para não quebrar a serialização JSON (patch da revisão).
  [`farol_comparativo_rel322.go:390`](../../backend/handlers/farol_comparativo_rel322.go#L390)

**Handler HTTP**

- `ComparativoRel322Handler` — bloqueia por escopo ANTES de gastar trabalho com o upload; `MaxBytesReader` + limpeza do multipart temporário (patches da revisão).
  [`farol_comparativo_rel322.go:529`](../../backend/handlers/farol_comparativo_rel322.go#L529)

- Rota registrada como `somente_leitura` — é POST só pelo transporte do arquivo, não por escrever dado.
  [`main.go:571`](../../backend/main.go#L571)

**Frontend — tela de upload e tabela**

- Componente principal: upload (com validação de tipo no drag-and-drop, patch da revisão) + tabela com selo de status por linha.
  [`FarolComparativoRel322.tsx:68`](../../frontend/src/pages/farol/FarolComparativoRel322.tsx#L68)

- Nova aba integrada ao menu Relatórios existente, mesmo padrão da aba irmã "Clientes com CNPJ irregular".
  [`FarolRelatorios.tsx:168`](../../frontend/src/pages/farol/FarolRelatorios.tsx#L168)

**Testes**

- Suíte do parser + cruzamento + escopo, incluindo o teste de integração `TestComparativoRel322_MontarComparativoRespeitaEscopoSupervisor` (roda contra Postgres real quando `DATABASE_URL` está setada).
  [`farol_comparativo_rel322_test.go:1`](../../backend/handlers/farol_comparativo_rel322_test.go#L1)

- Nova dependência de extração de texto de PDF (pdfcpu, já no projeto, não faz isso).
  [`go.mod:11`](../../backend/go.mod#L11)
