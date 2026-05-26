# FB_FAROL — Reescrita 2026

## What This Is

Sistema de Farol de Vendas (semáforo de objetivos) para distribuidoras que operam com força de vendas via ION VENDAS (WinThor/PC Sistemas). Oferece visões hierárquicas — Diretoria → GGV → Supervisor → RCA → Cliente → Produto — com indicadores de positivação, mix de itens, faturado vs transmitido, e farol binário (verde/vermelho) de atingimento de meta por indústria. Acessível via web autenticado (para Gerentes, GGVs e Supervisores) e via URL pública para o ION VENDAS (RCAs em campo).

## Core Value

**Mostrar ao gestor — em segundos — quem está atingindo meta e quem não está, com drill-down até cliente/produto, incluindo clientes sem venda.** Se algum indicador secundário falhar, isso ainda precisa funcionar.

## Requirements

### Validated

(None yet — esta é uma reescrita; ver "## Context" para o que o sistema atual já faz)

### Active

- [ ] **REQ-DATA-01** — Importação de CSV unificado com novo layout (gerente, supv, rca, cliente CNPJ/ramo/UF, produto EAN/embalagem, qt+pvenda)
- [ ] **REQ-DATA-02** — Duas cargas anuais separadas: Base Comparativa (ano anterior) e Base Atual (ano corrente); rotação anual automática
- [ ] **REQ-DATA-03** — Cliente sem venda aparece no painel (linha com qt=0, pvenda=0)
- [ ] **REQ-DATA-04** — Tabela de Objetivos por Indústria (cod_fornec) com fallback para histórico quando vazia
- [ ] **REQ-DATA-05** — Configuração de programa de distribuição por indústria com trava mínima de quantidade para considerar positivado
- [ ] **REQ-IND-01** — Indicador "Positivação" = atendidos / qtcli_rca (respeitando trava mínima)
- [ ] **REQ-IND-02** — Indicador "Média Itens por Cliente" = SUM(produtos distintos por cliente) / clientes ativos
- [ ] **REQ-IND-03** — Indicadores "Faturado" e "Transmitido" separados (estados do CSV)
- [ ] **REQ-IND-04** — Farol binário verde (≥100%) / vermelho (<100%)
- [ ] **REQ-VIEW-01** — Visão 01 "Fornecedor-led": Fornec → GGV → Supv → RCA → Cliente → Produto
- [ ] **REQ-VIEW-02** — Visão 02 "RCA-led": Supv → RCA → Fornec → Cliente → Produto
- [ ] **REQ-VIEW-03** — Visão 03 "Diretoria": Fornec(grupo) → Empresa → UF → GGV → Supv → RCA → Cliente → Produto
- [ ] **REQ-VIEW-04** — Toggle V01/V02 para SUPV/GGV; V03 fixa para Diretoria
- [ ] **REQ-AUTH-01** — Login web para Supervisor, Gerente, GGV, Diretoria com campo `tipo_persona` no cadastro de usuário
- [ ] **REQ-AUTH-02** — RCA acessa SEM login via URL pública `/m/CNPJ/RCA/cod` (compatibilidade ION)
- [ ] **REQ-AUTH-03** — Escopo de visão por persona (Supv vê seus RCAs, Ger vê seus supervisores, etc.)
- [ ] **REQ-UX-01** — Maquete HTML standalone (Fase 0) com importação CSV local para aprovação do gestor antes do backend
- [ ] **REQ-UX-02** — Manter visual "Clean Professional" atual (cards com barra de progresso, semáforo, KPIs)
- [ ] **REQ-FEAT-01** — Sparkline de evolução por entidade
- [ ] **REQ-FEAT-02** — Comparativo YoY (ano vs ano) usando Base Comparativa
- [ ] **REQ-FEAT-03** — Forecast de fechamento baseado no ritmo
- [ ] **REQ-FEAT-04** — Ranking (melhores/piores) por escopo
- [ ] **REQ-FEAT-05** — Multi-período comparativo (2-3 períodos lado a lado)

### Out of Scope

