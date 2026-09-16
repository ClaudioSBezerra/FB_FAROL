# Revisão Independente — ARCHITECTURE-SPINE.md (Integração Multi-Módulo FB)

**Revisor:** agente independente (checklist "good-spine")
**Data:** 2026-09-16
**Alvo:** `architecture-FB_FAROL-2026-09-16/ARCHITECTURE-SPINE.md`
**Método:** leitura do documento + verificação cruzada contra o código real de `FB_FAROL` (repo local) e contra uma cópia de `cerebro-jc-api` (main.go, go.mod, Dockerfile) disponível no scratchpad desta sessão. Achados marcados "[verificado no código]" foram confirmados por leitura direta de arquivo, não por suposição.

## Veredito

O documento é sólido nos pontos que decidiu cobrir — as ADs são majoritariamente verificáveis e batem com o código real (grande acerto: isso é raro) — mas tem uma lacuna crítica no envelope operacional (mecânica da própria migração que é o objetivo imediato desta rodada) e dois pontos de inventário/ratificação factualmente incompletos no brownfield do `cerebro-jc-api` que, se não corrigidos, vão gerar exatamente o tipo de divergência silenciosa que a espinha existe para prevenir.

---

## CRÍTICO

### C1 — Mecânica da migração para git+CI/CD (o objetivo imediato desta rodada) não está decidida nem em Deferred

AD-8 fixa o estado FINAL desejado ("todo serviço é um app Coolify git-based") mas não decide nada sobre a TRANSIÇÃO do estado atual (binário manual rodando em `/opt`, porta 8090, sem git) para esse estado final — que é literalmente o trabalho que motivou esta espinha agora.

Faltam, sem estarem decididos nem no Deferred:
- Como os secrets atuais (`DATABASE_URL`, `API_TOKEN`, lidos hoje de env vars do processo manual) migram pra env vars do Coolify sem ficarem expostos em trânsito ou em log de build.
- O que acontece com o processo manual já rodando na porta 8090 durante o cutover — conflito de porta, quem derruba o processo antigo e quando, se há downtime aceito ou janela de manutenção.
- Rollback: se o container buildado pelo Coolify se comportar diferente do binário manual atual (ex.: `go mod tidy` rodando no build — ver Dockerfile linha 6 — pode resolver dependências diferentes das que estão hoje no binário rodando), qual é o plano B.

Isso é exatamente o "envelope operacional/ambiental" que o brief pediu pra escrutinar com atenção especial. Está em silêncio total — nem AD, nem Deferred.

**Recomendação:** adicionar uma AD (ou um item de Deferred explicitamente delimitado, se a decisão for "cada implementador resolve na hora") cobrindo pelo menos: fonte da verdade dos secrets em produção, e o procedimento de corte (quem derruba o processo antigo, em que ordem).

---

## ALTO

### A1 — AD-3 subestima o inventário de rotas legadas: são 3 rotas com acesso direto ao banco, não 2 [verificado no código]

AD-3 e o item de Deferred correspondente falam em "as duas rotas legadas do `cerebro-jc-api` (Cobertura, Faturado)". Lendo `main.go` do `cerebro-jc-api` real, existem **três** handlers que abrem `pool.Query`/`pool.QueryRow` direto no Postgres do FB_FAROL:

- `coberturaHandler` (`/cobertura`)
- `resumoHandler` (`/resumo`) — **não citado em lugar nenhum do documento**
- `faturadoFornecedorHandler` (`/faturado-fornecedor`)

O item de Deferred nomeia como alvo de migração apenas `/api/farol-jc/cobertura` e `/api/farol-jc/faturado-fornecedor` — não existe alvo equivalente para `/resumo`. Se este documento for usado como o inventário definitivo da dívida técnica (é exatamente pra isso que ele existe), `/resumo` vai ficar esquecido: continuará lendo direto do Postgres do FB_FAROL indefinidamente, sem que ninguém perceba que ele nunca foi migrado, porque a espinha não o menciona como pendência.

**Recomendação:** corrigir AD-3 e o Deferred pra "três rotas legadas (Cobertura, Resumo, Faturado)" e nomear o facade-alvo equivalente para `/resumo` (ou justificar explicitamente por que ele fica de fora, se for esse o caso).

### A2 — AD-6 é declarada `[ADOPTED]` sem exceção, mas existe uma 4ª rota em produção que não tem nenhuma checagem de token [verificado no código]

`painelHandler` (rota `/painel`, servindo `painel-fornecedor.html` via `//go:embed`) é a única rota do `cerebro-jc-api` que **não** chama `authOK(r)` — ao contrário de `/cobertura`, `/resumo` e `/faturado-fornecedor`, que checam Bearer token. AD-6 diz "cada serviço lê um único token Bearer... nunca aceita token/empresa_id vindo do corpo da request" e está marcada `[ADOPTED]` para "todos os módulos, FB_CEREBRO" sem ressalva. O Structural Seed mantém `painel-fornecedor.html` como artefato ratificado, sem qualquer nota reconciliando essa contradição.

