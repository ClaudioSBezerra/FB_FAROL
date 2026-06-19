# Painel "Gestão de Compras" — Plano de implementação

> **Status**: ⏸️ AGUARDANDO feedback do Gestor Edinardo sobre o mockup
> **Mockup**: [`/home/claudio/Downloads/mockup-gestao-compras.html`](file:///home/claudio/Downloads/mockup-gestao-compras.html)
> **Criado**: 2026-06-09 por Claudio + Sally (UX agent)
> **Inspiração**: 2 vídeos WhatsApp gravados pelo Diretor de Compras e Vendas em evento (mural war-room Nestlé/OrigeNES)

---

## Contexto

O Diretor de Compras e Vendas viu em um evento um **mural de TVs (war-room style)** mostrando o painel de operações comerciais Nestlé × OrigeNES. Gostou do conceito e pediu algo parecido pra Gestão de Compras da JC Distribuição. Diferença fundamental:

- **No vídeo (Nestlé)**: fabricante vendo seu giro nos varejistas
- **No FB_FAROL (nós)**: distribuidora vendo a relação com o fornecedor

Métricas mudam de "minha venda" para "minha dependência do fornecedor X / minha equipe vendendo a linha dele / sua tendência YoY".

## Decisões já tomadas com o gestor

| Tópico | Resposta |
|---|---|
| Top N fornecedores | **Top 10 pelo Pareto do ANO CORRENTE** (YTD) — dinâmico, não hardcoded |
| Público | **Ambos** — Diretor com drill (modo executivo) + TV de parede sem login (modo apresentação) |

## Conceito do mockup (aprovação pendente)

Layout dark mode, otimizado pra TV 75" lida a 3-4m:

```
┌─────────────────────────────────────────────────────────────────┐
│ HEADER: Farol · Gestão de Compras + relógio LIVE                │
├─────────────────────────────────────────────────────────────────┤
│ 🚨 ALERTA de quedas │ Curva ABC Top 10 = 78,4%                 │
├─────────────────────────────────────────────────────────────────┤
│ 10 CARDS (5x2): nome · indústria · valor · delta YoY · sparkln │
├─────────────────────────────────────────────────────────────────┤
│ HEATMAP RCA × Top 10 Fornec. (penetração binária verde/vermelho)│
├─────────────────────────────────────────────────────────────────┤
│ FOOTER: rotação automática entre 4 visões                       │
└─────────────────────────────────────────────────────────────────┘
```

## Modos propostos (rotação automática a cada N min)

1. **Visão Geral** (mockup atual) — KPIs + cards + heatmap
2. **Heatmap detalhado** — todos os RCAs, não só top 8
3. **Top Movers** — quem subiu/caiu mais YoY no mês
4. **Comparar 2 fornec.** — split-screen estilo Nestlé × OrigeNES

## Plano técnico (depende do feedback)

### Backend

1. **Migration 169** (provável):
   - Nada de novo em vendas — reusa `agg_fat_v01_l0_mes` (já existente)
   - Possivelmente uma agg auxiliar `agg_fat_pareto_ytd_mes` pré-calculada (top 10 do YTD por empresa)
   - View virtual para "movers" — top 5 maiores quedas/altas vs YoY
   - View para "RCA × Top10 Fornec" (heatmap) — JOIN entre `agg_fat_v01_l0_mes` (top 10) e `agg_fat_v02_l1_mes` (RCAs)

2. **Handler novo** `/api/v2/compras/dashboard`:
   - retorna `{ top10: [...], alertas: [...], curva_abc: [...], heatmap: [...], periodo: ... }`
   - 1 endpoint single-shot para reduzir round-trips na TV

3. **Endpoint público** `/api/public/tv/compras/{empresa-id}/token/{token-fixo}`:
   - Sem login. Token simples no DB pra evitar URL aberta na internet.
   - Cache 5min server-side.

### Frontend

1. `FarolGestaoCompras.tsx` — versão executiva (com drill, dentro do app autenticado)
2. `FarolGestaoComprasTV.tsx` — versão TV (fullscreen, sem auth, rotação automática)
3. Componentes compartilhados:
   - `CardFornecedorCompacto` (card + sparkline SVG)
   - `CurvaABC` (barras empilhadas)
   - `HeatmapRcaFornec` (grid CSS)
   - `AlertaMovers` (badge + lista)

### Sidebar

- Novo item no `AppSidebar` para usuários com persona `diretor` / `ceo` / `gerente_geral` / `admin`

## Tradeoffs já discutidos

| Tema | Decisão |
|---|---|
| Margem (plucro) em destaque | ❌ Comprador olha primeiro volume, depois valor. Margem vai pro detalhe. |
| Texto pequeno | ❌ Nada abaixo de 16px (lido a 3m) |
| Gráficos 3D / animados | ❌ Densidade > fofura |
| Filtro UF/Filial no header | ❌ Atrapalha leitura. Só botão lateral discreto |
| Comparar 2 fornec. | ✅ Foi o que ele mais gostou no vídeo (Nestlé × OrigeNES) |
| Curva ABC | ✅ Pareto = identidade de Gestão de Compras |
| Alerta de quedas | ✅ Faz o diretor agir no dia, não no fechamento mensal |

## Próximos passos quando Edinardo aprovar

1. ✅ Mostrar mockup (FEITO — aguardando)
2. ⏳ Coletar ajustes (provavelmente vai pedir: ordenação, métricas extras, paleta)
3. ⏳ Confirmar período de comparação (YTD vs MoM vs YoY — provavelmente YoY)
4. ⏳ Confirmar se quer Faturado, Transmitido ou ambos
5. ⏳ Modelar migration 169 + handler
6. ⏳ Implementar versão executiva primeiro (drill, ajustes finos)
7. ⏳ Versão TV pública depois (com cache + rotação)
8. ⏳ Adicionar ao sidebar + testar em produção

## Referências da sessão

- Foto do mural Nestlé/OrigeNES analisada (4 quadrantes 2x2)
- Outros painéis Farol como referência visual: `FarolV2Dashboard`, `FarolMarketing`, `FarolExecutivo`, `FarolPublicPanel`
- Stack atual mantida: Go 8087 + React 3087 + Postgres + Coolify
