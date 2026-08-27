---
title: 'Comparativo REL 322 x Farol — seleção de fluxo (Faturado x Transmitido)'
type: 'feature'
created: '2026-08-27'
status: 'done'
review_loop_iteration: 1
context: []
baseline_commit: '1ecb0904be03b8866286001b16cc073962290323'
---

<!-- Escopo restrito ao núcleo pós-split (2026-08-27): PDF refletir o fluxo
     escolhido foi adiado — ver deferred-work.md, entrada
     "spec-comparativo-rel322-fluxo". -->

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** O Heverton pode gerar o REL 322 no WinThor sobre Faturado OU sobre Transmitido — o PDF não se autodeclara qual base foi usada (confirmado nos 4 PDFs de exemplo). O comparativo hoje (em produção) sempre compara contra Faturado; se o PDF enviado for de Transmitido, o resultado é enganoso sem o usuário saber.

**Approach:** Perguntar ao usuário, no upload, qual base o PDF representa (toggle Faturado/Transmitido, default Faturado — comportamento atual não muda pra quem não mexer). Usa `resolveFluxo` (já existente, usado por outras telas) pra apontar a query pra `vendas_faturadas` ou `vendas_transmitidas`. Faturado mantém Bruto e Líquido (venda real menos CANCELADO/DEVOLVIDO de `vendas_ccd`, como já era). Transmitido mostra **só Bruto** — mesma decisão já tomada no resto do Farol (o painel Transmitido não tem conceito de Líquido, sempre exibiu só o bruto); não inventa um Líquido novo por CORTADO.

## Boundaries & Constraints

**Always:**
- Usa `resolveFluxo` (não reimplementar) pra tabela/coluna principal: `resolveFluxo(fluxoParam)`.
- O parâmetro de fluxo aceito do usuário é estritamente `"transmitido"`/`"trans"` (case-insensitive) ou qualquer outra coisa → `faturado`. NUNCA repassar o valor bruto da querystring pra `resolveFluxo` sem essa normalização antes — `resolveFluxo` também aceita `"cancdev"`/`"cortado"` (uso interno, CCD) e um cliente mandando isso na API não pode fazer a query principal ler de `vendas_ccd` como se fosse a base comparada.
- Só Faturado calcula Líquido (venda real menos CANCELADO/DEVOLVIDO de `vendas_ccd`, comportamento inalterado). Transmitido só tem Bruto — `LiquidoFarol` vem `nil` nesse fluxo, igual a uma linha órfã (a tela já trata ponteiro nil como "—").
- O recorte por persona (`escopoCondRel322`/`escopoDoUsuario`) continua se aplicando à query em QUALQUER fluxo — precisa de teste que insere seus PRÓPRIOS dados sintéticos com escopo restrito (não pode depender de já existir 2 supervisores na base local, que hoje está vazia e faz o teste pular sem rodar de verdade).
- A tela (JSON) deixa explícito qual fluxo foi comparado. Trocar o toggle depois de já ter um resultado na tela limpa o resultado anterior (mesmo comportamento de trocar o arquivo) — nunca deixar o toggle mostrar um fluxo e a tabela/PDF baixado refletir outro.
- O toggle é enviado tanto em "Comparar" quanto em "Baixar PDF" (o PDF em si ainda não rotula o fluxo — ver Never).

**Never:**
- Não tenta adivinhar o fluxo a partir do conteúdo do PDF — é sempre escolha explícita do usuário.
- Não muda o comportamento de quem usa o fluxo padrão (Faturado) sem trocar o toggle.
- Não altera `comparativoRel322PDF` nesta spec — o PDF exportado continua funcionando (com os números corretos do fluxo escolhido), só ainda não indica visualmente qual fluxo é.
- Não calcula Líquido pro Transmitido — decisão confirmada com o usuário em 27/08/2026, pra não divergir do painel Transmitido que já existe.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Fluxo Faturado (padrão) | upload sem trocar o toggle | mesmo resultado de hoje, com Líquido | N/A |
| Fluxo Transmitido | upload + toggle em Transmitido | só Bruto de `vendas_transmitidas`; Líquido nil/"—"; rótulo claro na tela | N/A |
| Escopo por persona em Transmitido | usuário supervisor + fluxo Transmitido, dado sintético inserido pelo próprio teste | mesmo recorte que já existe em Faturado | N/A |
| `?fluxo=cancdev` ou `?fluxo=cortado` (valores internos, não documentados) | request direto na API com esses valores | tratado como valor não reconhecido → cai pro Faturado padrão | N/A |

</frozen-after-approval>

## Code Map

