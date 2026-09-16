# Revisão Adversarial — ARCHITECTURE-SPINE (Integração Multi-Módulo FB)

**Documento revisado:** `ARCHITECTURE-SPINE.md` (2026-09-16, status `draft`)
**Método:** para cada AD, construí um par de implementações concretas — "Time/Módulo A" e "Time/Módulo B" — que obedecem a regra ao pé da letra, mas que produzem contrato incompatível quando os dois lados se encontram no mundo real (via Gateway, via agente, ou via e-mail compartilhado). Cada achado é um buraco na espinha, não uma opinião de estilo.

## Veredito

A espinha acerta o essencial (dono do dado, não recalcular, Gateway como ponto único, dívida técnica nomeada). Mas ela especifica **formato de erro e não especifica formato de sucesso**, especifica **como autenticar mas não como escopar entre chamador-agente e chamador-Gateway**, e tem **uma contradição textual direta** entre AD-1 e a tabela de convenções sobre onde a rota de e-mail vive. Nenhum desses é hipotético de dev picuinha — são exatamente o tipo de coisa que dois times (ou dois agentes de IA autônomos) implementando a mesma espinha sem se falar vão resolver de formas diferentes, e o resultado só aparece quando o Gateway ou o CEO agent tenta consumir os dois de uma vez. Recomendo não promover esta espinha de `draft` sem fechar pelo menos os achados #1, #2, #3 e #6 abaixo — são os que quebram silenciosamente em produção, não em code review.

---

## Achado #1 — AD-5 define envelope de ERRO, mas não define envelope de SUCESSO

**AD envolvida:** AD-5 (Envelope de resposta e erro padronizado)

A regra diz: "JSON em `snake_case`... Erro sempre `{"error": "mensagem"}`". Isso resolve erro. Não diz nada sobre a forma do corpo de SUCESSO.

- **Time FB_FAROL** implementa `/api/farol-jc/objetivos` retornando um array bruto: `[{"cod_fornec": "123", "venda_atual": 45231.90}, ...]`.
- **Time FB_SMARTPICK** implementa `/api/smartpick-jc/mix` retornando `{"data": [...], "meta": {"total": 340, "gerado_em": "2026-09-16T10:00:00-03:00"}}`.

Os dois são 100% `snake_case`, os dois seguem o formato de erro à risca. Nenhum viola AD-5. Mas o FB_CEREBRO, ao agregar os dois pra montar a resposta pro agente do CEO, precisa de código de unwrap **diferente por módulo** (`resp.([]interface{})` vs `resp["data"].([]interface{})`) — exatamente o acoplamento silencioso caso-a-caso que a espinha tenta evitar em outros lugares (AD-1, AD-3). Pior: se o formato de "sucesso" nunca é fixado, qualquer módulo novo (FB_CONTROLADORIA, FB_COMPRAS) é uma nova aposta.

Isso também deixa em aberto: valores monetários como float (`45231.90`) vs string formatada (`"45.231,90"`) vs inteiro em centavos (`4523190`) — todos "snake_case"-compatíveis, todos incompatíveis entre si na hora de somar/subtotalizar no Gateway.

**Recomendação:** fechar um envelope de sucesso mínimo (ex.: sempre `{"data": ..., "meta": {...}}` ou sempre array/objeto bruto — escolher UM) e fixar a representação de valores monetários (unidade, tipo, casas decimais) como parte do AD-5, não deixar implícito.

---

## Achado #2 — Rota de e-mail: AD-1 e a tabela de Consistency Conventions se contradizem sobre o namespace

**ADs envolvidas:** AD-1, AD-7, tabela "Consistency Conventions"

AD-1 fixa: "todo dado exposto pra agente de IA passa pelo facade do módulo... (`/api/<modulo>-jc/*`)". Já a linha de "Naming (rotas)" na tabela de convenções diz, literalmente, lado a lado:

> `/api/<modulo>-jc/<recurso>` pro facade de cada módulo; `/<recurso>-email` pra variante que entrega por e-mail

O segundo padrão **não tem o prefixo** `/api/<modulo>-jc/`. Isso é ambíguo por construção, não só por interpretação livre:

