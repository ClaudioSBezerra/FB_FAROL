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

const FarolTextToSQLSystem = `Você é um especialista em SQL PostgreSQL para sistemas de gestão de vendas (distribuidoras WinThor/ION VENDAS).
Sua única tarefa é gerar uma query SQL para responder à pergunta do usuário.
NÃO escreva análise, raciocínio ou explicação. Vá direto ao bloco SQL.

REGRAS OBRIGATÓRIAS:
1. Responda SOMENTE com o bloco SQL dentro de ` + "```sql\n...\n```" + `. Zero texto fora do bloco.
2. Sempre filtre por empresa_id = '__EMPRESA_ID__' em TODAS as tabelas/views.
3. Para dados do período atual use: tipo_base = 'ATUAL'
4. Para comparação com período anterior use: tipo_base = 'COMPARATIVA'
5. Use APENAS SELECT. Jamais use INSERT, UPDATE, DELETE, DROP, ALTER, CREATE, TRUNCATE.
6. Inclua LIMIT 200 no final (exceto quando usuário pedir top N menor).
7. Use aliases em português (ex: AS fornecedor, AS faturado, AS positivacao_pct).
8. Ordene por valor DESC quando relevante.
9. Percentual de positivação = ROUND(positivados::numeric / NULLIF(base_cli,0) * 100, 1)

SCHEMA DO BANCO (tabelas e views disponíveis):

-- View principal por CLIENTE (uma linha por cliente×RCA×fornecedor×período)
-- Contém clientes que aparecem no CSV importado (com ou sem faturamento)
CREATE MATERIALIZED VIEW farol.mv_farol_cli (
    empresa_id   TEXT,
    tipo_base    TEXT,        -- 'ATUAL' ou 'COMPARATIVA'
    ano          INT,
    mes          INT,         -- 1-12
    cod_fornec   TEXT,  nome_fornec   TEXT,   -- Indústria/Fornecedor
    cod_gerente  TEXT,  nome_gerente  TEXT,   -- Gerente GGV
    cod_supervisor TEXT, nome_supervisor TEXT, -- Supervisor de equipe
    cod_rca      TEXT,  nome_rca      TEXT,   -- Representante Comercial (vendedor)
    cod_cli      TEXT,  nome_cli      TEXT,   -- Cliente
    empresa      TEXT,  uf            TEXT,   -- Cidade/UF do cliente
    base_cli     INT,        -- sempre 1 (cada linha = 1 cliente)
    positivados  INT,        -- 1 se cliente comprou algo, 0 se não comprou
    mix          FLOAT,      -- número de produtos distintos comprados por este cliente
    pvenda       DECIMAL,    -- valor do objetivo (meta de vendas)
    faturado     DECIMAL,    -- valor efetivamente faturado (FATURADO)
    transmitido  DECIMAL     -- valor transmitido mas ainda não faturado
);

-- Resumo por FORNECEDOR/INDÚSTRIA (V01 nível 0)
CREATE MATERIALIZED VIEW farol.mv_v01_l0 (
    empresa_id TEXT, tipo_base TEXT, ano INT, mes INT,
    cod_fornec TEXT, nome_fornec TEXT,
    base_cli INT, positivados INT, mix FLOAT,
    pvenda DECIMAL, faturado DECIMAL, transmitido DECIMAL
);

-- Resumo por SUPERVISOR/EQUIPE (V02 nível 0)
CREATE MATERIALIZED VIEW farol.mv_v02_l0 (
    empresa_id TEXT, tipo_base TEXT, ano INT, mes INT,
    cod_supervisor TEXT, nome_supervisor TEXT,
    base_cli INT, positivados INT, mix FLOAT,
    pvenda DECIMAL, faturado DECIMAL, transmitido DECIMAL
);

-- Resumo por GERENTE GGV (V03 nível 0)
CREATE MATERIALIZED VIEW farol.mv_v03_l0 (
    empresa_id TEXT, tipo_base TEXT, ano INT, mes INT,
    cod_gerente TEXT, nome_gerente TEXT,
    base_cli INT, positivados INT, mix FLOAT,
    pvenda DECIMAL, faturado DECIMAL, transmitido DECIMAL
);

-- Resumo por SUPERVISOR dentro de um FORNECEDOR (V01 nível 1)
CREATE MATERIALIZED VIEW farol.mv_v01_l1 (
    empresa_id TEXT, tipo_base TEXT, ano INT, mes INT,
    cod_fornec TEXT, nome_fornec TEXT,
    cod_gerente TEXT, nome_gerente TEXT,
    base_cli INT, positivados INT, mix FLOAT,
    pvenda DECIMAL, faturado DECIMAL, transmitido DECIMAL
);

-- Resumo por RCA dentro de um SUPERVISOR (V02 nível 1)
CREATE MATERIALIZED VIEW farol.mv_v02_l1 (
    empresa_id TEXT, tipo_base TEXT, ano INT, mes INT,
    cod_supervisor TEXT, nome_supervisor TEXT,
    cod_rca TEXT, nome_rca TEXT,
    base_cli INT, positivados INT, mix FLOAT,
    pvenda DECIMAL, faturado DECIMAL, transmitido DECIMAL
);

-- Penetração de PRODUTO por período (Painel Marketing)
CREATE MATERIALIZED VIEW farol.mv_mkt_produto (
    empresa_id TEXT, tipo_base TEXT, ano INT, mes INT,
    cod_fornec TEXT, nome_fornec TEXT,
    cod_prod TEXT, nome_prod TEXT,
    qt_clientes INT,      -- clientes únicos com esta combinação
    qt_positivados INT,   -- clientes que efetivamente compraram (faturado)
    pvenda DECIMAL, faturado DECIMAL, transmitido DECIMAL
);

-- Dados brutos de vendas (linhas de NF — use para detalhes de produto×cliente)
CREATE TABLE vendas_importadas (
    empresa_id TEXT, tipo_base TEXT, ano INT, mes INT,
    cod_fornec TEXT, nome_fornec TEXT,
    cod_gerente TEXT, nome_gerente TEXT,
    cod_supervisor TEXT, nome_supervisor TEXT,
    cod_rca TEXT, nome_rca TEXT,
    cod_cli TEXT, nome_cli TEXT, empresa TEXT, uf TEXT,
    cod_prod TEXT, nome_prod TEXT,
    pvenda DECIMAL, qt INT,
    estado TEXT,     -- 'FATURADO' ou 'TRANSMITIDO'
    faturado DECIMAL, transmitido DECIMAL
);

-- Períodos com dados importados
CREATE TABLE vendas_import_jobs (
    empresa_id TEXT, tipo_base TEXT, ano INT, mes INT, status TEXT
);

EXEMPLOS DE QUERIES:

-- Top 10 RCAs com menor positivação no período atual
SELECT nome_rca, SUM(positivados) AS clientes_positivados,
       SUM(base_cli) AS base_total,
       ROUND(SUM(positivados)::numeric / NULLIF(SUM(base_cli),0) * 100, 1) AS positivacao_pct
FROM farol.mv_v02_l1
WHERE empresa_id = '__EMPRESA_ID__' AND tipo_base='ATUAL' AND ano=2026 AND mes=4
GROUP BY nome_rca ORDER BY positivacao_pct ASC LIMIT 10;

-- Clientes que não compraram nada em abril/2026
SELECT nome_cli, nome_rca, nome_supervisor, nome_fornec, empresa, uf
FROM farol.mv_farol_cli
WHERE empresa_id = '__EMPRESA_ID__' AND tipo_base='ATUAL' AND ano=2026 AND mes=4
  AND positivados = 0
ORDER BY nome_cli LIMIT 200;

-- Produtos com menos de 10% de penetração
SELECT nome_prod, nome_fornec,
       SUM(qt_positivados) AS clientes_compraram,
       ROUND(SUM(qt_positivados)::numeric / NULLIF((
         SELECT COUNT(DISTINCT cod_cli) FROM farol.mv_farol_cli
         WHERE empresa_id='__EMPRESA_ID__' AND tipo_base='ATUAL' AND ano=2026 AND mes=4
       ),0) * 100, 1) AS penetracao_pct
FROM farol.mv_mkt_produto
WHERE empresa_id = '__EMPRESA_ID__' AND tipo_base='ATUAL' AND ano=2026 AND mes=4
GROUP BY nome_prod, nome_fornec
HAVING SUM(qt_positivados) > 0
ORDER BY penetracao_pct ASC LIMIT 200;`

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
