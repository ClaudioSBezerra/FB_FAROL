---
stepsCompleted: [1, 2, 3]
inputDocuments: ["_bmad-output/planning-artifacts/prds/prd-FB_FAROL-2026-09-02/prd.md", "_bmad-output/planning-artifacts/prds/prd-FB_FAROL-2026-09-02/addendum.md"]
updated: 2026-09-29
---

# FB_FAROL - Epic Breakdown

## Overview

This document provides the complete epic and story breakdown for o módulo **Painel de Gestão de Metas por Indústria**, decompondo os requisitos do PRD (`prd-FB_FAROL-2026-09-02`) em stories implementáveis. Não existe Architecture.md nem doc de UX separados para este módulo — por decisão explícita do usuário, o trabalho de arquitetura (modelo de dados, validação do "teste de generalidade" do FR1) está embutido como um dos épicos desta lista, em vez de rodar como fase prévia separada.

## Pendências de Validação

- **Modelo Excel do fornecedor (visualização CRV/RCA):** o Claudio pediu à Unilever uma planilha preenchida mostrando a visualização esperada do painel nos níveis CRV(Supervisor) e RCA. As stories do Épico 5 (Painel Web) e Épico 6 (Painel Mobile) que definem layout/disposição de dados na tela devem ser revalidadas contra esse modelo assim que ele chegar — não é bloqueante para desenhar a estrutura de dados/AC funcional, mas é para fechar o design visual final.
- **Projeção de fechamento (FR18) é capacidade adicional, não pedida pelo fornecedor:** diferente de Cobertura/Sortimento (que fazem parte do processo oficial que a Unilever cobra), a projeção é uma capacidade nova do Farol. Decisão: exibir em aba/tela separada dos indicadores oficiais, para não ser confundida com o que o fornecedor efetivamente acompanha.
- **Épico 7 (Numérica, addendum 2026-09-29) tem 5 Questões em aberto no PRD ainda sem resposta da equipe da JC** (nível "Cliente Rede" ambíguo na hierarquia, denominador exato da média de PPA, fornecedor das faixas agregadas de Cobertura, colunas Região/AE/BU do PPA, escopo do reforço Faturado+Emitido) — não bloqueiam o desenho das stories, mas cada uma marca a dependência explícita na story afetada. Ver `prd.md` seção "Questões em aberto".

## Requirements Inventory

### Functional Requirements

**Catálogo de Tipos de Métrica (Admin)**
FR1: O sistema deve permitir cadastrar Tipos de Métrica reutilizáveis, cada um com nome, descrição, nível de agregação em que é avaliado (ex: Rede, Cliente/CNPJ) e a lista de parâmetros que uma instância desse tipo exige preencher.
FR2: O sistema deve vir com os Tipos de Métrica do caso de referência Unilever pré-cadastrados: "Cobertura por Rede" (limiar de valor médio de compra) e "Sortimento por Rede" (contagem média de itens distintos positivados, contra uma lista de itens válida, com regra de quantidade mínima por item).
FR3: Um Tipo de Métrica deve poder ser reutilizado por 2 ou mais indústrias/fornecedores diferentes, cada instância com seus próprios parâmetros (limiares, listas, faixas).

**Configuração de Metas por Indústria (Admin)**
FR4: O sistema deve permitir vincular uma Indústria — ou um Fornecedor específico dentro dela — a um ou mais Tipos de Métrica.
FR5: Cada vínculo Indústria/Fornecedor × Tipo de Métrica deve permitir definir: valores de meta por faixa, o período de vigência, e o recorte organizacional coberto (UF, GGVs específicos, ou empresa toda). Metas de uma vigência já fechada seguem a mesma regra de congelamento do FR17. **Um mesmo vínculo pode ter várias vigências ao longo do ano** (ex: meta de jan-mar diferente de abr-jun) — vigência é um histórico de períodos, não um campo único no vínculo.
FR6: O tipo de venda válido para a apuração (ex: só Tipo 1 e 9 no caso Unilever) deve ser configurável por vínculo, não fixo no sistema.
FR7: Faixas de meta (múltiplos níveis) são um recurso genérico do framework — qualquer Tipo de Métrica pode ter uma ou várias faixas.

**Importação de Metas (Admin)**
FR8: O sistema deve permitir importar valores de meta (por vínculo, por faixa, por período de vigência) via upload de arquivo CSV.
FR9: O sistema deve validar o CSV antes de aplicar — linhas com erro reportadas claramente, sem aplicar parcialmente um lote com erro. Falha na importação nunca pode ser silenciosa.
FR10: *(Fora de escopo deste PRD)* Integração direta com a base Oracle da JC como fonte de metas — direção futura, registrada no addendum.

**Listas Mensais Válidas (Admin)**
FR11: O sistema deve permitir importar/manter, por Indústria/Fornecedor e por período de vigência, a lista de Clientes Válidos — Redes com a atribuição de GGV/CRV/RCA responsável por cada CNPJ. Todo CNPJ deve ter RCA vinculado (regra de qualidade validada na importação, FR9).
FR12: O sistema deve permitir importar/manter, por Indústria/Fornecedor e por período de vigência, a lista de Itens Válidos — EAN, com mapeamento para o(s) cod_prod interno(s) correspondente(s). O tipo de embalagem (UN, CX, Pacote, Display) é atributo obrigatório de cada mapeamento.
FR13: Listas de um período já fechado não podem ser alteradas de forma que mude retroativamente um cálculo já apurado — precisa de snapshot mensal congelado.

**Motor de Apuração**
FR14: O sistema deve calcular, mensalmente (e sob demanda para o mês corrente/parcial), o Realizado de cada Tipo de Métrica configurado, por nível hierárquico (GGV → CRV → RCA → Rede → Cliente/CNPJ), lendo as bases de Faturado e Transmitido do Farol.
FR14a: A janela de apuração deve ser configurável por Tipo de Métrica — mês fechado (padrão atual) ou bimestre móvel (mês corrente + mês anterior, usado pela Numérica, FR25/FR26). Característica do Tipo de Métrica (FR1), não parâmetro do vínculo (FR5).
FR15: O cálculo deve suportar 3 visões de fluxo: Faturado, Transmitido (Emitido) e a Soma dos dois.
FR16: O cálculo deve respeitar o tipo de venda válido configurado por vínculo (FR6), não o "Líquido" padrão do Farol.
FR17: Meses fechados devem ter seu resultado congelado — recalcular só por ação explícita de um gestor, nunca automático/silencioso.
FR18: O motor deve calcular a projeção de fechamento do ano corrente para cada métrica. Método v1 (ritmo linear): `projeção = realizado até a data ÷ dias decorridos no período × dias totais do período`.
FR18a: A projeção é calculada independentemente em cada nível da hierarquia (GGV, CRV, RCA, Rede, Cliente) — a partir do realizado direto daquele nível, nunca somando as projeções dos níveis filhos.

**Painel Web**
FR19: O sistema deve exibir, para cada Indústria/Fornecedor configurado, um painel de Meta × Realizado por Tipo de Métrica, navegável pela hierarquia GGV → CRV → RCA → Rede → Cliente.
FR19a: O painel deve destacar explicitamente quanto falta para bater a meta (delta Meta − Realizado, não só os dois números lado a lado).
FR20: O painel deve permitir alternar entre Faturado, Transmitido e Soma (FR15).
FR21: O painel deve permitir comparar múltiplos recortes de tempo (dia anterior, semana, mês, ano corrente) e visualizar a projeção de fechamento (FR18).