- **Time A** lê AD-1 como regra-mãe e implementa `/api/farol-jc/objetivos-industria-email` (aninhado, namespaced).
- **Time B**, seguindo a tabela ao pé da letra (que é onde o padrão de e-mail está de fato especificado, já que AD-7 só diz "rota tipo `/<recurso>-email?...`" sem prefixo), implementa `/objetivos-industria-email` na raiz do servidor HTTP do módulo.

Os dois estão citando uma parte diferente do MESMO documento como fonte de verdade, e ambos têm razão textualmente. Consequência prática: o agente de IA (ou o Gateway) que aprendeu o padrão com um módulo quebra ao tentar aplicar o mesmo template de URL no outro. Também há risco de colisão: uma rota de e-mail solta na raiz (`/objetivos-email`) pode colidir com a API interna do próprio módulo pro seu frontend React (que não é namespaced por `/api/<modulo>-jc/`).

**Recomendação:** escolher um dos dois padrões explicitamente e corrigir o outro lugar do documento. Dado que a Structural Seed mostra `farol_jc_email.go` ao lado de `farol_jc_rest.go` (mesmo pacote de handlers, sugerindo mesmo namespace), o mais provável é que a intenção seja `/api/<modulo>-jc/<recurso>-email` — mas isso precisa estar escrito, não inferido do nome de arquivo.

---

## Achado #3 — AD-6 não distingue "token pro agente" de "token pro Gateway", e a redação força um design inseguro

**AD envolvida:** AD-6

A regra diz "cada serviço lê **um único** token Bearer de uma env var no boot". Mas o Gateway (FB_CEREBRO) também é um chamador HTTP dos facades (AD-3) — ou seja, do ponto de vista do FB_FAROL, existem DOIS tipos de chamador externo: o agente de IA direto (Paperclip, quando o recurso é single-module, AD-2) e o Gateway (quando é multi-module). AD-6 não diz se esses dois chamadores usam o MESMO token ou tokens diferentes.

- **Time A (FB_FAROL)** interpreta "um único token" literalmente: emite UMA env var (`FAROL_JC_TOKEN`) usada tanto pelos agentes Paperclip quanto pelo `farolclient` do Gateway. Resultado: para revogar/rotacionar o acesso do Gateway (ex.: Gateway comprometido), é preciso revogar TAMBÉM o acesso de todo agente que chama o facade direto — não dá pra separar os dois sem quebrar AD-6 ("um único").
- **Time B (FB_SMARTPICK, futuro)**, percebendo esse problema de segurança, emite DUAS env vars (`SMARTPICK_JC_TOKEN_AGENT`, `SMARTPICK_JC_TOKEN_GATEWAY`) — o design mais correto, mas que **viola** a letra de AD-6 ("um único token Bearer").

Isso é o oposto do padrão dos outros achados: aqui a leitura mais literal da regra é que produz o design pior, e o design melhor é que desobedece a regra. Dois módulos vão divergir de propósito, e nenhum dos dois está "errado" segundo o texto atual.

**Recomendação:** AD-6 precisa dizer explicitamente se token-de-agente e token-de-Gateway são a mesma credencial ou credenciais distintas por chamador, e permitir explicitamente mais de uma env var de token por serviço quando o motivo for segmentar chamador (não seria "token por request", continuaria sendo estático e por env var).

---

## Achado #4 — AD-3 só vincula (`Binds`) o FB_CEREBRO; não diz nada sobre módulo acessando banco de outro módulo diretamente (fora do Gateway)

**AD envolvida:** AD-3

O texto da regra: "todo código NOVO **no Gateway** chama a API HTTP do módulo dono... nunca conecta direto no Postgres de outro módulo." O campo `Binds` lista só `FB_CEREBRO`.

- **Time A (FB_COMPRAS, futuro)** precisa do cadastro de clientes que hoje mora no FB_FAROL. Interpretando AD-3 à risca ("isso é uma regra pro Gateway"), conclui que nada impede FB_COMPRAS de abrir uma conexão Postgres read-only direta na base do FB_FAROL — porque ele não é o FB_CEREBRO, e AD-1/AD-3 não binds ele nessa proibição especificamente para leitura módulo-a-módulo (só pro Gateway).
- **Time B (FB_CONTROLADORIA, futuro)**, lendo o **espírito** do parágrafo de abertura ("Cada módulo FB_* é dono do seu domínio"), assume que a mesma proibição vale para qualquer módulo-a-módulo, não só Gateway-a-módulo, e implementa um cliente HTTP fino contra o facade do FB_FAROL.

