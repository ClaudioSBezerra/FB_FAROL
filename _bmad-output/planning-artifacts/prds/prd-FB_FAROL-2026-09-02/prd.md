---
title: Painel de Gestão de Metas por Indústria
status: final
created: 2026-09-02
updated: 2026-09-02
---

# Painel de Gestão de Metas por Indústria

## Visão

Hoje o GGV e o Supervisor ficam "às cegas" quanto ao que acontece em campo: não têm visão sintetizada do que o RCA está oferecendo/vendendo ao cliente, nem do realizado da semana/mês em tempo hábil — dependem exclusivamente de relatórios manuais do WinThor. Isso é especialmente crítico nos programas de metas ponderadas por indústria (ex: Programa Único Unilever), que hoje são apurados manualmente em planilha e decidem bônus/pagamento real do fornecedor.

O módulo dá a GGV e Supervisor visão completa e em tempo real de Meta × Realizado por indústria, tanto no painel web quanto no mobile (mesmo padrão do painel público já existente do Farol) — substituindo o processo manual de planilha por um motor de apuração automático, extensível a qualquer indústria.

## Loop de uso central

Um Supervisor (ou GGV) abre o painel — web ou mobile — e compara a meta da indústria com o realizado, em diferentes recortes de tempo: dia anterior, semana, mês, ano corrente, e **projeção de fechamento até o fim do ano corrente** (capacidade nova — o Farol hoje não projeta). Ao identificar um recorte abaixo da meta, cobra a ação em campo (ex: liga pro RCA, prioriza uma visita).

> **UJ-1 — Waliston (Supervisor/CRV V7-GO) cobra a meta em campo.** Abre o Farol mobile de manhã, entra no painel do Programa Único Unilever. Vê Cobertura e Sortimento comparados à meta, alterna entre dia anterior/semana/mês/ano corrente, olha a projeção de fechamento. Percebe que uma Rede está abaixo da meta de Cobertura — liga pro RCA responsável e cobra a visita antes do fim do mês.
>
> **GGV** usa o mesmo painel num nível de rollup acima — em vez de ver as Redes de um Supervisor/CRV só, vê o agregado de todos os CRVs sob ele, com o mesmo drill-down disponível caso precise descer até uma Rede/Cliente específico. O loop de ação é o mesmo (identifica quem está abaixo da meta, cobra), só muda o ponto de entrada na hierarquia.

## Métricas de Sucesso

- Uso regular do painel por GGV e Supervisor (web e mobile), não só consulta esporádica.
- Redução de casos de "não sabíamos que a Rede/Fornecedor tinha caído" — a informação chega antes do fechamento do mês, não depois.
- Substituição gradual do processo manual em planilha pelo motor de apuração automático.

*(Aceito no nível de detalhe acima — sem métrica numérica-alvo definida ainda; ver `.memlog.md`.)*

## Contexto de referência

A primeira indústria a entrar no módulo é a Unilever (fornecedores 131-Foods e 396-HC), com objetivos **independentes** entre os dois fornecedores. A especificação de referência (documentos em `/home/claudio/uploads`, ver `.memlog.md` para o registro completo da leitura) descreve duas métricas:

- **Cobertura (Positivação):** uma Rede (grupo de CNPJs/lojas) é considerada coberta quando a média de compra entre suas lojas ultrapassa um limiar em R$ (limiar próprio por fornecedor). Meta é uma contagem absoluta de Redes cobertas, em faixas (ex: Faixa 3/2/1, crescente).
- **Sortimento (Mix Positivado):** média de itens (EAN) distintos comprados entre as lojas da Rede, de uma lista de itens válida enviada mensalmente pelo fornecedor. Um EAN só conta como positivado numa loja com ≥3 unidades vendidas (regra que não se aplica a itens vendidos em caixa/pacote/display). Meta também em faixas, podendo variar por fornecedor.

**Importante:** este é o molde de partida, não o modelo final — o sistema é um **framework genérico**: cada indústria define seu próprio conjunto de métricas, e 2+ indústrias podem reutilizar o mesmo Tipo de Métrica (ex: "Cobertura por Rede") com parâmetros próprios.

