# Roadmap: FB_FAROL — Reescrita 2026

## Overview

Reescrita completa do Farol de Vendas em 5 fases sequenciais (modo MVP / vertical slice). Começa com uma maquete HTML local interativa que o gestor experimenta antes de qualquer linha de Go (Fase 0). Depois constrói schema novo + importação (Fase 1), camada de leitura via materialized views (Fase 2), frontend React integrado com autenticação por persona (Fase 3) e fecha com features avançadas — sparkline, YoY, forecast, ranking, multi-período (Fase 4). Cada fase entrega uma capacidade verificável de ponta a ponta para o gestor.

## Phases

**Phase Numbering:**
- Integer phases (0, 1, 2, 3, 4): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

- [ ] **Phase 0: Maquete HTML local** — HTML standalone com import CSV no browser, renderiza 3 visões com drill-down (prototype, sem Go)
- [ ] **Phase 1: Schema novo + Migration + Importação** — Cria schema v2, DROP do antigo, endpoints de import e UI de importação
- [ ] **Phase 2: Views materializadas + Endpoints de leitura** — 3 MVs (uma por visão) + endpoints públicos (ION) e autenticados (web) com cálculos de positivação
- [ ] **Phase 3: Frontend React integrado** — Substitui o JS mock por chamadas reais, drill-down nas 3 hierarquias, auth por persona e escopo automático
- [ ] **Phase 4: Features avançadas** — Sparklines, YoY, Forecast, Ranking, Multi-período comparativo

## Phase Details

### Phase 0: Maquete HTML local
**Goal**: Gestor experimenta o painel completo (3 visões com drill-down) no Chrome local com seus próprios CSVs antes de qualquer linha de Go ser escrita
**Depends on**: Nothing (first phase)
**Requirements**: UX-01, UX-02, VIEW-01, VIEW-02, VIEW-03, VIEW-05, VIEW-06, IND-04
**Success Criteria** (what must be TRUE):
  1. Gestor abre `/maquete/index.html` localmente no Chrome/Edge sem precisar de servidor
  2. Gestor importa via FileReader 1 CSV "Base Comparativa" + 1 CSV "Base Atual" e os dados aparecem renderizados em segundos
  3. As 3 visões hierárquicas (Fornecedor-led, RCA-led, Diretoria) renderizam com drill-down completo (cada clique desce um nível, voltar preserva contexto)
  4. Cards mostram lado a lado Anterior vs Atual, %, barra de progresso e farol binário verde/vermelho (sem amarelo)
  5. Gestor aprova a maquete (ou pede ajustes específicos) — gate de aprovação antes de iniciar Fase 1
**Plans**: TBD
**UI hint**: yes

### Phase 1: Schema novo + Migration + Importação
**Goal**: Backend aceita os novos CSVs (vendas, objetivos por indústria, config de programa) gravando no schema v2; schema antigo removido
**Depends on**: Phase 0
**Requirements**: DATA-01, DATA-02, DATA-03, DATA-04, DATA-05, DATA-06
**Success Criteria** (what must be TRUE):
  1. Admin sobe migration que cria `vendas_importadas`, `industrias_config`, `objetivos_industria` e estende `users` com `tipo_persona` + `cod_referencia`
  2. Migration faz DROP da tabela `objetivos_importados` e das views `vw_obj_*` antigas sem deixar resíduo no schema
  3. Admin importa CSV de vendas via UI (3 abas: Vendas / Objetivos / Configuração) e linhas com `qt=0`/`pvenda=0` (clientes sem venda) são gravadas como rows válidas
  4. Importação suporta `tipo_base` (COMPARATIVA | ATUAL) e `ano`, permitindo duas cargas separadas com rotação anual
  5. Endpoint `POST /api/v2/import/vendas` retorna SSE de progresso e finaliza com contagem de rows por tipo_base/estado
**Plans**: TBD
**UI hint**: yes

