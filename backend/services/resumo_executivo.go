package services

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// ── Tipos ─────────────────────────────────────────────────────────────────────

// KPIsResumoExecutivo agrega métricas do CD no período do resumo
type KPIsResumoExecutivo struct {
	CdID         int    `json:"cd_id"`
	CdNome       string `json:"cd_nome"`
	FilialNome   string `json:"filial_nome"`
	PeriodoInicio string `json:"periodo_inicio"` // YYYY-MM-DD
	PeriodoFim    string `json:"periodo_fim"`

	TotalPropostas    int `json:"total_propostas"`
	TotalAprovadas    int `json:"total_aprovadas"`
	TotalRejeitadas   int `json:"total_rejeitadas"`
	TotalPendentes    int `json:"total_pendentes"`
	TotalIgnorados    int `json:"total_ignorados"`

	Ampliar       int `json:"ampliar_slot"`
	Reduzir       int `json:"reduzir_slot"`
	Calibrados    int `json:"calibrados"`
	CurvaARevisar int `json:"curva_a_revisar"`

	TaxaAprovacaoPct float64 `json:"taxa_aprovacao_pct"`
	TaxaCompliancePct float64 `json:"taxa_compliance_pct"`

	TopMotivosRejeicao []KVPair `json:"top_motivos_rejeicao"`
	TopDeptosPendentes []KVPair `json:"top_deptos_pendentes"`
	TopProdutosCriticos []ProdutoCritico `json:"top_produtos_criticos"`

	AlertasUrgencia int `json:"alertas_urgencia"`
	AlertasAjustar  int `json:"alertas_ajustar"`
	AlertasCapMenor int `json:"alertas_cap_menor"`

	ImportsPeriodo []ImportInfo `json:"imports_periodo"`
	SemAtividade   bool         `json:"sem_atividade"` // true quando não houve aprovações/rejeições no período
}

type KVPair struct {
	Label string `json:"label"`
	Valor int    `json:"valor"`
}

type ProdutoCritico struct {
	CodProd      int    `json:"codprod"`
	Produto      string `json:"produto"`
	Departamento string `json:"departamento"`
	ClasseVenda  string `json:"classe_venda"`
	Delta        int    `json:"delta"`
	Prioridade   int    `json:"prioridade"`
}

// ImportInfo descreve um arquivo CSV importado no período do resumo
type ImportInfo struct {
	JobID        string `json:"job_id"`
	Filename     string `json:"filename"`
	Status       string `json:"status"`
	UploadedBy   string `json:"uploaded_by"`   // nome do usuário ou email
	UploadedEm   string `json:"uploaded_em"`   // YYYY-MM-DD HH:MM
	TotalLinhas  int    `json:"total_linhas"`
	LinhasOk     int    `json:"linhas_ok"`
	LinhasErro   int    `json:"linhas_erro"`
}

// ── Coleta de KPIs ────────────────────────────────────────────────────────────

