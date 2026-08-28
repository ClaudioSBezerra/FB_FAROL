---
title: 'Comparativo REL 322 x Farol — Líquido no fluxo Transmitido'
type: 'feature'
created: '2026-08-27'
status: 'done'
review_loop_iteration: 0
context: []
baseline_commit: '42225803dedab7e2cc17e6dedaa44672ac7ff0de'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** `spec-comparativo-rel322-fluxo.md` decidiu que o fluxo Transmitido do comparativo REL 322 só mostra Bruto — Líquido vem sempre `nil`/"—". O Claudio (dono da intenção) renegociou essa decisão em 27/08/2026: quer o Líquido também no Transmitido.

**Approach:** Líquido do Transmitido = Bruto (`vendas_transmitidas`) − Cortado (`vendas_ccd`, evento `CORTADO`, via `resolveFluxo("cortado")`) — reaproveita a mesma ESTRUTURA de CTE/join que o Faturado já usa (CTE do fluxo principal + CTE `ccd` do evento negativo, `FULL OUTER JOIN` entre as duas), mas não a mesma BASE da subtração: o Faturado subtrai CANCELADO/DEVOLVIDO de `venda_real` (o subconjunto de Bruto filtrado por `tipo_venda`), enquanto o Transmitido subtrai Cortado direto de Bruto — sem filtro de `tipo_venda` (Transmitido nunca teve esse conceito; Bruto já é a soma incondicional). Isso também fecha, sem precisar de tratamento extra, a preocupação original que motivou a rejeição (dado antigo sem `tipo_venda` classificado zerando `venda_real` e deixando o Líquido negativo): como o novo Líquido não depende de `tipo_venda`, esse mecanismo específico não se aplica mais. `comLiquido` deixa de existir como parâmetro — os dois fluxos sempre calculam Líquido.

## Boundaries & Constraints

**Always:**
- Líquido Transmitido = `Bruto − Cortado`, sem filtro de `tipo_venda`. Cortado vem de `vendas_ccd` com `evento = 'CORTADO'` via `resolveFluxo("cortado").eventoFilter`, mesma CTE-pattern (`FROM vendas_ccd v`) que o Faturado já usa para o evento negativo dele.
- O recorte por persona (`escopoCondRel322`) se aplica às duas CTEs (bruto e cortado), igual ao Faturado.
- Líquido pode vir **negativo** (Cortado > Bruto) sem clamping nem erro — mesmo comportamento não-protegido que o Faturado já tem hoje para o próprio Líquido. Não é regressão nova, é paridade.
- `cruzarComRel322` perde o parâmetro `comLiquido` (sempre trata Líquido como presente nos dois fluxos) — remover, não deixar morto.
- Front e PDF já tratam `LiquidoFarol` genericamente (nil vira "—"); como o backend passa a sempre popular o campo para os dois fluxos, front/PDF só precisam parar de tratar Transmitido como caso especial — não precisam de lógica nova de exibição.

**Never:**
- Não filtra Bruto do Transmitido por `tipo_venda` — continua sendo `SUM(pvenda)` incondicional, como já é hoje.
- Não reintroduz o parâmetro `comLiquido` nem qualquer outro jeito de "desligar" Líquido por fluxo.
- Não muda o fluxo Faturado (Bruto/Líquido dele ficam exatamente como estão).

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Transmitido normal | Bruto=1200, Cortado=777 (dado sintético) | LiquidoFarol = 423 | N/A |
| Cortado maior que Bruto | Bruto=500, Cortado=800 | LiquidoFarol = -300 (negativo, sem clamp, sem erro) | N/A |
| Escopo por persona no Transmitido | supervisor restrito + dado sintético fora do escopo | Cortado/Bruto de fora do escopo não entram no Líquido | N/A |
| Faturado (regressão) | mesmo fixture de sempre | Bruto/Líquido idênticos a antes, comportamento inalterado | N/A |

</frozen-after-approval>

## Code Map

