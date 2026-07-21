---
title: 'Venda Líquida — composição do faturado (toggles) + abas CCD'
type: 'feature'
created: '2026-07-21'
status: 'draft-aguardando-aprovacao'
context: ['spec-tipo-venda-filtro-faturado.md']
baseline_commit: '43613da'
branch: '(a criar) feat/venda-liquida-composicao'
---

## Intent

**Problem:** Hoje o painel faturado mostra o faturado BRUTO (todas as linhas FATURADO, todos os tipo_venda). O gestor trabalha 95% do tempo com a **venda líquida** = faturamento real, sem bonificação/transferência/remessa e descontando devoluções e NFs canceladas. Além disso, os eventos negativos (CORTADO/CANCELADO/DEVOLVIDO) hoje são importados em `vendas_ccd` mas **nunca aparecem** em lugar nenhum.

**Approach:** Tornar o **Líquido** o padrão do painel faturado, com a venda decomposta em categorias somáveis. Cada categoria excluída ganha um **botão "Incluir X"** que soma seu valor de volta ao total exibido (para devolução/cancelada, ligar = deixar de subtrair). O filtro por tipo (já entregue no spec anterior, cross-filter) serve para *isolar* um tipo quando tudo está somado. Duas **novas abas** dão visibilidade aos eventos negativos: uma para CANCELADO+DEVOLVIDO (lado faturado), outra para CORTADO (lado transmitido).

## Classificação de tipo_venda (CONFIRMADA 2026-07-21)

| Categoria | Códigos | Entra no Líquido? | Botão |
|-----------|---------|-------------------|-------|
| **Venda real** | 1 Padrão, 4 Simples Fatura, 7 Entrega Futura, 8 Simples Entrega, 9 CFOP Específico, 11 Venda c/ Troca, 14 Venda Manifesto, 20 Consignada | SIM (base) | — |
| Bonificação | 5 | não | Incluir Bonificação |
| Transferência | 10 | não | Incluir Transferência |
| Remessa | 13 Remessa Manifesto | não | Incluir Remessa |

Os 11 códigos estão todos mapeados (8 viram venda; 5/10/13 são as únicas exclusões). Não há bucket "Outros".

Eventos (ESTADO, tabela `vendas_ccd`):

| Evento | Origem | Efeito no Líquido | Botão |
|--------|--------|-------------------|-------|
| DEVOLVIDO | NF faturada e devolvida | subtrai | Incluir Devoluções (ligar = não subtrai) |
| CANCELADO | NF cancelada do faturado | subtrai | Incluir Canceladas (ligar = não subtrai) |
| CORTADO | item cortado antes da NF (lado transmitido) | não afeta faturado | (só na aba Cortado) |

**Fórmula:** `Líquido = Σ venda_real − Σ devolvido − Σ cancelado`
**Total exibido** = Líquido + (bonif se ligado) + (transf se ligado) + (remessa se ligado) + (devol se ligado) + (cancel se ligado). Com todos ligados → faturado bruto sem desconto de devol/cancel.

## Boundaries & Constraints

**Always:**
- Líquido é o PADRÃO ao abrir o painel faturado (0 botões ligados).
- Positivação/mix/base_cli seguem o **Líquido** (venda real) — bonificação não infla positivação. *(decisão D3 — confirmar)*
- O total é sempre **soma de colunas pré-agregadas** (rápido, sem re-agrupar por tipo). NUNCA colocar tipo_venda no grão das agg (quebraria positivação/mix — ver spec anterior).
- `vendas_ccd` passa a carregar `tipo_venda` (import) para permitir as abas e filtros por tipo.

**Ask First:**
- Se o custo do upsert subir de forma relevante ao agregar `vendas_ccd` por nível — reavaliar (talvez só l0–l2 para devol/cancel).

**Never:**
- Não mudar a semântica do fluxo transmitido além da nova aba Cortado.
- Não somar devolução/cancelada como valor positivo no Líquido — no Líquido elas SUBTRAEM.

## Modelo de dados (proposto)

Colunas de valor por categoria em cada `agg_fat_*_mes` (populadas pelo upsert; grão inalterado):

