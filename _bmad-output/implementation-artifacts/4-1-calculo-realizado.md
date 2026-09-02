---
epic: 4
story: 1
story_key: 4-1-calculo-realizado
baseline_commit: 3c4272e
---

# Story 4.1: Cálculo do Realizado por Tipo de Métrica e nível hierárquico

Status: review

## Story

Como Supervisor/GGV,
Eu quero que o sistema calcule automaticamente, mensalmente e sob demanda para o mês corrente, o Realizado de cada Tipo de Métrica configurado em cada nível da hierarquia (GGV→CRV→RCA→Rede→Cliente),
Para que eu não dependa de relatório manual do WinThor pra saber onde estou.

Fonte: [`epics.md`](../planning-artifacts/epics.md) (Épico 4, Story 4.1) · FR14, FR16 · GitHub issue #18. Story mais complexa do módulo até agora — o motor de apuração de verdade.

## Descobertas e decisões de arquitetura tomadas nesta story

**1. Lacuna real no modelo: faltava um "seletor de algoritmo".** `parametros_schema` (Story 1.1) descreve o SHAPE dos parâmetros, mas nunca disse qual código Go calcula cada Tipo de Métrica. Migration 222 adiciona `formula_codigo` a `farol.tipos_metrica`, com dispatch em `farol_metas_calculo.go`. Isso é reconhecido explicitamente como limite do "framework genérico": o DADO é genérico (Story 1.1 provou isso), a MATEMÁTICA exige código novo por shape — um Tipo de Métrica novo (como o hipotético "Frequência de Visita") vai sempre precisar de uma calculadora Go nova pra ser apurado de verdade.

**2. Schema real de `vendas_faturadas`/`vendas_transmitidas` diferente do esperado.** Ao escrever as queries, descobri que `tipo_base` (ATUAL/COMPARATIVA) foi **removida** do schema (migration 158 — "atual vs comparativa é propriedade da CONSULTA, não do dado"), e existe uma coluna `cnpj` **separada** de `cod_cli` (migration 160, criada exatamente pra positivação precisa — "positivação por cod_cli era impreciso"). Usei `cnpj` (não `cod_cli`) e removi o filtro de `tipo_base` das queries — descoberta feita rodando os testes contra o banco local de verdade, não assumindo o schema da migration original.

**3. "Líquido padrão do Farol" = nenhum filtro de tipo_venda.** Confirmado: `vendas_faturadas`/`vendas_transmitidas` não têm as colunas de Bruto (`pv_bonif` etc. — essas só existem nas `agg_fat_*_mes`). O dado bruto já É o "Líquido". FR16 vira: `tipos_venda_validos` vazio = sem filtro adicional; preenchido = `WHERE tipo_venda = ANY(...)`.

**4. Atribuição de Rede a um RCA "representante".** Uma Rede pode ter CNPJs de RCAs diferentes (multi-loja). Decidi: RCA do CNPJ de menor valor (ordem alfabética) — regra simples e determinística, documentada como **assunção a validar com o Claudio/PM quando o painel (Épico 5/6) estiver em revisão** — é isso que o botão "ligar pro RCA" (UJ-1) vai usar.

**5. Rollup por nível nunca soma valor pré-agregado** (FR18a) — RCA/CRV/GGV são recalculados a partir da lista de Redes (grão atômico), nunca somando um "Realizado de RCA" que não existe como conceito primário. Hierarquia CRV/GGV resolvida lendo a linha de venda mais recente daquele RCA (mesmo padrão que o motor V2 já usa — `farol_v2_api.go` não tem tabela separada de RCA→Supervisor, deriva do dado denormalizado).

## Acceptance Criteria

