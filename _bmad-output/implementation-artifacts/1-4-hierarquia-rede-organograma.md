---
epic: 1
story: 4
story_key: 1-4-hierarquia-rede-organograma
baseline_commit: 6972c24
---

# Story 1.4: Hierarquia Rede combinada ao organograma

Status: review

## Story

Como admin,
Eu quero que uma Rede (grupo de CNPJs) resolva corretamente para o GGV/CRV/RCA responsável na hierarquia organizacional,
Para que o cálculo e o painel (Épicos 4-6) consigam navegar GGV → CRV → RCA → Rede → Cliente sem ambiguidade.

Fonte: [`epics.md`](../planning-artifacts/epics.md) (Épico 1, Story 1.4) · "Escopo organizacional e hierarquia" do PRD · GitHub issue #9.

## ⚠️ Achado durante a implementação — mesma classe de problema da Story 1.3

O AC original desta story pedia pra "aplicar o modelo de dados... com dados de teste inseridos diretamente", mas ao desenhar a tabela de verdade percebi um problema estrutural: **FR11 é explícito que a lista de Clientes Válidos (Rede + CNPJ + RCA responsável) é escopada por Vínculo Indústria/Fornecedor × período de vigência** — não é uma hierarquia global única do Farol, é dado mensal por programa. Isso significa que a tabela real precisa de `vinculo_id` (Épico 2, Story 2.1) e `vigencia_id` (Épico 2, Story 2.2) como FK — nenhum dos dois existe ainda nesta altura da sequência.

Criar a tabela agora, sem essas FKs, e alterá-la depois pra adicioná-las violaria o princípio "criar tabela só quando a story que precisa dela existir" (regra do próprio `bmad-create-story`) — e além disso ia contra a decisão de design já tomada na Story 1.1 (nenhuma coluna nova em tabela existente por causa de uma necessidade que já era previsível).

**Decisão de arquitetura tomada agora (isto É o trabalho de arquitetura embutido no Épico 1, não um adiamento vazio):**

1. A tabela real (`farol.clientes_validos` ou nome equivalente) nasce na **Story 3.2** (Épico 3, Importação de Clientes Válidos), com FK pra `vinculo_id` e `vigencia_id` do Épico 2.
2. **Modelo decidido:** `rede_nome` é `TEXT` livre (não uma FK pra uma tabela mestra `farol.redes`) — porque a definição de "Rede" varia por fornecedor/programa (é o fornecedor que manda a lista mensal, não o Farol que possui um conceito canônico de Rede). Isso é a decisão que evita fechar a porta pro "Farol de Compras" citada no AC original: se cada fornecedor tem sua própria noção de Rede, uma FK rígida pra uma tabela `farol.redes` compartilhada entre fornecedores seria a decisão errada — prenderia o modelo a uma hipótese não validada.
3. A resolução RCA→CRV→GGV **não precisa de tabela nova** — já existe no Farol via `cod_rca`/`cod_supervisor`/`cod_gerente` (hierarquia organizacional real, usada em V02/V03). A tabela de Clientes Válidos só precisa guardar `cod_rca` por CNPJ; subir pra CRV/GGV é um JOIN com as tabelas organizacionais já existentes, não dado duplicado.
4. **Teste de generalidade equivalente ao da Story 1.1, mas pra este pedaço**: o desenho acima (Rede = texto livre por vínculo, RCA por CNPJ, resolução de CRV/GGV via JOIN com organograma existente) funciona pra qualquer fornecedor futuro sem exigir schema novo — só novas linhas.

Esta decisão fica registrada aqui e será o Dev Notes de partida da Story 3.2 quando ela for implementada.

## Acceptance Criteria

1. ~~Modelo de dados aplicado com dados de teste diretos~~ — **substituído**: decisão de arquitetura documentada e validada contra o teste de generalidade (funciona pra qualquer fornecedor sem schema novo).
2. O modelo de dados decidido evita decisões que fechem a porta pro "Farol de Compras" futuro — `rede_nome` como texto livre por vínculo, não uma tabela mestra compartilhada.
3. A resolução RCA→CRV→GGV reaproveita a hierarquia organizacional já existente no Farol, sem duplicar dado.

## Tasks / Subtasks

- [x] **Task 1: Decisão de arquitetura — onde a tabela nasce e seu shape** (AC: 1, 2, 3)
  - [x] Confirmado: tabela real nasce na Story 3.2, com FK pra vínculo (Épico 2) e vigência (Épico 2) — não em Épico 1
  - [x] Confirmado: `rede_nome TEXT` livre, não FK pra tabela mestra de Redes
  - [x] Confirmado: RCA→CRV→GGV via JOIN com hierarquia organizacional existente, sem tabela nova pra isso

## Dev Agent Record

### Agent Model Used

Claude Sonnet 5

### Completion Notes List

Sem código nesta story — é puramente uma decisão de arquitetura (o trabalho de arquitetura embutido no Épico 1, conforme pedido pelo usuário). A decisão está documentada aqui e deve ser o ponto de partida do Dev Notes da Story 3.2.

### File List

(nenhum arquivo alterado nesta story)

### Change Log

- 2026-09-02: Decisão de arquitetura tomada e documentada — shape da tabela de Clientes Válidos (Rede×organograma) definido, execução real adiada pra Story 3.2 por dependência de FK genuína (vínculo/vigência do Épico 2).
