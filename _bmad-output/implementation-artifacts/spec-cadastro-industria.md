---
title: 'Cadastro de Indústrias — mapeamento de fornecedores'
type: 'feature'
created: '2026-08-28'
status: 'done'
review_loop_iteration: 0
context: []
baseline_commit: '42225803dedab7e2cc17e6dedaa44672ac7ff0de'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** A empresa tem 400+ fornecedores (`cod_fornec`) cadastrados via WinThor, mas o MESMO fabricante às vezes tem `cod_fornec` diferente por filial (ex.: Reckitt = 47753 num grupo de filiais, 44957 noutro — imagem enviada pelo Claudio em 27/08/2026). Hoje não existe onde registrar essa equivalência.

**Approach:** Criar cadastro de "Indústrias" (`farol.industrias` + `farol.industria_fornecedores`) que mapeia N `cod_fornec` pra 1 indústria canônica, com tela de CRUD própria (`/gestao/industrias`), seguindo o padrão vivo de `SpFiliaisHandler`/`SpFilialItemHandler` (`backend/handlers/sp_ambiente.go`) e sua tela `SpAmbiente.tsx`. Este é o goal 1 de um pedido maior que foi dividido — ver `deferred-work.md`, entrada 2026-08-28: o filtro cruzado "Indústria" em todas as visões e o rename da hierarquia V01 atual ficam para depois. Esta spec entrega SÓ o cadastro; nenhuma tela/query existente passa a consumir essa tabela ainda.

## Boundaries & Constraints

**Always:**
- `farol.industrias`: `id`, `empresa_id` (FK `companies`, `ON DELETE CASCADE`), `nome` (rótulo canônico, ex. "UNILEVER HC"), `razao_social` (nullable, ex. "UNILEVER BRASIL LTDA-396"), `ativo` (default true), `created_at`, `updated_at`. `UNIQUE (empresa_id, nome)`.
- `farol.industria_fornecedores`: `id`, `empresa_id`, `industria_id` (FK `farol.industrias`, `ON DELETE CASCADE`), `cod_fornec` (TEXT), `rotulo` (nullable, anotação livre do usuário, ex. "MTZ/MS/BA" — só documentação, não usado em nenhuma query). `UNIQUE (empresa_id, cod_fornec)` — um `cod_fornec` só pode pertencer a UMA indústria por empresa.
- API `/api/farol/industrias` (GET lista + POST cria) e `/api/farol/industrias/{id}` (PUT atualiza + DELETE remove), role mínima `gestor_filial` (`withSP`, mesmo nível de `/api/sp/filiais`), leitura sem `RequireWrite`, escrita (POST/PUT/DELETE) com `RequireWrite(spCtx, w)`. Tudo escopado por `spCtx.EmpresaID`.
- POST/PUT recebem `nome`, `razao_social` opcional, `ativo` opcional (default true) e a lista completa de `fornecedores` (`cod_fornec` + `rotulo` opcional). PUT faz REPLACE total do conjunto de fornecedores daquela indústria (apaga e reinsere, numa transação) — mesma filosofia de "trocar limpa o anterior" já usada em `FarolComparativoRel322.tsx`.
- Violação do `UNIQUE (empresa_id, cod_fornec)` (um `cod_fornec` já mapeado noutra indústria) devolve **409** com mensagem que identifica QUAL indústria já usa aquele código — não um 500 genérico. Mesmo tratamento pro `UNIQUE (empresa_id, nome)`.
- Tela `/gestao/industrias`: lista de indústrias com seus `cod_fornec` mapeados, criar/editar (nome + razão social + lista dinâmica de `cod_fornec`/rótulo, adicionar/remover linhas) e excluir. `ProtectedRoute`, mesmo padrão de `/gestao/filiais`.