- `backend/handlers/farol_v2_api.go:196-222` (`fluxoCtx`, `resolveFluxo`) -- reaproveitar direto
- `backend/handlers/farol_comparativo_rel322.go:343-385` (`farolBrutoLiquidoPorSupervisor`) -- hardcoded hoje pra `vendas_faturadas`/`vendas_ccd`/`DEVOLVIDO,CANCELADO`; precisa `fluxo fluxoCtx`, usando `fluxo.tableName`/`fluxo.dateCol` (CTE `fat`) + `eventoFilter` do negativo correspondente (CTE `ccd`, alias `v` novo pra bater com o formato de `eventoFilter`)
- `backend/handlers/farol_comparativo_rel322.go:89-114` (`comparativoRel322Resposta`) -- acrescentar campo do fluxo usado
- `backend/handlers/farol_comparativo_rel322.go:540-643` (`ComparativoRel322Handler`) -- ler `?fluxo=` (padrão de `?formato=`), repassar
- `frontend/src/pages/farol/FarolPublicPanel.tsx:425-445` -- padrão visual EXATO do toggle a copiar
- `frontend/src/pages/farol/FarolComparativoRel322.tsx` -- toggle, estado `fluxo`, enviar em `enviar()` e `baixarPDF()`

## Tasks & Acceptance

**Execution:**
- [x] `backend/handlers/farol_comparativo_rel322.go` -- `farolBrutoLiquidoPorSupervisor` recebe `fluxo fluxoCtx` e despacha: Transmitido (`brutoPorSupervisorTransmitido`) lê só `vendas_transmitidas`, sem CTE `ccd`/venda_real; Faturado (`brutoLiquidoPorSupervisorFaturado`) mantém Bruto+Líquido via `vendas_ccd`/CANCELADO,DEVOLVIDO (`eventoFilter` de `resolveFluxo("cancdev")`, alias `v`) -- núcleo da troca de fonte, sem inventar Líquido por CORTADO
- [x] `backend/handlers/farol_comparativo_rel322.go` -- `comparativoRel322Resposta` ganha campo `Fluxo string` (`"faturado"`/`"transmitido"`); `montarComparativo` propaga; `cruzarComRel322` ganha `comLiquido bool` (`fluxo.name != "transmitido"`) -- `LiquidoFarol` vem `nil` em toda linha e a tolerância de 0,5% considera só o Bruto quando Transmitido
- [x] `backend/handlers/farol_comparativo_rel322.go` -- `ComparativoRel322Handler` lê `?fluxo=` via `normalizarFluxoParam` (só aceita `"transmitido"`/`"trans"`, qualquer outra coisa vira `"faturado"` -- inclui `"cancdev"`/`"cortado"`, valores internos do `resolveFluxo`) antes de chamar `resolveFluxo` -- expõe o parâmetro sem deixar a query principal ler de `vendas_ccd`
- [x] `frontend/src/pages/farol/FarolComparativoRel322.tsx` -- toggle Faturado/Transmitido (padrão visual de `FarolPublicPanel.tsx`), default Faturado, enviado em `enviar()` e `baixarPDF()`; trocar o toggle com um resultado na tela limpa esse resultado; "Total Líquido"/rodapé mostram "—"/texto próprio quando Transmitido -- UI
- [x] Testes: fluxo Transmitido só-Bruto com `LiquidoFarol` nil (dado sintético em `vendas_transmitidas` + `vendas_faturadas` + `vendas_ccd` propositalmente divergentes, provando isolamento); regressão Faturado inalterado; `normalizarFluxoParam` (pura) e handler real rejeitando `?fluxo=cancdev`/`cortado`; escopo por persona reescrito para inserir os próprios `cod_supervisor` sintéticos (não depende mais da base local, que está vazia, e por isso sempre pulava)

**Acceptance Criteria:**
- Given um PDF do REL 322 e o toggle em Faturado (padrão), when o usuário compara, then o resultado é idêntico ao comportamento já em produção (Bruto + Líquido via `vendas_ccd`/CANCELADO,DEVOLVIDO).
- Given o toggle em Transmitido, when o usuário compara, then Bruto vem de `vendas_transmitidas`; Líquido vem `nil`/"—" (não calculado), com rótulo claro na tela.
- Given um usuário com escopo restrito por persona, when compara em qualquer fluxo, then só vê os supervisores do próprio escopo.
- Given um request com `?fluxo=cancdev` ou `?fluxo=cortado` (valores internos do CCD), when o comparativo roda, then o fluxo cai no default faturado — nunca lê `vendas_ccd` como base principal.

## Spec Change Log

