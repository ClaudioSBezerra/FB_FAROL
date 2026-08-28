# Trabalho adiado

Itens levantados nas revisões que **não** foram corrigidos, com a razão.

## 2026-07-22 — spec-bi-kpis-industria-uf (revisão adversarial, 3 revisores)

### Corrigidos (commit de follow-up) — não ficam aqui, só registro
Concentração >100% (denominador só-positivo), fatia negativa no donut (pula ≤0),
`ufs` null→[], erro do REFRESH CONCURRENTLY logado, teste de cor alinhado ao
`pickCor`, asserção de reconciliação somaUF×KPI no teste.

### Adiados / aceitos com documentação

- **MV de UF não reconcilia ao centavo com os aggs em 2 cascos raros**
  (Auditor #1, Edge #2). A fórmula por-linha é idêntica à mig 190, mas:
  (a) os aggs excluem chave vazia (`cod_fornec/gerente <> ''`), a MV inclui —
  órfãos viram sentinela `99999999` (não vazio), então raro; (b) devolução/
  cancelamento sem faturado casado no mês: o agg só subtrai via JOIN por chave,
  a MV subtrai por UF sempre. Não corrigido porque replicar os quirks do agg
  por UF é impraticável (uf não é chave do JOIN) e propagaria limitações do
  agg. O total por UF é o líquido geográfico "limpo"; documentado no código.
  **Verificar em produção** se a soma-UF bate com o headline no dado real.

- **UF que zerou este ano some do ranking** (Edge #4). `biFaturadoPorUF` itera
  só o mapa do período atual; um estado que vendia e caiu a zero não aparece
  (nenhuma barra vermelha sinaliza a perda). Mostrar quedas exigiria iterar
  `atual ∪ ant` — mudança de semântica que o gestor não pediu. O bloco lista
  UFs com atividade no período atual.

- **Cor YoY compara parcial × ano anterior inteiro** (Edge #5). No `ytd`, atual
  (jan→hoje, parcial) × ano anterior inteiro (jan–dez) → no meio do ano quase
  toda UF/indústria fica vermelha. É herança do `deriveCompRange` do painel
  (mesma régua dos gauges e do /cards); corrigir para YTD-vs-YTD mexeria em
  lógica compartilhada e violaria "não divergir do /cards". Aceito como está.

- **REFRESH global da MV a cada import** (Blind #3, Edge #7). `refreshUFMV`
  recomputa a MV de todas as empresas em todo import; dois imports simultâneos
  disputam o mesmo REFRESH. Monoempresa hoje → custo baixo. Multi-tenant
  exigiria MV por empresa (tabela upsertada como os aggs) ou advisory lock.

- **Sem cap de top-N no ranking de UF** (Blind #7). Lista todas as UFs (~8
  reais, teto 27 + bucket '—'); o container faz scroll. Sem risco prático.

## 2026-07-22 — spec-bi-performance-refresh (revisão adversarial, 3 revisores)

### Adiados por escopo/risco

- **Sem singleflight e sem timeout/contexto no `/api/v2/farol/bi`**
  (Blind Hunter #7, Edge Case Hunter #6). No instante do miss — logo após um
  import — N clientes disparam N cálculos idênticos, e nenhuma query usa
  `r.Context()`, então o trabalho continua mesmo se o navegador fechar.
  Não corrigido porque o projeto inteiro segue esse padrão (o singleflight
  foi removido junto com `queryMixTotal`) e o BI tem poucos espectadores.
  Vale mexer se aparecer pico de CPU no Postgres logo após import.

- **Sessão expirada na TV não redireciona para login** (Edge #12).
  Com a correção aplicada, o painel agora mantém o último dado válido com
  faixa de aviso em vez de apagar a tela — mas um 401 depois de 24h ligado
  continua sem levar ao login. Tratar 401/403 separadamente do erro de rede
  exige mexer no padrão de auth do front inteiro, não só do BI.

- **Fatia "Outros" negativa some do donut** (Edge #8). Quando a cauda tem
  devolução maior que venda, `outros < 0` e a fatia é omitida — o donut deixa
  de fechar com o KPI. Mantido de propósito: é exatamente o comportamento que
  o front já tinha, e mudar agora alteraria número em tela sem o gestor pedir.
  Um fornecedor com valor negativo DENTRO do top 8 continua indo para o
  recharts com ângulo negativo — esse caso merece tratamento se aparecer.

- **`atualizado_em` vazio significa duas coisas** (Blind #10): "empresa nunca
  importou" e "a query do carimbo falhou" caem ambos em `""` → a tela diz
  "sem import registrado". Distinguir exigiria um campo a mais no payload.

- **`refreshAllFarolViews` é global, `invalidateBICache` é por empresa**
  (Blind #5). Um refresh manual disparado pela empresa A não limpa o cache do
  BI da empresa B. Baixo risco na prática (a base é monoempresa hoje) e o TTL
  de 10 min cobre.

### Pré-existentes, fora desta mudança

- **`objetivos.go` tem 2 erros de `go vet`** (`fmt.Sprintf %s` com `int64` na
  linha 280; `log.Printf %d` com `string` na 1070). Bloqueiam `go test` do
  pacote inteiro — os testes do BI precisam de `-vet=off` por causa disso.
  Não toquei: é código de outra área.

- **Thundering herd no vencimento de `baseCache`/`vendasPeriodoCache`.**
  Mesmo padrão do item de singleflight acima, já existente antes desta
  mudança.

### Ajuste de texto pendente na spec

- O Acceptance Criterion #2 ("alternar Ano ↔ Mês e voltar é servido do cache
  do React Query") só é verdadeiro dentro de 5 min — `staleTime` caiu de 1h
  para 5 min de propósito, para encurtar a janela de dado velho (Auditor #5).
  O critério, como escrito, é falso além disso. O comportamento está certo;
  o texto é que ficou impreciso.

## 2026-08-27 — spec-comparativo-rel322 (split por limite de tokens)

- source_spec: `_bmad-output/implementation-artifacts/spec-comparativo-rel322.md`
  summary: Histórico consultável dos comparativos REL 322 — persistir cada upload/resultado (tabela `farol.comparativo_rel322`) e expor um GET para revisitar comparativos anteriores sem subir o PDF de novo.
  evidence: Spec ultrapassou o limite de 1600 tokens (2422 medidos); usuário escolheu [S] Split — core (upload + parse + comparação exibida na tela, sem persistência) primeiro, histórico persistido fica para depois.

## 2026-08-27 — spec-comparativo-rel322 (revisão adversarial, 3 revisores)

### Adiados por exigir decisão de produto/dado

- **Comparativo compara contra Faturado, mas o WinThor pode estar mostrando Transmitido** (levantado pelo Claudio em 27/08/2026, depois do primeiro teste em produção). O Farol tem tabelas/views DISTINTAS para Faturado (`vendas_faturadas`, `agg_fat_*`) e Transmitido (`vendas_transmitidas`, `agg_trans_*`) — são fluxos diferentes (pedido do RCA vs NF emitida). O comparativo REL 322 hoje só busca `vendas_faturadas` (Bruto/Líquido faturado); não está confirmado se o "Vl.Vendido" do relatório WinThor corresponde a isso ou ao transmitido. Decisão explícita do Claudio: pausar essa investigação por ora e seguir com a exportação em PDF sobre os dados atuais (Faturado) — não trocar a fonte nem adicionar Transmitido sem decisão. Quando retomar, a opção mais provável (padrão já usado no Bruto x Líquido) é mostrar os dois lados (Faturado e Transmitido, Bruto e Líquido) e deixar o gestor comparar contra o número real do PDF.

- ~~**"Filiai(s) :" do cabeçalho do PDF é ignorado no filtro do Farol**~~ — **RESOLVIDO em 28/08/2026**, ver spec-comparativo-rel322-vm-liquido.md. Confirmado: a coluna é `empresa` (mesma que `farol_v2_api.go` usa como filtro "Filial"). `rel322Parsed.Filiais` agora captura o cabeçalho e filtra tanto o Farol (Postgres) quanto a nova consulta à VM (Oracle).

### Adiados por baixa probabilidade prática

- **Órfãos só-no-Farol aparecem na tabela sem o nome do supervisor** (Blind Hunter). A query de `farolBrutoLiquidoPorSupervisor` não faz join com uma tabela de nomes — o código volta, mas o campo `Supervisor` fica vazio para essas linhas (o front mostra "—"). O gestor ainda consegue identificar pelo código, então não bloqueia; mas seria mais rápido de ler com o nome.

## 2026-08-27 — spec-comparativo-rel322-pdf (revisão adversarial, 3 revisores)

### Adiados por serem pré-existentes ou desproporcionais a este diff

- **Frontend não tem infraestrutura de teste nenhuma** (Verification Gap). Não existe `vitest`/`jest` configurado (`frontend/src/lib/utils.test.ts` existe mas não roda — falta o módulo `vitest`), nem `e2e/`/`playwright`/`cypress`. `baixarPDF` (o botão "Baixar PDF") ficou sem teste automatizado por causa disso, não por falta de tentativa — corrigir a lacuna de infraestrutura é maior que este diff.
- **`http.Error(w, jsonErrorRel322(...), ...)` sempre envia `Content-Type: text/plain` apesar do corpo ser JSON** (Blind Hunter). Já era assim em TODOS os erros deste arquivo antes desta mudança (não é regressão do PDF) — mexer nisso é uma limpeza maior no padrão de erro do arquivo inteiro.
- **`logoRelatorio` nunca foi testado contra uma linha real de `companies.logo_data`** (Verification Gap) — os testes (deste diff e do anterior) sempre usam um PNG 1x1 fabricado. É o mesmo ponto cego que causou o bug da logo errada em 25/08/2026 (`farol_cnpj_receita.go`), só que ainda não fechado.

### Adiados por baixa probabilidade prática

- **Descrição do supervisor com um token puramente numérico isolado desalinha o parser** (Blind Hunter + Edge Case Hunter). O regex que separa "descrição" de "números" não distingue um token numérico dentro da descrição (ex.: um número de zona como palavra solta) de um dos 11 valores da tabela. Na prática, os nomes reais de supervisor observados nos 4 PDFs de exemplo nunca têm token puramente numérico solto (são sempre "UF - REGIÃO - NOME"), e quando o desalinhamento acontece o parser majoritariamente ainda aborta com erro (porque o próximo token esperado como código de supervisor não bate) — não gera dado silenciosamente errado no caso comum. Corrigir direito exigiria uma heurística de lookahead mais esperta; não vale o custo até aparecer um caso real.

## 2026-08-27 — spec-comparativo-rel322-fluxo (split por limite de tokens)

- source_spec: `_bmad-output/implementation-artifacts/spec-comparativo-rel322-fluxo.md`
  summary: PDF exportado (`?formato=pdf`) refletir o fluxo escolhido (Faturado/Transmitido) no cabeçalho e nos rótulos das colunas — hoje, com o core implementado, o PDF gera normalmente mas sem indicar explicitamente qual dos dois fluxos foi comparado.
  evidence: Spec ultrapassou o limite de 1600 tokens (2095 medidos); usuário escolheu [S] Split — core (toggle + troca de fonte no backend + tela) primeiro, rótulo do fluxo no PDF fica pra depois.

## 2026-08-28 — cadastro de Indústria (multi-goal split)

- source_spec: none
  summary: Filtro cruzado "Indústria" (deduplicado por cliente, mesma classe de risco que UF/Filial — ver migrations 197 e 199) pluggável em TODAS as visões existentes (Por Gerência, Por Equipe, Por Rede, Por Departamento), MAIS renomear a hierarquia V01 atual (`cod_fornec` cru, rotulada "Por Indústria") para "Por FORN.GERAL", liberando o rótulo "Por Indústria" pro novo conceito canônico deduplicado.
  evidence: Pedido original do Claudio ("montar cadastro de indústrias + filtro do lado do fornecedor") se desdobrou, na clarificação, em bem mais que um cadastro — cross-filter em 5 visões + rename de hierarquia existente + garantia de não duplicar positivação quando uma indústria mapeia 2+ cod_fornec (mesmo problema que a mig 199 resolveu pra filial: ~23% dos clientes compram de 2+ filiais). Usuário escolheu [S] Split — cadastro + tela CRUD de Indústria primeiro (goal independente e útil sozinho); o cross-filter fica para depois que o cadastro estiver validado.
  status_2026-08-28-noite: **RESOLVIDO (caminho ao vivo)** — ver spec-cadastro-industria-crossfilter.md. Rename feito, cross-filter plugado em V02/V03/V06/V07 via `resolveIndustriaFilter` (funde no filtro `cod_fornec` já existente), SEM tabelas agg novas — decisão explícita de manter o caminho ao vivo (mais lento, mas sem o custo de ~30-40 tabelas novas replicando o padrão V10/V11). Achado colateral corrigido: `pickAggForCrossFilter` não tinha guard contra 2+ `cod_fornec` filtrados batendo numa tabela agg pré-computada — mesma classe de bug de Filial/UF, nunca corrigida pra fornecedor. Trabalho feito sem supervisão direta (commitado, não pushado — Claudio revisa de manhã antes do push).