**Never:**
- Não cria nem altera nenhum filtro cruzado, nenhuma hierarquia V01-V07, nem toca em `farol_v2_api.go` — isso é o goal 2, deferido.
- Não popula a tabela via migration/seed com dados da imagem — a carga é 100% pela tela CRUD que esta spec entrega.
- Não exige que TODOS os 400+ fornecedores tenham indústria cadastrada — é um mapeamento parcial, opt-in; um `cod_fornec` sem indústria simplesmente não aparece em `industria_fornecedores`.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Criar indústria nova | nome + 2 `cod_fornec` (ex.: 47753, 44957) | Indústria criada, ambos códigos vinculados | N/A |
| `cod_fornec` já mapeado noutra indústria | POST/PUT reusa um `cod_fornec` existente noutra indústria | 409, mensagem cita a indústria conflitante | 409 JSON claro |
| Nome de indústria duplicado (mesma empresa) | POST com `nome` já existente | 409, mensagem clara | 409 JSON claro |
| Editar indústria trocando o conjunto de códigos | PUT com lista de `cod_fornec` diferente da atual | Conjunto antigo é totalmente substituído pelo novo | N/A |
| Excluir indústria | DELETE `/api/farol/industrias/{id}` | Indústria e seus vínculos em `industria_fornecedores` somem (cascade) | N/A |
| Escopo entre empresas | usuário da empresa A tenta acessar/editar indústria da empresa B | 404 (não vaza existência) | 404 |

</frozen-after-approval>

## Code Map

- `backend/handlers/sp_ambiente.go:153-280` (`SpFiliaisHandler`, `SpFilialItemHandler`, `pathSegment`) -- padrão vivo exato a seguir: rota pai (GET lista/POST cria) + rota item (PUT/DELETE via `pathSegment`), `GetSpContext(r)`, `RequireWrite(spCtx, w)` só em métodos de escrita
- `backend/handlers/smartpick_auth.go:317-323` (`RequireWrite`) -- reaproveitar direto, não reimplementar
- `backend/migrations/102_sp_filiais_cds.sql` -- padrão de migration a seguir (tabela + índice + unique constraint), mas em schema `farol` (não `smartpick` — esta tabela é domínio Farol, referenciando `cod_fornec` usado em `vendas_faturadas`/`vendas_transmitidas`)
- `backend/migrations/156_vendas_split.sql:35-48` -- confirma `cod_fornec TEXT NOT NULL DEFAULT ''` como o tipo/convenção já usado nas tabelas de venda; a nova tabela usa o mesmo tipo TEXT pra `cod_fornec` (sem FK formal — não existe uma tabela mestra de fornecedores pra referenciar)
- `backend/main.go:574` (comentário "`/api/cadastros/*` desativado em 2026-05-27") -- NÃO reativar nem reusar `backend/handlers/cadastros.go` (código morto, referente a outro domínio); registrar as rotas novas perto de `/api/sp/filiais` (linha ~587), seguindo o mesmo `withSP(..., "gestor_filial")`
- `frontend/src/pages/SpAmbiente.tsx` -- padrão vivo de tela CRUD a seguir: `useState` local, `fetch` direto com `headers` de auth, diálogos de criar/editar inline
- `frontend/src/App.tsx:280-281` (rotas `/gestao/filiais`, `/gestao/regras`) -- acrescentar `<Route path="/gestao/industrias" element={<ProtectedRoute><GestaoIndustrias /></ProtectedRoute>} />` no mesmo bloco
- Menu/sidebar que lista `/gestao/filiais` -- localizar via grep por essa rota fora de `App.tsx` e acrescentar entrada paralela pra `/gestao/industrias`

## Tasks & Acceptance