### Phase 2: Views materializadas + Endpoints de leitura
**Goal**: API de leitura entrega as 3 visões hierárquicas pré-agregadas com positivação, mix, faturado vs transmitido e farol binário
**Depends on**: Phase 1
**Requirements**: IND-01, IND-02, IND-03, IND-05, IND-06
**Success Criteria** (what must be TRUE):
  1. 3 materialized views (uma por visão: Fornecedor-led, RCA-led, Diretoria) são criadas e populadas a partir de `vendas_importadas`
  2. Positivação respeita a `trava_minima_qt` por fornecedor de `industrias_config` (default = 1 quando não configurada)
  3. Endpoint público `/api/farol/v2/m/:cnpj/:tipo/:cod` retorna dados sem auth (compatibilidade ION VENDAS) e endpoint autenticado `/api/farol/v2/web/*` retorna JSON com indicadores pré-calculados
  4. Quando `objetivos_industria` está vazia para o período, a API retorna meta inferida da Base Comparativa com label explícita ("Meta inferida do histórico")
  5. Refresh das MVs roda automaticamente após cada importação e endpoints respondem < 1s para queries dos filtros principais (empresa, periodo, fornecedor, supervisor, rca)
**Plans**: TBD

### Phase 3: Frontend React integrado
**Goal**: Painel web React substitui a maquete consumindo a API real, com auth por persona e escopo de visão automático
**Depends on**: Phase 2
**Requirements**: AUTH-01, AUTH-02, AUTH-03, AUTH-04, VIEW-04, UX-03, UX-04, UX-05, UX-06, UX-07
**Success Criteria** (what must be TRUE):
  1. Supervisor, Gerente, GGV e Diretoria fazem login web com `tipo_persona` correto e veem apenas seu escopo automaticamente (Supervisor → seus RCAs; Gerente → seus supervisores; GGV → seus gerentes; Diretoria → tudo do grupo)
  2. SUPV/GGV alternam entre Visão 01 (Fornec-led) e Visão 02 (RCA-led) via toggle; Diretoria vê apenas Visão 03 fixa
  3. URLs públicas `/m/:cnpj/SUP/:cod` e `/m/:cnpj/RCA/:cod` continuam funcionando para o WebView do ION VENDAS sem exigir login
  4. Cards renderizam padrão "Clean Professional" com Anterior vs Atual, %, barra de progresso, farol binário, positivação, mix, faturado/transmitido — ordenados por % DESC com filtros e busca por nível
  5. Painel funciona no mobile via WebView ION (responsivo, sem PWA/offline)
**Plans**: TBD
**UI hint**: yes

### Phase 4: Features avançadas
**Goal**: Painel ganha sparkline, comparativo YoY, forecast de fechamento, ranking e multi-período comparativo
**Depends on**: Phase 3
**Requirements**: FEAT-01, FEAT-02, FEAT-03, FEAT-04, FEAT-05
**Success Criteria** (what must be TRUE):
  1. Cada card exibe sparkline de evolução (mensal ou trimestral) da entidade naquele nível
  2. Card mostra comparativo YoY (atual vs mesmo período do ano anterior) usando dados da Base Comparativa
  3. Forecast de fechamento aparece em cada card calculado pelo ritmo do período corrente
  4. Persona acessa Ranking (top melhores / top piores) dentro do seu escopo
  5. Usuário seleciona 2-3 períodos e visualiza-os lado a lado no comparativo multi-período
**Plans**: TBD
**UI hint**: yes

## Progress

**Execution Order:**
Phases execute in numeric order: 0 → 1 → 2 → 3 → 4

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 0. Maquete HTML local | 0/TBD | Not started | - |
| 1. Schema novo + Migration + Importação | 0/TBD | Not started | - |
| 2. Views materializadas + Endpoints de leitura | 0/TBD | Not started | - |
| 3. Frontend React integrado | 0/TBD | Not started | - |
| 4. Features avançadas | 0/TBD | Not started | - |
