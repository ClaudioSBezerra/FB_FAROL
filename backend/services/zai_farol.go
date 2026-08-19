package services

// zai_farol.go — Cliente Z.AI e prompt Text-to-SQL para o Farol de Vendas.
// Reutiliza a mesma API GLM (OpenAI-compatible) do FB_APU01.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

// ─── Cliente Z.AI ─────────────────────────────────────────────────────────────

const zaiEndpoint = "https://api.z.ai/api/paas/v4/chat/completions"

// Modelo por variável de ambiente — trocar exige só reiniciar, não deploy.
//
// O padrão continua no tier GRATUITO (glm-4.7-flash) de propósito: mudar o
// default para um modelo pago numa atualização de código geraria fatura que
// ninguém pediu. Quem decide gastar é quem paga.
//
// Para o text-to-SQL, o degrau que importa é sair do flash. Os erros que
// aparecem na prática — ramo de UNION sem parênteses, alias no WHERE, coluna
// que não existe — são de escrita, e é exatamente onde o modelo maior ganha.
// Medido em 19/08/2026 (por 1M de tokens, entrada/saída):
//
//	glm-4.7-flash    grátis   — atual; erra sintaxe com frequência
//	glm-4.7-flashx   $0,07 / $0,40
//	glm-4.7          $0,60 / $2,20   — recomendado
//	glm-5.1          $1,40 / $4,40
//
// Com o prompt atual (~5,5k entrada, ~500 saída), o glm-4.7 sai a ~R$ 0,024
// por pergunta.
//
// ⚠ NÃO aponte para o endpoint do GLM Coding Plan (api/coding/paas/v4). Além
// de ser outro endereço, aquela cota é vendida para ferramenta de
// desenvolvimento, não para aplicação servindo usuário final.
func envModelo(chave, padrao string) string {
	if v := strings.TrimSpace(os.Getenv(chave)); v != "" {
		return v
	}
	return padrao
}

var (
	ZAIModelPrimary  = envModelo("ZAI_MODEL", "glm-4.7-flash")
	ZAIModelFallback = envModelo("ZAI_MODEL_FALLBACK", "glm-4.5-flash")
)

type ZAIClient struct {
	apiKey     string
	httpClient *http.Client
}

type zaiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type zaiRequest struct {
	Model     string       `json:"model"`
	MaxTokens int          `json:"max_tokens"`
	Messages  []zaiMessage `json:"messages"`
}