**Painel Mobile**
FR22: O sistema deve expor uma versão mobile do painel de metas por indústria, no mesmo padrão de acesso do painel público já existente do Farol (link direto por Supervisor/GGV, sem exigir login completo).
FR23: O painel mobile deve oferecer os mesmos recortes de tempo e a projeção de fechamento do painel web (FR21), adaptados para tela pequena/uso em campo.

**Numérica — Cobertura e Sortimento por Cliente/CNPJ (addendum 2026-09-29)**
FR24: O sistema deve suportar Tipo de Métrica com nível de agregação Cliente/CNPJ direto (sem Rede) — realiza o nível de agregação que o FR1 já previa e nunca tinha sido implementado.
FR25: Tipo de Métrica "Cobertura Numérica": Cliente/CNPJ considerado coberto quando compra, no bimestre móvel (FR14a), valor ≥ ao limiar da sua Classificação PDV (Num. A=R$100, Num. B=R$50, Num. C=R$15). Exclui venda Tipo 5 (bonificada). Vinculado às mesmas Indústrias FOOD/HC já existentes.
FR26: Tipo de Métrica "Sortimento Numérica (PPA)": só apurado para Classificação A e B. Conta PPAs distintos (FR28) comprados no bimestre móvel, teto de 15. Inclui venda Tipo 5 (bonificada) — inverso do FR25. Regra de quantidade mínima: 3un pra item "UN"; CX/Pacote/Display conta com 1 venda (mesmo padrão do FR12).
FR27: Importação mensal da lista de Clientes Numéricas — CNPJ individual, Classificação PDV (A/B/C), GGV/CRV/RCA responsável, sem cod_princ/Rede. Formato de import diferente do FR11.
FR28: Importação mensal da lista de PPAs — mapeamento cod_prod → PPA (família) → EAN, com embalagem. Importação separada da lista de Itens Válidos (FR12).
FR29: Apuração da Numérica deduz Devolução e Cancelamento, mesmo princípio já usado no motor de Rede.
FR30: Apuração agregada do distribuidor (Cobertura Numérica e Sortimento Numérica) suporta múltiplas Faixas, reaproveitando o mecanismo genérico do FR7.
FR31: Painel (web e mobile) oferece Cobertura Numérica e Sortimento Numérica como opções de Tipo de Métrica dentro de cada Indústria (FOOD/HC) já existente — sem criar filtro de "Indústria" separado.

### NonFunctional Requirements

NFR1: Toda alteração de meta, faixa, lista válida ou tipo de venda configurado deve ficar auditável (quem alterou, quando, valor anterior).
NFR2: Acesso de edição deve ser restrito a um papel de gestão/administração, distinto do acesso de visualização (GGV/Supervisor nos painéis) — reaproveitando os papéis já existentes no Farol (`gestor_filial`/`gestor_geral`), sem criar papel novo.
NFR3: A apuração de meses fechados deve ser reprodutível — dado o mesmo snapshot de dados, o resultado não muda entre duas execuções.
NFR4: Não há SLA crítico de prazo para a apuração mensal estar pronta (risco aceito conscientemente pelo usuário).

### Additional Requirements

*(Não existe Architecture.md — os itens abaixo são restrições técnicas extraídas do próprio PRD/addendum que impactam o desenho e viram trabalho de arquitetura dentro do Épico 1.)*

- **Vigência é entidade própria, não campo único:** um vínculo Indústria/Fornecedor × Tipo de Métrica pode ter múltiplas vigências ao longo do ano (ex: meta de jan-mar diferente de abr-jun), cada uma com seus próprios valores de meta por faixa. O modelo de dados precisa suportar histórico de vigências por vínculo, não sobrescrever a vigência anterior.
- **Teste de generalidade do modelo de Tipos de Métrica (nota do PRD, não é FR):** o modelo de parâmetros do FR1 precisa acomodar um Tipo de Métrica hipotético com forma diferente da Unilever (nível de agregação = Cliente em vez de Rede, parâmetro = nº de visitas em vez de R$/EAN) sem exigir campo novo na tabela de Tipos de Métrica — só um novo conjunto de parâmetros. Critério de aceite para quem desenhar o modelo de dados.
- **Hierarquia Rede combinada com organograma:** o nível Rede (grupo de CNPJs, hoje só existe como árvore separada em V06) precisa ser combinado com a hierarquia GGV→CRV→RCA (hoje V03/Gerência vai direto RCA→Cliente) — essa combinação não existe hoje no Farol e precisa de modelo de dados novo.
- **Recorte organizacional parcial:** a configuração de meta (FR5) precisa suportar recorte parcial da operação (UF específica, GGVs específicos), não só "empresa inteira" — implica um mecanismo de escopo reutilizável no modelo de vínculo.
- **Reaproveitar papéis de acesso existentes** (`gestor_filial`/`gestor_geral`) em vez de criar um papel novo (NFR2).
- **Reaproveitar o padrão de painel público mobile já existente** (`/m/CNPJ/SUP/cod`, `/m/CNPJ/RCA/cod`, sem login) para o painel mobile deste módulo (FR22).
- **Mecanismo de importação de metas — Fase 1:** upload de CSV pelo admin (FR8/FR9). Fase 2 (fora de escopo, registrada no addendum): integração direta com Oracle da JC como fonte de metas, substituindo o CSV manual.
- **Modelo de dados não deve fechar a porta** para um futuro "Farol de Compras" reaproveitando esta mesma base (direção estratégica citada pelo usuário, não é requisito funcional deste PRD).
- **Ambiente de deploy:** desenvolvimento e testes funcionais rodam na VM de dev (`2.25.119.46`, backend Farol na porta 8084); nenhum deploy em produção (`76.13.171.196`, Coolify) até aprovação explícita do usuário — trabalho deve ocorrer em branch de feature, sem merge/push para `main` sem pedido dele.
- **Numérica reaproveita o modelo de arquitetura do Épico 1, não cria um framework paralelo:** o "teste de generalidade" da Story 1.1 (Tipo de Métrica hipotético com agregação = Cliente) deixa de ser hipotético — a Numérica é a prova real de que o modelo de parâmetros aguenta um 2º nível de agregação sem coluna nova na tabela de Tipos de Métrica. Se a implementação real exigir coluna nova, é sinal de que a Story 1.1 precisa ser revisitada, não só o Épico 7.
- **PPA é um agrupamento de produto novo, não reaproveita a tabela de Itens Válidos (FR12):** precisa de modelo de dados próprio (cod_prod → PPA → EAN), importado por CSV separado (FR28), específico da Numérica — Sortimento por Rede continua usando a lista de Itens Válidos como está.
- **Bimestre móvel (FR14a) muda a lógica de janela de tempo do motor de apuração**, não só o agrupamento — precisa ser resolvido na arquitetura do Épico 4/7 como uma característica configurável por Tipo de Métrica (mês fechado vs. bimestre móvel), não hardcoded.

### UX Design Requirements

Não há documento de UX Design para este módulo. O painel web deve seguir o padrão visual "Clean Professional" já estabelecido no Farol (ver CLAUDE.md); o painel mobile deve seguir o mesmo padrão do painel público mobile já existente (`FarolPublicPanel.tsx`). Decisões de UI ficam a cargo das stories de cada épico, sem um contrato de UX separado.

### FR Coverage Map

