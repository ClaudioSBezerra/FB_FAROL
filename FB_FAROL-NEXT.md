# FB_FAROL — Reescrita 2026 (Idea Document)

> **Status:** consolidado com decisões do gestor.
> **Próximo passo:** instalar `gsd-sdk` (via sudo) e rodar `/gsd:new-project --auto @FB_FAROL-NEXT.md`.

---

## 1. Resumo executivo

O Farol atual é um painel snapshot por período que cruza supervisores/RCAs/fornecedores e calcula um semáforo de 3 cores. Funciona, mas é limitado para a operação gerencial real:

- **Não mostra cliente sem venda** (só quem comprou aparece)
- **Não tem o nível Gerente** (atual: Empresa → Supervisor → RCA)
- **Não tem visão de UF/Diretoria/Grupo**
- **Não diferencia faturado vs transmitido**
- **Meta é implícita** (vl_anterior) — não tem objetivo explícito por indústria
- **Indicadores fracos:** sem média de itens, sem positivação real com trava mínima
- **Drill-down raso** (2-3 níveis)

Esta reescrita refunda o modelo de dados e a UX para:

1. **Modelo expandido** — Grupo → Empresa → UF → Gerente → Supervisor → RCA → Cliente → Produto, com base de clientes do RCA explícita
2. **4 visões hierárquicas** com drill-down completo
3. **Indicadores corretos** — positivação real, média itens/cliente, faturado vs transmitido, meta vs realizado
4. **Tabela de objetivos por indústria** com trava mínima e fallback histórico
5. **Farol simplificado** 2 cores (atingiu meta = verde, não atingiu = vermelho)
6. **Personas com login** (Gerente, GGV, Supervisor) + acesso público via ION (RCA / mobile)

---

## 2. Metodologia de desenvolvimento

**Phase 0 vai ser uma maquete HTML local interativa**, não código backend. O fluxo:

1. Eu codifico `/maquete/` no projeto — HTML/JS standalone (sem servidor Go, sem auth)
2. Maquete importa CSV via FileReader/papaparse no browser
3. Renderiza TODAS as 4 visões com drill-down funcional usando dados reais
4. Permite carregar 1 mês "Base Comparativa" + 1 mês "Base Atual"
5. Gestor experimenta no localhost antes de qualquer linha de Go
6. Depois de aprovado → migramos a UI para React + backend Go

**Por quê:** evita retrabalho de UI/UX. Maquete em 1 sprint vale meses de iteração depois.

---

## 3. Modelo de dados

### 3.1. CSV de vendas — layout novo (uma carga por base)

```
PERIODO              — tipo+ano+seq+estado (FATURADO | TRANSMITIDO)
CODGERENTE / GERENTE — rotina 582
CODSUPERVISOR / SUPERVISOR / QTRCA_SUPERVISOR — rotina 516+517
CODUSUR / RCA / QTCLI_RCA   — rotina 517+302
CODFORNEC / FORNECEDOR      — rotina 202 (= indústria)
CODCLI / CLIENTE / CNPJ     — rotina 302
CODRAMO / RAMO              — rotina 512+302
UF                          — rotina 533+302
CODPROD / PRODUTO / EMBALAGEM / QTUNIT / QTUNITCX / EAN — rotina 203
QT                          — quantidade vendida
PVENDA                      — valor vendido
```

**Importante (decisão do gestor):** o CSV traz TODOS os clientes da base do RCA, mesmo os sem venda. Para clientes sem venda no período: `QT = 0` e `PVENDA = 0`. Isso permite calcular positivação correta (atendidos / base) e listar não-compradores.

### 3.2. Duas cargas anuais separadas

- **Base Comparativa:** dados do ano anterior. Importada UMA vez quando vira o ano.
- **Base Atual:** dados do ano corrente. Importada periodicamente (mensal/trimestral/etc).
- Cada `vendas_importadas` row tem coluna `tipo_base` (COMPARATIVA | ATUAL) e `ano`.
- **Rotação anual:** a Base Atual de 2026, em 2027, vira a Base Comparativa de 2027.

### 3.3. Schema proposto