- Notificações push/email, alertas, planos de ação, comentários — Farol é painel de visualização, não ferramenta de workflow
- PWA / modo offline / atalhos mobile extras — manter mobile atual via WebView ION
- Exportação Excel / PDF / link público compartilhável — visualização online apenas
- Dashboards personalizáveis individualmente / threshold de semáforo configurável — comportamento fixo
- Cadastro de produtos com nome (continua só código + descrição vinda do CSV)
- Migração dos dados atuais — começamos com schema limpo, dados antigos descartados
- Visão 04 "UF-led" para diretoria — só V03 atende
- Indústria como agrupador de fornecedores — indústria = cod_fornec direto

## Context

**Sistema atual** (sendo refundado):
- 150+ commits em produção em `farol.fbtax.cloud`
- Stack: Go (porta 8087) + React (porta 3087) + Postgres + Docker + Coolify
- Tabela `objetivos_importados` com colunas vl_anterior/vl_corrente
- Materialized views: `vw_obj_rca_fornecedor`, `vw_obj_supervisor`, `vw_obj_rca_produto`
- Painéis web: "Objetivo RCA" e "Objetivo Supervisor" (tabelas com filtros)
- Farol Mobile via WebView ION: `/m/CNPJ/SUP/cod` e `/m/CNPJ/RCA/cod`
- Farol Web autenticado: `/farol` com abas Fornec/Supervisor + drill-downs
- Semáforo 3 cores (verde ≥100% / amarelo 75-99% / vermelho <75%)
- Cadastros: Gestores, RCAs, Empresas, Grupos (enterprise_groups), Filiais, Ambientes
- Auth JWT com sp_role (admin_fbtax | gestor_geral | gestor_filial | somente_leitura)

**Motivação da reescrita** (briefing do gestor):
- Painel atual não traz cliente sem venda (essencial para gestão real)
- Não tem nível Gerente acima de Supervisor
- Não tem visão Diretoria/UF/Grupo
- Não diferencia faturado vs transmitido
- Meta é implícita (vl_anterior) — falta tabela de objetivos por indústria
- Falta indicadores como média de itens/cliente e positivação real com trava
- Farol 3 cores é ruído visual — gestor quer "atingiu OU não atingiu"

**Documento referência:** [FB_FAROL-NEXT.md](../FB_FAROL-NEXT.md) — idea document completo com modelo de dados, fases, visões, fórmulas.

## Constraints

- **Stack:** Go 8087 + React 3087 + Postgres + Coolify — sem mudança de stack
- **Compatibilidade ION VENDAS:** URLs `/m/CNPJ/SUP/cod` e `/m/CNPJ/RCA/cod` devem continuar funcionando para o aplicativo de campo
- **Visual:** padrão "Clean Professional" atual mantido e estendido
- **Banco:** continua Postgres com materialized views (sem migração para OLAP / cubo)
- **Migração:** dados atuais descartados (schema limpo) — gestor aceitou esta perda

## Key Decisions

| Decisão | Racional | Outcome |
|---|---|---|
| Maquete HTML local antes do backend (Fase 0) | Gestor experimenta antes de gastar tempo de Go — evita retrabalho de UI | — Pending |
| Farol 2 cores (verde/vermelho) | Gestor disse: "atingiu ou não" — zona amarela é ruído | — Pending |
| Duas cargas CSV anuais (Comparativa + Atual) | Modelo do gestor: separa o histórico do corrente para clareza de comparação | — Pending |
| Indústria = Fornecedor (cod_fornec) | Decisão direta do gestor: sem agrupador adicional | — Pending |
| Base de clientes RCA no mesmo CSV (com qt=0) | Simplifica fluxo: uma carga só traz tudo | — Pending |
| RCA sem login (só via ION) | RCAs em campo já usam ION VENDAS — não precisam de senha web | — Pending |
| Dados atuais descartados | Schema novo é incompatível — não vale a engenharia de conversão | — Pending |

## Evolution

This document evolves at phase transitions and milestone boundaries.

**After each phase transition** (via `/gsd-transition`):
1. Requirements invalidated? → Move to Out of Scope with reason
2. Requirements validated? → Move to Validated with phase reference
3. New requirements emerged? → Add to Active
4. Decisions to log? → Add to Key Decisions
5. "What This Is" still accurate? → Update if drifted

**After each milestone** (via `/gsd:complete-milestone`):
1. Full review of all sections
2. Core Value check — still the right priority?
3. Audit Out of Scope — reasons still valid?
4. Update Context with current state

---
*Last updated: 2026-05-27 after initialization (auto mode, idea doc: FB_FAROL-NEXT.md)*