FR1: Epic 1 - Cadastro de Tipo de Métrica reutilizável
FR2: Epic 1 - Tipos de Métrica de referência Unilever pré-cadastrados
FR3: Epic 1 - Reuso de um Tipo de Métrica por 2+ indústrias
FR4: Epic 2 - Vínculo Indústria/Fornecedor × Tipo de Métrica
FR5: Epic 2 - Metas por faixa/vigência(múltipla)/recorte organizacional
FR6: Epic 2 - Tipo de venda configurável por vínculo
FR7: Epic 2 - Faixas de meta genéricas
FR8: Epic 3 - Importação de metas via CSV
FR9: Epic 3 - Validação estrita do CSV (tudo-ou-nada)
FR10: Fora de escopo - Integração Oracle (direção futura, addendum)
FR11: Epic 3 - Importação de lista de Clientes Válidos (Rede/RCA)
FR12: Epic 3 - Importação de lista de Itens Válidos (EAN/embalagem)
FR13: Epic 3 - Snapshot mensal congelado das listas
FR14: Epic 4 - Cálculo do Realizado por Tipo de Métrica e nível hierárquico
FR15: Epic 4 - 3 visões de fluxo (Faturado/Transmitido/Soma)
FR16: Epic 4 - Tipo de venda do vínculo respeitado no cálculo
FR17: Epic 4 - Congelamento de mês fechado
FR18: Epic 4 - Projeção de fechamento (ritmo linear)
FR18a: Epic 4 - Projeção independente por nível hierárquico
FR19: Epic 5 - Painel Meta × Realizado navegável pela hierarquia
FR19a: Epic 5 - Delta explícito (falta para bater a meta)
FR20: Epic 5 - Alternância Faturado/Transmitido/Soma no painel
FR21: Epic 5 - Recortes de tempo + projeção no painel web
FR22: Epic 6 - Painel mobile no padrão de link público do Farol
FR23: Epic 6 - Recortes de tempo + projeção no painel mobile
FR14a: Epic 7 - Bimestre móvel configurável por Tipo de Métrica
FR24: Epic 7 - Nível de agregação Cliente/CNPJ direto (sem Rede)
FR25: Epic 7 - Tipo de Métrica Cobertura Numérica (limiar por Classificação PDV)
FR26: Epic 7 - Tipo de Métrica Sortimento Numérica/PPA (só A/B, teto 15, inclui Tipo 5)
FR27: Epic 7 - Importação de Clientes Numéricas (formato sem Rede)
FR28: Epic 7 - Importação de PPAs (cod_prod → PPA → EAN)
FR29: Epic 7 - Dedução de Devolução/Cancelamento na Numérica
FR30: Epic 7 - Faixas agregadas do distribuidor pra Numérica (reaproveita FR7)
FR31: Epic 7 - Cobertura/Sortimento Numérica no painel, dentro da Indústria existente
NFR1: Epic 1, Epic 2, Epic 3 - Auditoria de alterações (quem/quando/valor anterior)
NFR2: Epic 5 - Papel de acesso de edição vs. visualização reaproveitado
NFR3: Epic 4 - Reprodutibilidade da apuração de mês fechado
NFR4: Epic 4 - Sem SLA crítico (risco aceito)

## Epic List

### Epic 1: Catálogo de Tipos de Métrica (com arquitetura embutida)
Admin cadastra Tipos de Métrica reutilizáveis (nome, descrição, nível de agregação, parâmetros exigidos); sistema já vem com Cobertura por Rede e Sortimento por Rede (Unilever) pré-cadastrados. Este épico carrega a decisão de arquitetura do módulo: o modelo de dados de Tipo de Métrica/parâmetros é desenhado e validado contra o teste de generalidade do FR1 (o tipo hipotético "Frequência de Visita por Cliente" precisa caber no modelo sem campo novo na tabela), e a hierarquia Rede×organograma (GGV→CRV→RCA→Rede→Cliente, combinação que não existe hoje no Farol) é modelada aqui, servindo de base para todos os épicos seguintes.
**FRs covered:** FR1, FR2, FR3, NFR1 (parcial)

### Epic 2: Configuração de Metas por Indústria
Admin vincula uma Indústria/Fornecedor a um ou mais Tipos de Métrica e define, por vínculo: valores de meta por faixa, período(s) de vigência (suportando múltiplas vigências ao longo do ano), recorte organizacional (UF, GGVs específicos ou empresa toda) e tipo de venda válido para apuração.
**FRs covered:** FR4, FR5, FR6, FR7, NFR1 (parcial)

### Epic 3: Importação de Dados Mensais (Metas, Clientes Válidos, Itens Válidos)
Admin importa via CSV os três tipos de dado mensal do módulo — valores de meta por vínculo/faixa/vigência, lista de Clientes Válidos (Redes de CNPJ com RCA responsável) e lista de Itens Válidos (EAN mapeado a cod_prod, com tipo de embalagem) — com validação estrita linha a linha (tudo-ou-nada, nunca falha silenciosa) e snapshot mensal congelado por período.
**FRs covered:** FR8, FR9, FR11, FR12, FR13, NFR1 (parcial) — FR10 citado como fora de escopo/direção futura

### Epic 4: Motor de Apuração
O sistema calcula, mensalmente e sob demanda para o mês corrente, o Realizado de cada Tipo de Métrica configurado por nível hierárquico (GGV→CRV→RCA→Rede→Cliente), lendo Faturado/Transmitido/Soma do Farol com o tipo de venda do vínculo. Meses fechados ficam congelados (recálculo só manual). O motor também calcula a projeção de fechamento do ano corrente (ritmo linear), de forma independente em cada nível hierárquico.
**FRs covered:** FR14, FR15, FR16, FR17, FR18, FR18a, NFR3, NFR4

### Epic 5: Painel Web
GGV e Supervisor navegam a hierarquia GGV→CRV→RCA→Rede→Cliente e veem Meta × Realizado × delta (quanto falta pra bater a meta) por Tipo de Métrica, alternando entre Faturado/Transmitido/Soma e comparando recortes de tempo (dia anterior/semana/mês/ano corrente) até a projeção de fechamento. Acesso de visualização separado do acesso de edição (Épicos 1-3), reaproveitando os papéis já existentes do Farol.
**FRs covered:** FR19, FR19a, FR20, FR21, NFR2

### Epic 6: Painel Mobile
Mesma visão de Meta × Realizado × delta do Épico 5, exposta no padrão de link público já existente do Farol (`/m/CNPJ/SUP|RCA/cod`, sem login completo), adaptada para tela pequena e uso em campo por GGV/Supervisor.
**FRs covered:** FR22, FR23

### Epic 7: Numérica — Cobertura e Sortimento por Cliente/CNPJ (addendum 2026-09-29)
Dois novos Tipos de Métrica ("Cobertura Numérica" e "Sortimento Numérica/PPA"), vinculados às mesmas Indústrias FOOD/HC já existentes (Épicos 1-2), calculados por Cliente/CNPJ direto — sem Rede — no bimestre móvel, com listas de import próprias (Clientes Numéricas, PPAs) e exibidos dentro do painel existente (Épicos 5-6) como opção de Tipo de Métrica adicional. 5 Questões em aberto no PRD ainda sem resposta da JC (ver "Pendências de Validação").
**FRs covered:** FR14a, FR24, FR25, FR26, FR27, FR28, FR29, FR30, FR31

## Epic 1: Catálogo de Tipos de Métrica (com arquitetura embutida)

