---
title: 'Comparativo REL 322 x Farol — VM (base de origem), só Líquido, filtros de Filial e Tipo de Venda'
type: 'feature'
created: '2026-08-28'
status: 'done'
review_loop_iteration: 0
context: []
baseline_commit: 'd23064d7683adf9d668469383261d64aaee33892'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** O comparativo REL 322 só cruzava PDF (WinThor) x Farol (Postgres). Quando divergiam, não dava pra saber se o problema estava no próprio relatório WinThor, na importação pro Farol, ou nos dois. Além disso: (a) Bruto poluía a tela sem ser o número que o gestor realmente usa pra decidir; (b) o filtro "Filiai(s) :" do cabeçalho do PDF era ignorado (achado adiado desde spec-comparativo-rel322.md — um REL 322 rodado só pra um subconjunto de filiais comparava contra o Farol da empresa INTEIRA); (c) não existia jeito de isolar por Tipo de Venda ao investigar uma divergência.

**Approach:** Um terceiro lado no comparativo — "VM" (a base Oracle de origem, a mesma que o JC lê todo dia via `jc_extrator.go`/`JC_ORACLE_*`), consultada AO VIVO no momento do comparativo (custo conhecido: ~1 minuto fixo por consulta, independente do volume). Três diferenças percentuais: PDF×VM, PDF×Farol, VM×Farol — cada uma isola uma causa. Bruto sai da tela: só Líquido. O filtro de filial do cabeçalho do PDF passa a recortar tanto o Farol (Postgres) quanto a VM (Oracle). Um filtro "Tipo de Venda" (multi-seleção) na tela permite o gestor sobrescrever a classificação default de "venda real" ao investigar.

## Boundaries & Constraints

**Always:**
- `rel322Parsed.Filiais` captura o cabeçalho "Filiai(s) :" — lista de códigos, ou nil quando "Todas as Filiais"/campo ausente (não filtra).
- Filial filtra pela coluna `empresa` (Postgres: `vendas_faturadas`/`vendas_transmitidas`/`vendas_ccd`; Oracle: `EMPRESA`) — mesma coluna que `farol_v2_api.go` já usa como "Filial".
- Tipo de Venda: `?tipo_venda=1,4,7` (CSV). Vazio cai no default de cada fluxo via `tipoVendaSelecionado` — Faturado usa `tipoVendaReal` (comportamento já em produção), Transmitido fica sem filtro (Bruto incondicional, como sempre foi). Seleção do usuário sobrescreve os dois, nos dois lados (Farol e VM).
- VM: consulta Oracle ao vivo (`vmLiquidoPorSupervisor`, `farol_comparativo_rel322_vm.go`), reaproveitando `dsnJC()`/`envJC()`/credenciais `JC_ORACLE_*` já usadas pelo JC. Classificação de evento por `LIKE` sobre `ESTADO` (TRANS/CORT/CANCEL/DEVOL), replicando `detectEvento` de `farol_v2_import.go` — nunca igualdade exata, porque o import também não assume literais fixos.
- Falha da VM (Oracle inalcançável, sem credenciais, timeout) NUNCA aborta o comparativo — só marca `VMIndisponivel`/`VMErro` na resposta. PDF×Farol continua sendo o resultado principal.
- Status da linha (`ok`/`divergencia`) é decidido SEMPRE por PDF×Farol (tolerância 0,5%, herdada) — a VM é diagnóstico adicional, nunca decide o Status.
- PDF de exportação vira paisagem (orientação horizontal) — não cabem 9 colunas (Código, Supervisor, PDF, Líquido Farol, Líquido VM, 3 diferenças, Status) em retrato sem espremer valores em moeda.

**Never:**
- Não expõe mais `bruto_farol`/`total_bruto_farol` em lugar nenhum (JSON, PDF, tela) — Bruto foi removido, não só escondido.
- Não filtra Transmitido por Tipo de Venda quando o usuário não seleciona nada — preserva "Bruto incondicional" (decisão já renegociada antes, ver spec-comparativo-rel322-liquido-transmitido.md).
- Não adiciona um novo filtro de "Indústria" (cadastro de indústrias, feature separada) a este comparativo nem a nenhuma view existente — fora de escopo aqui.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| VM alcançável | Oracle responde dentro do timeout | `liquido_vm`/diferenças populados por linha; `total_liquido_vm` preenchido | N/A |
| VM indisponível | Oracle fora do ar, timeout, ou sem credenciais | `vm_indisponivel=true`, `vm_erro` preenchido; PDF×Farol intacto; campos da VM nil | Nunca 500 — sempre 200 com o comparativo PDF×Farol |
| PDF com filial restrita | "Filiai(s) :" lista um subconjunto | Farol e VM só somam essas filiais | N/A |
| PDF "Todas as Filiais" (ou campo ausente) | — | Sem filtro de filial (comportamento anterior) | N/A |
| Usuário seleciona Tipo de Venda | `?tipo_venda=5,10` | Numerador (venda real / Bruto) filtrado por esses códigos, nos dois lados (Farol e VM) | N/A |
| Sem seleção de Tipo de Venda | — | Faturado usa `tipoVendaReal`; Transmitido sem filtro (defaults inalterados) | N/A |

</frozen-after-approval>

## Code Map

- `backend/handlers/farol_comparativo_rel322.go` — `rel322Parsed.Filiais`, `parseFiliaisRel322`, `filialCondRel322`, `tipoVendaSelecionado`, `somaComTipoVendaRel322`, `farolLiquidoPorSupervisor`/`liquidoPorSupervisorFaturado`/`liquidoPorSupervisorTransmitido` (Bruto removido, filial+tipo_venda aplicados), `cruzarComRel322` (3 diferenças + VM), `comparativoRel322PDF` (paisagem, novas colunas)
- `backend/handlers/farol_comparativo_rel322_vm.go` (novo) — `vmLiquidoPorSupervisor`, `colOracleRel322`
- `backend/handlers/jc_extrator.go` — reaproveitado (`dsnJC`, `envJC`), não modificado
- `frontend/src/pages/farol/FarolComparativoRel322.tsx` — filtro Tipo de Venda, colunas Líquido (VM) + 3 diferenças, aviso de VM indisponível, aviso de latência

## Verification

**Commands:**
- `cd backend && go build ./...` — compila sem erro
- `cd backend && go test ./handlers/... -run ComparativoRel322 -v` — inclui os testes novos de filial, tipo_venda, VM (merge e indisponibilidade)
- `cd frontend && npx tsc --noEmit` — sem erro

**Aviso conhecido:** `vmLiquidoPorSupervisor` (Oracle) nunca foi exercitada contra o Oracle real — o ambiente de dev não tem `JC_ORACLE_USER`/`JC_ORACLE_PASS`. A sintaxe foi revisada com cuidado, mas o primeiro upload real em produção deve ser conferido manualmente (comparar o Líquido (VM) exibido contra uma consulta feita à mão na mesma base) antes de confiar cegamente no resultado.
