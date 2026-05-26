# FB_FAROL — Requirements (v2.0)

> **Origem:** sintetizado de [FB_FAROL-NEXT.md](../FB_FAROL-NEXT.md) e PROJECT.md.
> **Versão:** v2.0 (reescrita 2026). v1.0 está em produção mas será substituído.

---

## v1 Requirements

### DATA — Modelo de Dados e Importação

- [ ] **DATA-01** — Importação de CSV unificado com layout novo (Gerente, Supervisor, RCA, Cliente CNPJ/Ramo/UF, Produto EAN/embalagem/qtUnit/qtUnitCx, QT, PVENDA, estado FATURADO/TRANSMITIDO)
- [ ] **DATA-02** — Duas cargas CSV anuais separadas: Base Comparativa (ano anterior) + Base Atual (ano corrente); rotação automática na virada de ano
- [ ] **DATA-03** — Schema novo: `vendas_importadas` (com tipo_base, estado, todos os níveis hierárquicos), `industrias_config` (programa + trava mínima), `objetivos_industria` (metas explícitas)
- [ ] **DATA-04** — Estende tabela `users` com `tipo_persona` e `cod_referencia` para autenticação por papel
- [ ] **DATA-05** — DROP do schema antigo (`objetivos_importados`, views `vw_obj_*`) — migração descarta dados atuais
- [ ] **DATA-06** — Cliente sem venda no período aparece como linha do CSV com `qt=0` e `pvenda=0` (permite calcular positivação correta e listar não-compradores)

### IND — Indicadores e Fórmulas

- [ ] **IND-01** — Positivação = COUNT(DISTINCT cod_cli WHERE qt ≥ trava_minima[fornec]) / qtcli_rca × 100
- [ ] **IND-02** — Média de Itens por Cliente = SUM(produtos distintos por cliente ativo) / clientes ativos
- [ ] **IND-03** — Faturado e Transmitido como duas métricas separadas (filter por estado do CSV)
- [ ] **IND-04** — Farol binário: 🟢 Verde ≥ 100% atingimento / 🔴 Vermelho < 100% (sem zona amarela)
- [ ] **IND-05** — Trava mínima de quantidade por indústria configurável; default = 1 (positivado = qt ≥ 1)
- [ ] **IND-06** — Fallback de meta: se `objetivos_industria` vazia para o período → usa Base Comparativa como meta + label "Meta inferida do histórico"

### VIEW — Visões Hierárquicas e Navegação

- [ ] **VIEW-01** — Visão 01 "Fornecedor-led" (SUPV/GGV): `Fornec → GGV → Supv → RCA → Cliente → Produto`
- [ ] **VIEW-02** — Visão 02 "RCA-led" (SUPV/GGV): `Supv → RCA → Fornec → Cliente → Produto`
- [ ] **VIEW-03** — Visão 03 "Diretoria" (CEO/Grupo): `Fornec(grupo) → Empresa → UF → GGV → Supv → RCA → Cliente → Produto`
- [ ] **VIEW-04** — Toggle V01 ↔ V02 para Supervisor/GGV (mantém comportamento atual de abas Fornec/RCA)
- [ ] **VIEW-05** — Drill-down totalmente navegável (clica e desce nível, voltar preserva contexto)
- [ ] **VIEW-06** — Em cada nível: card com semáforo, % atingimento, anterior, atual, indicadores secundários (positivação, média itens, faturado/transmitido)

### AUTH — Autenticação e Permissões

- [ ] **AUTH-01** — Login web para: Supervisor, Gerente, GGV, Diretoria (campo `tipo_persona` no cadastro de usuário)
- [ ] **AUTH-02** — RCA NÃO tem login web; acessa via URL pública `/m/CNPJ/RCA/cod` (compatibilidade ION VENDAS)
- [ ] **AUTH-03** — Escopo de visão automático: Supervisor vê só seus RCAs; Gerente vê seus supervisores; GGV vê seus gerentes; Diretoria vê tudo do grupo
- [ ] **AUTH-04** — Mantém URLs públicas `/m/CNPJ/SUP/cod` e `/m/CNPJ/RCA/cod` para WebView do ION

### UX — Experiência de Usuário

- [ ] **UX-01** — Maquete HTML standalone (Fase 0) em `/maquete/` — sem backend, importação CSV client-side via FileReader/papaparse, renderiza todas 3 visões com drill-down completo
- [ ] **UX-02** — Maquete permite o gestor importar 1 mês de Base Comparativa + 1 mês de Base Atual e simular comportamento real
- [ ] **UX-03** — Padrão visual "Clean Professional" mantido (cards com barra de progresso, semáforo, KPIs)
- [ ] **UX-04** — Cards mostram lado a lado: Anterior vs Atual + % + barra de progresso + semáforo
- [ ] **UX-05** — Ordenação por % DESC (melhores primeiro) em todos os listings
- [ ] **UX-06** — Filtros e busca em cada nível da hierarquia
- [ ] **UX-07** — Mobile via WebView ION mantido e responsivo (sem PWA / offline / atalhos extras)