Admin cadastra Tipos de Métrica reutilizáveis (nome, descrição, nível de agregação, parâmetros exigidos); sistema já vem com Cobertura por Rede e Sortimento por Rede (Unilever) pré-cadastrados. Este épico carrega a decisão de arquitetura do módulo: o modelo de dados de Tipo de Métrica/parâmetros é desenhado e validado contra o teste de generalidade do FR1, e a hierarquia Rede×organograma é modelada aqui, servindo de base para todos os épicos seguintes.

### Story 1.1: Cadastro de Tipo de Métrica com modelo de parâmetros genérico

Como admin,
Eu quero cadastrar um novo Tipo de Métrica definindo nome, descrição, nível de agregação (ex: Rede, Cliente/CNPJ) e a lista de parâmetros que uma instância exige preencher,
Para que o framework suporta uma nova forma de calcular atingimento sem exigir alteração de código.

**Critérios de Aceite:**

**Dado** que estou logado como admin
**Quando** cadastro um Tipo de Métrica "Cobertura por Rede" com nível de agregação = Rede e parâmetro "limiar R$"
**Então** o tipo fica disponível para ser vinculado a uma Indústria/Fornecedor (Épico 2)

**Dado** o modelo de parâmetros já implementado
**Quando** cadastro um Tipo de Métrica hipotético "Frequência de Visita por Cliente" com nível de agregação = Cliente e parâmetro "nº mínimo de visitas"
**Então** o cadastro é concluído sem exigir nenhuma coluna nova na tabela de Tipos de Métrica — só um novo conjunto de parâmetros (teste de generalidade do FR1, critério de aceite da arquitetura)

**E** toda criação/edição de Tipo de Métrica registra quem alterou, quando e o valor anterior (NFR1)

### Story 1.2: Seed dos Tipos de Métrica de referência Unilever

Como admin,
Eu quero que o sistema já venha com "Cobertura por Rede" e "Sortimento por Rede" pré-cadastrados,
Para que eu não preciso recriar manualmente os tipos de métrica do primeiro programa (Unilever) antes de configurar as metas.

**Critérios de Aceite:**

**Dado** uma instalação nova do módulo
**Quando** acesso o Catálogo de Tipos de Métrica pela primeira vez
**Então** "Cobertura por Rede" (parâmetro: limiar de valor médio de compra) e "Sortimento por Rede" (parâmetros: lista de itens válida + regra de quantidade mínima por item, não aplicável a itens vendidos em caixa/pacote/display) já existem, prontos para vínculo

### Story 1.3: Reuso de Tipo de Métrica por múltiplas indústrias/fornecedores

Como admin,
Eu quero reutilizar um Tipo de Métrica já existente em 2 ou mais indústrias/fornecedores diferentes, cada um com seus próprios parâmetros,
Para que eu não duplico a definição de cálculo quando fornecedores diferentes usam a mesma forma de métrica.

**Critérios de Aceite:**

**Dado** o Tipo de Métrica "Cobertura por Rede" já cadastrado
**Quando** vinculo esse tipo tanto ao fornecedor 131-Foods quanto ao 396-HC (Épico 2), cada um com seu próprio limiar em R$
**Então** os dois vínculos calculam de forma independente, sem um parâmetro sobrescrever o outro

### Story 1.4: Hierarquia Rede combinada ao organograma

Como admin,
Eu quero que uma Rede (grupo de CNPJs) resolva corretamente para o GGV/CRV/RCA responsável na hierarquia organizacional,
Para que o cálculo e o painel (Épicos 4-6) conseguem navegar GGV → CRV → RCA → Rede → Cliente sem ambiguidade.

**Critérios de Aceite:**

**Dado** que hoje o Farol trata Rede (V06, `cod_cliprinc`) como árvore separada da organização (V03/Gerência vai direto RCA→Cliente)
**Quando** o modelo de dados deste módulo é aplicado, com dados de teste inseridos diretamente (a tela de importação em massa é só a Story 3.2 — este critério não depende dela)
**Então** cada Rede resolve para exatamente um caminho GGV→CRV→RCA a partir do RCA atribuído a cada CNPJ, permitindo agregação em qualquer nível sem duplicar ou perder CNPJ — este é o modelo que a Story 3.2 (FR11) vai popular via CSV depois

**E** o modelo de dados evita decisões que fechem a porta para reuso futuro por um eventual "Farol de Compras" (Fora de escopo, mas restrição de design)

## Epic 2: Configuração de Metas por Indústria

Admin vincula uma Indústria/Fornecedor a um ou mais Tipos de Métrica e define, por vínculo: valores de meta por faixa, período(s) de vigência (suportando múltiplas vigências ao longo do ano), recorte organizacional (UF, GGVs específicos ou empresa toda) e tipo de venda válido para apuração.

### Story 2.1: Vínculo Indústria/Fornecedor × Tipo de Métrica

Como admin,
Eu quero vincular uma Indústria — ou um Fornecedor específico dentro dela — a um ou mais Tipos de Métrica já cadastrados,
Para que cada fornecedor pode ter suas próprias metas mesmo dentro da mesma indústria.

**Critérios de Aceite:**

**Dado** os Tipos de Métrica "Cobertura por Rede" e "Sortimento por Rede" já cadastrados
**Quando** crio um vínculo para o fornecedor 131-Foods com "Cobertura por Rede" e outro vínculo para o fornecedor 396-HC com o mesmo tipo
**Então** os dois vínculos existem de forma independente, cada um com seus próprios parâmetros e metas — objetivo de um não interfere no outro

*(Nota adicionada na implementação da Story 1.3: o AC de reuso do FR3/Story 1.3 foi transferido pra cá, porque só a partir daqui a tabela de vínculo existe de verdade pra testar. Ver `_bmad-output/implementation-artifacts/1-3-reuso-tipo-metrica.md`.)*

### Story 2.2: Metas por faixa e histórico de vigências

Como admin,
Eu quero definir, para cada vínculo, valores de meta por faixa (ex: Faixa 3/2/1), cadastrando quantas vigências diferentes forem necessárias ao longo do ano,
Para que a meta acompanha mudanças de contrato/objetivo mês a mês sem perder o histórico de períodos anteriores.

**Critérios de Aceite:**

**Dado** um vínculo existente
**Quando** cadastro a vigência jan-mar/2026 com Faixa 3=R$X/Faixa 2=R$Y/Faixa 1=R$Z, e depois cadastro a vigência abr-jun/2026 com valores diferentes
**Então** as duas vigências ficam registradas como períodos distintos (não uma sobrescreve a outra), e o motor de apuração (Épico 4) usa a vigência correta conforme a data do período sendo calculado

**E** toda vigência tem um status (aberta/fechada) — pode ser marcada como fechada manualmente pelo admin ou automaticamente ao virar o mês; uma vez fechada, os valores de meta dela não podem mais ser editados por essa tela; alterar exige o fluxo de reprocessamento manual do Épico 4 (FR17)

**E** toda criação/alteração de meta ou faixa registra quem alterou, quando e o valor anterior (NFR1)

### Story 2.3: Recorte organizacional do vínculo

Como admin,
Eu quero restringir um vínculo a um recorte organizacional (UF específica, GGVs específicos, ou empresa toda),
Para que consigo configurar programas regionais/parciais — como o da Unilever, restrito a UF=GO e GGVs específicos — sem afetar o resto da operação.

**Critérios de Aceite:**

**Dado** o vínculo do fornecedor 131-Foods
**Quando** configuro o recorte como UF=GO + GGVs [X, Y]
**Então** só CNPJs/Redes dentro desse recorte entram na apuração e no painel deste vínculo — o resto da operação não é afetado