// ColetarKPIs busca os indicadores agregados do CD no período informado
func ColetarKPIs(db *sql.DB, cdID int, inicio, fim time.Time) (*KPIsResumoExecutivo, error) {
	k := &KPIsResumoExecutivo{
		CdID:                cdID,
		PeriodoInicio:       inicio.Format("2006-01-02"),
		PeriodoFim:          fim.Format("2006-01-02"),
		TopMotivosRejeicao:  []KVPair{},
		TopDeptosPendentes:  []KVPair{},
		TopProdutosCriticos: []ProdutoCritico{},
		ImportsPeriodo:      []ImportInfo{},
	}
	// Limite superior do range (exclusivo) — fim do dia. Usado nas queries
	// como `created_at < $3` para incluir o dia inteiro do fim.
	fimExclusivo := fim.AddDate(0, 0, 1)

	// Nome do CD e filial
	if err := db.QueryRow(`
		SELECT c.nome, COALESCE(f.nome, '')
		  FROM farol.sp_centros_dist c
	     LEFT JOIN farol.sp_filiais f ON f.id = c.filial_id
		 WHERE c.id = $1
	`, cdID).Scan(&k.CdNome, &k.FilialNome); err != nil {
		return nil, fmt.Errorf("CD %d não encontrado: %w", cdID, err)
	}

	// Totais por status (no período)
	if err := db.QueryRow(`
		SELECT
		  COUNT(*),
		  COUNT(*) FILTER (WHERE status = 'aprovada'),
		  COUNT(*) FILTER (WHERE status = 'rejeitada'),
		  COUNT(*) FILTER (WHERE status = 'pendente')
		  FROM farol.sp_propostas
		 WHERE cd_id = $1
		   AND created_at >= $2 AND created_at < $3
	`, cdID, inicio, fimExclusivo).Scan(&k.TotalPropostas, &k.TotalAprovadas, &k.TotalRejeitadas, &k.TotalPendentes); err != nil {
		log.Printf("[resumo] erro totais: %v", err)
	}

	// Ignorados ativos (estado atual — não há soft delete)
	_ = db.QueryRow(`SELECT COUNT(*) FROM farol.sp_ignorados WHERE cd_id = $1`, cdID).Scan(&k.TotalIgnorados)

	// Quebra por tipo (delta > 0 = ampliar, delta < 0 = reduzir, delta = 0 = calibrado)
	_ = db.QueryRow(`
		SELECT
		  COUNT(*) FILTER (WHERE delta > 0),
		  COUNT(*) FILTER (WHERE delta < 0),
		  COUNT(*) FILTER (WHERE delta = 0)
		  FROM farol.sp_propostas
		 WHERE cd_id = $1
		   AND created_at >= $2 AND created_at < $3
	`, cdID, inicio, fimExclusivo).Scan(&k.Ampliar, &k.Reduzir, &k.Calibrados)

	// Curva A com sugestão de redução mantida pendente (proxy de "Curva A Revisar")
	_ = db.QueryRow(`
		SELECT COUNT(*) FROM farol.sp_propostas
		 WHERE cd_id = $1
		   AND classe_venda = 'A'
		   AND delta < 0
		   AND status = 'pendente'
		   AND created_at >= $2 AND created_at < $3
	`, cdID, inicio, fimExclusivo).Scan(&k.CurvaARevisar)

	// Taxa de aprovação e compliance
	if k.TotalPropostas > 0 {
		processadas := k.TotalAprovadas + k.TotalRejeitadas
		if processadas > 0 {
			k.TaxaAprovacaoPct = float64(k.TotalAprovadas) / float64(processadas) * 100
		}
		k.TaxaCompliancePct = float64(processadas) / float64(k.TotalPropostas) * 100
	}

	// Top 5 motivos de rejeição
	rows, err := db.Query(`
		SELECT COALESCE(mr.descricao, 'Sem motivo'), COUNT(*) AS qtd
		  FROM farol.sp_propostas p
	     LEFT JOIN farol.sp_tipo_rejeicao mr ON mr.id = p.motivo_rejeicao_id
		 WHERE p.cd_id = $1 AND p.status = 'rejeitada'
		   AND p.created_at >= $2 AND p.created_at < $3
		 GROUP BY mr.descricao
		 ORDER BY qtd DESC
		 LIMIT 5
	`, cdID, inicio, fimExclusivo)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var kv KVPair
			if rows.Scan(&kv.Label, &kv.Valor) == nil {
				k.TopMotivosRejeicao = append(k.TopMotivosRejeicao, kv)
			}
		}
	}

	// Top 5 departamentos com mais pendentes (departamento vem do sp_enderecos)
	rows2, err := db.Query(`
		SELECT COALESCE(e.departamento, '—'), COUNT(*) AS qtd
		  FROM farol.sp_propostas p
		  JOIN farol.sp_enderecos e ON e.id = p.endereco_id
		 WHERE p.cd_id = $1 AND p.status = 'pendente'
		   AND p.created_at >= $2 AND p.created_at < $3
		 GROUP BY e.departamento
		 ORDER BY qtd DESC
		 LIMIT 5
	`, cdID, inicio, fimExclusivo)
	if err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var kv KVPair
			if rows2.Scan(&kv.Label, &kv.Valor) == nil {
				k.TopDeptosPendentes = append(k.TopDeptosPendentes, kv)
			}
		}
	}

	// Top 10 produtos críticos (Curva A com maior delta absoluto)
	rows3, err := db.Query(`
		SELECT p.codprod, p.produto, COALESCE(e.departamento,'—'), COALESCE(p.classe_venda,'—'), p.delta
		  FROM farol.sp_propostas p
		  JOIN farol.sp_enderecos e ON e.id = p.endereco_id
		 WHERE p.cd_id = $1 AND p.status = 'pendente'
		   AND p.created_at >= $2 AND p.created_at < $3
		   AND p.classe_venda = 'A'
		 ORDER BY ABS(p.delta) DESC
		 LIMIT 10
	`, cdID, inicio, fimExclusivo)
	if err == nil {
		defer rows3.Close()
		for rows3.Next() {
			var pc ProdutoCritico
			if rows3.Scan(&pc.CodProd, &pc.Produto, &pc.Departamento, &pc.ClasseVenda, &pc.Delta) == nil {
				k.TopProdutosCriticos = append(k.TopProdutosCriticos, pc)
			}
		}
	}

	// Imports CSV no período (úteis quando não houve atividade na semana)
	rows4, err := db.Query(`
		SELECT j.id::text,
		       j.filename,
		       j.status,
		       COALESCE(NULLIF(u.email,''), 'desconhecido'),
		       to_char(j.created_at AT TIME ZONE 'America/Sao_Paulo', 'YYYY-MM-DD HH24:MI'),
		       COALESCE(j.total_linhas, 0),
		       COALESCE(j.linhas_ok, 0),
		       COALESCE(j.linhas_erro, 0)
		  FROM farol.sp_csv_jobs j
	     LEFT JOIN public.users u ON u.id = j.uploaded_by
		 WHERE j.cd_id = $1
		   AND j.created_at >= $2 AND j.created_at < $3
		 ORDER BY j.created_at DESC
		 LIMIT 20
	`, cdID, inicio, fimExclusivo)
	if err == nil {
		defer rows4.Close()
		for rows4.Next() {
			var imp ImportInfo
			if rows4.Scan(&imp.JobID, &imp.Filename, &imp.Status, &imp.UploadedBy, &imp.UploadedEm,
				&imp.TotalLinhas, &imp.LinhasOk, &imp.LinhasErro) == nil {
				k.ImportsPeriodo = append(k.ImportsPeriodo, imp)
			}
		}
	}

	// Marca como "sem atividade" quando não houve aprovações nem rejeições
	k.SemAtividade = (k.TotalAprovadas + k.TotalRejeitadas) == 0

	// Alertas atuais usando o último csv_job do CD
	_ = db.QueryRow(`
		WITH job AS (
		  SELECT id FROM farol.sp_csv_jobs
		   WHERE cd_id = $1 AND status = 'done'
		   ORDER BY created_at DESC LIMIT 1
		)
		SELECT
		  COUNT(*) FILTER (WHERE COALESCE(e.med_venda_cx,0) >= COALESCE(e.capacidade,0) AND COALESCE(e.capacidade,0) > 0),
		  COUNT(*) FILTER (WHERE COALESCE(e.ponto_reposicao,0) > 0 AND COALESCE(e.med_venda_cx,0) >= e.ponto_reposicao),
		  COUNT(*) FILTER (WHERE COALESCE(e.capacidade,0) > 0 AND COALESCE(e.med_venda_cx,0) > 0 AND e.capacidade::numeric/e.med_venda_cx < 2)
		  FROM farol.sp_enderecos e
		 WHERE e.job_id = (SELECT id FROM job)
	`, cdID).Scan(&k.AlertasUrgencia, &k.AlertasAjustar, &k.AlertasCapMenor)

	return k, nil
}