Os dois times leram o mesmo documento e chegaram a arquiteturas de acoplamento opostas — um dos dois vai estar acessando o Postgres do FB_FAROL sem contrato, sem teste, exposto a quebrar a cada migration do FB_FAROL (exatamente o risco que AD-3 nomeia como motivo de existir, só que agora fora do escopo que o `Binds` cobre). O parágrafo "Deferred" até menciona isso de leve ("Assumido apenas que, quando existirem, herdam AD-1/AD-3"), mas "herdar" não é a mesma coisa que "Binds" formal — e um assumido em prosa de rodapé não é uma regra com o mesmo peso de cumprimento que as outras.

**Recomendação:** ou generalizar o `Binds` de AD-3 para "todos os módulos FB_*, não só o Gateway", ou criar uma AD-3b explícita cobrindo acesso módulo-a-módulo fora do Gateway (que hoje simplesmente não existe como caminho sancionado nem proibido).

---

## Achado #5 — AD-4: "AAAA-MM" não tem fuso, não tem regra de "até quando" e não tem tratamento de intervalo invertido

**AD envolvida:** AD-4

A regra fixa dois formatos de entrada, mas não fixa a semântica de resolução:

- **Time A** resolve `"2026-09"` como o mês calendário inteiro (01/09 a 30/09), fuso America/Sao_Paulo, inclusive nos dois extremos.
- **Time B**, replicando o comportamento já existente no FB_FAROL segundo a própria memória do time (presets capados em `ultimo_dia_importado`, D-1), resolve `"2026-09"` como 01/09 até o último dia efetivamente importado — que em pleno mês corrente pode ser 14/09, não 30/09.

Os dois "replicaram exatamente as duas formas de entrada" (o que AD-4 exige), mas o MESMO parâmetro `"2026-09"` produz dois intervalos de dados diferentes em dois módulos. Quando o Gateway soma "vendas de setembro" do módulo A com "vendas de setembro" do módulo B pro dashboard do CEO, está somando dois recortes de tempo diferentes sob o mesmo rótulo — um erro que não aparece em nenhum teste unitário de parser (porque cada parser, isoladamente, está "correto"), só aparece na consolidação.

Também não especificados: fuso horário do parsing (relevante perto de meia-noite), se o intervalo `"DD/MM/AAAA a DD/MM/AAAA"` é inclusive nos dois extremos, e o que fazer se a data final vier antes da inicial.

**Recomendação:** AD-4 precisa dizer não só o formato de string, mas a regra de resolução (mês calendário cheio vs capado em último dado importado) e o fuso horário canônico — como parte do contrato replicável, não como implementação implícita de `parsePeriodo`.

---

## Achado #6 — Recurso reivindicado por dois donos: "dono do dado" pressupõe um mapa de domínio→módulo que a espinha não fornece

**AD envolvida:** AD-1 (e por extensão AD-3)

AD-1 previne "módulo A reimplementando... lógica que já existe no módulo B". Isso só funciona se "quem é dono de qual dado" for uma pergunta com resposta única e conhecida por todos os times ao mesmo tempo — e a espinha não lista esse mapa em lugar nenhum (não há glossário de domínio, nem uma tabela "dado X → módulo dono").

- **Time A (FB_FAROL)** expõe `/api/farol-jc/faturado` calculando "faturado" como valor transmitido/faturado do lado da força de vendas (ION VENDAS).
- **Time B (FB_CONTROLADORIA, em negociação)**, construindo independentemente e sem saber que o FB_FAROL já expõe algo com esse nome, expõe `/api/controladoria-jc/faturado` calculando "faturado" como receita reconhecida contabilmente (podem divergir por conta de devolução, glosa fiscal, etc. — divergência que a própria memória do projeto já registrou como fonte real de bug: "REL 322... Líquido").

Os dois obedecem AD-1 ao pé da letra do próprio ponto de vista ("estou expondo o dado que meu módulo já calcula, não estou recalculando o de ninguém") — porque nenhum dos dois tinha como saber que o outro também se considera dono do conceito "faturado". Isso é exatamente o padrão "duas entidades reivindicando dono do mesmo dado" pedido no brief: a colisão não é de código, é de nome de conceito de negócio sem árbitro.