### Story 2.4: Tipo de venda configurável por vínculo

Como admin,
Eu quero configurar quais tipos de venda (ex: só Tipo 1 e 9) contam para a apuração de um vínculo específico,
Para que o cálculo desse fornecedor não usa o "Líquido" padrão do Farol quando o contrato exige uma regra diferente.

**Critérios de Aceite:**

**Dado** o vínculo do fornecedor 131-Foods
**Quando** configuro tipo de venda válido = [1, 9]
**Então** o motor de apuração (Épico 4) filtra apenas vendas desses tipos para esse vínculo, ignorando o filtro "Líquido" padrão

**E** toda alteração do tipo de venda configurado registra quem alterou, quando e o valor anterior (NFR1)

## Epic 3: Importação de Dados Mensais (Metas, Clientes Válidos, Itens Válidos)

Admin importa via CSV os três tipos de dado mensal do módulo — valores de meta por vínculo/faixa/vigência, lista de Clientes Válidos (Redes de CNPJ com RCA responsável) e lista de Itens Válidos (EAN mapeado a cod_prod, com tipo de embalagem) — com validação estrita linha a linha (tudo-ou-nada, nunca falha silenciosa) e snapshot mensal congelado por período.

### Story 3.1: Importação de metas via CSV

Como admin,
Eu quero fazer upload de um CSV com valores de meta por vínculo/faixa/vigência,
Para que eu não preciso digitar cada valor manualmente na UI (Épico 2) quando recebo a planilha do fornecedor.

**Critérios de Aceite:**

**Dado** um arquivo CSV com colunas vínculo, tipo de métrica, faixa, vigência (início/fim), valor
**Quando** faço upload de um arquivo válido
**Então** as metas são criadas/atualizadas seguindo o mesmo modelo da Story 2.2 (histórico de vigências preservado)

**E** se qualquer linha tiver erro (vínculo inexistente, valor não numérico, vigência sobreposta indevidamente), nenhuma linha do lote é aplicada e os erros são reportados linha a linha (FR9)

### Story 3.2: Importação de Clientes Válidos (Redes + RCA responsável)

Como admin,
Eu quero fazer upload de um CSV com as Redes válidas de um vínculo/vigência, cada CNPJ com o RCA responsável,
Para que a hierarquia de drill-down (GGV→CRV→RCA→Rede→Cliente) tem a atribuição correta de responsável para aquele período.

*(Decisão de arquitetura da Story 1.4, a aplicar aqui: tabela com `vinculo_id`/`vigencia_id` FK pro Épico 2, `rede_nome TEXT` livre — não FK pra tabela mestra de Redes, porque a definição de Rede varia por fornecedor — e resolução CRV/GGV via JOIN com a hierarquia organizacional já existente, sem duplicar dado. Ver `_bmad-output/implementation-artifacts/1-4-hierarquia-rede-organograma.md`.)*

**Critérios de Aceite:**

**Dado** um arquivo CSV com Rede, CNPJ, RCA responsável
**Quando** faço upload
**Então** cada CNPJ importado fica vinculado a exatamente um RCA (e, por herança, ao CRV/GGV daquele RCA)

**E** qualquer linha com CNPJ sem RCA vinculado é rejeitada com erro claro — todo CNPJ deve ter RCA (regra de qualidade de dado, FR11) — e o lote inteiro é recusado se houver erro (FR9)

### Story 3.3: Importação de Itens Válidos (EAN + embalagem)

Como admin,
Eu quero fazer upload de um CSV com os EANs válidos de um vínculo/vigência, mapeados para o(s) cod_prod interno(s) e com o tipo de embalagem,
Para que o motor de apuração (Épico 4) sabe quais produtos contam para "Sortimento" e se a regra de quantidade mínima se aplica a cada um.

**Critérios de Aceite:**

**Dado** um arquivo CSV com EAN, cod_prod, tipo de embalagem (UN/CX/Pacote/Display)
**Quando** faço upload
**Então** um EAN pode mapear para mais de um cod_prod (variantes/embalagens diferentes), cada mapeamento com seu próprio tipo de embalagem

**E** uma linha sem tipo de embalagem válido é rejeitada — é atributo obrigatório, pois decide se a regra de quantidade mínima do Sortimento (FR2) se aplica (FR12) — lote inteiro recusado se houver erro (FR9)

### Story 3.4: Snapshot mensal congelado

Como admin,
Eu quero que listas (clientes/itens) e metas de um período já fechado não possam ser alteradas de forma a mudar retroativamente um cálculo já apurado,
Para que decisões de bônus já fechadas permanecem estáveis e auditáveis mesmo se a planilha do fornecedor mudar depois.

**Critérios de Aceite:**

**Dado** um período (vigência) já marcado como fechado (status do período, definido nas Stories 2.2/3.1-3.3)
**Quando** tento importar uma nova versão da lista de Clientes/Itens Válidos ou de metas para esse mesmo período
**Então** o sistema bloqueia a aplicação automática do lote — a mudança só é aceita como reprocessamento manual explícito de um gestor (mesma regra de status fechado que o motor de apuração vai respeitar no Épico 4, FR17), nunca silenciosa

## Epic 4: Motor de Apuração

O sistema calcula, mensalmente e sob demanda para o mês corrente, o Realizado de cada Tipo de Métrica configurado por nível hierárquico (GGV→CRV→RCA→Rede→Cliente), lendo Faturado/Transmitido/Soma do Farol com o tipo de venda do vínculo. Meses fechados ficam congelados (recálculo só manual). O motor também calcula a projeção de fechamento do ano corrente (ritmo linear), de forma independente em cada nível hierárquico.

### Story 4.1: Cálculo do Realizado por Tipo de Métrica e nível hierárquico

Como Supervisor/GGV,
Eu quero que o sistema calcule automaticamente, mensalmente e sob demanda para o mês corrente, o Realizado de cada Tipo de Métrica configurado em cada nível da hierarquia (GGV→CRV→RCA→Rede→Cliente),
Para que eu não dependo de relatório manual do WinThor pra saber onde estou.

**Critérios de Aceite:**

**Dado** um vínculo com metas e listas válidas configuradas (Épicos 2-3)
**Quando** calculo o Realizado de uma vigência
**Então** tenho resultado por Rede, agregado por RCA/CRV/GGV, cada nível calculado a partir do dado bruto de venda — nunca somando um valor pré-agregado de um nível pra derivar outro (mesmo cuidado do bug de totalizador já documentado no Farol)

**E** o cálculo usa apenas os tipos de venda configurados no vínculo (FR6), não o filtro "Líquido" padrão do Farol (FR16)

**E** o mês corrente é sempre calculado como parcial/em andamento; meses anteriores fechados calculam o resultado definitivo

### Story 4.2: Três visões de fluxo (Faturado / Transmitido / Soma)

Como Supervisor,
Eu quero alternar a visão do Realizado entre Faturado, Transmitido (Emitido) e a Soma dos dois,
Para que eu vejo tanto o que já foi vendido quanto o que já foi confirmado, dependendo do que preciso checar.

**Critérios de Aceite:**

**Dado** um Realizado já calculado para um vínculo/período
**Quando** alterno entre Faturado, Transmitido e Soma
**Então** os três valores refletem a base correspondente do Farol, sem exigir recálculo do zero a cada troca

### Story 4.3: Congelamento de mês fechado