// ── Geração de narrativa via Z.AI (mesmo endpoint do assistente) ─────────────

const promptResumoExecutivo = `Você é um analista sênior de logística e calibragem de picking, escrevendo um resumo executivo SEMANAL para o gestor de um centro de distribuição (CD) brasileiro.

Receberá um JSON com KPIs do CD na semana. Sua tarefa:
1. Escrever um resumo executivo em português (markdown) com estrutura:
   - Parágrafo de abertura com a situação geral (2-3 frases, números-chave)
   - Lista "## Pontos de atenção" (3 itens críticos no máximo, baseados nos dados)
   - Lista "## Tendências detectadas" (1-3 itens — só se houver sinal claro nos dados)
   - Bloco "## Sugestão de ação" (1 ação concreta para a próxima semana)
2. Tom: direto, executivo, sem jargão técnico
3. Use os números EXATOS do JSON. Se algum dado não estiver disponível, omita o ponto sem inventar
4. Máximo de ~250 palavras totais
5. Não repita o JSON — só análise narrativa

CASO ESPECIAL — sem_atividade=true:
   Se o campo "sem_atividade" estiver true, significa que NÃO houve aprovações nem rejeições no período. Nesse caso:
   - Abra reconhecendo a baixa atividade ("A semana não registrou movimentações de calibragem...")
   - Se houver imports_periodo: liste cada arquivo importado (filename, uploaded_by, uploaded_em, total_linhas, status) em "## Importações do período"
   - Em "## Sugestão de ação": cobre o gestor para revisar as propostas pendentes ou importar dados se nada chegou
   - Não invente alertas que não estão nos dados

Não inclua saudações, despedidas ou nome do destinatário — apenas o conteúdo do resumo.`