## Escopo organizacional e hierarquia

O programa Unilever é restrito a um recorte (UF=GO, GGVs específicos) — a configuração de meta precisa suportar recorte parcial da operação, não só "empresa inteira".

Hierarquia de drill-down: **GGV (Gerente) → CRV (Supervisor) → RCA → Rede (grupo de CNPJs) → Cliente (CNPJ)**. O nível Rede entre RCA e Cliente é uma combinação que o Farol não tem hoje (V03/Gerência vai direto RCA→Cliente; V06/Rede é uma árvore à parte, sem cruzar com a hierarquia organizacional) — este módulo precisa dessa hierarquia combinada.

## Glossário

| Termo | Significado |
|---|---|
| **GGV** | Gerente — mesmo nível de `cod_gerente` no Farol. |
| **CRV** | Supervisor — mesmo nível de `cod_supervisor` no Farol. **CRV é só o nome que a Unilever usa no contrato pro mesmo papel** — confirmado com o usuário, não é um nível organizacional diferente. |
| **RCA** | Representante de vendas em campo — mesmo `cod_rca` do Farol. |
| **Rede** | Grupo de CNPJs de um mesmo cliente/grupo econômico — mesmo conceito de `cod_cliprinc` (V06 do Farol). |
| **Cliente / CNPJ** | Ponto de venda individual dentro de uma Rede. |
| **Tipo de Métrica** | Definição reutilizável de uma forma de calcular atingimento (ex: "Cobertura por Rede"). Ver FR1-FR3. |
| **Vínculo** | Associação entre uma Indústria/Fornecedor e um Tipo de Métrica, com seus parâmetros específicos (limiares, listas, faixas). Ver FR4-FR7. |
| **Faixa** | Nível de meta dentro de um vínculo (ex: Faixa 3/2/1) — recurso genérico, qualquer Tipo de Métrica pode ter uma ou várias. |
| **Vigência** | Período (normalmente um mês) em que um conjunto de metas/listas/parâmetros vale. |
| **Realizado** | Resultado apurado de uma métrica para um recorte e período — mesmo conceito usado em FR14/FR17 ("resultado apurado"). |

## Requisitos Funcionais

### Catálogo de Tipos de Métrica (Admin)

- **FR1.** O sistema deve permitir cadastrar Tipos de Métrica reutilizáveis, cada um com nome, descrição, nível de agregação em que é avaliado (ex: Rede, Cliente/CNPJ) e a lista de parâmetros que uma instância desse tipo exige preencher.
- **FR2.** O sistema deve vir com os Tipos de Métrica do caso de referência Unilever pré-cadastrados: "Cobertura por Rede" (limiar de valor médio de compra) e "Sortimento por Rede" (contagem média de itens distintos positivados, contra uma lista de itens válida, com regra de quantidade mínima por item).
- **FR3.** Um Tipo de Métrica deve poder ser reutilizado por 2 ou mais indústrias/fornecedores diferentes, cada instância com seus próprios parâmetros (limiares, listas, faixas).

> **Teste de generalidade (não é requisito, é validação do FR1):** os dois Tipos de Métrica do Unilever calculam uma MÉDIA num nível de agregação (Rede) contra um LIMIAR. Pra provar que o modelo de parâmetros do FR1 não é só isso com nome genérico, um Tipo de Métrica hipotético diferente — ex: "Frequência de Visita por Cliente" (RCA precisa visitar cada CNPJ da lista pelo menos N vezes no período; nível de agregação = Cliente, não Rede; parâmetro = nº mínimo de visitas, não R$/EAN) — precisa caber no mesmo modelo sem exigir campo novo na tabela de Tipos de Métrica, só um novo conjunto de parâmetros. Isso fica como critério de aceite pra quem desenhar o modelo de dados na fase de arquitetura. **Grau de confiança:** validado em desenho, não em produção — só 1 indústria real rodando até o momento (Unilever, 2 métricas do mesmo formato "média vs limiar por Rede").