**Execution:**
- [x] `backend/migrations/209_industrias.sql` -- `farol.industrias` + `farol.industria_fornecedores`, constraints `UNIQUE (empresa_id, nome)` e `UNIQUE (empresa_id, cod_fornec)`, índice em `industria_id` -- schema novo
- [x] `backend/handlers/farol_industrias.go` (novo arquivo) -- `IndustriasHandler` (GET lista com fornecedores aninhados / POST cria, numa transação) e `IndustriaItemHandler` (PUT substitui fornecedores + campos / DELETE cascade), erro 409 com mensagem identificando o conflito nas duas constraints -- núcleo do CRUD
- [x] `backend/main.go` -- registra `/api/farol/industrias` e `/api/farol/industrias/` com `withSP(..., "gestor_filial")`, perto do bloco de `/api/sp/filiais` -- expõe a API
- [x] `frontend/src/pages/GestaoIndustrias.tsx` (novo arquivo) -- lista + criar/editar/excluir indústria, campos dinâmicos de `cod_fornec`/rótulo -- UI
- [x] `frontend/src/App.tsx` -- rota `/gestao/industrias` -- liga a tela
- [x] Menu/sidebar -- `/gestao/filiais` acabou não tendo link real em lugar nenhum (só `SpAmbiente.tsx`/`App.tsx`/um mapa de títulos do chat de ajuda do SmartPick, um módulo órfão diferente do Farol) -- em vez de replicar esse padrão órfão, a entrada "Indústrias" foi ao menu que Farol realmente usa: `frontend/src/lib/navigation.ts`, aba nova no módulo `config` (mesmo nível de "Obj. Manutenção", sem `adminOnly` extra por aba — o módulo já é `adminOnly`)
- [x] Testes Go cobrindo a I/O Matrix: criar com sucesso, conflito de `cod_fornec` entre indústrias (409 + mensagem), conflito de nome (409), replace total no PUT, delete em cascade, isolamento entre empresas (404) -- `backend/handlers/farol_industrias_test.go`, 6 testes, todos passando contra banco local real

**Acceptance Criteria:**
- Given uma indústria nova com 2 `cod_fornec`, when consulto a lista, then vejo a indústria com os 2 códigos vinculados.
- Given um `cod_fornec` já vinculado à indústria A, when tento vinculá-lo à indústria B (criar ou editar), then recebo 409 com mensagem citando a indústria A.
- Given uma indústria com 3 códigos, when edito trocando pra 2 códigos diferentes, then só os 2 novos ficam vinculados (os 3 antigos somem).
- Given um usuário da empresa B, when tenta acessar/editar/excluir uma indústria da empresa A por ID, then recebe 404.

## Design Notes

Não existe tabela mestra de fornecedores no schema atual (`cod_fornec` é sempre TEXT solto nas tabelas de venda) — por isso `industria_fornecedores.cod_fornec` também é TEXT sem FK, só com o `UNIQUE (empresa_id, cod_fornec)` garantindo 1:1 código→indústria.

`rotulo` (ex.: "MTZ/MS/BA", "PA", copiando as colunas da imagem original) é só anotação do usuário — não precisa saber qual filial cada `cod_fornec` realmente atende, porque isso já está implícito na própria transação de venda (coluna `empresa` de `vendas_faturadas`/`vendas_transmitidas`, ver migration 199). Guardar isso explicitamente seria duplicar informação que o sistema já tem.

## Verification

**Commands:**
- `cd backend && go build ./...` -- compila sem erro
- `cd backend && go test ./handlers/... -run Industria -v` -- inclui os testes novos do CRUD
- `cd frontend && npx tsc --noEmit` -- sem erro

**Manual checks:**
- Rodar a migration local, abrir `/gestao/industrias`, cadastrar as ~20 linhas da imagem enviada (`/home/claudio/uploads/WhatsApp Image 2026-08-27 at 11.30.57.jpeg`) manualmente pela tela, conferir que aparecem na lista.

## Spec Change Log

- **2026-08-28 — renegociação do "Never: não popula via migration/seed".** O Claudio pediu pra já entrar carregado com as 21 linhas da imagem, em vez de digitar manualmente. `backend/migrations/210_industrias_seed_inicial.sql` insere essas linhas (nome/razão social/cod_fornec/rótulo transcritos da imagem), uma vez, com `ON CONFLICT DO NOTHING` pra não duplicar nem sobrescrever cadastro manual feito antes do deploy. Depois deste seed, o "Never" original volta a valer — qualquer ajuste é pela tela, sem migration nova.
