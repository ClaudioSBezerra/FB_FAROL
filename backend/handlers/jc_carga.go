// jc_carga.go — orquestra a carga diária do JC: extrai, importa, avisa.
//
// Separado do jc_extrator.go de propósito: lá é só "ler o Oracle e virar CSV",
// aqui é a política (qual dia, o que fazer com o resultado, quem avisar).
package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"fb_farol/services"
)

const jcDestinatariosPadrao = "claudiosousadebezerra@gmail.com,keslley.paula@jcdistribuicao.com.br"

// tzBrasil — todo horário exibido no e-mail é de Brasília. O container roda em
// UTC; sem isso o resumo diria "concluído às 09:30" para um job das 06:30.
func tzBrasil() *time.Location {
	if loc, err := time.LoadLocation("America/Sao_Paulo"); err == nil {
		return loc
	}
	return time.FixedZone("BRT", -3*3600)
}

// ExecutarCargaJC roda o ciclo de UM dia e manda o e-mail do dia. Consolida os
// agregados no mesmo passo (skipRefresh=false): na carga diária é um dia só, o
// custo é aceitável e o painel precisa estar atualizado logo em seguida.
func ExecutarCargaJC(db *sql.DB, dataRef time.Time) *ResultadoExtracao {
	res := executarCargaJCSemEmail(db, dataRef, false)
	enviarResumoJC(res)
	return res
}

// executarCargaJCSemEmail faz o trabalho e NÃO notifica — é o que o backfill de
// intervalo usa, para mandar um e-mail consolidado no fim em vez de um por dia.
//
// Nunca entra em pânico: qualquer falha vira res.Erro e chega ao relatório —
// silêncio é o pior desfecho possível numa carga automática, porque ninguém
// descobre que o painel parou de atualizar até alguém estranhar um número.
func executarCargaJCSemEmail(db *sql.DB, dataRef time.Time, pularConsolidacao bool) *ResultadoExtracao {
	res := &ResultadoExtracao{DataRef: dataRef, Inicio: time.Now()}
	defer func() {
		res.Fim = time.Now()
		if r := recover(); r != nil {
			res.Erro = fmt.Errorf("pânico durante a carga: %v", r)
		}
	}()

	empresaID := strings.TrimSpace(os.Getenv("JC_EMPRESA_ID"))
	if empresaID == "" {
		res.Erro = fmt.Errorf("JC_EMPRESA_ID não configurado")
		return res
	}

	log.Printf("[jc:carga] iniciando dia=%s empresa=%s", dataRef.Format("2006-01-02"), empresaID)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()

	arquivo, err := ExtrairDiaJC(ctx, dataRef, res)
	if err != nil {
		res.Erro = err
		log.Printf("[jc:carga] extração FALHOU: %v", err)
		return res
	}

	// Zero linhas NÃO é sucesso silencioso. Pode ser dia sem movimento
	// (domingo/feriado) ou pode ser o JOB deles não ter rodado ainda — e as duas
	// situações precisam chegar diferentes de "importei tudo certo". Não
	// chamamos o import (não faz sentido apagar o dia e recarregar vazio).
	if res.LinhasLidas == 0 {
		os.Remove(arquivo)
		res.StatusImport = "sem_dados"
		log.Printf("[jc:carga] dia=%s veio VAZIO — import não executado", dataRef.Format("2006-01-02"))
		return res
	}

	// Job na mesma tabela do upload manual: a tela de histórico de importações
	// passa a mostrar a carga automática junto com as manuais, sem tela nova.
	var jobID string
	if err := db.QueryRow(`
		INSERT INTO vendas_import_jobs (empresa_id, ano, mes, status, total_lines)
		VALUES ($1, $2, $3, 'pending', $4) RETURNING id`,
		empresaID, dataRef.Year(), int(dataRef.Month()), res.LinhasLidas,
	).Scan(&jobID); err != nil {
		os.Remove(arquivo)
		res.Erro = fmt.Errorf("criar job de import: %w", err)
		return res
	}
	res.JobID = jobID

	// processImportJob é síncrono aqui (no upload manual roda em goroutine para
	// devolver o job_id na hora). Precisamos esperar para relatar o resultado
	// real no e-mail — e ele já apaga o arquivo no fim, via defer próprio.
	tImp := time.Now()
	impCtx, impCancel := context.WithCancel(ctx)
	importJobs.Store(jobID, impCancel)
	spCtx := &FarolContext{EmpresaID: empresaID, AllFiliais: true}
	processImportJob(impCtx, db, jobID, arquivo, true, spCtx,
		dataRef.Year(), int(dataRef.Month()), pularConsolidacao)
	res.DuracaoImport = time.Since(tImp)

	// Status final vem do banco, não do que achamos que aconteceu.
	// Colunas conforme migration 140: `importados` e `message` (não
	// processed_lines/error_message — errei isso na 1ª versão e o erro só
	// apareceu em produção, porque a consulta falha em runtime, não no build).
	var status string
	var processadas sql.NullInt64
	var erroMsg sql.NullString
	if err := db.QueryRow(`
		SELECT status, importados, message
		FROM vendas_import_jobs WHERE id = $1`, jobID,
	).Scan(&status, &processadas, &erroMsg); err != nil {
		res.Erro = fmt.Errorf("ler status do job %s: %w", jobID, err)
		return res
	}
	res.StatusImport = status
	res.LinhasImportad = int(processadas.Int64)
	if status != "done" {
		msg := "sem detalhe"
		if erroMsg.Valid && erroMsg.String != "" {
			msg = erroMsg.String
		}
		res.Erro = fmt.Errorf("import terminou como %q: %s", status, msg)
	}

	// time.Since(res.Inicio), NÃO res.Duracao(): o `Fim` só é preenchido no
	// defer, que roda DEPOIS deste log. Usar Duracao() aqui lê um Fim zerado e
	// estoura o time.Duration — em produção saiu "-2562047h47m16s". O e-mail
	// não tinha o problema (o defer preenche o Fim antes de enviar).
	log.Printf("[jc:carga] dia=%s CONCLUÍDO status=%s lidas=%d importadas=%d em %v",
		dataRef.Format("2006-01-02"), status, res.LinhasLidas, res.LinhasImportad,
		time.Since(res.Inicio).Round(time.Second))
	return res
}

