# Trabalho adiado

Itens levantados nas revisões que **não** foram corrigidos, com a razão.

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