// GerarNarrativaIA chama a Z.AI com os KPIs serializados em JSON e retorna o markdown
func GerarNarrativaIA(kpis *KPIsResumoExecutivo) (string, error) {
	apiKey := os.Getenv("ZAI_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("ZAI_API_KEY não configurada")
	}

	dadosJSON, _ := json.MarshalIndent(kpis, "", "  ")

	type msg struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	body, _ := json.Marshal(map[string]interface{}{
		"model":       "glm-4.5-air",
		"max_tokens":  1024,
		"temperature": 0.4,
		"messages": []msg{
			{Role: "system", Content: promptResumoExecutivo},
			{Role: "user", Content: "KPIs do CD nesta semana:\n\n" + string(dadosJSON)},
		},
	})

	client := &http.Client{Timeout: 30 * time.Second}
	req, _ := http.NewRequest("POST", "https://api.z.ai/api/coding/paas/v4/chat/completions", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("falha ao chamar Z.AI: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Z.AI retornou status %d: %s", resp.StatusCode, string(raw))
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
		return "", fmt.Errorf("parse falhou: %w", err)
	}
	if len(r.Choices) == 0 {
		return "", fmt.Errorf("Z.AI sem choices")
	}
	out := r.Choices[0].Message.Content
	if out == "" {
		out = r.Choices[0].Message.ReasoningContent
	}
	return strings.TrimSpace(out), nil
}

// ── Persistência ──────────────────────────────────────────────────────────────

// SalvarRelatorio insere o relatório gerado e retorna o id criado
func SalvarRelatorio(db *sql.DB, kpis *KPIsResumoExecutivo, narrativa string, criadoPor string) (int, error) {
	dadosJSON, _ := json.Marshal(kpis)
	var id int
	err := db.QueryRow(`
		INSERT INTO farol.sp_relatorios_semanais (cd_id, periodo_inicio, periodo_fim, dados_json, narrativa_md, criado_por)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`, kpis.CdID, kpis.PeriodoInicio, kpis.PeriodoFim, dadosJSON, narrativa, criadoPor).Scan(&id)
	return id, err
}

// MarcarEnviado atualiza o relatório com os destinatários e timestamp de envio
func MarcarEnviado(db *sql.DB, relatorioID int, enviadoPara []string, erroEnvio string) error {
	// Postgres TEXT[] literal: '{a@b.com,c@d.com}'
	_, err := db.Exec(`
		UPDATE farol.sp_relatorios_semanais
		   SET enviado_em = NOW(), enviado_para = $2, erro_envio = NULLIF($3, '')
		 WHERE id = $1
	`, relatorioID, "{"+strings.Join(enviadoPara, ",")+"}", erroEnvio)
	return err
}