**Recomendação:** antes de o segundo módulo (FB_CONTROLADORIA) nascer, a espinha (ou um companion dela) precisa de um mini-glossário de domínio: quais conceitos de negócio (faturado, cobertura, positivação, mix) são de qual módulo, para que a checagem de AD-1 seja factível e não dependa de boa vontade/coincidência entre times que nunca se falam.

---

## Achado #7 — AD-9: a escapatória "sem DATABASE_URL, pula, nunca falha" pode mascarar exatamente a regressão que a AD diz prevenir

**AD envolvida:** AD-9

A regra distingue lógica pura (parsing de período, montagem de HTML, roteamento) — testável sem infra — de chamada real a banco/HTTP — testes de integração, "ausente, o teste pula (nunca falha o build)".

O problema é que a fronteira entre "pura" e "precisa de banco" não é auto-evidente, e a AD delega essa classificação a cada implementador:

- **Time A** implementa `parsePeriodo` recebendo todos os parâmetros necessários (incluindo `ultimo_dia_importado`, se for o caso) como argumento — função pura, 100% testável sem rede, exatamente como AD-9 pede.
- **Time B**, num módulo novo, implementa a mesma função de parsing/resolução de período mas, por conveniência de reuso de código, faz ela consultar o próprio banco internamente pra descobrir "qual é o último dia com dado" (em vez de receber isso como parâmetro). Sem querer, transformou lógica de parsing em lógica que "precisa de banco" — e agora o teste dessa função só roda com `DATABASE_URL` setado. Em CI sem banco configurado, o teste **pula silenciosamente** (comportamento explicitamente sancionado por AD-9: "nunca falha o build"), e a suíte fica verde com ZERO cobertura real de parsing de período — o cenário exato que a própria AD-9 lista como o que ela quer evitar ("suíte que só roda com banco real disponível, escondendo regressão de lógica pura").

AD-9 acerta a intenção mas não dá ao Time B nenhum sinal de que ele cruzou a linha — não há uma regra tipo "toda função que a AD-4 classifica como parser de período DEVE ser pura, ponto", só uma categoria geral ("lógica pura") que depende de quão disciplinado é quem escreve o código.

**Recomendação:** tornar explícito, por nome de responsabilidade (não só por AD-4/AD-7 "tipo de lógica"), que period-parsing e HTML-de-e-mail NÃO PODEM ter I/O de nenhum tipo dentro de si — e cobrar isso via lint/CI (ex.: teste falha se a função de parsing receber um `*sql.DB` ou `context` com timeout de rede), não só via convenção de "nível de teste".

---

## Achado #8 — SMTP compartilhado (AD-7): recurso mutável compartilhado sem política de alocação

**AD envolvida:** AD-7

A regra diz: "Mesma conta SMTP (Hostinger)... reaproveitada por todos os módulos via env vars — mas cada módulo implementa seu próprio sender pequeno (sem lib compartilhada)."

Isso é deliberado (evitar lib compartilhada entre repos/linguagens diferentes) mas cria um recurso compartilhado — quota/reputação de envio da conta Hostinger — sem NENHUMA coordenação entre os "senders" independentes:

- **Time A (FB_FAROL)** implementa envio síncrono, um e-mail por chamada de agente, uso esporádico.
- **Time B (FB_SMARTPICK ou FB_COMPRAS, futuro)** implementa um "disparo em lote" (ex.: manda o painel pra 50 GGVs de uma vez, cada request HTTP disparando um e-mail), porque nada em AD-7 proíbe isso — a regra só fala do formato da entrega (server-side, rota `-email`), não de volume/taxa.

Individualmente cada um está certo. Juntos, o segundo pode estourar rate-limit/reputação da conta Hostinger compartilhada e derrubar a entrega de e-mail de TODOS os módulos (incluindo o FB_FAROL, que é o "core value" do sistema per o CLAUDE.md do projeto). Não há dono de "quanto cada módulo pode mandar por hora/dia", nem convenção de remetente (`from`) — o que também abre risco de dois módulos usando o mesmo endereço de exibição pra coisas diferentes, confundindo quem recebe.

**Recomendação:** AD-7 (ou um companion de operação) precisa de uma política mínima de uso do recurso compartilhado: limite de envio por módulo/por janela de tempo, e convenção de remetente (`from`/nome de exibição) por módulo, para que a conta continue saudável pra todos.

