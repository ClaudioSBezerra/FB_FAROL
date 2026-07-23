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