```sql
-- Carga principal de vendas (uma row por venda OU por cliente-base-sem-venda)
vendas_importadas (
  id, empresa_id, tipo_base,     -- COMPARATIVA | ATUAL
  tipo_periodo, ano, periodo_seq,
  estado,                         -- FATURADO | TRANSMITIDO
  cod_gerente,  nome_gerente,
  cod_supervisor, nome_supervisor, qtrca_supervisor,
  cod_rca, nome_rca, qtcli_rca,
  cod_fornec, nome_fornec,
  cod_cli, nome_cli, cnpj_cli, cod_ramo, ramo, uf,
  cod_prod, nome_prod, embalagem, qt_unit, qt_unit_cx, ean,
  qt, pvenda,
  importado_em
)

-- Programa de distribuição por indústria/fornecedor (configuração)
industrias_config (
  empresa_id, cod_fornec,
  programa_distribuicao,         -- nome livre do programa
  trava_minima_qt,                -- positivado só conta se qt >= X (default 1)
  atualizado_em
)

-- Objetivo de vendas por indústria/período (meta explícita)
objetivos_industria (
  empresa_id, cod_fornec,
  tipo_periodo, ano, periodo_seq,
  meta_valor, meta_quantidade,
  valor_minimo_rca,              -- opcional, trava por RCA
  importado_em
)

-- Usuários têm tipo (no cadastro)
-- Estende tabela users existente:
users.tipo_persona            -- 'gerente' | 'ggv' | 'supervisor' | NULL (só admin)
users.cod_referencia          -- código do gerente/ggv/supervisor (FK lógica)
```

**Fallback de meta:** se `objetivos_industria` está vazia para um período → usa `vendas_importadas` da Base Comparativa do ano anterior como meta. Documenta isso no card ("Meta inferida do histórico 2025").

### 3.4. O que MORRE

- Tabela `objetivos_importados` atual
- Views `vw_obj_rca_fornecedor`, `vw_obj_supervisor`, `vw_obj_rca_produto`
- Endpoint `/api/objetivos/upload-csv` antigo
- Painéis web "Objetivo RCA" e "Objetivo Supervisor"
- Lógica do farol 3 cores (75/100)
- Migração: dados atuais são **descartados** (não convertidos)

### 3.5. O que SOBREVIVE

- Stack: Go 8087 + React 3087 + Postgres + Coolify
- Sistema de cadastro de Gestores, RCAs, Empresas, Grupos, Ambientes
- Autenticação web JWT (estende com `tipo_persona`)
- URL pública `/m/CNPJ/SUP/cod` do ION VENDAS
- Visual "Clean Professional"

---

## 4. Indicadores e fórmulas

### 4.1. Positivação

```
Positivados = COUNT(DISTINCT cod_cli WHERE qt >= trava_minima_qt[cod_fornec])
Base        = qtcli_rca (declarado no CSV) OU COUNT(DISTINCT cod_cli) na base do RCA
Positivação % = Positivados / Base × 100
```

**Trava mínima:** se `industrias_config.trava_minima_qt = 3` para Alpargatas, vender 1 unidade NÃO conta como positivado. Default = 1 quando não configurada.

### 4.2. Média de Itens por Cliente

```
Mix = SUM(COUNT DISTINCT cod_prod por cod_cli WHERE qt > 0) / COUNT(DISTINCT cod_cli WHERE qt > 0)

Exemplo (Alpargatas):
  CLI 1 → 3 produtos diferentes
  CLI 2 → 5 produtos diferentes
  CLI 3 → 10 produtos diferentes
  Mix = (3+5+10) / 3 = 6 itens/cliente
```

### 4.3. Média Faturado vs Transmitido

Duas médias separadas no card:
- **Faturado** = SUM(pvenda WHERE estado='FATURADO')
- **Transmitido** = SUM(pvenda WHERE estado='TRANSMITIDO')

### 4.4. Farol 2 cores

- 🟢 **Verde:** % atingimento ≥ 100% (atingiu meta)
- 🔴 **Vermelho:** % atingimento < 100% (não atingiu)
- Sem zona amarela.

---

## 5. Visões hierárquicas (3 visões)

### 5.1. Visão 01 — Fornecedor-led (Supervisor / GGV)

`Fornecedor → GGV → Supervisor → RCA → Cliente → Produto`

Persona: SUPV ou GGV vê o panorama "do fornecedor pra dentro". Útil quando o foco é "como está cada indústria".

### 5.2. Visão 02 — RCA-led (Supervisor / GGV)

`Supervisor → RCA → Fornecedor → Cliente → Produto`

Persona: SUPV / GGV. Útil quando o foco é "como está cada RCA com suas indústrias".

### 5.3. Visão 03 — Diretoria Grupo (CEO)

`Fornecedor (consolidado do grupo) → Empresa → UF → GGV → Supervisor → RCA → Cliente → Produto`

Persona: CEO / diretoria. Mostra total do fornecedor agregando TODAS as empresas do grupo. Drill por empresa, depois UF, depois GGV, depois supervisor, RCA, cliente, produto.

### 5.4. Toggle no painel

GGV e Supervisor escolhem entre V01 e V02 (abas Fornec/RCA, mantém comportamento atual). Diretoria tem apenas a V03 (sem toggle).

---

## 6. Personas e autenticação

### 6.1. Quem tem login web

- **Gerente Geral de Vendas (GGV)** — vê todos os supervisores sob ele
- **Gerente** — nível intermediário, vê seus supervisores
- **Supervisor** — vê seus RCAs
- **Diretoria/CEO** — vê tudo do grupo

### 6.2. Quem NÃO tem login