- `backend/handlers/farol_comparativo_rel322.go:338-352` (comentário de doc de `farolBrutoLiquidoPorSupervisor`) -- descreve a decisão antiga ("Transmitido... devolve SÓ Bruto"); reescrever para refletir Bruto−Cortado
- `backend/handlers/farol_comparativo_rel322.go:356-394` (`farolBrutoLiquidoPorSupervisor`, `brutoPorSupervisorTransmitido`) -- reescrever `brutoPorSupervisorTransmitido` para `brutoLiquidoPorSupervisorTransmitido`, no padrão de `brutoLiquidoPorSupervisorFaturado` (linhas 401-449), CTE `trans` + CTE `ccd` (evento CORTADO via `resolveFluxo("cortado")`), sem filtro de `tipo_venda`
- `backend/handlers/farol_comparativo_rel322.go:454-464` (`montarComparativo`) -- remove o cálculo de `comLiquido` (linha 461) e o argumento correspondente
- `backend/handlers/farol_comparativo_rel322.go:475-601` (`cruzarComRel322`) -- remove o parâmetro `comLiquido`, sempre popula `LiquidoFarol`/`TotalLiquidoFarol`
- `backend/handlers/farol_comparativo_rel322_test.go:510-561` (`TestComparativoRel322_Cruzamento_ComLiquidoFalse_SoBruto`) -- remover (cobre um caminho que deixa de existir)
- `backend/handlers/farol_comparativo_rel322_test.go:378,411,461,489,583` (chamadas a `cruzarComRel322(..., true)`) -- remover o terceiro argumento
- `backend/handlers/farol_comparativo_rel322_test.go:743-808` (`TestComparativoRel322_Fluxo_Transmitido`) -- reescrever: CORTADO=777 deixa de ser "não deve vazar" e passa a ser "deve subtrair" (Líquido = 1200-777 = 423)
- `frontend/src/pages/farol/FarolComparativoRel322.tsx:289-293,324-330,380-394` -- remover o branch `fluxo === 'transmitido'` que mostra "—"/texto de "não calcula Líquido"; texto do rodapé passa a explicar a composição do Líquido Transmitido (Bruto − Cortado) em vez de dizer que não existe
- `_bmad-output/implementation-artifacts/spec-comparativo-rel322-fluxo.md` -- Spec Change Log ganha entrada documentando a renegociação (não editar o `frozen-after-approval` original, é histórico)

## Tasks & Acceptance

**Execution:**
- [x] `backend/handlers/farol_comparativo_rel322.go` -- `brutoLiquidoPorSupervisorTransmitido` -- CTE `trans` (Bruto = `SUM(pvenda)` de `vendas_transmitidas`) + CTE `ccd` (Cortado via `resolveFluxo("cortado").eventoFilter` sobre `vendas_ccd v`), `FULL OUTER JOIN`, Líquido = Bruto − Cortado, `escopoCondRel322` aplicado às duas CTEs -- núcleo do cálculo
- [x] `backend/handlers/farol_comparativo_rel322.go` -- comentário de doc de `farolBrutoLiquidoPorSupervisor` -- atualiza para não descrever mais "Transmitido devolve SÓ Bruto" -- evita documentação desatualizada
- [x] `backend/handlers/farol_comparativo_rel322.go` -- `montarComparativo`/`cruzarComRel322` -- remove `comLiquido`, sempre popula Líquido -- simplifica o dispatch, elimina parâmetro morto
- [x] `backend/handlers/farol_comparativo_rel322_test.go` -- remove teste do caminho `comLiquido=false`; ajusta as 5 chamadas restantes de `cruzarComRel322`; reescreve `TestComparativoRel322_Fluxo_Transmitido` (Líquido=423, não nil); acrescenta teste de Cortado > Bruto (Líquido negativo, sem erro); acrescenta/ajusta teste de escopo por persona cobrindo Líquido no Transmitido
- [x] `frontend/src/pages/farol/FarolComparativoRel322.tsx` -- remove os 3 branches de "Transmitido não calcula Líquido"; atualiza o texto do rodapé para explicar Bruto − Cortado
- [x] `_bmad-output/implementation-artifacts/spec-comparativo-rel322-fluxo.md` -- acrescenta entrada no Spec Change Log registrando a renegociação de 27/08/2026 e apontando para esta spec

**Acceptance Criteria:**
- Given um upload no fluxo Transmitido com dado sintético de Bruto e Cortado, when o comparativo roda, then `LiquidoFarol` = Bruto − Cortado (não nil) para cada supervisor com dado.
- Given Cortado maior que Bruto num supervisor, when o comparativo roda, then `LiquidoFarol` vem negativo, sem erro e sem clamp.
- Given um usuário com escopo restrito por persona, when compara no fluxo Transmitido, then Cortado/Bruto de fora do escopo não entram no cálculo.
- Given o fluxo Faturado (qualquer fixture já existente), when o comparativo roda, then Bruto/Líquido permanecem idênticos ao comportamento anterior a esta mudança.

