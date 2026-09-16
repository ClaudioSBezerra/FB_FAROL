# Revisão independente — Seção "Stack" do ARCHITECTURE-SPINE.md

**Documento revisado:** `architecture-FB_FAROL-2026-09-16/ARCHITECTURE-SPINE.md`
**Foco:** toda versão nomeada na seção "Stack" (linhas 137-148) foi checada contra
a realidade (código existente / web), ou foi assumida por conhecimento de
treinamento?
**Método:** (1) leitura do `go.mod`/`go.sum` real do FB_FAROL; (2) tentativa de
localizar o repo `FB_CEREBRO`/`cerebro-jc-api` neste ambiente (não encontrado —
vive no host de produção, fora do meu alcance nesta sessão); (3) busca na web
para confirmar existência/plausibilidade de cada versão.

## Veredito geral

**A metade do quadro que descreve o FB_FAROL está solidamente verificada
contra o código real — inclusive com evidência forte de que foi checada, não
assumida. A metade que descreve o FB_CEREBRO/cerebro-jc-api é plausível mas
NÃO pôde ser confirmada por mim nem tem evidência interna de ter sido checada
contra o host real — é o ponto de maior risco de "afirmação por
treinamento" no documento.**

---

## Item a item

### Go (FB_FAROL) — `1.26.1`

- **Verificado:** SIM, contra código real. `/home/claudio/projetos/FB_FAROL/backend/go.mod` linha 3: `go 1.26.1`, match exato.
- **Existe/é real?** Sim. Busca na web confirma Go 1.26.1 lançado em 2026-03-05 (patch de segurança sobre `crypto/x509`, `html/template`, `net/url`, `os`), anunciado no golang-announce.
- **Achado relevante:** meu *knowledge cutoff* é janeiro/2026. Go 1.26.1 saiu em março/2026 — **depois** do meu cutoff. Isso é evidência forte (não só circunstancial) de que quem escreveu o documento leu o `go.mod` real em vez de recitar de memória: não seria possível "lembrar" dessa versão de treinamento.
- **Risco:** nenhum. Está correto e é a versão mais atual da série 1.26 no momento (havia também uma 1.26.2 de segurança citada nos resultados de busca — vale conferir se o `go.mod` não ficou defasado por essa patch, mas isso é rotina de manutenção, não erro de arquitetura).

### Go (cerebro-jc-api / FB_CEREBRO) — `1.22`

- **Verificado:** NÃO pude confirmar contra o código real. O repo/host `/opt/cerebro-jc` não existe neste ambiente (é citado no próprio documento como "sem git", vivendo só no servidor). Tentativa de acesso SSH foi bloqueada pelo classificador de permissão desta sessão (exploração de credenciais).
- **Existe/é real?** Sim, Go 1.22 é uma versão real, lançada em 2024-02-06.
- **Risco / achado:** Go 1.22 é uma versão **antiga e fora da janela de suporte oficial** (o time Go só dá suporte às duas últimas versões maiores; em set/2026, com Go 1.26 corrente, o 1.22 está sem patch de segurança há mais de um ano). Isso por si não é implausível — é coerente com a narrativa do próprio documento de que o `cerebro-jc-api` é código legado, "binário buildado manualmente, sem git" (AD-8). Mas o documento **não distingue** "essa versão foi lida do `go.mod` real no host" de "essa é uma suposição de stack legado plausível" — e não registra o risco de rodar um serviço de produção em Go EOL há >1 ano em nenhuma seção (nem em Deferred). Recomendo ao autor confirmar explicitamente (via SSH já documentado em memória do projeto: `claude_ro`/`coolify_hostinger`) se `1.22` veio de um `go version`/`go.mod` real, e considerar registrar o EOL como item de dívida técnica.

### `github.com/jackc/pgx/v5` (FB_CEREBRO) — `v5.5.5`

- **Verificado:** NÃO pude confirmar contra o código real (mesmo motivo acima — repo fora do meu alcance).
- **Existe/é real?** Sim. Confirmado via GitHub: tag `v5.5.5` existe em `jackc/pgx`.
- **Risco / achado:** v5.5.5 é de ~janeiro/2024 — bem anterior ao meu cutoff (jan/2026), portanto **não há como distinguir, só pela versão, se foi lida do código real ou "chutada" de treinamento** (ao contrário do caso do Go 1.26.1 acima, que só podia vir de leitura real). É uma versão razoável para um serviço legado não tocado há tempos, mas isso é exatamente o tipo de detalhe que merece confirmação explícita (grep no `go.sum` do host), não inferência.

### `github.com/lib/pq` (FB_FAROL) — `v1.11.2`

- **Verificado:** SIM, contra código real. `go.mod` linha 7 e `go.sum` (hash presente) confirmam `v1.11.2` exato.
- **Existe/é real?** Sim. Busca na web confirma release `v1.11.2` do `lib/pq`, publicado em 2026-02-10 por `arp242`, corrigindo duas regressões (compatibilidade com Supavisor e envio de `dbname`).
- **Achado relevante:** de novo, essa release é de **fevereiro/2026 — depois do meu cutoff de treinamento**. Não há como eu (nem, por extensão, quem escreveu o documento, se fosse só "recitar de memória") saber dessa versão sem ter lido o `go.mod`/`go.sum` real. Evidência forte de verificação real, não suposição.
- **Nota lateral:** o próprio `go.mod` do FB_FAROL declara `require github.com/lib/pq v1.11.2` fora do bloco padrão de dependências diretas junto com `golang-jwt/jwt/v5` — sem inconsistência com o documento.