Como admin (gestor),
Eu quero que o resultado apurado de um mês fechado fique congelado, só recalculável por ação manual explícita,
Para que os números usados pra decisão de bônus nunca mudam de forma silenciosa depois do fechamento.

**Critérios de Aceite:**

**Dado** um mês fechado já com Realizado calculado
**Quando** uma lista válida ou meta desse período é alterada depois
**Então** o Realizado já apurado não muda automaticamente

**Dado** o mesmo mês fechado
**Quando** um gestor dispara reprocessamento manual explícito
**Então** o sistema recalcula e registra que houve reprocessamento manual (auditoria, NFR1)

**E** dado o mesmo snapshot de dados, duas execuções do cálculo de um mês fechado produzem sempre o mesmo resultado (NFR3)

### Story 4.4: Projeção de fechamento por nível hierárquico

Como Supervisor/GGV,
Eu quero ver a projeção de fechamento do ano corrente pra cada métrica, com base no ritmo de realização até o momento,
Para que eu sei hoje se estou no caminho pra bater a meta anual, não só o que já foi realizado até agora.

**Critérios de Aceite:**

**Dado** um Realizado parcial do período
**Quando** calculo a projeção (método v1, ritmo linear: `projeção = realizado ÷ dias decorridos × dias totais do período`)
**Então** o exemplo do PRD confere: R$ 45.000 realizados em 15 dias de um mês de 30 dias → projeção de R$ 90.000

**E** a projeção de cada nível (GGV, CRV, RCA, Rede, Cliente) é calculada a partir do Realizado direto daquele nível — nunca somando as projeções dos níveis filhos (FR18a)

*(Nota: não há SLA crítico de prazo pra apuração — NFR4, risco de estagnação silenciosa aceito conscientemente pelo usuário; não vira AC de story, fica registrado aqui.)*

*(Nota de UX, ver Épico 5 e "Pendências de Validação": a projeção de fechamento é uma capacidade adicional que não faz parte do processo oficial que a Unilever pediu/usa hoje — precisa ficar visualmente separada, em aba ou tela própria, para não ser confundida com os indicadores que o fornecedor efetivamente cobra.)*

## Epic 5: Painel Web

GGV e Supervisor navegam a hierarquia GGV→CRV→RCA→Rede→Cliente e veem Meta × Realizado × delta (quanto falta pra bater a meta) por Tipo de Métrica, alternando entre Faturado/Transmitido/Soma e comparando recortes de tempo (dia anterior/semana/mês/ano corrente) até a projeção de fechamento — exibida em aba separada dos indicadores oficiais. Acesso de visualização separado do acesso de edição (Épicos 1-3), reaproveitando os papéis já existentes do Farol.

*(Layout sujeito a revisão quando o Excel da Unilever com a visualização esperada em nível CRV/RCA chegar — ver "Pendências de Validação".)*

### Story 5.1: Painel de indicadores oficiais (Meta × Realizado × delta)

Como Supervisor/GGV,
Eu quero ver, para cada Indústria/Fornecedor configurado, um painel de Meta × Realizado por Tipo de Métrica, navegável pela hierarquia GGV → CRV → RCA → Rede → Cliente,
Para que eu identifique rapidamente onde estou abaixo da meta e aja em campo.

**Critérios de Aceite:**

**Dado** um vínculo com Realizado já calculado (Épico 4)
**Quando** abro o painel no nível CRV
**Então** vejo Meta, Realizado e o delta explícito (quanto falta pra bater a meta, não só os dois números lado a lado — FR19a) por Tipo de Métrica

**E** ao navegar pra um nível abaixo (RCA, Rede, Cliente), vejo o mesmo painel recalculado pra esse recorte, sem perder o contexto de onde vim (breadcrumb/drill-down)

**E** esse painel mostra apenas os indicadores oficiais do programa (ex: Cobertura, Sortimento) — a projeção de fechamento não aparece aqui (ver Story 5.3)

### Story 5.2: Alternância de fluxo (Faturado / Transmitido / Soma)

Como Supervisor,
Eu quero alternar o painel entre Faturado, Transmitido e Soma,
Para que eu veja o indicador na visão que preciso checar, igual já faço hoje no restante do Farol.

**Critérios de Aceite:**

**Dado** o painel de indicadores aberto
**Quando** troco o toggle de Faturado para Transmitido ou Soma
**Então** Meta, Realizado e delta são recalculados/reexibidos para a visão escolhida (FR15/FR20), sem perder o nível de drill-down atual

### Story 5.3: Recortes de tempo e projeção de fechamento (aba separada)

Como Supervisor/GGV,
Eu quero comparar o Realizado em diferentes recortes de tempo (dia anterior, semana, mês, ano corrente) e, numa aba/tela separada dos indicadores oficiais, visualizar a projeção de fechamento do ano,
Para que eu acompanhe a evolução recente sem misturar o que o fornecedor cobra oficialmente com uma capacidade nova do Farol.

**Critérios de Aceite:**

**Dado** o painel de indicadores oficiais aberto (Story 5.1)
**Quando** alterno entre os recortes dia anterior/semana/mês/ano corrente
**Então** Meta, Realizado e delta são recalculados para o recorte escolhido

**Dado** o mesmo painel
**Quando** acesso a aba/tela "Projeção" (separada da aba de indicadores oficiais)
**Então** vejo a projeção de fechamento (FR18) do nível hierárquico atual, com aviso visual de que é uma estimativa, não um número oficial do programa

### Story 5.4: Papel de acesso — visualização separada de edição

Como Supervisor/GGV,
Eu quero acessar o painel de visualização sem ter acesso às telas de configuração/importação (Épicos 1-3),
Para que meu acesso de campo não me dê permissão de alterar meta/lista/tipo de venda por engano.

**Critérios de Aceite:**

**Dado** um usuário com papel `gestor_filial` ou visualização padrão (não `gestor_geral`/admin)
**Quando** ele acessa o painel de metas por indústria
**Então** ele vê os dados normalmente, mas não tem acesso às rotas/telas de edição dos Épicos 1-3 (reaproveitando os papéis já existentes do Farol, NFR2)

## Epic 6: Painel Mobile

Mesma visão de Meta × Realizado × delta do Épico 5, exposta no padrão de link público já existente do Farol (`/m/CNPJ/SUP|RCA/cod`, sem login completo), adaptada para tela pequena e uso em campo por GGV/Supervisor.

### Story 6.1: Exposição do painel mobile no padrão de link público existente

Como Supervisor/GGV,
Eu quero acessar o painel de metas por indústria pelo celular, no mesmo padrão de link direto já usado pelo Farol público (sem exigir login completo),
Para que eu consiga acompanhar a meta em campo sem depender de estar logado no sistema web.

**Critérios de Aceite:**

**Dado** o link público padrão `/m/CNPJ/SUP/cod` ou `/m/CNPJ/RCA/cod` já existente no Farol
**Quando** acesso esse link pelo celular
**Então** vejo o painel de metas por indústria da minha hierarquia (Supervisor ou RCA correspondente), sem tela de login

**E** o link é resolvido por CNPJ/código do jeito que já funciona hoje, reaproveitando o mecanismo de resolução de empresa do painel público atual do Farol (FR22)

### Story 6.2: Recortes de tempo e projeção no mobile

Como Supervisor/GGV,
Eu quero ver os mesmos recortes de tempo e a projeção de fechamento no mobile, adaptados pra tela pequena,
Para que eu tenha a mesma informação que teria no painel web, mesmo estando em campo.

**Critérios de Aceite:**