// EnviarResumoPorEmail busca destinatários ativos do CD e envia o relatório
// retornando lista de emails enviados e mensagem de erro (se houver).
// Reaproveita GetEmailConfig + sendMailSSL do email.go.
func EnviarResumoPorEmail(db *sql.DB, relatorioID int) ([]string, error) {
	log.Printf("[resumo] enviando relatório %d por email", relatorioID)
	// Carrega o relatório
	var (
		cdID                   int
		periodoIni, periodoFim string
		dadosJSON, narrativa   string
	)
	err := db.QueryRow(`
		SELECT cd_id,
		       to_char(periodo_inicio, 'DD/MM/YYYY'),
		       to_char(periodo_fim, 'DD/MM/YYYY'),
		       dados_json::text, narrativa_md
		  FROM farol.sp_relatorios_semanais
		 WHERE id = $1
	`, relatorioID).Scan(&cdID, &periodoIni, &periodoFim, &dadosJSON, &narrativa)
	if err != nil {
		return nil, fmt.Errorf("relatório não encontrado: %w", err)
	}

	// Nome do CD e da filial (para mensagens de erro claras)
	var cdNome, filialNome string
	_ = db.QueryRow(`
		SELECT c.nome, COALESCE(f.nome, '')
		  FROM farol.sp_centros_dist c
	     LEFT JOIN farol.sp_filiais f ON f.id = c.filial_id
		 WHERE c.id = $1
	`, cdID).Scan(&cdNome, &filialNome)
	if cdNome == "" {
		cdNome = fmt.Sprintf("CD %d", cdID)
	}

	var kpis KPIsResumoExecutivo
	if err := json.Unmarshal([]byte(dadosJSON), &kpis); err != nil {
		return nil, fmt.Errorf("parse dados_json: %w", err)
	}

	// Lista de destinatários ativos do CD
	rows, err := db.Query(`
		SELECT email, nome_completo
		  FROM farol.sp_destinatarios_resumo
		 WHERE cd_id = $1 AND ativo = TRUE
	`, cdID)
	if err != nil {
		return nil, fmt.Errorf("listar destinatários: %w", err)
	}
	defer rows.Close()

	type destinatario struct{ Email, Nome string }
	var destinos []destinatario
	for rows.Next() {
		var d destinatario
		if rows.Scan(&d.Email, &d.Nome) == nil {
			destinos = append(destinos, d)
		}
	}

	if len(destinos) == 0 {
		// Diagnóstico: contadores e onde o usuário pode estar cadastrado
		var total, inativos int
		_ = db.QueryRow(`SELECT COUNT(*), COUNT(*) FILTER (WHERE NOT ativo) FROM farol.sp_destinatarios_resumo WHERE cd_id = $1`, cdID).Scan(&total, &inativos)

		// Lista CDs (com nome) onde HÁ destinatários ativos para orientar o usuário
		outrosRows, _ := db.Query(`
			SELECT DISTINCT c.id, c.nome, COUNT(d.id) AS qtd
			  FROM farol.sp_destinatarios_resumo d
			  JOIN farol.sp_centros_dist c ON c.id = d.cd_id
			 WHERE d.ativo = TRUE
			 GROUP BY c.id, c.nome
			 ORDER BY c.nome
			 LIMIT 5
		`)
		outros := []string{}
		if outrosRows != nil {
			defer outrosRows.Close()
			for outrosRows.Next() {
				var id, qtd int
				var nome string
				if outrosRows.Scan(&id, &nome, &qtd) == nil {
					outros = append(outros, fmt.Sprintf("%s (id=%d, %d destinatários)", nome, id, qtd))
				}
			}
		}

		log.Printf("[resumo] CD=%d (%s) sem destinatários ativos. total=%d inativos=%d. CDs com destinatários: %v",
			cdID, cdNome, total, inativos, outros)

		msg := fmt.Sprintf("Nenhum destinatário ativo cadastrado para o CD %q (id=%d).", cdNome, cdID)
		if total > 0 {
			msg += fmt.Sprintf(" Há %d cadastrado(s) neste CD mas todos estão inativos.", total)
		}
		if len(outros) > 0 {
			msg += " Destinatários ativos existem em: " + strings.Join(outros, "; ") + "."
		} else {
			msg += " Cadastre em Configurações → Destinatários Resumo."
		}
		return nil, fmt.Errorf("%s", msg)
	}
	log.Printf("[resumo] CD=%d (%s) %d destinatários ativos: enviando", cdID, cdNome, len(destinos))

	cfg := GetEmailConfig()
	if cfg.Password == "" {
		return nil, fmt.Errorf("SMTP não configurado")
	}

	subject := fmt.Sprintf("Farol - Resumo Executivo %s (%s)", kpis.CdNome, periodoFim)
	html := buildResumoHTML(&kpis, narrativa, periodoIni, periodoFim)
	plain := buildResumoPlainText(&kpis, narrativa, periodoIni, periodoFim)

	enviados := []string{}
	for _, d := range destinos {
		boundary := fmt.Sprintf("rs_%d", time.Now().UnixNano())
		var msg strings.Builder
		fmt.Fprintf(&msg, "From: %s\r\n", cfg.From)
		fmt.Fprintf(&msg, "To: %s <%s>\r\n", d.Nome, d.Email)
		fmt.Fprintf(&msg, "Subject: %s\r\n", subject)
		msg.WriteString("MIME-Version: 1.0\r\n")
		fmt.Fprintf(&msg, "Content-Type: multipart/alternative; boundary=%q\r\n\r\n", boundary)
		// plain
		fmt.Fprintf(&msg, "--%s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s\r\n", boundary, plain)
		// html
		fmt.Fprintf(&msg, "--%s\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n%s\r\n", boundary, html)
		fmt.Fprintf(&msg, "--%s--\r\n", boundary)

		var sendErr error
		if cfg.Port == 465 {
			sendErr = sendMailSSL(cfg, []string{d.Email}, []byte(msg.String()))
		} else {
			sendErr = fmt.Errorf("porta SMTP %d não suportada (somente 465)", cfg.Port)
		}
		if sendErr != nil {
			log.Printf("[resumo] ✗ erro envio para %s: %v", d.Email, sendErr)
			continue
		}
		log.Printf("[resumo] ✓ enviado para %s", d.Email)
		enviados = append(enviados, d.Email)
	}
	log.Printf("[resumo] envio concluído: %d/%d destinatários receberam o relatório %d",
		len(enviados), len(destinos), relatorioID)

	if len(enviados) == 0 {
		return nil, fmt.Errorf("falha ao enviar para todos os %d destinatários", len(destinos))
	}
	return enviados, nil
}