1. Dado um vínculo com metas e listas válidas configuradas, calculo o Realizado de uma vigência — resultado por Rede, agregado por RCA/CRV/GGV.
2. **Verificado com o exemplo numérico exato do PRD** (Rede 1, 4 lojas, compras R$1.000/0/20.000/40.000 → média R$15.250) — não é só "roda sem erro", é matematicamente correto contra a especificação de referência.
3. Cálculo usa os tipos de venda configurados no vínculo (FR16), não "Líquido" padrão.
4. Mês corrente (vigência cuja `data_fim` ainda não passou) é marcado como parcial.

## Tasks / Subtasks

- [x] **Task 1: Migration 222 — `formula_codigo`** (AC: 1, 2)
  - [x] ALTER + backfill dos 2 tipos seedados (`cobertura_rede`, `sortimento_rede`)
  - [x] `farol_tipos_metrica.go` estendido (campo + CRUD) — **bug pego e corrigido**: um SELECT esquecido (a query "antes" do PUT) não tinha a coluna nova, causando 500 em edição — achado rodando os testes existentes de Tipos de Métrica antes de fechar
- [x] **Task 2: Motor de cálculo** (AC: 1, 2, 3, 4)
  - [x] `backend/handlers/farol_metas_calculo.go` — `CalcularRealizado` (dispatch), `calcularCoberturaPorRede`, `calcularSortimentoPorRede`, `agregarPorNivel`, `resolverHierarquiaRCA`
  - [x] `GET /api/farol/metas-realizado?vinculo_id=&vigencia_id=&fluxo=&nivel=` — acesso `somente_leitura` (visualização ampla, NFR2)
- [x] **Task 3: Testes — o exemplo do PRD é o mais importante** (AC: 1, 2, 3, 4)
  - [x] `TestCalcularRealizado_Cobertura_ExemploDoPRD` — **reproduz o número exato do PRD (R$15.250)** — PASS
  - [x] `TestCalcularRealizado_Cobertura_AbaixoDoLimiar_NaoAtinge` — PASS
  - [x] `TestCalcularRealizado_RespeitaTipoVendaDoVinculo` — PASS
  - [x] `TestCalcularRealizado_Sortimento_RegraQuantidadeMinima` — PASS
  - [x] `TestCalcularRealizado_AgregacaoPorRCA` — confirma rollup sem duplicar/perder Rede — PASS
  - [x] `TestCalcularRealizado_MesCorrenteEhParcial` — PASS
  - [x] `TestCalcularRealizado_SemClientesValidos_ErroClaro` — PASS
  - [x] **Descoberta durante o teste**: schema real diverge da migration original (`tipo_base` removida, `cnpj` separado de `cod_cli`) — corrigido antes de fechar, documentado acima

## Dev Agent Record

### Agent Model Used

Claude Sonnet 5

### Completion Notes List

7 testes novos, todos passando — incluindo verificação matemática exata contra o exemplo do PRD. 2 bugs reais pegos e corrigidos durante a implementação (SELECT desatualizado em Tipos de Métrica; schema real de vendas_faturadas diferente do assumido). Regressão: mesmas falhas pré-existentes (`TestBIParidadeComCards`, `TestBICache`, `TestFarolV2Cards_FiltroIndustria_MesCompletoIgnoraAggCorrompida` — esta última intermitente na suíte completa mas reproduz de forma consistente isolada, confirmado não relacionada a esta story).

### File List

- `backend/migrations/222_tipos_metrica_formula.sql` (novo)
- `backend/handlers/farol_metas_calculo.go` (novo)
- `backend/handlers/farol_metas_calculo_test.go` (novo)
- `backend/handlers/farol_tipos_metrica.go` (modificado — formula_codigo + bug fix)
- `backend/main.go` (modificado)
- `frontend/src/pages/ConfigTiposMetrica.tsx` (modificado — campo formula_codigo)

### Change Log

- 2026-09-02: Motor de apuração implementado e verificado contra o exemplo numérico exato do PRD. Descobertas de schema real documentadas (tipo_base removida, cnpj separado de cod_cli).