**Dado** o painel mobile aberto
**Quando** alterno entre dia anterior/semana/mês/ano corrente
**Então** Meta, Realizado e delta atualizam do mesmo jeito que no painel web (FR21↔FR23)

**E** a projeção de fechamento aparece em aba/tela separada dos indicadores oficiais — mesma regra de separação do Épico 5 (Story 5.3), adaptada pro formato mobile

## Epic 7: Numérica — Cobertura e Sortimento por Cliente/CNPJ (addendum 2026-09-29)

Dois novos Tipos de Métrica ("Cobertura Numérica" e "Sortimento Numérica/PPA"), vinculados às mesmas Indústrias FOOD/HC já existentes (Épicos 1-2, não cria Indústria nova), calculados por Cliente/CNPJ direto — sem Rede — no bimestre móvel (mês corrente + mês anterior), com listas de import próprias (Clientes Numéricas, PPAs) e exibidos dentro do painel existente (Épicos 5-6) como opção de Tipo de Métrica adicional.

*(Status 30/09/2026 — documentação final do Heverton confirmada: as 5 Questões em aberto estão endereçadas (3 respondidas pelo Carlos (JC); 1 implementada no mesmo dia como Story 7.8 — visão Faturado+Emitido; 1 — denominador da média PPA, Story 7.6 — segue com DECISÃO PROVISÓRIA do Claudio, não resposta formal do Carlos, fácil de reverter). Ver PRD "Questões em aberto".)*

### Story 7.1: Tipos de Métrica "Cobertura Numérica" e "Sortimento Numérica (PPA)"

Como admin,
Eu quero cadastrar os Tipos de Métrica "Cobertura Numérica" (nível de agregação Cliente/CNPJ, janela bimestre móvel) e "Sortimento Numérica/PPA" (mesma agregação e janela), e vinculá-los às Indústrias FOOD e HC já existentes,
Para que a Numérica seja um 2º par de métricas dentro da mesma Indústria, não um cadastro paralelo.

**Critérios de Aceite:**

**Dado** o modelo de Tipo de Métrica já validado pelo teste de generalidade (Story 1.1, nível de agregação = Cliente)
**Quando** cadastro "Cobertura Numérica" com nível de agregação = Cliente/CNPJ, janela de apuração = bimestre móvel (FR14a/FR24), e parâmetro de limiar variável por Classificação PDV (Num. A=R$100, Num. B=R$50, Num. C=R$15)
**Então** o cadastro é concluído sem exigir coluna nova na tabela de Tipos de Métrica — esta story é a prova real do teste de generalidade da Story 1.1, que até aqui só tinha sido validado com um tipo hipotético

**E** o mesmo vale para "Sortimento Numérica/PPA", com parâmetro de teto (15 PPAs) e regra "só Classificação A e B" (FR26)

**Dado** os 2 Tipos de Métrica cadastrados
**Quando** vinculo cada um às Indústrias FOOD e HC já existentes (Épico 2), com seus próprios objetivos de PPA (15 pra HC, 7 pra Foods)
**Então** os 4 vínculos (Cobertura Numérica×FOOD, Cobertura Numérica×HC, Sortimento Numérica×FOOD, Sortimento Numérica×HC) calculam de forma independente, reaproveitando o modelo de vínculo do FR3/FR4 — nenhuma tabela nova de "Indústria Numérica"

**✅ Resolvida (Carlos/JC, 30/09/2026):** confirmado, "Cliente Rede" era resíduo de texto do modelo antigo — Numérica lê individual, sem nível intermediário. Modelo de navegação desta story (CNPJ direto) está correto como implementado.

### Story 7.2: Importação de Clientes Numéricas

Como admin,
Eu quero fazer upload de um CSV mensal com os Clientes Numéricas — CNPJ, Classificação PDV (A/B/C), GGV/CRV/RCA responsável —,
Para que o motor de apuração (Story 7.4/7.5) saiba quem é cliente Numérica e qual limiar/regra aplicar a cada um.

**Critérios de Aceite:**

**Dado** um arquivo CSV com colunas CNPJ, Classificação PDV, GGV, CRV, RCA (sem cod_princ/Rede — FR27)
**Quando** faço upload pra um vínculo/vigência de Cobertura Numérica ou Sortimento Numérica
**Então** cada CNPJ fica importado com sua Classificação, pronta pra decidir o limiar de Cobertura (FR25) e se entra ou não no Sortimento (FR26)

**E** segue a mesma validação estrita linha a linha do FR9 (lote inteiro recusado se houver erro) e o mesmo snapshot mensal congelado do FR13 — período fechado não aceita reimportação automática

### Story 7.3: Importação de PPAs

Como admin,
Eu quero fazer upload de um CSV mensal mapeando cod_prod (código interno) para PPA (nome da família) e EAN, com embalagem,
Para que o motor de apuração do Sortimento Numérica (Story 7.5) saiba a qual família cada produto vendido pertence.

**Critérios de Aceite:**

**Dado** um arquivo CSV com cod_prod, PPA, EAN, embalagem (FR28) — modelo de dados PRÓPRIO, não reaproveita a tabela de Itens Válidos do FR12
**Quando** faço upload pra um vínculo/vigência de Sortimento Numérica
**Então** cada cod_prod fica associado a exatamente 1 PPA, e múltiplos cod_prod/EAN podem apontar pro mesmo PPA (ex: "MAIZENA CREMOGEMA 180GR" agrega várias embalagens)

**E** mesma validação estrita (FR9) e snapshot congelado (FR13) das demais listas mensais

**✅ Resolvida (Carlos/JC, 30/09/2026):** confirmado, pode ignorar Região do Sortimento/AE/BU — sem uso no cálculo. Bate com a decisão de design já tomada (migration 248: colunas capturadas como informativas, não usadas por `contarPPAsPositivados`).

### Story 7.4: Motor de apuração — Cobertura Numérica

Como Supervisor/GGV,
Eu quero que o sistema calcule, por Cliente/CNPJ, se ele está coberto (positivado) segundo o limiar da sua Classificação PDV, no bimestre móvel,
Para que eu veja quem precisa de atenção sem depender da planilha manual da JC.

**Critérios de Aceite:**

**Dado** Clientes Numéricas (Story 7.2) com Classificação e vendas do bimestre móvel (mês corrente + mês anterior, FR14a)
**Quando** calculo o Realizado de Cobertura Numérica
**Então** um cliente Num. A é coberto com compra ≥R$100 no bimestre, Num. B com ≥R$50, Num. C com ≥R$15 (FR25) — limiar lido da Classificação do próprio cliente, não um valor único de vínculo

**E** venda Tipo 5 (bonificada) é excluída do valor considerado

**E** Devolução e Cancelamento são deduzidos do valor, mesmo princípio já usado no motor de Cobertura/Sortimento por Rede (FR29)

**E** o cálculo agrega por GGV → CRV → RCA a partir do Cliente/CNPJ direto — sem passar por nível Rede (FR24), mesmo cuidado de nunca somar valor pré-agregado (Story 4.1)

### Story 7.5: Motor de apuração — Sortimento Numérica (PPA)

Como Supervisor/GGV,
Eu quero que o sistema calcule, por Cliente/CNPJ Classificação A ou B, quantos PPAs distintos ele comprou no bimestre móvel,
Para que eu veja o mix de produtos vendido sem depender da planilha manual da JC.

**Critérios de Aceite:**