// ── Renderização do email ─────────────────────────────────────────────────────

func buildResumoHTML(k *KPIsResumoExecutivo, narrativa, periodoIni, periodoFim string) string {
	narrativaHTML := convertMarkdownToHTML(narrativa)

	var sb strings.Builder
	sb.WriteString(`<!DOCTYPE html><html><head><meta charset="UTF-8"><style>
body{font-family:Arial,sans-serif;line-height:1.6;color:#333;max-width:680px;margin:0 auto;background:#f4f4f8}
.wrap{padding:20px}
.hdr{background:#2d3748;color:#fff;padding:18px 22px;border-radius:8px 8px 0 0;text-align:center}
.hdr-logo{font-size:20px;font-weight:700}
.hdr-sub{font-size:13px;color:#cbd5e0;margin-top:4px}
.body{background:#fff;padding:22px;border-radius:0 0 8px 8px}
.info-box{background:#ebf8ff;border-left:4px solid #3182ce;padding:10px 14px;margin:0 0 18px;border-radius:0 6px 6px 0;font-size:12px;color:#2c5282}
.sec{margin:18px 0}
.sec-title{font-size:12px;font-weight:700;text-transform:uppercase;letter-spacing:.06em;color:#718096;border-bottom:2px solid #e2e8f0;padding-bottom:6px;margin-bottom:12px}
.kpi-table{width:100%;border-collapse:separate;border-spacing:6px}
.kpi-cell{border:1px solid #e2e8f0;border-radius:6px;padding:10px;text-align:center;background:#f7fafc}
.kpi-label{font-size:9px;text-transform:uppercase;letter-spacing:.06em;color:#718096}
.kpi-val{font-size:18px;font-weight:700;color:#2d3748;margin:2px 0}
.ai-box{background:#f7fafc;border:1px solid #e2e8f0;border-radius:8px;padding:18px;margin:18px 0}
.ai-label{font-size:11px;font-weight:700;text-transform:uppercase;color:#a0aec0;margin-bottom:10px}
table.dt{width:100%;border-collapse:collapse;font-size:12px;margin:8px 0}
table.dt th{background:#4a5568;color:#fff;padding:6px 10px;text-align:left;font-size:11px}
table.dt td{padding:6px 10px;border-bottom:1px solid #e2e8f0}
.footer{text-align:center;padding:14px;color:#a0aec0;font-size:11px}
</style></head><body><div class="wrap">`)

	fmt.Fprintf(&sb, `<div class="hdr"><div class="hdr-logo">Farol</div><div class="hdr-sub">Resumo Executivo Semanal &mdash; %s</div></div>`, k.CdNome)

	sb.WriteString(`<div class="body">`)
	fmt.Fprintf(&sb, `<div class="info-box"><strong>CD:</strong> %s &nbsp;|&nbsp; <strong>Filial:</strong> %s &nbsp;|&nbsp; <strong>Per&iacute;odo:</strong> %s a %s</div>`,
		k.CdNome, k.FilialNome, periodoIni, periodoFim)

	// KPIs principais
	sb.WriteString(`<div class="sec"><div class="sec-title">Resumo da Semana</div><table class="kpi-table"><tr>`)
	fmt.Fprintf(&sb, `<td class="kpi-cell"><div class="kpi-label">Propostas Geradas</div><div class="kpi-val">%d</div></td>`, k.TotalPropostas)
	fmt.Fprintf(&sb, `<td class="kpi-cell"><div class="kpi-label">Aprovadas</div><div class="kpi-val" style="color:#16a34a">%d</div></td>`, k.TotalAprovadas)
	fmt.Fprintf(&sb, `<td class="kpi-cell"><div class="kpi-label">Rejeitadas</div><div class="kpi-val" style="color:#dc2626">%d</div></td>`, k.TotalRejeitadas)
	fmt.Fprintf(&sb, `<td class="kpi-cell"><div class="kpi-label">Pendentes</div><div class="kpi-val" style="color:#ca8a04">%d</div></td>`, k.TotalPendentes)
	sb.WriteString(`</tr><tr>`)
	fmt.Fprintf(&sb, `<td class="kpi-cell"><div class="kpi-label">Ampliar Slot</div><div class="kpi-val" style="color:#dc2626">%d</div></td>`, k.Ampliar)
	fmt.Fprintf(&sb, `<td class="kpi-cell"><div class="kpi-label">Reduzir Slot</div><div class="kpi-val" style="color:#ca8a04">%d</div></td>`, k.Reduzir)
	fmt.Fprintf(&sb, `<td class="kpi-cell"><div class="kpi-label">Calibrados</div><div class="kpi-val" style="color:#2563eb">%d</div></td>`, k.Calibrados)
	fmt.Fprintf(&sb, `<td class="kpi-cell"><div class="kpi-label">Curva A Revisar</div><div class="kpi-val" style="color:#d97706">%d</div></td>`, k.CurvaARevisar)
	sb.WriteString(`</tr><tr>`)
	fmt.Fprintf(&sb, `<td class="kpi-cell"><div class="kpi-label">Taxa Aprovação</div><div class="kpi-val">%.0f%%</div></td>`, k.TaxaAprovacaoPct)
	fmt.Fprintf(&sb, `<td class="kpi-cell"><div class="kpi-label">Compliance</div><div class="kpi-val">%.0f%%</div></td>`, k.TaxaCompliancePct)
	fmt.Fprintf(&sb, `<td class="kpi-cell"><div class="kpi-label">Ignorados</div><div class="kpi-val">%d</div></td>`, k.TotalIgnorados)
	fmt.Fprintf(&sb, `<td class="kpi-cell"><div class="kpi-label">Alertas Críticos</div><div class="kpi-val" style="color:#dc2626">%d</div></td>`,
		k.AlertasUrgencia+k.AlertasAjustar+k.AlertasCapMenor)
	sb.WriteString(`</tr></table></div>`)

	// Narrativa IA
	sb.WriteString(`<div class="ai-box"><div class="ai-label">&#129302; An&aacute;lise da Intelig&ecirc;ncia Artificial</div>`)
	sb.WriteString(narrativaHTML)
	sb.WriteString(`</div>`)

	// Top motivos rejeição
	if len(k.TopMotivosRejeicao) > 0 {
		sb.WriteString(`<div class="sec"><div class="sec-title">Top motivos de rejei&ccedil;&atilde;o</div><table class="dt"><thead><tr><th>Motivo</th><th style="text-align:right">Qtd</th></tr></thead><tbody>`)
		for _, m := range k.TopMotivosRejeicao {
			fmt.Fprintf(&sb, `<tr><td>%s</td><td style="text-align:right">%d</td></tr>`, m.Label, m.Valor)
		}
		sb.WriteString(`</tbody></table></div>`)
	}

	// Top produtos críticos
	if len(k.TopProdutosCriticos) > 0 {
		sb.WriteString(`<div class="sec"><div class="sec-title">Top produtos cr&iacute;ticos (Curva A)</div><table class="dt"><thead><tr><th>C&oacute;d.</th><th>Produto</th><th>Depto</th><th style="text-align:right">&Delta;</th></tr></thead><tbody>`)
		for _, p := range k.TopProdutosCriticos {
			color := "#16a34a"
			signal := ""
			if p.Delta > 0 {
				color = "#dc2626"
				signal = "+"
			} else if p.Delta < 0 {
				color = "#ca8a04"
			}
			fmt.Fprintf(&sb, `<tr><td>%d</td><td>%s</td><td>%s</td><td style="text-align:right;color:%s;font-weight:600">%s%d CX</td></tr>`,
				p.CodProd, p.Produto, p.Departamento, color, signal, p.Delta)
		}
		sb.WriteString(`</tbody></table></div>`)
	}

	// Importações do período (úteis principalmente quando sem_atividade=true)
	if len(k.ImportsPeriodo) > 0 {
		sb.WriteString(`<div class="sec"><div class="sec-title">Importa&ccedil;&otilde;es do per&iacute;odo</div><table class="dt"><thead><tr><th>Data</th><th>Arquivo</th><th>Importado por</th><th style="text-align:right">Linhas</th><th>Status</th></tr></thead><tbody>`)
		for _, imp := range k.ImportsPeriodo {
			statusColor := "#16a34a"
			switch imp.Status {
			case "failed":
				statusColor = "#dc2626"
			case "pending", "processing":
				statusColor = "#ca8a04"
			}
			fmt.Fprintf(&sb, `<tr><td>%s</td><td>%s</td><td>%s</td><td style="text-align:right">%d</td><td style="color:%s;font-weight:600">%s</td></tr>`,
				imp.UploadedEm, imp.Filename, imp.UploadedBy, imp.TotalLinhas, statusColor, imp.Status)
		}
		sb.WriteString(`</tbody></table></div>`)
	}

	appURL := os.Getenv("APP_URL")
	if appURL == "" {
		appURL = "https://farol.fbtax.cloud"
	}
	fmt.Fprintf(&sb, `<div style="text-align:center;margin:22px 0"><a href="%s/resumos" style="display:inline-block;padding:10px 24px;background:#2d3748;color:#fff;text-decoration:none;border-radius:6px;font-weight:700;font-size:13px">Acessar Painel Completo</a></div>`, appURL)

	sb.WriteString(`</div><div class="footer">&copy; Farol &mdash; Calibragem Inteligente de Picking</div></div></body></html>`)
	return sb.String()
}