---

## Achado #9 — AD-2/AD-3: nada tecnicamente distingue "chamada do Gateway" de "chamada direta de agente" no facade do módulo

**ADs envolvidas:** AD-2, AD-6

AD-2 permite que um agente single-module pule o Gateway e chame o facade direto. Isso é uma convenção de **uso**, não uma restrição técnica: o facade do módulo, do ponto de vista do request HTTP, recebe um Bearer token válido tanto se vier do Gateway quanto se vier de um agente Paperclip direto — nada no AD-6 (auth) diferencia os dois.

- **Time A**, construindo o facade do FB_FAROL, não se importa com quem está chamando — qualquer portador do token é igual, exatamente como AD-6 pede.
- **Time B**, construindo o agente de monitoramento do CEO (multi-module por definição, deveria SEMPRE passar pelo Gateway per AD-2), decide — por latência, ou por engano — chamar o facade do FB_FAROL diretamente para um dos módulos, e só passar pelo Gateway pros outros. Nada no lado do servidor detecta ou impede isso; a violação de AD-2 é silenciosa e só seria pega por auditoria manual de logs, se alguém procurar.

Isso não quebra nada tecnicamente hoje (o dado retornado é o mesmo), mas mina o valor central de AD-2 ("lógica de agregação num lugar só testável") sem deixar rastro, e — combinado com o Achado #1 (envelope de sucesso não padronizado) — significa que se o Gateway algum dia decidir transformar/renomear campos ao agregar (para evitar colisão entre módulos, um caso de uso legítimo), o agente que "furou" o Gateway vê um formato diferente do que veria se tivesse passado por ele — inconsistência que ninguém vai prever em design, só descobrir em produção.

**Recomendação:** ao menos logar/instrumentar, no facade de cada módulo, se o Bearer token que chegou é o "token de Gateway" ou "token de agente direto" (retomando o Achado #3 — tokens distintos por chamador resolveria os dois problemas de uma vez), para que AD-2 seja auditável e não apenas uma convenção de cavalheiros.

---

## Resumo priorizado

| # | Achado | Severidade | Por quê |
|---|--------|------------|---------|
| 1 | Envelope de sucesso não definido (só erro) | Alta | Gateway precisa de unwrap bespoke por módulo; quebra objetivo central de AD-5 |
| 2 | Rota de e-mail: AD-1 vs tabela de convenções se contradizem | Alta | Ambiguidade textual direta no próprio documento, não interpretação |
| 3 | AD-6 não separa token-agente de token-Gateway | Alta | Segurança: revogar um revoga o outro; ou força violação literal da AD pra ser seguro |
| 6 | "Dono do dado" sem glossário de domínio | Alta | É o caso clássico de "duas entidades reivindicando dono do mesmo dado" pedido na tarefa |
| 4 | AD-3 só vincula o Gateway, não módulo-a-módulo | Média-Alta | Path de acoplamento direto a banco alheio fica sancionado por omissão fora do Gateway |
| 5 | AD-4 não fixa fuso/resolução de "AAAA-MM" | Média | Mesmo rótulo de período, dois recortes de dado diferentes agregados como se fossem iguais |
| 7 | Escapatória de AD-9 pode esconder zero cobertura de lógica pura | Média | O próprio mecanismo de skip que a AD normatiza é o vetor do problema que ela quer evitar |
| 8 | SMTP compartilhado sem política de quota/remetente | Média | Um módulo em lote pode derrubar e-mail de todos, inclusive do core value do FB_FAROL |
| 9 | Nada distingue chamada Gateway vs chamada direta no server | Baixa-Média | Convenção sem enforcement; risco maior é silêncio/auditoria, não quebra imediata |

**Recomendação geral:** antes de sair de `status: draft`, fechar pelo menos #1, #2, #3 e #6 com uma frase explícita cada um na própria espinha (não como nota de rodapé em Deferred) — são os quatro que geram dado divergente ou credencial mal-desenhada por leitura literal legítima, não por desvio de time. Os demais (#4, #5, #7, #8, #9) podem virar itens de Deferred com dono e prazo, mas não deveriam ficar de fora do radar: cada um já tem um par concreto de implementação que os quebra.