- **2026-08-27, revisão adversarial (3 revisores) da primeira implementação.** Achados que motivaram mudança no `Intent`/`Boundaries` (fora do que já estava congelado):
  1. **Verification Gap** apontou que o resto do Farol trata Transmitido como só Bruto (`farol_v2_api.go`, painel Transmitido: "sem líquido, exibe bruto") — o Líquido-via-CORTADO que a primeira versão desta spec pedia era uma invenção nova, divergente do resto do produto. Confirmado com o usuário: Transmitido não calcula Líquido. `Approach`, `Boundaries` e a I/O Matrix foram reescritos para refletir isso.
  2. Essa mesma decisão fechou de graça um bug real que o **Edge Case Hunter** e o **Verification Gap** encontraram independentemente: dado antigo de Transmitido sem `tipo_venda` classificado (anterior à migration 203) fazia o Líquido-via-CORTADO ficar NEGATIVO (o filtro de venda real não casava nada, então `venda_real=0` e `Líquido = 0 − CORTADO`). Sem Líquido no Transmitido, esse caso não existe mais.
  3. **Edge Case Hunter** demonstrou, testando contra a API real, que `?fluxo=cancdev` e `?fluxo=cortado` (valores internos do `resolveFluxo`, nunca documentados como fluxo válido) eram aceitos verbatim pelo handler e faziam a query principal ler de `vendas_ccd` como se fosse a base comparada. Virou uma regra explícita em `Boundaries`.
  4. **Verification Gap** também mostrou que o teste de escopo por persona (`TestComparativoRel322_MontarComparativoRespeitaEscopoSupervisor`) depende de já existirem 2 `cod_supervisor` na base local — que está vazia — e por isso sempre pulou, nunca validando de verdade o recorte por persona no fluxo novo. Virou requisito explícito em `Boundaries`: o teste precisa inserir seus próprios dados.
- **KEEP** (validado, sobrevive à re-derivação): uso de `resolveFluxo` pra tabela/coluna do fluxo principal; alias `v` na CTE `ccd`; toggle visual copiado de `FarolPublicPanel.tsx`; envio do fluxo em `enviar()` e `baixarPDF()`; campo `Fluxo` na resposta.

## Design Notes

`eventoFilter` de `resolveFluxo` (ex.: `"AND v.evento IN (...)"`) assume tabela aliada `v` — a CTE `ccd` atual não tem alias. Trocar pra `FROM vendas_ccd v` deixa `eventoFilter` reaproveitável verbatim. Isso só se aplica ao fluxo Faturado agora (Transmitido não tem CTE `ccd`/Líquido).

## Verification

**Commands:**
- `cd backend && go build ./...` -- compila sem erro
- `cd backend && go test ./handlers/... -run ComparativoRel322 -v` -- inclui os testes novos de fluxo Transmitido
- `cd frontend && npx tsc --noEmit` -- sem erro

**Manual checks:**
- Upload de um PDF de exemplo com o toggle em Transmitido e conferir que a query bate contra `vendas_transmitidas` (dado sintético local, já que a base local está vazia).

## Suggested Review Order

**Normalização do parâmetro — o bug mais sério da revisão**

- `normalizarFluxoParam` — só `"transmitido"`/`"trans"` viram Transmitido; `"cancdev"`/`"cortado"` (uso interno do `resolveFluxo`) e qualquer outra coisa caem pro Faturado. Sem isso, `?fluxo=cancdev` fazia a query principal ler de `vendas_ccd` como se fosse a base comparada — confirmado rodando contra a API real.
  [`farol_comparativo_rel322.go:628`](../../backend/handlers/farol_comparativo_rel322.go#L628)

**Bruto/Líquido por fluxo**

- `farolBrutoLiquidoPorSupervisor` — despacha pra `brutoPorSupervisorTransmitido` (só Bruto) ou `brutoLiquidoPorSupervisorFaturado` (Bruto+Líquido, como sempre foi); decisão confirmada com o usuário depois do Verification Gap apontar que o resto do Farol trata Transmitido como só Bruto.
  [`farol_comparativo_rel322.go:356`](../../backend/handlers/farol_comparativo_rel322.go#L356)

- `brutoPorSupervisorTransmitido` — sem CTE `ccd`, sem cálculo de venda real; elimina de graça o bug do Líquido negativo em dado antigo sem `tipo_venda` classificado.
  [`farol_comparativo_rel322.go:367`](../../backend/handlers/farol_comparativo_rel322.go#L367)

- `cruzarComRel322` — novo parâmetro `comLiquido`; quando falso, `LiquidoFarol` fica `nil` em toda linha e a tolerância de 0,5% considera só o Bruto.
  [`farol_comparativo_rel322.go:475`](../../backend/handlers/farol_comparativo_rel322.go#L475)

**Frontend**

- `trocarFluxo` — limpa o resultado anterior ao trocar o toggle, pra tela/PDF nunca mostrarem um fluxo diferente do que está selecionado.
  [`FarolComparativoRel322.tsx:176`](../../frontend/src/pages/farol/FarolComparativoRel322.tsx#L176)

**Testes — o que a revisão forçou a existir de verdade**

- `TestComparativoRel322_MontarComparativoRespeitaEscopoSupervisor` — reescrito pra inserir os próprios dados; antes dependia de dado pré-existente na base local (vazia) e sempre pulava, nunca validando o recorte por persona de verdade.
  [`farol_comparativo_rel322_test.go`](../../backend/handlers/farol_comparativo_rel322_test.go)

- `TestComparativoRel322_Handler_FluxoInterno_NaoAceito` / `TestComparativoRel322_NormalizarFluxoParam` — pinam o fix do item 1.
  [`farol_comparativo_rel322_test.go:909`](../../backend/handlers/farol_comparativo_rel322_test.go#L909)