### Configuração de Metas por Indústria (Admin)

- **FR4.** O sistema deve permitir vincular uma Indústria — ou um Fornecedor específico dentro dela, já que fornecedores da mesma indústria podem ter metas independentes (caso Unilever 131 × 396) — a um ou mais Tipos de Métrica.
- **FR5.** Cada vínculo Indústria/Fornecedor × Tipo de Métrica deve permitir definir: valores de meta por faixa (múltiplas faixas, ex: Faixa 1/2/3), o período de vigência (a meta pode mudar mês a mês — "mobilidade de ajuste, otimizando o processo"), e o recorte organizacional coberto (UF, GGVs específicos, ou empresa toda). **Os valores de meta de uma vigência já fechada seguem a mesma regra de congelamento do FR17** — mudar a meta de um mês fechado não pode alterar retroativamente o resultado já apurado daquele mês.
- **FR6.** O tipo de venda válido para a apuração (ex: só Tipo 1 e 9 no caso Unilever, diferente do "Líquido" padrão do Farol) deve ser configurável por vínculo, não fixo no sistema.
- **FR7.** Faixas de meta (múltiplos níveis, ex: 3/2/1) são um recurso genérico do framework — qualquer Tipo de Métrica pode ter uma ou várias faixas; não é exclusivo de Cobertura/Sortimento.

### Importação de Metas (Admin)

- **FR8.** O sistema deve permitir importar valores de meta (por vínculo Indústria/Fornecedor × Tipo de Métrica, por faixa, por período de vigência) via upload de arquivo CSV.
- **FR9.** O sistema deve validar o CSV antes de aplicar — linhas com erro reportadas claramente, sem aplicar parcialmente um lote com erro. Dado o impacto financeiro do cálculo (bônus de fornecedor), falha na importação nunca pode ser silenciosa.
- **FR10.** *(Fora de escopo deste PRD — direção futura registrada no addendum)* Integração direta com a base Oracle da JC como fonte de metas, substituindo a importação manual por CSV.

### Listas Mensais Válidas (Admin)

- **FR11.** O sistema deve permitir importar/manter, por Indústria/Fornecedor e por período de vigência, a lista de Clientes Válidos — Redes (grupos de CNPJ) com a atribuição de GGV/CRV/RCA responsável por cada CNPJ. **Todo CNPJ deve ter RCA vinculado** — é regra de qualidade de dado validada na importação (FR9), não um caso que a navegação da hierarquia precisa tratar com fallback.
- **FR12.** O sistema deve permitir importar/manter, por Indústria/Fornecedor e por período de vigência, a lista de Itens Válidos — EAN, com mapeamento para o(s) cod_prod interno(s) correspondente(s) (um EAN pode ter mais de uma variante/embalagem mapeada). **O tipo de embalagem (UN, CX, Pacote, Display) é um atributo obrigatório de cada mapeamento** — não é só detalhe de variante de SKU, é o que decide se a regra de quantidade mínima do Tipo de Métrica "Sortimento" (FR2) se aplica ou não a esse item.
- **FR13.** Listas de um período já fechado não podem ser alteradas de forma que mude retroativamente um cálculo já apurado — precisa de snapshot mensal congelado.

### Motor de Apuração