Isso deixa duas leituras possíveis e nenhuma delas está escrita:
1. `/painel` precisa ganhar checagem de token pra cumprir AD-6 (o que quebra quem hoje acessa a página direto do navegador sem cabeçalho Authorization);
2. `/painel` é uma exceção intencional (página pública tipo dashboard), e AD-6 deveria dizer isso explicitamente — inclusive porque o projeto já tem precedente formal de "exceção precisa de decisão explícita, não de precedente" (ver CLAUDE.md sobre `/farol/dinheiro-na-mesa`).

Sem essa decisão escrita, quem for portar `cerebro-jc-api` pra git decide sozinho, e o critério "todo serviço" da AD-6 vira falso perante o próprio código que a espinha está ratificando.

### A3 — Possível contradição entre AD-7 ("painel sempre por e-mail") e o `/painel` ao vivo mantido no Structural Seed

AD-7 estabelece que entrega de painel pra humano é sempre e-mail server-side, e lista como motivo a instabilidade de anexos de agente de IA. Isso é uma decisão sólida pro fluxo *agente → e-mail*. Mas o Structural Seed ratifica, sem comentário, que `painel-fornecedor.html` continua existindo como página web ao vivo — um canal de distribuição diferente (URL persistente, não e-mail on-demand). O documento nunca diz se:
- os dois canais coexistem por design (live dashboard pra quem quer olhar agora + e-mail pra entrega assíncrona), ou
- `/painel` é dívida a ser eventualmente substituída pelo padrão e-mail e simplesmente ainda não foi migrada.

Como a AD-7 usa a palavra "sempre", um implementador lendo literalmente pode concluir que `/painel` está em desacordo com a própria regra que o documento acabou de fixar — ou pode simplesmente ignorar a regra achando que "painel ao vivo" é um caso diferente, sem isso estar escrito em lugar nenhum.

### A4 — "Operação" (health-check, monitoramento, alertas) está em silêncio total, apesar de precedente já existir no código [verificado no código]

O FB_FAROL já tem uma rota `/api/health` registrada em `main.go`. A espinha não ratifica esse padrão pra FB_CEREBRO nem pros módulos futuros, e não há nenhuma AD ou item de Deferred cobrindo observabilidade (health-check, logs estruturados, alerta de falha) — nem para os módulos hoje, nem para o Gateway que vai depender deles no ar pra responder ao agente do CEO. Dado que o pedido de revisão pediu atenção especial ao envelope operacional, esse é um silêncio real: nem decidido, nem adiado.

---

## MÉDIO

### M1 — AD-2 não é tecnicamente verificável/executável, só é uma convenção de configuração de agente

A regra de AD-2 ("agente que consulta mais de um módulo chama SÓ o FB_CEREBRO") não tem nenhum mecanismo técnico que a garanta — nenhum controle de rede, nenhum escopo de token que impeça um agente Paperclip mal configurado (ou um agente futuro) de chamar dois facades de módulo diretamente, sem passar pelo Gateway. O "Prevents" da AD-2 (lógica de agregação espalhada, N URLs) só se sustenta enquanto a configuração de cada agente no Paperclip for disciplinada — o que é uma garantia social/de processo, não arquitetural. Não é necessariamente errado manter assim, mas o documento apresenta como se fosse uma garantia da arquitetura quando na prática depende inteiramente de como cada agente é instruído.

### M2 — Não há estratégia de contrato/versionamento entre o facade de um módulo e o cliente HTTP do Gateway

AD-3 resolve *onde* o dado deve ser buscado (HTTP, nunca banco direto), mas não resolve o problema que ela mesma cita como motivação ("uma migration no FB_FAROL quebrando o FB_CEREBRO sem nenhum contrato/teste entre os dois repos"). Trocar acesso a banco por chamada HTTP elimina o acoplamento de schema, mas não cria nenhum contrato entre a resposta do facade (`/api/farol-jc/*`) e o cliente (`farolclient/`) que a consome — sem versionamento de rota, sem schema compartilhado, sem teste de contrato. Se o FB_FAROL mudar o formato de resposta do facade, nada no repo do FB_CEREBRO detecta isso além de quebra em runtime. Dado que os repos podem divergir de linguagem/ritmo de release, esse é um ponto real de divergência silenciosa que a AD-3 não fecha, apesar de nomear o problema.

### M3 — Nenhuma política de resiliência (timeout/retry/falha parcial) para as chamadas de agregação do Gateway