func buildResumoPlainText(k *KPIsResumoExecutivo, narrativa, periodoIni, periodoFim string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Farol - Resumo Executivo Semanal\n\nCD: %s\nFilial: %s\nPeriodo: %s a %s\n\n", k.CdNome, k.FilialNome, periodoIni, periodoFim)
	sb.WriteString("=== RESUMO DA SEMANA ===\n")
	fmt.Fprintf(&sb, "Propostas geradas: %d\n", k.TotalPropostas)
	fmt.Fprintf(&sb, "  - Aprovadas:  %d\n", k.TotalAprovadas)
	fmt.Fprintf(&sb, "  - Rejeitadas: %d\n", k.TotalRejeitadas)
	fmt.Fprintf(&sb, "  - Pendentes:  %d\n", k.TotalPendentes)
	fmt.Fprintf(&sb, "Ampliar slot: %d | Reduzir slot: %d | Calibrados: %d | Curva A revisar: %d\n", k.Ampliar, k.Reduzir, k.Calibrados, k.CurvaARevisar)
	fmt.Fprintf(&sb, "Taxa de aprovacao: %.0f%% | Compliance: %.0f%% | Ignorados: %d\n\n", k.TaxaAprovacaoPct, k.TaxaCompliancePct, k.TotalIgnorados)

	if len(k.ImportsPeriodo) > 0 {
		sb.WriteString("=== IMPORTACOES DO PERIODO ===\n")
		for _, imp := range k.ImportsPeriodo {
			fmt.Fprintf(&sb, "%s | %s | por %s | %d linhas | %s\n",
				imp.UploadedEm, imp.Filename, imp.UploadedBy, imp.TotalLinhas, imp.Status)
		}
		sb.WriteString("\n")
	}

	sb.WriteString("=== ANALISE DA IA ===\n")
	sb.WriteString(narrativa)
	sb.WriteString("\n\n---\n(c) Farol\n")
	return sb.String()
}