- **RCA** — acessa via URL pública ION (`/m/CNPJ/RCA/cod`)
- **Visualização ION** — continua funcional como hoje (compatibilidade obrigatória)

### 6.3. Tipo de persona no cadastro de usuário

Campo `users.tipo_persona` definido na criação do usuário (Cadastros → Usuários):
- `gerente` | `ggv` | `supervisor` | `diretoria` | `admin` (admin = MASTER do sistema)

Campo `users.cod_referencia` aponta para o código operacional (cod_gerente, cod_ggv, cod_supervisor) — usado para filtrar visão.

### 6.4. Escopo de visão por persona

| Persona | Vê |
|---|---|
| Supervisor | Só seus RCAs |
| Gerente | Só seus supervisores |
| GGV | Só seus gerentes |
| Diretoria | Tudo do grupo (todas empresas) |
| Admin | Tudo + cadastros |

---

## 7. Fases de execução

### **Fase 0 — Maquete HTML local (1 sprint)**

**Entregável:** `/maquete/index.html` standalone + JS de processamento + amostras de CSV.

- HTML/Tailwind via CDN (sem React build pra simplificar)
- Importação CSV no browser (papaparse)
- Renderiza todas 4 visões hierárquicas com drill-down funcional
- Todos os indicadores calculados em JS (positivação, média itens, faturado vs transmitido, % vs meta)
- Farol 2 cores
- Permite carregar 1 mês Base Comparativa + 1 mês Base Atual e simular
- Gestor abre no Chrome/Edge local e experimenta
- **Saída:** aprovação ou ajustes — antes de qualquer linha de Go

### **Fase 1 — Schema novo + Importação (1 sprint)**

- Migration: cria `vendas_importadas`, `industrias_config`, `objetivos_industria`
- Migration: DROP `objetivos_importados` e views relacionadas
- Migration: estende `users` com `tipo_persona` e `cod_referencia`
- Endpoint `POST /api/v2/import/vendas` (aceita novo CSV, SSE de progresso)
- Endpoint `POST /api/v2/import/objetivos-industria` (CSV simples com metas)
- Endpoint `POST /api/v2/import/industrias-config` (opcional, com programa e trava)
- UI de importação atualizada (3 abas: Vendas, Objetivos, Configuração)

### **Fase 2 — Views materializadas + Endpoints de leitura (1 sprint)**

- 4 materialized views (uma por visão hierárquica) com TODOS os indicadores pré-calculados
- Endpoints públicos `/api/farol/v2/*` (para ION via CNPJ)
- Endpoints autenticados `/api/farol/v2/web/*` (para personas com login)
- Refresh automático pós-import
- Índices otimizados para os filtros principais (empresa, periodo, fornecedor, supervisor, rca)

### **Fase 3 — Frontend React integrado (1-2 sprints)**

- Substitui o JS mock da maquete por chamadas à API
- Implementa as 4 visões com drill-down
- Auth por persona (Supervisor / Gerente / GGV / Diretoria)
- Escopo de visão automático (Supervisor só vê seu escopo, etc.)
- Migra/elimina painéis "Objetivo RCA" e "Objetivo Supervisor" antigos
- Mantém Farol mobile via WebView ION funcional

### **Fase 4 — Features avançadas (1 sprint)**

- Sparkline de evolução no card (mensal/trimestral)
- YoY (atual vs mesmo período do ano anterior usando Base Comparativa)
- Forecast de fechamento (regra de 3 ponderada pelo ritmo)
- Ranking (top melhores / top piores por escopo)
- Multi-período comparativo (selecionar 2-3 períodos lado a lado)

---

## 8. Decisões consolidadas

| # | Decisão | Valor |
|---|---|---|
| 1 | Threshold farol | Verde ≥ 100% / Vermelho < 100% (2 cores) |
| 2 | Indústria = Fornecedor | Sim, mesma coisa (cod_fornec) |
| 3 | Base clientes RCA | No mesmo CSV (QT=0, PVENDA=0 para não-compradores) |
| 4 | Persistência Comparativa | Anual rotativa — Atual 2026 vira Comparativa em 2027 |
| 5 | Login web | Até Supervisor; tipo definido no cadastro do usuário |
| 6 | Diretoria | Fornec do grupo → Empresa → UF → GGV → SUPV → RCA |
| 7 | Migração dados atuais | Descartados |
| 8 | Maquete | Completa (todas 4 visões + drill-down) |

---

## 9. Restrições técnicas (não negociáveis)

- Stack continua: Go 8087 + React 3087 + Postgres + Coolify
- ION VENDAS continua acessando via `/m/CNPJ/...` (compatibilidade)
- Materialized views como base de leitura (não migrar para OLAP)
- Padrão visual "Clean Professional" mantido e estendido

---

## 10. Status

Documento fechado. Pronto para servir como idea document do `/gsd:new-project --auto`.