## Design Notes

`resolveFluxo("cortado")` já devolve `fluxoCtx{tableName: "vendas_ccd", dateCol: "data_evento", eventoFilter: "AND v.evento = 'CORTADO'", isCCD: true}` — mesmo mecanismo que o Faturado usa para `resolveFluxo("cancdev")`. A CTE `ccd` do Transmitido é estruturalmente idêntica à do Faturado, só troca o `eventoFilter`; não reimplementar `resolveFluxo`.

O front/PDF não precisam de lógica nova de exibição — `LiquidoFarol *float64` já é tratado genericamente (nil → "—", valor → formatado) nos dois lugares. O único trabalho de UI é REMOVER os branches que hoje forçam "—" para Transmitido.

## Verification

**Commands:**
- `cd backend && go build ./...` -- compila sem erro
- `cd backend && go test ./handlers/... -run ComparativoRel322 -v` -- inclui os testes reescritos/novos de Líquido Transmitido
- `cd frontend && npx tsc --noEmit` -- sem erro

**Manual checks:**
- Upload de um PDF de exemplo com o toggle em Transmitido e conferir que a coluna Líquido (Farol) mostra um valor calculado, não "—".

## Suggested Review Order

**Cálculo do Líquido Transmitido**

- Entry point: `brutoLiquidoPorSupervisorTransmitido` — CTE `trans` (Bruto) + CTE `ccd` (Cortado via `resolveFluxo("cortado")`), `FULL OUTER JOIN`, Líquido = Bruto − Cortado sem filtro de `tipo_venda`.
  [`farol_comparativo_rel322.go:369`](../../backend/handlers/farol_comparativo_rel322.go#L369)

- `montarComparativo` — não decide mais `comLiquido`; os dois fluxos sempre calculam Líquido.
  [`farol_comparativo_rel322.go:476`](../../backend/handlers/farol_comparativo_rel322.go#L476)

- `cruzarComRel322` — perdeu o parâmetro `comLiquido`; `LiquidoFarol` sempre populado, tolerância de 0,5% considera a menor distância entre Bruto e Líquido nos dois fluxos.
  [`farol_comparativo_rel322.go:496`](../../backend/handlers/farol_comparativo_rel322.go#L496)

**Frontend — remoção do caso especial**

- Rodapé do fluxo Transmitido explica Bruto − Cortado e avisa que pode vir negativo.
  [`FarolComparativoRel322.tsx:375`](../../frontend/src/pages/farol/FarolComparativoRel322.tsx#L375)

- Card "Total Líquido (Farol)" não força mais "—" pro Transmitido.
  [`FarolComparativoRel322.tsx:320`](../../frontend/src/pages/farol/FarolComparativoRel322.tsx#L320)

**Testes — o que a revisão (3 revisores) forçou a existir**

- `TestComparativoRel322_Fluxo_Transmitido_CortadoSemLinhaTransmitida` — achado independente de 2 revisores (Blind Hunter + Verification Gap): supervisor que só existe em `vendas_ccd` (CORTADO), sem linha em `vendas_transmitidas`, tem que aparecer com Bruto=0/Líquido negativo, não sumir do resultado nem confundir `SemDadoFarolNoPeriodo`.
  [`farol_comparativo_rel322_test.go:876`](../../backend/handlers/farol_comparativo_rel322_test.go#L876)

- `TestComparativoRel322_Fluxo_Transmitido_CortadoMaiorQueBruto` — Líquido negativo sem clamp nem erro, paridade com o Faturado.
  [`farol_comparativo_rel322_test.go:830`](../../backend/handlers/farol_comparativo_rel322_test.go#L830)

- `TestComparativoRel322_MontarComparativoRespeitaEscopoSupervisor_Transmitido` — Cortado de um supervisor fora do escopo não vaza pro Líquido de dentro do escopo, nem via `FULL OUTER JOIN`.
  [`farol_comparativo_rel322_test.go:675`](../../backend/handlers/farol_comparativo_rel322_test.go#L675)