// ── Orquestração: gerar + (opcionalmente) enviar ─────────────────────────────

// GerarResumoExecutivo coleta os KPIs da última semana, gera a narrativa via IA, salva e retorna o id
func GerarResumoExecutivo(db *sql.DB, cdID int, criadoPor string) (int, *KPIsResumoExecutivo, string, error) {
	fim := time.Now()
	inicio := fim.AddDate(0, 0, -7)
	log.Printf("[resumo] CD=%d gerando resumo período %s → %s", cdID, inicio.Format("2006-01-02"), fim.Format("2006-01-02"))

	kpis, err := ColetarKPIs(db, cdID, inicio, fim)
	if err != nil {
		log.Printf("[resumo] CD=%d coletar KPIs FALHOU: %v", cdID, err)
		return 0, nil, "", fmt.Errorf("coletar KPIs: %w", err)
	}
	log.Printf("[resumo] CD=%d KPIs coletados: total=%d aprovadas=%d rejeitadas=%d imports=%d sem_atividade=%v",
		cdID, kpis.TotalPropostas, kpis.TotalAprovadas, kpis.TotalRejeitadas, len(kpis.ImportsPeriodo), kpis.SemAtividade)

	narrativa, err := GerarNarrativaIA(kpis)
	if err != nil {
		log.Printf("[resumo] CD=%d gerar narrativa FALHOU: %v", cdID, err)
		return 0, nil, "", fmt.Errorf("gerar narrativa: %w", err)
	}
	log.Printf("[resumo] CD=%d narrativa gerada (%d chars)", cdID, len(narrativa))

	id, err := SalvarRelatorio(db, kpis, narrativa, criadoPor)
	if err != nil {
		log.Printf("[resumo] CD=%d salvar relatório FALHOU: %v", cdID, err)
		return 0, nil, "", fmt.Errorf("salvar relatório: %w", err)
	}
	log.Printf("[resumo] CD=%d relatório %d salvo com sucesso", cdID, id)

	return id, kpis, narrativa, nil
}