### `github.com/modelcontextprotocol/go-sdk` (FB_FAROL) — `v1.8.0`

- **Verificado:** SIM, contra código real. `go.mod` linha 15 e `go.sum` confirmam `v1.8.0` exato.
- **Existe/é real?** Sim. Múltiplos PRs de dependabot/renovate em repositórios reais confirmam a existência de `v1.8.0`. As notas de release mencionam que a versão mais nova de protocolo negociada é de 2026-07-28 — ou seja, o SDK em si teve atividade de release **depois** do meu cutoff.
- **Achado relevante:** mesmo padrão dos dois itens anteriores — versão real, recente, só verificável por leitura do código, não por treinamento. O comentário do documento ("existe, não é o caminho de integração ativo") também bate com a realidade: `farol_mcp.go` existe no repo (16.586 bytes, editado há poucas horas) mas as rotas em uso ativo hoje (conforme `git log` recente: "endpoint de descoberta de indústrias", "API REST simples pro mesmo dado do MCP, pro Paperclip") são as REST novas, não o MCP.

### Postgres (compartilhado) — sem versão

- Documento não afirma uma versão de Postgres — só descreve topologia (instância compartilhada, dívida técnica no AD-3). Não há o que verificar como "versão inventada"; é uma omissão razoável, não uma afirmação arriscada.

### Coolify (deploy) — sem versão, "instância própria, já em uso pro FB_FAROL"

- Coerente com o CLAUDE.md do projeto ("Stack: Go 8087 + React 3087 + Postgres + Coolify — sem mudança de stack") e com a memória do projeto sobre acesso SSH/infra de produção. Não é uma versão nomeada, é reafirmação de infraestrutura já documentada em outro lugar do próprio repo — baixo risco.

### SMTP (Hostinger) — sem versão, "já configurado, reaproveitado por todos os módulos"

- Mesma categoria do item anterior: não é uma "versão" tecnicamente, é uma credencial/infra já existente, e o AD-7 do próprio documento já contextualiza isso corretamente ("mesma conta SMTP... reaproveitada... via env vars"). Nada a verificar como se fosse invenção de versão.

---

## Achados principais (resumo para ação)

1. **As 4 versões do FB_FAROL (Go 1.26.1, lib/pq v1.11.2, go-sdk v1.8.0, e a topologia de arquivos citada em "Structural Seed") batem exatamente com o `go.mod`/`go.sum` real e com os arquivos no disco** (`farol_jc_rest.go`, `farol_jc_email.go`, `farol_mcp.go` todos existem, editados nas últimas horas, consistente com o `git log` recente). Três dessas quatro versões (Go 1.26.1, lib/pq v1.11.2, go-sdk v1.8.0) só existem no mundo real **depois** do meu cutoff de treinamento (jan/2026) — o que é evidência circunstancial forte de que foram lidas do código, não recitadas de memória.
2. **As 2 versões do FB_CEREBRO (Go 1.22, pgx/v5 v5.5.5) não puderam ser confirmadas por mim** — o repo/host onde esse código vive não está acessível nesta sessão (tentativa de SSH foi bloqueada pelo classificador de permissões). Ambas são versões reais e existentes, mas antigas o bastante (2024) para serem indistinguíveis de um "chute plausível de stack legado" feito por conhecimento de treinamento. Recomendo ao autor do documento confirmar explicitamente que essas duas vieram de um `go version`/`cat go.mod` real no host de produção (a infra de acesso SSH já existe e está documentada na memória do projeto), e registrar essa confirmação no documento ou nas fontes (`sources: []` no frontmatter está vazio — nenhuma fonte é citada para nenhuma das 8 linhas da tabela).
3. **Risco não capturado no documento:** Go 1.22 está fora da janela de suporte oficial do time Go há mais de um ano (em set/2026, com Go 1.26 corrente). Isso não invalida a decisão, mas é uma dívida técnica que nem AD-8 nem a seção "Deferred" mencionam — vale uma linha explícita se a intenção é que esse serviço legado continue rodando por mais tempo.
4. **Metaproblema estrutural:** o frontmatter do documento declara `sources: []` — nenhuma fonte é citada para nenhuma decisão da seção Stack. Mesmo que 6 das 8 linhas estejam corretas (verificadas por mim independentemente), o documento não deixa rastro de COMO cada uma foi obtida, o que é exatamente o tipo de lacuna que motiva esta revisão. Recomendo, no mínimo, anotar na tabela qual linha veio de leitura de arquivo (`go.mod` local) vs. qual veio de acesso remoto (SSH em produção) vs. qual é suposição a confirmar.
5. Nenhuma das 8 entradas é uma versão **inventada/inexistente** — todas existem no mundo real. O problema encontrado não é fabricação, é ausência de rastro de verificação para a metade que descreve um sistema fora do repo local.