// destinatariosJC aceita vírgula, ponto-e-vírgula, espaço ou quebra de linha
// como separador. O `;` é o padrão do Outlook e foi o que veio configurado na
// primeira tentativa (29/07): o SMTP recusou com
// `501 Bad recipient address syntax` porque a lista inteira virou UM endereço.
// Separador é detalhe de digitação, não decisão — aceitar todos evita um erro
// que só aparece quando o e-mail deixa de chegar.
// jcMaxDiasIntervalo — teto de segurança. Cada dia custa ~5min (1min de
// consulta no Oracle + import + reconsolidação dos agregados), então 92 dias já
// são ~8h de carga contínua. O limite existe para transformar um erro de
// digitação de ano em erro imediato, não em 3 dias de martelada no Oracle deles.
const jcMaxDiasIntervalo = 92

// diaJaTemDados — usado pelo modo "pular existentes" do backfill. Olha as três
// tabelas de destino: basta uma ter linha para o dia contar como carregado.
func diaJaTemDados(db *sql.DB, empresaID string, dia time.Time) bool {
	var existe bool
	err := db.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM vendas_faturadas
			 WHERE empresa_id=$1 AND data_faturamento=$2
			UNION ALL
			SELECT 1 FROM vendas_transmitidas
			 WHERE empresa_id=$1 AND data_transmissao=$2
			UNION ALL
			SELECT 1 FROM vendas_ccd
			 WHERE empresa_id=$1 AND data_evento=$2
		)`, empresaID, dia).Scan(&existe)
	if err != nil {
		// Na dúvida, NÃO pula: recarregar um dia é barato e idempotente;
		// deixar buraco no histórico não é.
		log.Printf("[jc:carga] checagem de dia existente falhou (%v) — vai recarregar %s",
			err, dia.Format("2006-01-02"))
		return false
	}
	return existe
}

// ExecutarCargaJCIntervalo roda a carga dia a dia, em SEQUÊNCIA, e manda UM
// e-mail com o consolidado.
//
// Sequencial de propósito: em paralelo, várias consultas de 1min no Oracle deles
// competiriam entre si e com o JOB de replicação, e do nosso lado cada import
// dispara reconsolidação de agregados. É a mesma lição do prewarmDailyRanges,
// que saturava o banco rodando 6 scans concorrentes.
//
// Falha de um dia NÃO aborta o resto — num backfill, perder os 20 dias
// seguintes porque um deu erro é pior que ter um buraco conhecido e relatado.
func ExecutarCargaJCIntervalo(db *sql.DB, de, ate time.Time, pularExistentes bool) {
	empresaID := strings.TrimSpace(os.Getenv("JC_EMPRESA_ID"))
	inicio := time.Now()

	var resultados []*ResultadoExtracao
	var pulados []time.Time
	meses := map[aggMesYM]struct{}{}

	for dia := de; !dia.After(ate); dia = dia.AddDate(0, 0, 1) {
		if pularExistentes && empresaID != "" && diaJaTemDados(db, empresaID, dia) {
			log.Printf("[jc:carga] %s já tem dados — pulado", dia.Format("2006-01-02"))
			pulados = append(pulados, dia)
			continue
		}
		// skipRefresh=true: a reconsolidação sai do laço e roda UMA vez no fim.
		// Sem isso o backfill é quadrático — `upsert_aggs_mes` recalcula o MÊS
		// INTEIRO a cada dia importado, então o custo sobe conforme os dias se
		// acumulam. Medido em 29/07: 2m23 no dia 01, 3m06 no 02, 3m59 no 03, e
		// o total do dia já em 7m43 no 04. Extrapolando daria mais de 4h para o
		// mês; pulando, cada dia fica nos ~2,5min da extração+import.
		//
		// É seguro porque upsert_aggs_mes recomputa o mês inteiro de qualquer
		// forma — fazer 29 vezes o mesmo trabalho não produz resultado melhor
		// que fazer uma vez no fim.
		res := executarCargaJCSemEmail(db, dia, true)
		resultados = append(resultados, res)
		if res.Erro == nil && res.StatusImport == "done" {
			meses[aggMesYM{Ano: dia.Year(), Mes: int(dia.Month())}] = struct{}{}
		}
	}

	// ── Consolidação única ──────────────────────────────────────────────────
	if len(meses) > 0 && empresaID != "" {
		lista := make([]aggMesYM, 0, len(meses))
		for m := range meses {
			lista = append(lista, m)
		}
		t0 := time.Now()
		log.Printf("[jc:carga] consolidando %d mês(es) ao final do intervalo", len(lista))

		for _, mv := range []string{"farol.mv_fat_carteira_rca", "farol.mv_trans_carteira_rca"} {
			if _, err := db.Exec(`REFRESH MATERIALIZED VIEW CONCURRENTLY ` + mv); err != nil {
				if _, err2 := db.Exec(`REFRESH MATERIALIZED VIEW ` + mv); err2 != nil {
					log.Printf("[jc:carga] REFRESH %s ERRO: %v", mv, err2)
				}
			}
			db.Exec(`ANALYZE ` + mv)
		}
		upsertAggsMesParallel(db, empresaID, lista, 4)

		// Invalida o cache DEPOIS da consolidação: invalidar antes deixaria a
		// janela em que uma request repovoaria o cache com agregado velho.
		for _, m := range lista {
			ym := m.Ano*100 + m.Mes
			invalidateBaseCacheMeses(empresaID, ym, ym)
			invalidateVendasPeriodoCacheMeses(empresaID, ym, ym)
		}
		log.Printf("[jc:carga] consolidação final concluída em %v", time.Since(t0).Round(time.Second))
	}

	enviarResumoIntervaloJC(de, ate, inicio, resultados, pulados)
}

// corpoResumoIntervaloJC — uma tabela dia a dia. O veredito consolidado vem na
// primeira linha pelo mesmo motivo do resumo diário: notificação de celular
// mostra só o começo.
func corpoResumoIntervaloJC(de, ate, inicio time.Time, res []*ResultadoExtracao, pulados []time.Time) (string, string) {
	loc := tzBrasil()
	fim := time.Now()

	var okN, falhaN, vazioN, totalLinhas int
	for _, r := range res {
		switch {
		case r.Erro != nil:
			falhaN++
		case r.StatusImport == "sem_dados":
			vazioN++
		default:
			okN++
			totalLinhas += r.LinhasImportad
		}
	}

	veredito := fmt.Sprintf("%d OK", okN)
	if falhaN > 0 {
		veredito += fmt.Sprintf(", %d FALHARAM", falhaN)
	}
	if vazioN > 0 {
		veredito += fmt.Sprintf(", %d sem dados", vazioN)
	}

	assunto := fmt.Sprintf("[FAROL] Carga JC %s a %s — %s",
		de.Format("02/01"), ate.Format("02/01/2006"), veredito)

	var b strings.Builder
	fmt.Fprintf(&b, "Carga automática do Farol (intervalo) — %s\n\n", veredito)
	fmt.Fprintf(&b, "Período   : %s a %s\n", de.Format("02/01/2006"), ate.Format("02/01/2006"))
	fmt.Fprintf(&b, "Início    : %s\n", inicio.In(loc).Format("02/01/2006 15:04:05"))
	fmt.Fprintf(&b, "Conclusão : %s\n", fim.In(loc).Format("02/01/2006 15:04:05"))
	fmt.Fprintf(&b, "Duração   : %s\n", fim.Sub(inicio).Round(time.Second))
	fmt.Fprintf(&b, "Importado : %s linhas\n\n", milhar(totalLinhas))

	if len(pulados) > 0 {
		fmt.Fprintf(&b, "%d dia(s) pulado(s) por já terem dados.\n\n", len(pulados))
	}

	b.WriteString("Dia         Situação      Linhas       Tempo\n")
	b.WriteString("----------  ------------  -----------  --------\n")
	for _, r := range res {
		situacao := "OK"
		switch {
		case r.Erro != nil:
			situacao = "FALHOU"
		case r.StatusImport == "sem_dados":
			situacao = "sem dados"
		}
		fmt.Fprintf(&b, "%-10s  %-12s  %11s  %8s\n",
			r.DataRef.Format("02/01/2026"), situacao,
			milhar(r.LinhasImportad), r.Duracao().Round(time.Second))
	}

	// Erros detalhados no fim: quem só quer saber se deu certo lê o topo; quem
	// precisa agir lê aqui.
	if falhaN > 0 {
		b.WriteString("\n--- erros ---\n")
		for _, r := range res {
			if r.Erro != nil {
				fmt.Fprintf(&b, "%s: %s\n", r.DataRef.Format("02/01/2026"), r.Erro.Error())
			}
		}
		b.WriteString("\nDias com falha podem ser reprocessados individualmente:\n")
		b.WriteString("POST /api/v2/jc/carga?data=AAAA-MM-DD\n")
	}

	b.WriteString("\nMensagem automática do FB_FAROL.\n")
	return assunto, b.String()
}

func enviarResumoIntervaloJC(de, ate, inicio time.Time, res []*ResultadoExtracao, pulados []time.Time) {
	assunto, corpo := corpoResumoIntervaloJC(de, ate, inicio, res, pulados)
	para := destinatariosJC()
	if err := services.SendPlainReport(para, assunto, corpo); err != nil {
		log.Printf("[jc:carga] FALHA ao enviar resumo do intervalo para %v: %v", para, err)
		return
	}
	log.Printf("[jc:carga] resumo do intervalo enviado para %s", strings.Join(para, ", "))
}

func destinatariosJC() []string {
	raw := strings.TrimSpace(os.Getenv("JC_EXTRACAO_EMAILS"))
	if raw == "" {
		raw = jcDestinatariosPadrao
	}
	campos := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\n' || r == '\r' || r == '\t'
	})
	var out []string
	for _, e := range campos {
		if e = strings.TrimSpace(e); e != "" {
			out = append(out, e)
		}
	}
	return out
}

// corpoResumoJC — texto puro, curto e com o veredito na PRIMEIRA linha. Quem lê
// pela notificação do celular vê só o começo, então "OK"/"FALHOU" tem que vir
// antes de qualquer detalhe.
func corpoResumoJC(res *ResultadoExtracao) (assunto, corpo string) {
	loc := tzBrasil()
	dia := res.DataRef.Format("02/01/2006")

	veredito := "OK"
	switch {
	case res.Erro != nil:
		veredito = "FALHOU"
	case res.StatusImport == "sem_dados":
		veredito = "SEM DADOS"
	}
	assunto = fmt.Sprintf("[FAROL] Carga JC %s — %s", dia, veredito)

	var b strings.Builder
	fmt.Fprintf(&b, "Carga automática do Farol — %s\n\n", veredito)
	fmt.Fprintf(&b, "Dia de referência : %s\n", dia)
	fmt.Fprintf(&b, "Início            : %s\n", res.Inicio.In(loc).Format("02/01/2006 15:04:05"))
	fmt.Fprintf(&b, "Conclusão         : %s\n", res.Fim.In(loc).Format("02/01/2006 15:04:05"))
	fmt.Fprintf(&b, "Duração total     : %s\n\n", res.Duracao().Round(time.Second))

	if res.Erro != nil {
		fmt.Fprintf(&b, "ERRO\n%s\n\n", res.Erro.Error())
	}

	if res.StatusImport == "sem_dados" {
		b.WriteString("Nenhuma linha encontrada para este dia.\n")
		b.WriteString("Pode ser dia sem movimento, ou a carga da origem ainda\n")
		b.WriteString("não ter rodado no horário em que consultamos.\n\n")
	}

	fmt.Fprintf(&b, "Linhas lidas na origem : %s\n", milhar(res.LinhasLidas))
	if res.LinhasImportad > 0 || res.StatusImport == "done" {
		fmt.Fprintf(&b, "Linhas importadas      : %s\n", milhar(res.LinhasImportad))
	}
	if len(res.PorEstado) > 0 {
		b.WriteString("\nPor tipo de movimento:\n")
		for _, e := range []string{"FATURADO", "TRANSMITIDO", "DEVOLVIDO", "CANCELADO", "CORTADO"} {
			if n, ok := res.PorEstado[e]; ok {
				fmt.Fprintf(&b, "  %-12s %s\n", e, milhar(n))
			}
		}
		// Qualquer ESTADO fora da lista conhecida aparece — se a origem criar um
		// tipo novo, é melhor descobrir pelo e-mail que por número errado na tela.
		for e, n := range res.PorEstado {
			switch e {
			case "FATURADO", "TRANSMITIDO", "DEVOLVIDO", "CANCELADO", "CORTADO":
			default:
				fmt.Fprintf(&b, "  %-12s %s  <-- tipo não previsto\n", e, milhar(n))
			}
		}
	}

	b.WriteString("\n--- detalhes técnicos ---\n")
	fmt.Fprintf(&b, "Consulta no Oracle : %s\n", res.DuracaoQuery.Round(time.Millisecond))
	if res.DuracaoImport > 0 {
		fmt.Fprintf(&b, "Importação         : %s\n", res.DuracaoImport.Round(time.Second))
	}
	if res.BytesCSV > 0 {
		fmt.Fprintf(&b, "Arquivo gerado     : %s (gzip)\n", tamanhoLegivel(res.BytesCSV))
	}
	if res.JobID != "" {
		fmt.Fprintf(&b, "Job de importação  : %s (status %s)\n", res.JobID, res.StatusImport)
	}
	b.WriteString("\nMensagem automática do FB_FAROL.\n")
	return assunto, b.String()
}

func enviarResumoJC(res *ResultadoExtracao) {
	assunto, corpo := corpoResumoJC(res)
	para := destinatariosJC()
	if err := services.SendPlainReport(para, assunto, corpo); err != nil {
		// Falha de e-mail não pode derrubar a carga — mas tem que ficar no log,
		// senão vira dois problemas invisíveis em vez de um.
		log.Printf("[jc:carga] FALHA ao enviar resumo para %v: %v", para, err)
		return
	}
	log.Printf("[jc:carga] resumo enviado para %s", strings.Join(para, ", "))
}

func milhar(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, '.')
		}
		out = append(out, c)
	}
	return string(out)
}
