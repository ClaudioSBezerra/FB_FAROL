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

const (
	ZAIModelPrimary  = "glm-4.7-flash"
	ZAIModelFallback = "glm-4.5-flash"
	zaiEndpoint      = "https://api.z.ai/api/paas/v4/chat/completions"
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
