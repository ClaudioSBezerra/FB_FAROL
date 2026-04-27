package services

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

// ── Schema documentado para a IA ──────────────────────────────────────────────

const dataSystemPrompt = `Você é um assistente que converte perguntas em português para SQL PostgreSQL para o sistema Farol (calibragem de slots de picking).

REGRAS ABSOLUTAS:
- Responda APENAS com a query SQL final, dentro de um único bloco ` + "```sql" + `…` + "```" + `.
- NÃO adicione explicação, comentário ou texto fora do bloco.
- Use APENAS as views listadas abaixo. Nada de tabelas brutas, schemas externos ou funções de sistema.
- A query deve começar com SELECT ou WITH.
- NUNCA use INSERT, UPDATE, DELETE, ALTER, DROP, CREATE, GRANT, REVOKE, COPY, etc.
- NÃO inclua filtro por empresa_id — o sistema injeta automaticamente.
- Use sempre LIMIT (máximo 100).
- Quando o usuário disser "hoje", "esta semana", "no mês", "últimos N dias" → use NOW(), CURRENT_DATE, INTERVAL.

VIEWS DISPONÍVEIS:

vw_propostas_chat — propostas de calibragem
  colunas: id, cd_id, cd_nome, cod_filial, filial_nome, codprod, produto,
           departamento, secao, classe_venda (A/B/C),
           capacidade_atual, sugestao_calibragem, delta, status (pendente/aprovada/rejeitada/ignorado),
           justificativa, giro_dia_cx, med_venda_cx, ponto_reposicao,
           participacao, created_at, aprovado_em
  observação: delta > 0 = ampliar slot; delta < 0 = reduzir; delta = 0 = calibrado.

vw_imports_chat — imports CSV
  colunas: job_id, cd_id, cd_nome, filial_nome, filename, uploaded_by_email,
           total_linhas, linhas_ok, linhas_erro,
           status (pending/processing/done/failed), created_at, finished_at

vw_destinatarios_chat — destinatários do resumo executivo
  colunas: id, cd_id, cd_nome, filial_nome, nome_completo, cargo, email,
           ativo, criado_em

vw_ignorados_chat — produtos ignorados
  colunas: id, cd_id, cd_nome, filial_nome, codprod, produto, tipo,
           ignorado_por_email, ignorado_em

vw_resumo_executivo_chat — resumos semanais gerados
  colunas: id, cd_id, cd_nome, filial_nome, periodo_inicio, periodo_fim,
           criado_em, enviado_em, qtd_enviados

EXEMPLOS:

Usuário: "Quantas propostas pendentes temos no CD FL 11?"
` + "```sql" + `
SELECT COUNT(*) AS total FROM vw_propostas_chat WHERE cd_nome ILIKE '%FL 11%' AND status = 'pendente'
` + "```" + `

Usuário: "Top 10 produtos com maior delta absoluto pendentes"
` + "```sql" + `
SELECT codprod, produto, cd_nome, departamento, delta FROM vw_propostas_chat WHERE status = 'pendente' ORDER BY ABS(delta) DESC LIMIT 10
` + "```" + `

Usuário: "Quem importou CSV essa semana?"
` + "```sql" + `
SELECT filename, cd_nome, uploaded_by_email, total_linhas, status, created_at FROM vw_imports_chat WHERE created_at >= NOW() - INTERVAL '7 days' ORDER BY created_at DESC LIMIT 50
` + "```" + `

Usuário: "Listar destinatários ativos do CD FL 11"
` + "```sql" + `
SELECT nome_completo, email, cargo FROM vw_destinatarios_chat WHERE cd_nome ILIKE '%FL 11%' AND ativo = TRUE LIMIT 50
` + "```" + ``

const narrarSystemPrompt = `Você é um analista do sistema Farol. Receberá:
1. A pergunta original do usuário
2. O resultado da consulta em JSON (lista de linhas com colunas)

Sua tarefa: escrever uma narrativa CURTA (máximo 3 frases) em português que responda diretamente à pergunta usando os números do resultado.
- Não repita o JSON — só o insight em linguagem natural.
- Se o resultado for vazio: explique gentilmente que não há dados.
- Use os números EXATOS — sem aproximação nem invenção.
- Não inclua saudações nem despedidas.`

// ── Tipos ─────────────────────────────────────────────────────────────────────

// DataQueryResult é o que o handler devolve para o frontend.
type DataQueryResult struct {
	Reply       string                   `json:"reply"`
	SQL         string                   `json:"sql"`
	Columns     []string                 `json:"columns"`
	Rows        []map[string]interface{} `json:"rows"`
	Truncado    bool                     `json:"truncado"`
	ErroDetalhe string                   `json:"erro,omitempty"`
}