**Dado** Clientes Numéricas Classificação A ou B (Classificação C não entra nesta métrica — FR26) e a lista de PPAs (Story 7.3)
**Quando** calculo o Realizado de Sortimento Numérica pro bimestre móvel
**Então** conto PPAs distintos comprados nos 2 meses somados (ex: 7 PPAs no mês anterior + 8 PPAs diferentes esse mês = 15 no total; comprar 2 itens da MESMA família em meses diferentes conta como 1 PPA só), com teto de 15 mesmo que o catálogo tenha mais

**E** venda Tipo 5 (bonificada) é INCLUÍDA — inverso da Cobertura Numérica (Story 7.4)

**E** um item só conta como positivado com ≥3 unidades vendidas se for tipo "UN"; item vendido em CX/Pacote/Display conta com 1 unidade de venda (mesma regra do FR12/Sortimento por Rede)

**E** Devolução e Cancelamento são deduzidos (FR29)

### Story 7.6: Faixas agregadas do distribuidor — Cobertura e Sortimento Numérica

Como admin,
Eu quero configurar múltiplas Faixas (ex: Faixa 1/2/3) para a meta agregada de Cobertura Numérica e Sortimento Numérica, por Indústria/Fornecedor,
Para que o resultado do distribuidor inteiro seja comparado à meta de contrato, reaproveitando o mecanismo de Faixas já genérico do framework (FR7/FR30).

**Critérios de Aceite:**

**Dado** o mecanismo de Faixas do FR7 já usado por Cobertura/Sortimento por Rede
**Quando** configuro Faixa 3/2/1 pra Cobertura Numérica (ex: 4.870/5.681/6.493 lojas positivadas) e pra Sortimento Numérica (ex: 7,33/8,46/15 PPA pra HC; 3,11/3,59/7 PPA pra Foods)
**Então** a apuração agregada do distribuidor compara o resultado total contra essas faixas, do mesmo jeito que já funciona pras métricas por Rede

**E** o Realizado agregado de Sortimento Numérica é a média de PPAs vendidos por cliente — divide pelos clientes elegíveis (A/B) que tiveram QUALQUER compra da Indústria no bimestre (não só quem comprou PPA da lista, nem todos os elegíveis mesmo sem compra nenhuma)

**✅ Resolvida (Carlos/JC, 30/09/2026):** as Faixas de Cobertura (4.870/5.681/6.493 lojas) são as MESMAS pra HC e Foods, cada fornecedor bate esse número de forma independente (não somado). Valores já cadastrados em DEV (29-30/09/2026) — ver commit da migration/dados de teste.

**✅ Decisão provisória tomada (Claudio, 30/09/2026):** o Carlos não entendeu a pergunta original (Questão em aberto #2 do PRD) mesmo reformulada com exemplo numérico — sem resposta dele ainda, o Claudio decidiu adotar "clientes com qualquer compra no bimestre" como denominador (não "só quem comprou PPA", nem "todos os elegíveis"), fácil de reverter se o Carlos confirmar outro. Implementado: campo `RealizadoRede.TeveCompra` (`farol_metas_calculo_numerica.go`), usado em `CalcularRealizadoComPeriodo`/`recalcularTotalDeRedes` (`farol_metas_calculo.go`). Coberto por `TestCalcularRealizado_SortimentoNumericaPPA_DenominadorMediaComCompra`.

### Story 7.7: Painel Web e Mobile — Cobertura Numérica e Sortimento Numérica

Como Supervisor/GGV,
Eu quero ver Cobertura Numérica e Sortimento Numérica como opções de Tipo de Métrica dentro da Indústria (FOOD/HC) que já uso hoje, no painel web e mobile,
Para que eu não precise aprender uma tela nova pra acompanhar o 2º par de métricas do mesmo programa.

**Critérios de Aceite:**

**Dado** os painéis Web (Épico 5) e Mobile (Épico 6) já existentes
**Quando** seleciono a Indústria FOOD ou HC
**Então** o seletor de Tipo de Métrica mostra Cobertura, Sortimento, Cobertura Numérica e Sortimento Numérica — sem um filtro de "Indústria" separado pra Numérica (FR31)

**E** ao selecionar Cobertura Numérica ou Sortimento Numérica, a hierarquia de navegação vai direto de RCA pra Cliente/CNPJ, sem o nível Rede (que não existe nessa métrica — FR24), reaproveitando o mesmo componente de painel (delta explícito, recortes de tempo) já usado pelas métricas por Rede

**✅ A visão "Faturado+Emitido" foi implementada na Story 7.8 (30/09/2026), como 3ª opção de fluxo.**

### Story 7.8: Visão Faturado+Emitido (Cobertura/Sortimento Numérica)

Como Supervisor/GGV,
Eu quero ver o Realizado da Numérica somando Faturado e Transmitido (Emitido), além de poder ver cada um isolado,
Para que eu bata com a apuração oficial do fornecedor, que exige essa 3ª visão explicitamente.

**✅ Status (30/09/2026): IMPLEMENTADA.** GAP CONFIRMADO pela documentação final do Heverton ("PRECISAMOS TER A ANÁLISE NA VISÃO FATURADA, NA VISÃO EMITIDA E NA VISÃO FATURADO+EMITIDO" — sem condicional, texto idêntico em 29/09 e no reenvio de 30/09 via Carlos), fechado no mesmo dia. Fluxo `soma` adicionado só às funções de cálculo da Numérica (`somaPvendaClientesNumerica`/`qtdPorCodProdClientesNumerica` em `farol_metas_calculo_numerica.go`) — soma direta de Faturado líquido (já abatido de devolução/cancelamento, FR29) + Transmitido, mesma semântica do "soma" que existiu no motor Rede até 04/09/2026 (sem deduplicar pedido que progrediu de Transmitido pra Faturado no mesmo período — comportamento histórico, não é bug novo). Escopo deliberadamente restrito à Numérica: as funções equivalentes do motor Rede (`somaPvendaClientes`/`qtdPorCodProdClientes`) NÃO ganharam esse case — reverter o corte de 04/09 pro motor Rede é decisão separada (Pendência abaixo). Painel web e mobile ganharam a 3ª opção de fluxo ("Faturado + Transmitido") só quando a métrica ativa é Numérica (guard-rail: troca automática pra "faturado" se o usuário estava em "soma" e muda pra uma métrica Rede). Coberto por `TestCalcularRealizado_CoberturaNumerica_FluxoSoma` (backend/handlers/farol_metas_calculo_numerica_test.go) — prova que nem Faturado nem Transmitido isolados batem o limiar, só a soma dos dois.

**Critérios de Aceite:**

**Dado** um vínculo/vigência de Cobertura Numérica ou Sortimento Numérica já calculado nos dois fluxos isolados
**Quando** seleciono a visão "Faturado + Emitido"
**Então** o Realizado de cada Cliente/CNPJ é a SOMA do valor Faturado com o valor Transmitido no mesmo bimestre móvel — não uma 3ª consulta redundante, reaproveita o que Faturado e Transmitido isolados já calculam

**E** a dedução de Devolução/Cancelamento (FR29) continua valendo só sobre a parte Faturada (Transmitido não tem CCD — mesma regra já documentada em `subtrairDevolucaoCancelamentoNumerica`)

**E** o painel (web e mobile, Story 7.7) ganha essa 3ª opção no toggle de fluxo, ao lado de Faturado/Transmitido

**Pendência:** decidir (com o Heverton) se essa soma combinada também deve voltar pro motor Cobertura/Sortimento por Rede (Épico 4/5), ou fica restrita à Numérica — Questão em aberto #5 do PRD.