- **FR14.** O sistema deve calcular, mensalmente (e sob demanda para o mês corrente/parcial), o Realizado de cada Tipo de Métrica configurado, por nível hierárquico (GGV → CRV → RCA → Rede → Cliente/CNPJ), lendo as bases de Faturado e Transmitido do Farol.
- **FR15.** O cálculo deve suportar 3 visões de fluxo: Faturado, Transmitido (Emitido) e a Soma dos dois — capacidade nova; hoje o Farol só alterna entre um ou outro.
- **FR16.** O cálculo deve respeitar o tipo de venda válido configurado por vínculo (FR6), não o "Líquido" padrão do Farol.
- **FR17.** Meses fechados (fora do mês corrente) devem ter seu resultado congelado — recalcular um mês fechado só deve acontecer por ação explícita (ex: reprocessamento manual disparado por um gestor), nunca de forma automática/silenciosa por causa de uma lista ou meta atualizada depois.
- **FR18.** O motor deve calcular a projeção de fechamento do ano corrente para cada métrica. **Método v1 (ritmo linear):** `projeção = realizado até a data ÷ dias decorridos no período × dias totais do período`. Exemplo: realizado de R$ 45.000 em 15 dias de um mês de 30 dias → projeção de fechamento = R$ 90.000. *(Refinamento futuro, fora de escopo deste PRD: método ajustado por sazonalidade, reaproveitando o índice por Seção construído para o SmartPick nesta mesma sessão — ver `infra`/memória de sessão.)*
  - **FR18a.** A projeção é calculada **independentemente em cada nível da hierarquia** (GGV, CRV, RCA, Rede, Cliente) — a partir do realizado direto daquele nível, nunca somando as projeções dos níveis filhos. Isso é o mesmo princípio já usado no motor de cálculo existente do Farol (nunca somar métrica pré-agregada de um nível pra derivar outro — ver `farol_kpi_totalizador_bugs.md`).

### Painel Web

- **FR19.** O sistema deve exibir, para cada Indústria/Fornecedor configurado, um painel de Meta × Realizado por Tipo de Métrica, navegável pela hierarquia GGV → CRV → RCA → Rede → Cliente.
- **FR19a.** O painel deve destacar explicitamente **quanto falta para bater a meta** (delta Meta − Realizado, não só os dois números lado a lado) — é esse número que direciona a ação de cobrança em campo (ver UJ-1).
- **FR20.** O painel deve permitir alternar entre Faturado, Transmitido e Soma (FR15).
- **FR21.** O painel deve permitir comparar múltiplos recortes de tempo (dia anterior, semana, mês, ano corrente) e visualizar a projeção de fechamento (FR18).

### Painel Mobile

- **FR22.** O sistema deve expor uma versão mobile do painel de metas por indústria, no mesmo padrão de acesso do painel público já existente do Farol (link direto por Supervisor/GGV, sem exigir login completo).
- **FR23.** O painel mobile deve oferecer os mesmos recortes de tempo e a projeção de fechamento do painel web (FR21), adaptados para tela pequena/uso em campo.

## Requisitos Não-Funcionais

- **NFR1.** Toda alteração de meta, faixa, lista válida ou tipo de venda configurado deve ficar auditável (quem alterou, quando, valor anterior) — dado o impacto financeiro em contrato de fornecedor.
- **NFR2.** Acesso de edição (Catálogo de Tipos de Métrica, Configuração de Metas, Importação, Listas Válidas) deve ser restrito a um papel de gestão/administração, distinto do acesso de visualização (GGV/Supervisor nos painéis) — reaproveitando os papéis de acesso já existentes no Farol (`gestor_filial`/`gestor_geral`), sem criar um papel novo.
- **NFR3.** A apuração de meses fechados deve ser reprodutível — dado o mesmo snapshot de dados, o resultado de um mês fechado não muda entre duas execuções.
- **NFR4.** Não há SLA crítico de prazo para a apuração mensal estar pronta (confirmado com o usuário) — a apuração do mês corrente é sempre parcial/em andamento até o fechamento do mês. **Risco aceito:** sem SLA, uma apuração que trave silenciosamente por dias durante o mês só seria percebida no fechamento — o usuário aceitou esse risco conscientemente; não é lacuna, é decisão.

## Fora de escopo (neste PRD)

- Integração direta com Oracle como fonte de metas (FR10) — fica como direção futura, hoje é CSV.
- Uso deste módulo como base para um futuro "Farol de Compras" — mencionado pelo usuário como direção estratégica, mas não é requisito funcional deste PRD. O modelo de dados deve evitar decisões que fechem essa porta, sem se comprometer com o escopo agora.

## Questões em aberto

Nenhuma pendente — as 3 levantadas durante a Discovery (faixas genéricas, SLA de apuração, papel de gestão) foram resolvidas com o usuário (ver `.memlog.md`).