// ── Cliente Z.AI (reaproveita o padrão dos outros serviços) ──────────────────

type zaiMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func chamarZAI(systemPrompt, userMsg string, maxTokens int) (string, error) {
	apiKey := os.Getenv("ZAI_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("ZAI_API_KEY não configurada")
	}
	body, _ := json.Marshal(map[string]interface{}{
		"model":       "glm-4.5-air",
		"max_tokens":  maxTokens,
		"temperature": 0.1,
		"messages": []zaiMsg{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userMsg},
		},
	})
	client := &http.Client{Timeout: 25 * time.Second}
	req, _ := http.NewRequest("POST", "https://api.z.ai/api/coding/paas/v4/chat/completions", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Z.AI status %d: %s", resp.StatusCode, string(raw))
	}
	var r struct {
		Choices []struct {
			Message struct {
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return "", err
	}
	if len(r.Choices) == 0 {
		return "", fmt.Errorf("sem resposta")
	}
	out := r.Choices[0].Message.Content
	if out == "" {
		out = r.Choices[0].Message.ReasoningContent
	}
	return strings.TrimSpace(out), nil
}

// Lista de views que precisam ser prefixadas com "farol." se vierem sem schema.
var chatViews = []string{
	"vw_propostas_chat",
	"vw_imports_chat",
	"vw_destinatarios_chat",
	"vw_ignorados_chat",
	"vw_resumo_executivo_chat",
}

// qualificarSchema substitui referências a "vw_xxx_chat" por "farol.vw_xxx_chat"
// quando a view aparece sem schema. Necessário porque alguns ambientes não têm
// "farol" no search_path da conexão.
func qualificarSchema(sql string) string {
	for _, v := range chatViews {
		// (?i) case-insensitive; \b boundary; lookbehind não é suportado em Go,
		// então testamos com captura (charNotDot ou início) antes do nome.
		rx := regexp.MustCompile(`(?i)(^|[^\w.])` + v + `\b`)
		sql = rx.ReplaceAllString(sql, "${1}farol."+v)
	}
	return sql
}

// extrairSQL pega o conteúdo de um bloco ```sql ... ``` ou da string crua.
var rxSQLBlock = regexp.MustCompile("(?s)```(?:sql)?\\s*(.*?)```")

func extrairSQL(texto string) string {
	if m := rxSQLBlock.FindStringSubmatch(texto); len(m) >= 2 {
		return strings.TrimSpace(m[1])
	}
	return strings.TrimSpace(texto)
}

// injetarFiltroEmpresa aplica WHERE/AND empresa_id = $1 antes das cláusulas
// finais (GROUP/HAVING/ORDER/LIMIT/OFFSET/WINDOW). Como todas as views do chat
// têm a coluna empresa_id, isso isola dados por empresa.
//
// IMPORTANTE: o validador pode ter quebrado a query em duas linhas para anexar
// LIMIT 100 — busca insensível a quebras de linha.
func injetarFiltroEmpresa(sqlClean string, empresaID string) (string, []interface{}) {
	// Normaliza pra detectar palavras-chave finais — busca pelo
	// MENOR índice entre todas elas, mesmo se aparecerem em qualquer ordem
	// (palavras só "finais" depois do WHERE).
	upper := strings.ToUpper(sqlClean)
	idx := len(sqlClean)
	for _, rxKw := range []string{
		`(?i)\bGROUP\s+BY\b`,
		`(?i)\bHAVING\b`,
		`(?i)\bORDER\s+BY\b`,
		`(?i)\bLIMIT\b`,
		`(?i)\bOFFSET\b`,
		`(?i)\bWINDOW\b`,
	} {
		if loc := regexp.MustCompile(rxKw).FindStringIndex(sqlClean); loc != nil && loc[0] < idx {
			idx = loc[0]
		}
	}
	_ = upper // não usado mais, fica para legibilidade

	prefix := strings.TrimRight(sqlClean[:idx], " \n\t")
	suffix := sqlClean[idx:]

	// Decide se já há WHERE no prefix
	hasWhere := regexp.MustCompile(`(?i)\bWHERE\b`).MatchString(prefix)
	if hasWhere {
		prefix += " AND empresa_id = $1::uuid"
	} else {
		prefix += " WHERE empresa_id = $1::uuid"
	}
	if suffix != "" {
		return prefix + "\n" + suffix, []interface{}{empresaID}
	}
	return prefix, []interface{}{empresaID}
}

// ── Função orquestradora ─────────────────────────────────────────────────────

// HistoricoMsg representa uma troca anterior do chat (papel + conteúdo).
type HistoricoMsg struct {
	Role    string `json:"role"`    // user | assistant
	Content string `json:"content"` // pergunta ou narrativa+SQL
}

// ResponderPerguntaDados é o pipeline completo:
//  1. Z.AI gera SQL a partir da pergunta + histórico recente
//  2. Validador rejeita SQL inseguro
//  3. Injeta filtro por empresa_id
//  4. Executa em transação READ ONLY com timeout de 5s, LIMIT 100
//  5. Z.AI gera narrativa curta sobre o resultado
//  6. Retorna { reply, sql, columns, rows }
func ResponderPerguntaDados(db *sql.DB, pergunta, empresaID string, historico []HistoricoMsg) (*DataQueryResult, error) {
	if empresaID == "" {
		return nil, fmt.Errorf("empresa não identificada no contexto")
	}

	// 1. Geração do SQL — anexa histórico relevante (últimas 4 trocas)
	userPrompt := pergunta
	if len(historico) > 0 {
		var hb strings.Builder
		hb.WriteString("CONVERSA ANTERIOR (use como contexto se a pergunta for follow-up):\n")
		// só as últimas 4 mensagens para não estourar tokens
		start := 0
		if len(historico) > 4 {
			start = len(historico) - 4
		}
		for _, m := range historico[start:] {
			hb.WriteString(fmt.Sprintf("[%s]: %s\n", m.Role, truncar(m.Content, 500)))
		}
		hb.WriteString("\nPERGUNTA ATUAL: ")
		hb.WriteString(pergunta)
		userPrompt = hb.String()
	}
	respIA, err := chamarZAI(dataSystemPrompt, userPrompt, 512)
	if err != nil {
		return nil, fmt.Errorf("IA falhou ao gerar SQL: %w", err)
	}
	sqlGerado := extrairSQL(respIA)
	log.Printf("[chat-dados] pergunta=%q sql_gerado=%q", pergunta, sqlGerado)

	// 2. Validação
	sqlClean, err := ValidarSQL(sqlGerado)
	if err != nil {
		return nil, fmt.Errorf("SQL inválido (%s). SQL gerado: %s", err.Error(), sqlGerado)
	}

	// 3. Injeta filtro empresa
	sqlFinal, args := injetarFiltroEmpresa(sqlClean, empresaID)
	// Garante que as referências às views usem o schema "farol." (alguns
	// ambientes não têm farol no search_path da conexão).
	sqlFinal = qualificarSchema(sqlFinal)
	log.Printf("[chat-dados] sql_final=%q", sqlFinal)

	// 4. Execução em transação read-only com timeout
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("início de transação: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "SET LOCAL statement_timeout = '5s'"); err != nil {
		return nil, fmt.Errorf("statement_timeout: %w", err)
	}

	rows, err := tx.QueryContext(ctx, sqlFinal, args...)
	if err != nil {
		return nil, fmt.Errorf("execução: %w", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("columns: %w", err)
	}

	const maxRows = 100
	var resultado []map[string]interface{}
	truncado := false
	for rows.Next() {
		if len(resultado) >= maxRows {
			truncado = true
			break
		}
		vals := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			continue
		}
		linha := map[string]interface{}{}
		for i, c := range cols {
			linha[c] = normalizarValor(vals[i])
		}
		resultado = append(resultado, linha)
	}

	// 5. Narrativa
	narrativaUserMsg := fmt.Sprintf(
		"Pergunta: %s\n\nResultado (%d linhas%s):\n%s",
		pergunta,
		len(resultado),
		map[bool]string{true: ", truncado em 100", false: ""}[truncado],
		jsonOrEmpty(resultado),
	)
	narrativa, errNarra := chamarZAI(narrarSystemPrompt, narrativaUserMsg, 256)
	if errNarra != nil {
		log.Printf("[chat-dados] narrativa falhou: %v", errNarra)
		narrativa = fmt.Sprintf("Encontrei %d resultado(s). Veja a tabela abaixo.", len(resultado))
	}

	return &DataQueryResult{
		Reply:    narrativa,
		SQL:      sqlFinal,
		Columns:  cols,
		Rows:     resultado,
		Truncado: truncado,
	}, nil
}

// normalizarValor converte tipos do Postgres em valores JSON-friendly.
func normalizarValor(v interface{}) interface{} {
	switch x := v.(type) {
	case []byte:
		return string(x)
	case time.Time:
		return x.Format(time.RFC3339)
	default:
		return x
	}
}

func truncar(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

func jsonOrEmpty(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil || len(b) == 0 {
		return "[]"
	}
	// trunca em ~3000 chars para não estourar tokens
	s := string(b)
	if len(s) > 3000 {
		s = s[:3000] + "...(truncado)"
	}
	return s
}