- `pv_venda`   — Σ pvenda FILTER (tipo_venda ∈ {1,4,7,8,9,11,14,20})   ← base do Líquido
- `pv_bonif`   — Σ pvenda FILTER (tipo_venda = '5')
- `pv_transf`  — Σ pvenda FILTER (tipo_venda = '10')
- `pv_remessa` — Σ pvenda FILTER (tipo_venda = '13')
- `pv_devol`   — Σ pvenda de `vendas_ccd` (evento=DEVOLVIDO) no mesmo nó hierárquico
- `pv_cancel`  — Σ pvenda de `vendas_ccd` (evento=CANCELADO) no mesmo nó hierárquico

`positivados`/`mix`/`base_cli` recalculados sobre venda_real. `pvenda` legado mantém = soma bruta (compat) OU é aposentado — decisão D4.

`vendas_ccd` recebe `tipo_venda TEXT NOT NULL DEFAULT ''` (mig) + import passa a gravar (copyColsCcd + processFlowCcd com a coluna).

## Mecanismos de UI

1. **Base Líquido** — cards/KPIs carregam com o Líquido.
2. **Botões "Incluir"** (toggles, topo do painel faturado): Bonificação, Transferência, Remessa, Devoluções, Canceladas. Ligado → soma a coluna correspondente ao total (devol/cancel: remove a subtração). Recalcula cards/KPIs client-side ou via param na API.
3. **Filtros por tipo** — JÁ ENTREGUE (cross-filter, spec anterior). Isola um tipo específico.
4. **Nova aba "Cancelado/Devolvido"** — hierarquia por fornec/equipe sobre `vendas_ccd` (evento ∈ {CANCELADO, DEVOLVIDO}), lado faturado.
5. **Nova aba "Cortado"** — hierarquia sobre `vendas_ccd` (evento=CORTADO), lado transmitido (venda perdida).

## Open Decisions (resolver antes de congelar)

- **D1 — Outros:** ✅ RESOLVIDO (2026-07-21) — 8/9/14 viram VENDA. Não há bucket Outros; exclusões = só 5/10/13.
- **D2 — Agregação do CCD:** devol/cancel viram colunas nas `agg_fat_*` (recomendado, total = soma de colunas) OU tabelas agg próprias de CCD para as novas abas? (provável: colunas nas agg_fat p/ o Líquido + agg/scan próprio p/ as abas detalhadas)
- **D3 — Positivação segue o Líquido?** (recomendado: sim)
- **D4 — `pvenda` legado:** ✅ RESOLVIDO (2026-07-21) — `pvenda` PERMANECE bruto (preserva objetivos/KPIs). Líquido entra como COLUNA NOVA `liquido`. Painel faturado passa a exibir `liquido` por padrão; toggles somam as categorias de volta.
- **D5 — Recalc dos toggles:** client-side (API devolve todas as colunas e o front soma) — mais rápido e sem round-trip — ou server-side por querystring? (recomendado: client-side)

## Fases — TODAS CONCLUÍDAS (2026-07-21, direto na main)

- **Fase 1 ✅** — `tipo_venda` em `vendas_ccd` + 6 colunas nas agg_fat (mig 189, import). commit 02edaac.
- **Fase 2 ✅** — `farol.upsert_venda_liquida_cols` popula liquido/pv_* (mig 190, validado em Postgres). commit ef503f9.
- **Fase 3 ✅** — API devolve composição; painel abre no Líquido; botões "Incluir" com recálculo client-side (semáforo segue a tela). commit b843d9c.
- **Fase 4 ✅** — abas Cancel./Devol. (evento IN CANCELADO,DEVOLVIDO) e Cortado (evento=CORTADO), via eventoFilter no fluxoCtx (scan de vendas_ccd, sem agg/positivação). commit c94be34.

**Pendente:** reimportar no ambiente (liquido só popula no import) e validar com o gestor. Se a classificação de tipos precisar de ajuste, editar `farol.tipo_venda_label`/`upsert_venda_liquida_cols` (mig 190) e o FILTER da venda_real.

## Verification

- `SELECT tipo_venda, count(*), sum(pvenda_total) FROM vendas_faturadas GROUP BY 1 ORDER BY 3 DESC` — calibra pesos por tipo (peso de bonif/transf/remessa mostra o quanto o Líquido difere do bruto).
- Líquido do painel = bruto − bonif − transf − remessa − outros − devol − cancel (conferir com soma manual de um fornecedor).
- Ligar todos os toggles → total = faturado bruto sem desconto de devol/cancel.
- Abas Cancelado/Devolvido e Cortado batem com `SELECT evento, sum(pvenda) FROM vendas_ccd GROUP BY 1`.