FB_CEREBRO existe pra agregar N módulos numa única resposta pro agente do CEO. O documento não decide o que acontece quando um dos N facades está fora do ar: o Gateway falha a chamada inteira (500), ou devolve uma resposta parcial com o que conseguiu agregar? Sem isso decidido — nem deferido — cada par Gateway↔módulo (hoje FB_FAROL, amanhã SMARTPICK/CONTROLADORIA/COMPRAS) pode implementar um comportamento diferente, o que é justamente o tipo de inconsistência que uma espinha "initiative" deveria fechar, dado que times/módulos diferentes vão implementar isso de forma independente.

### M4 — Estratégia de ambientes (dev/staging/produção) não é mencionada

AD-8 decide só o mecanismo de deploy (push pro `main` → build+redeploy). Não há menção a se existe (ou deveria existir) um ambiente de staging/preview antes de produção, ou se — como aparenta ser o padrão atual do FB_FAROL pelo histórico de commits — todo merge no `main` vai direto pra produção. Se essa for de fato a convenção assumida, vale declarar explicitamente (ratificando o padrão já em uso), em vez de deixar implícito.

---

## BAIXO

### B1 — Frontmatter com `binds`, `sources` e `companions` vazios

O documento analisa e referencia arquivos concretos (`farol_jc_rest.go`, `farol_jc_email.go`, `cerebro-jc-api/main.go`) mas o frontmatter não lista nenhuma fonte. Não é um erro de conteúdo, mas reduz a rastreabilidade do documento — um item de baixo custo para melhorar.

### B2 — AD-9 ainda não é cumprível pelo código real do `cerebro-jc-api` sem refactor prévio [verificado no código]

A convenção de teste da AD-9 (lógica pura isolada e testável sem dependência externa) está corretamente ratificada contra o FB_FAROL (confirmado por grep: `farol_tipos_metrica_test.go` etc. usam exatamente esse padrão). Mas no `cerebro-jc-api` atual, toda a lógica (parsing de query params, cálculo de totais/percentuais em `faturadoFornecedorHandler`, classificação de "crítico"/"atenção" por `dias_sem_comprar` em `resumoHandler`) está misturada dentro dos handlers HTTP, junto com a chamada ao banco — nada disso é extraível pra teste de unidade sem refactor. Não é um erro do documento (é uma regra válida pra código novo), mas vale um Deferred explícito reconhecendo que aplicar AD-9 ao código legado do CEREBRO exige separar lógica pura dos handlers antes.

### B3 — Tabela de convenções de nomenclatura é ambígua sobre onde o `-email` route vive

A tabela em "Consistency Conventions" lista `/api/<modulo>-jc/<recurso>` e `/<recurso>-email` como se fossem dois padrões paralelos e independentes. No código real (ratificado no Structural Seed), a rota de e-mail vive DENTRO do prefixo do módulo (`/api/farol-jc/objetivos-industria-email`, confirmado em `farol_jc_rest.go`/`farol_jc_email.go`), não como rota solta no nível raiz. Vale deixar isso explícito na tabela pra não incentivar alguém a registrar a variante de e-mail fora do namespace do módulo (como o `cerebro-jc-api` legado faz hoje com `/cobertura`, `/resumo`, `/faturado-fornecedor`, todos sem prefixo).

### B4 — Go 1.22 (FB_CEREBRO) vs Go 1.26.1 (FB_FAROL): não há decisão sobre se a migração pra git+CI/CD deve incluir upgrade da toolchain

Ambos os valores estão corretos e verificados contra os `go.mod` reais — não é erro factual. Mas a espinha não diz se, ao trazer o serviço pra git, a versão do Go deve ser alinhada à do resto da plataforma ou fica deliberadamente como está. Baixo risco, mas é uma decisão pendente que a rodada de migração provavelmente vai precisar tomar de qualquer forma.

---

## Pontos fortes (não é só achado negativo)

- As ADs 4, 5, 6 e 9 batem exatamente com o código real do FB_FAROL quando verificadas diretamente (`parsePeriodo` em `farol_mcp.go`, envelope de erro, padrão de token único via env var em `farol_jc_rest.go`, convenção de teste com skip por `DATABASE_URL`). Isso é o oposto de "documento de arquitetura aspiracional que não bate com o código" — aqui bate, e isso é o valor central de uma boa espinha.
- O Stack table está com números plausíveis E confirmados: Go 1.26.1 (FB_FAROL) e Go 1.22 / pgx v5.5.5 (cerebro-jc-api) batem exatamente com os `go.mod` reais dos dois serviços.
- O Deferred é honesto sobre dívida técnica em vez de escondê-la (AD-3 nomeia explicitamente as rotas legadas como problema aceito, não como se não existisse) — só está incompleto (ver A1).
- O diagrama Mermaid marca corretamente a aresta `GW -.-> FDB` como "DÍVIDA TÉCNICA", o que é uma forma honesta de expor visualmente o desvio da regra proposta.