type zaiResponse struct {
	Choices []struct {
		Message struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// ZAIResult é o resultado retornado ao handler.
type ZAIResult struct {
	Text  string
	Model string
}

func NewZAIClient() *ZAIClient {
	key := os.Getenv("ZAI_API_KEY")
	if key == "" {
		return nil
	}
	return &ZAIClient{
		apiKey:     key,
		httpClient: &http.Client{Timeout: 90 * time.Second},
	}
}

func (c *ZAIClient) IsAvailable() bool { return c != nil && c.apiKey != "" }

func (c *ZAIClient) Ask(system, user, model string, maxTokens int) (*ZAIResult, error) {
	if !c.IsAvailable() {
		return nil, fmt.Errorf("ZAI_API_KEY não configurado")
	}
	if model == "" {
		model = ZAIModelPrimary
	}
	if maxTokens == 0 {
		maxTokens = 3000
	}

	msgs := []zaiMessage{}
	if system != "" {
		msgs = append(msgs, zaiMessage{Role: "system", Content: system})
	}
	msgs = append(msgs, zaiMessage{Role: "user", Content: user})

	result, err := c.call(zaiRequest{Model: model, MaxTokens: maxTokens, Messages: msgs})
	if err != nil && strings.Contains(err.Error(), "429") && model == ZAIModelPrimary {
		result, err = c.call(zaiRequest{Model: ZAIModelFallback, MaxTokens: maxTokens, Messages: msgs})
	}
	return result, err
}

// ZAIChatTurn — um turno de conversa (role user/assistant) para AskChat.
type ZAIChatTurn struct {
	Role    string
	Content string
}

// AskChat — como Ask, mas com histórico de conversa (system + N turnos).
// Usado pelo assistente de treinamento do Farol, no MESMO endpoint/modelos/chave
// do text-to-SQL (ZAI_API_KEY, /paas/v4), com fallback em 429.
func (c *ZAIClient) AskChat(system string, turns []ZAIChatTurn, maxTokens int) (*ZAIResult, error) {
	if !c.IsAvailable() {
		return nil, fmt.Errorf("ZAI_API_KEY não configurado")
	}
	if maxTokens == 0 {
		maxTokens = 1024
	}
	msgs := make([]zaiMessage, 0, len(turns)+1)
	if system != "" {
		msgs = append(msgs, zaiMessage{Role: "system", Content: system})
	}
	for _, t := range turns {
		msgs = append(msgs, zaiMessage{Role: t.Role, Content: t.Content})
	}
	result, err := c.call(zaiRequest{Model: ZAIModelPrimary, MaxTokens: maxTokens, Messages: msgs})
	if err != nil && strings.Contains(err.Error(), "429") {
		result, err = c.call(zaiRequest{Model: ZAIModelFallback, MaxTokens: maxTokens, Messages: msgs})
	}
	return result, err
}

func (c *ZAIClient) call(req zaiRequest) (*ZAIResult, error) {
	body, _ := json.Marshal(req)
	httpReq, err := http.NewRequest("POST", zaiEndpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("Z.AI request error: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == 429 {
		return nil, fmt.Errorf("429 rate limit")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Z.AI HTTP %d: %.300s", resp.StatusCode, string(raw))
	}

	var zr zaiResponse
	if err := json.Unmarshal(raw, &zr); err != nil {
		return nil, fmt.Errorf("Z.AI parse error: %w", err)
	}
	if zr.Error != nil {
		return nil, fmt.Errorf("Z.AI error [%s]: %s", zr.Error.Code, zr.Error.Message)
	}
	if len(zr.Choices) == 0 {
		return nil, fmt.Errorf("Z.AI returned no choices")
	}

	text := zr.Choices[0].Message.Content
	if text == "" {
		text = zr.Choices[0].Message.ReasoningContent
	}
	return &ZAIResult{Text: strings.TrimSpace(text), Model: req.Model}, nil
}

// ─── System Prompt Text-to-SQL — Farol de Vendas ─────────────────────────────

const FarolTextToSQLSystem = `Você é um especialista em SQL PostgreSQL para o Farol de Vendas (distribuidora; dados do ION VENDAS/WinThor). Sua ÚNICA tarefa é gerar UMA query SQL que responda à pergunta. NÃO escreva análise nem explicação — vá direto ao bloco SQL.

REGRAS OBRIGATÓRIAS:
1. Responda SOMENTE com o bloco SQL dentro de ` + "```sql\n...\n```" + `. Zero texto fora do bloco.
2. Sempre filtre por empresa_id = '__EMPRESA_ID__' em TODAS as tabelas.
3. Use APENAS SELECT. Jamais INSERT, UPDATE, DELETE, DROP, ALTER, CREATE, TRUNCATE.
4. Inclua LIMIT 200 no final (exceto quando o usuário pedir um "top N" menor).
5. Aliases em português (ex.: AS industria, AS faturado, AS positivacao_pct).
6. Ordene por valor DESC quando fizer sentido.
7. AGRUPE SEMPRE PELO CÓDIGO, nunca pelo nome. Use MAX(nome_x) para exibir.
   O nome da mesma entidade varia entre linhas — acento gravado de formas
   diferentes na origem já partiu "NÚCLEO DE VENDAS" em duas linhas de um
   TOP 10, com o faturamento dividido entre elas. O código é estável.
   CERTO:  SELECT cod_rca, MAX(nome_rca) AS rca, SUM(pvenda) ... GROUP BY cod_rca
   ERRADO: SELECT nome_rca AS rca, SUM(pvenda) ... GROUP BY nome_rca

TABELAS (grão = 1 linha por item de nota/pedido):

-- FATURAMENTO (notas fiscais emitidas)
vendas_faturadas(
  empresa_id, data_faturamento DATE,
  cod_gerente, nome_gerente, cod_supervisor, nome_supervisor,
  cod_rca, nome_rca, cod_fornec, nome_fornec,
  cod_cli, nome_cli, cnpj, uf, empresa,            -- empresa = filial
  cod_cliprinc,                                    -- rede (cliente principal)
  cod_depto, depto, cod_sec, secao, cod_categoria, categoria,
  cod_prod, nome_prod, ean,
  qt,                                              -- quantidade vendida
  pvenda,                                          -- VALOR TOTAL da venda (qt × preço). SOME esta coluna para faturamento
  plucro,
  tipo_venda,                                      -- código: 1 Normal, 4 Simples Fatura, 5 Bonificação, 7 Entrega Futura, 8 Simples Entrega, 9 CFOP Específico, 10 Transferência, 11 Venda c/ Troca, 13 Remessa, 14 Venda Manifesto, 20 Consignada
  desc_condvenda                                   -- descrição do tipo de venda
)

-- TRANSMITIDO (pedidos digitados pelo RCA; ainda não faturados)
vendas_transmitidas( MESMAS colunas de vendas_faturadas, PORÉM:
  data_transmissao DATE no lugar de data_faturamento;
  NÃO possui tipo_venda nem desc_condvenda )

-- AGREGADOS MENSAIS (farol.agg_*) — PREFIRA ESTES quando servirem
-- São rollups já calculados, com MILHARES de linhas em vez de milhões. Um
-- ranking que leva ~1 minuto na tabela crua responde em milissegundos aqui.
--
-- farol.agg_fat_v01_l0_mes(empresa_id, ano, mes, cod_fornec, nome_fornec, ...)
-- farol.agg_fat_v02_l0_mes(empresa_id, ano, mes, cod_supervisor, nome_supervisor, ...)
-- farol.agg_fat_v03_l0_mes(empresa_id, ano, mes, cod_gerente, nome_gerente, ...)
-- farol.agg_fat_v04_l0_mes(empresa_id, ano, mes, cod_rca, nome_rca, ...)
--
-- Colunas de medida em TODAS elas:
--   pvenda      faturamento BRUTO
--   liquido     faturamento LÍQUIDO (já descontadas devolução e cancelamento)
--   pv_bonif, pv_transf, pv_remessa, pv_devol, pv_cancel
--   plucro, qt
--   base_cli, positivados, mix
--
-- QUANDO USAR: ranking ou total por UMA dessas quatro dimensões
-- (indústria, supervisor, gerente, RCA), com ou sem recorte de período.
--
-- QUANDO NÃO USAR — vá para vendas_faturadas:
--   • a pergunta envolve cliente, produto, UF, filial, categoria ou seção
--   • cruza DUAS dimensões (ex.: "RCA por indústria")
--   • precisa de tipo_venda que não seja bruto/líquido/bonificação/transferência
--
-- PERÍODO nos agregados é ano/mes, NUNCA data:
--   2026 inteiro      → WHERE ano = 2026
--   janeiro/2025      → WHERE ano = 2025 AND mes = 1
--   mês mais recente  → subquery com MAX sobre (ano, mes) da própria tabela
--
-- ⚠ SOMÁVEL entre meses: pvenda, liquido, pv_*, plucro, qt.
-- ⚠ NÃO SOMÁVEL: positivados, base_cli e mix são fotos DO MÊS. Somar 12 meses
--   de positivados conta o mesmo cliente 12 vezes. Para positivação num período
--   com mais de um mês, use vendas_faturadas com COUNT(DISTINCT cnpj).

MÉTRICAS:
- Faturamento (bruto)  = SUM(pvenda)
- Faturamento LÍQUIDO  = SUM(pvenda) FILTER (WHERE tipo_venda IN ('1','4','7','8','9','11','14','20'))   -- exclui Bonificação(5), Transferência(10), Remessa(13)
- Bonificação = SUM(pvenda) FILTER (WHERE tipo_venda='5'); Transferência = '10'; Remessa = '13'
- Quantidade           = SUM(qt)
- Clientes positivados = COUNT(DISTINCT cnpj) FILTER (WHERE qt > 0)
- Produtos distintos   = COUNT(DISTINCT cod_prod)
- Positivação %        = ROUND(COUNT(DISTINCT cnpj) FILTER (WHERE qt>0)::numeric / NULLIF(COUNT(DISTINCT cnpj),0) * 100, 1)

PERÍODO:
- Filtre por data_faturamento (ou data_transmissao). Ex. de um mês: data_faturamento >= '2025-01-01' AND data_faturamento < '2025-02-01'.
- Se o usuário NÃO indicar período, use o mês mais recente disponível:
  data_faturamento >= date_trunc('month', (SELECT MAX(v.data_faturamento) FROM vendas_faturadas v WHERE v.empresa_id='__EMPRESA_ID__'))

UNION / TOP E PIOR NA MESMA RESPOSTA:
No PostgreSQL, cada ramo de um UNION que tenha ORDER BY ou LIMIT PRECISA vir
entre parênteses — sem eles é erro de sintaxe. Mas prefira evitar o UNION:
para "os 10 melhores e os 10 piores", use ROW_NUMBER() nos dois sentidos sobre
uma CTE e marque o grupo numa coluna. Sai em uma passada e já vem rotulado.

GEOGRAFIA:
- uf guarda a SIGLA de 2 letras. O usuário fala o nome; traduza:
  Goiás=GO, Mato Grosso=MT, Mato Grosso do Sul=MS, Distrito Federal=DF,
  Minas Gerais=MG, São Paulo=SP, Bahia=BA, Tocantins=TO, Pará=PA.
- A coluna empresa é a FILIAL (código numérico em texto). Não confundir com uf.

PERGUNTAS EM DUAS ETAPAS:
Quando a pergunta escolhe uma entidade e depois pergunta algo SOBRE ela
("o melhor cliente ... e quais produtos ELE comprou"), use CTE: a primeira
seleciona a entidade, a segunda responde. Não devolva duas queries.

COMPROU EM X E NÃO COMPROU EM Y:
Diferença de conjuntos com NOT EXISTS sobre cod_prod, nunca com NOT IN
(NOT IN devolve zero linhas se algum valor for NULL).

EXEMPLOS (apenas para orientar o formato):

Pergunta: "top 10 indústrias por faturamento"
SELECT cod_fornec, MAX(nome_fornec) AS industria, SUM(pvenda) AS faturado
FROM farol.agg_fat_v01_l0_mes
WHERE empresa_id = '__EMPRESA_ID__'
GROUP BY cod_fornec
ORDER BY faturado DESC
LIMIT 10;

Pergunta: "top 10 RCAs de 2026"
SELECT cod_rca, MAX(nome_rca) AS rca, SUM(pvenda) AS faturamento
FROM farol.agg_fat_v04_l0_mes
WHERE empresa_id = '__EMPRESA_ID__' AND ano = 2026
GROUP BY cod_rca
ORDER BY faturamento DESC
LIMIT 10;

Pergunta: "faturamento líquido por supervisor em janeiro/2025"
SELECT cod_supervisor, MAX(nome_supervisor) AS supervisor,
       SUM(pvenda) FILTER (WHERE tipo_venda IN ('1','4','7','8','9','11','14','20')) AS faturado_liquido
FROM vendas_faturadas
WHERE empresa_id = '__EMPRESA_ID__'
  AND data_faturamento >= '2025-01-01' AND data_faturamento < '2025-02-01'
GROUP BY cod_supervisor
ORDER BY faturado_liquido DESC
LIMIT 200;

Pergunta: "top 10 RCAs de 2026 e os 10 piores"
WITH base AS (
  SELECT cod_rca, MAX(nome_rca) AS rca, SUM(pvenda) AS faturamento
  FROM farol.agg_fat_v04_l0_mes
  WHERE empresa_id = '__EMPRESA_ID__' AND ano = 2026
  GROUP BY cod_rca
), ranqueado AS (
  SELECT base.*,
         ROW_NUMBER() OVER (ORDER BY faturamento DESC) AS pos_melhor,
         ROW_NUMBER() OVER (ORDER BY faturamento ASC)  AS pos_pior
  FROM base
)
SELECT rca,
       faturamento,
       CASE WHEN pos_melhor <= 10 THEN 'Top 10' ELSE 'Piores 10' END AS grupo
FROM ranqueado
WHERE pos_melhor <= 10 OR pos_pior <= 10
ORDER BY faturamento DESC
LIMIT 200;

Pergunta: "qual foi o melhor cliente em Goiás e quais produtos ele comprou em 2025 e não comprou em 2026"
WITH melhor_cliente AS (
  SELECT cod_cli
  FROM vendas_faturadas
  WHERE empresa_id = '__EMPRESA_ID__' AND uf = 'GO'
    AND data_faturamento >= '2025-01-01' AND data_faturamento < '2026-01-01'
  GROUP BY cod_cli
  ORDER BY SUM(pvenda) DESC
  LIMIT 1
)
SELECT v.cod_prod, MAX(v.nome_prod) AS produto,
       SUM(v.pvenda) AS faturado_2025, SUM(v.qt) AS quantidade_2025
FROM vendas_faturadas v
WHERE v.empresa_id = '__EMPRESA_ID__'
  AND v.cod_cli = (SELECT cod_cli FROM melhor_cliente)
  AND v.data_faturamento >= '2025-01-01' AND v.data_faturamento < '2026-01-01'
  AND NOT EXISTS (
    SELECT 1 FROM vendas_faturadas w
    WHERE w.empresa_id = v.empresa_id AND w.cod_cli = v.cod_cli
      AND w.cod_prod = v.cod_prod
      AND w.data_faturamento >= '2026-01-01' AND w.data_faturamento < '2027-01-01')
GROUP BY v.cod_prod
ORDER BY faturado_2025 DESC
LIMIT 200;

Pergunta: "positivação por RCA no mês mais recente"
SELECT cod_rca, MAX(nome_rca) AS rca,
       COUNT(DISTINCT cnpj) FILTER (WHERE qt > 0) AS positivados,
       COUNT(DISTINCT cnpj) AS clientes_no_periodo
FROM vendas_faturadas
WHERE empresa_id = '__EMPRESA_ID__'
  AND data_faturamento >= date_trunc('month', (SELECT MAX(v.data_faturamento) FROM vendas_faturadas v WHERE v.empresa_id='__EMPRESA_ID__'))
GROUP BY cod_rca
ORDER BY positivados DESC
LIMIT 200;`

// ─── Extração de SQL da resposta da IA ───────────────────────────────────────

var reSQLBlock = regexp.MustCompile("(?s)```(?:sql)?\\s*\n?(.*?)\\s*```")

// ExtractFarolSQL extrai o bloco SQL da resposta do modelo.
func ExtractFarolSQL(text string) (string, error) {
	matches := reSQLBlock.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		// Sem code block — tenta tratar como SQL direto se começa com SELECT
		trimmed := strings.TrimSpace(text)
		if strings.HasPrefix(strings.ToUpper(trimmed), "SELECT") {
			return trimmed, nil
		}
		return "", fmt.Errorf("nenhum bloco SQL encontrado na resposta")
	}
	// Pega o último bloco (GLM às vezes raciocina antes de gerar o SQL final)
	last := matches[len(matches)-1]
	return strings.TrimSpace(last[1]), nil
}

// BuildFarolSQLPrompt monta o prompt de usuário com a pergunta.
func BuildFarolSQLPrompt(pergunta string) string {
	return fmt.Sprintf("Pergunta: %s\n\nGere a query SQL PostgreSQL para responder.", pergunta)
}

// BuildFarolSQLFixPrompt — segunda passada: o SQL que o modelo gerou não rodou.
// Dar o erro do banco de volta resolve a maior parte dos casos, porque a
// mensagem do Postgres aponta a posição exata e o modelo raramente errou o
// ENTENDIMENTO da pergunta — errou a escrita.
func BuildFarolSQLFixPrompt(pergunta, sqlRuim, erroBanco string) string {
	return fmt.Sprintf(`A query abaixo, gerada para responder a pergunta, FALHOU no PostgreSQL.

Pergunta original: %s

SQL que falhou:
%s

Erro do banco:
%s

Corrija e devolva SOMENTE o bloco SQL corrigido. Mantenha a mesma intenção da
pergunta. Erros comuns: ramo de UNION com ORDER BY/LIMIT sem parênteses;
coluna que não existe no schema informado; GROUP BY faltando coluna do SELECT;
alias usado no WHERE (no PostgreSQL o alias do SELECT não vale no WHERE).`,
		pergunta, sqlRuim, erroBanco)
}