### FEAT — Features Avançadas (entregar após o core)

- [ ] **FEAT-01** — Sparkline de evolução mensal/trimestral por entidade (no card)
- [ ] **FEAT-02** — Comparativo YoY (atual vs mesmo período do ano anterior usando Base Comparativa)
- [ ] **FEAT-03** — Forecast de fechamento baseado no ritmo do período
- [ ] **FEAT-04** — Ranking (top melhores / top piores) dentro do escopo da persona
- [ ] **FEAT-05** — Multi-período comparativo (selecionar 2-3 períodos lado a lado)

---

## Out of Scope

| # | Item | Por quê |
|---|---|---|
| 1 | Notificações push/email, alertas, planos de ação, comentários | Farol é painel de visualização, não ferramenta colaborativa de workflow |
| 2 | PWA / modo offline / atalhos mobile extras | Mobile via WebView ION já atende; sem necessidade de complexidade adicional |
| 3 | Exportação Excel / PDF / link público compartilhável | Visualização online é suficiente; gestor não pediu exportação |
| 4 | Dashboards personalizáveis individualmente | Layout uniforme por persona; complexidade de configuração não compensa |
| 5 | Threshold de semáforo configurável | Decisão: 100% é o threshold único; sem zona configurável |
| 6 | Cadastro de produtos com nome no banco | Produto vem com descrição do CSV; sem cadastro próprio |
| 7 | Migração dos dados atuais para o novo schema | Schema é incompatível; engenharia de conversão não vale o esforço |
| 8 | Visão 04 "UF-led" para diretoria | V03 (`Fornec → Empresa → UF → ...`) atende; UF como primeiro nível foi descartado |
| 9 | Indústria como agrupador de fornecedores | Indústria = cod_fornec direto; sem hierarquia adicional |

---

## Traceability

Mapeamento 1:1 — cada requisito pertence a exatamente uma fase. Coverage: **34/34 ✓**

| Req-ID | Phase | Status |
|---|---|---|
| DATA-01 | Phase 1 — Schema novo + Migration + Importação | Pending |
| DATA-02 | Phase 1 — Schema novo + Migration + Importação | Pending |
| DATA-03 | Phase 1 — Schema novo + Migration + Importação | Pending |
| DATA-04 | Phase 1 — Schema novo + Migration + Importação | Pending |
| DATA-05 | Phase 1 — Schema novo + Migration + Importação | Pending |
| DATA-06 | Phase 1 — Schema novo + Migration + Importação | Pending |
| IND-01 | Phase 2 — Views materializadas + Endpoints de leitura | Pending |
| IND-02 | Phase 2 — Views materializadas + Endpoints de leitura | Pending |
| IND-03 | Phase 2 — Views materializadas + Endpoints de leitura | Pending |
| IND-04 | Phase 0 — Maquete HTML local | Pending |
| IND-05 | Phase 2 — Views materializadas + Endpoints de leitura | Pending |
| IND-06 | Phase 2 — Views materializadas + Endpoints de leitura | Pending |
| VIEW-01 | Phase 0 — Maquete HTML local | Pending |
| VIEW-02 | Phase 0 — Maquete HTML local | Pending |
| VIEW-03 | Phase 0 — Maquete HTML local | Pending |
| VIEW-04 | Phase 3 — Frontend React integrado | Pending |
| VIEW-05 | Phase 0 — Maquete HTML local | Pending |
| VIEW-06 | Phase 0 — Maquete HTML local | Pending |
| AUTH-01 | Phase 3 — Frontend React integrado | Pending |
| AUTH-02 | Phase 3 — Frontend React integrado | Pending |
| AUTH-03 | Phase 3 — Frontend React integrado | Pending |
| AUTH-04 | Phase 3 — Frontend React integrado | Pending |
| UX-01 | Phase 0 — Maquete HTML local | Pending |
| UX-02 | Phase 0 — Maquete HTML local | Pending |
| UX-03 | Phase 3 — Frontend React integrado | Pending |
| UX-04 | Phase 3 — Frontend React integrado | Pending |
| UX-05 | Phase 3 — Frontend React integrado | Pending |
| UX-06 | Phase 3 — Frontend React integrado | Pending |
| UX-07 | Phase 3 — Frontend React integrado | Pending |
| FEAT-01 | Phase 4 — Features avançadas | Pending |
| FEAT-02 | Phase 4 — Features avançadas | Pending |
| FEAT-03 | Phase 4 — Features avançadas | Pending |
| FEAT-04 | Phase 4 — Features avançadas | Pending |
| FEAT-05 | Phase 4 — Features avançadas | Pending |
