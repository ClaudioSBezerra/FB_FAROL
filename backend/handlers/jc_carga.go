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
	return executarCargaJCPeriodo(db, dataRef, dataRef, pularConsolidacao)
}

// executarCargaJCPeriodo processa o intervalo [de..ate] como UM arquivo. Com
// de==ate é a carga de um dia; com um mês é a fatia do backfill.
func executarCargaJCPeriodo(db *sql.DB, de, ate time.Time, pularConsolidacao bool) *ResultadoExtracao {
	res := &ResultadoExtracao{DataRef: de, DataFim: ate, Inicio: time.Now()}
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

	log.Printf("[jc:carga] iniciando %s empresa=%s", res.Rotulo(), empresaID)

	// Timeout generoso: um mês inteiro chega a ~2,7M linhas, e o import de 6M
	// linhas já levou dezenas de minutos no histórico de jobs.
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Hour)
	defer cancel()

	arquivo, err := ExtrairPeriodoJC(ctx, de, ate, res)
	if err != nil {
		res.Erro = err
		log.Printf("[jc:carga] extração FALHOU: %v", err)
		return res
	}

	// Zero linhas NÃO é sucesso silencioso. Pode ser período sem movimento
	// (domingo/feriado) ou pode ser o JOB deles não ter rodado ainda — e as duas
	// situações precisam chegar diferentes de "importei tudo certo". Não
	// chamamos o import (não faz sentido apagar o período e recarregar vazio).
	if res.LinhasLidas == 0 {
		os.Remove(arquivo)
		res.StatusImport = "sem_dados"
		log.Printf("[jc:carga] %s veio VAZIO — import não executado", res.Rotulo())
		return res
	}

	// Job na mesma tabela do upload manual: a tela de histórico de importações
	// passa a mostrar a carga automática junto com as manuais, sem tela nova.
	var jobID string
	if err := db.QueryRow(`
		INSERT INTO vendas_import_jobs (empresa_id, ano, mes, status, total_lines)
		VALUES ($1, $2, $3, 'pending', $4) RETURNING id`,
		empresaID, de.Year(), int(de.Month()), res.LinhasLidas,
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
	// ano/mes aqui são só FALLBACK para linha sem data válida no CSV — a fonte
	// da verdade é a coluna DATA de cada linha. Passar o início da fatia basta.
	processImportJob(impCtx, db, jobID, arquivo, true, spCtx,
		de.Year(), int(de.Month()), pularConsolidacao)
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
	log.Printf("[jc:carga] %s CONCLUÍDO status=%s lidas=%d importadas=%d em %v",
		res.Rotulo(), status, res.LinhasLidas, res.LinhasImportad,
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

// jcMaxMesesIntervalo — teto do modo mensal. 24 meses cobrem a recarga de 2025
// até hoje com folga; cada mês custa ~10min, então 24 são ~4h.
const jcMaxMesesIntervalo = 24

// fatiaPeriodo — um pedaço do backfill, processado como UM arquivo.
type fatiaPeriodo struct{ de, ate time.Time }

func primeiroDoMes(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

// fatiarPeriodo divide [de..ate] em pedaços de um dia ou de um mês.
//
// O modo mensal existe porque a consulta ao Oracle custa ~1 MINUTO FIXO,
// independente do volume (medido em 29/07: 694 linhas levaram os mesmos 59s que
// 154 mil). Puxar mês a mês paga esse minuto 19 vezes em vez de 576 num
// backfill de 2025 até hoje — ~3h contra ~29h.
//
// As pontas são respeitadas: um mês parcial na borda vira fatia parcial, nunca
// puxa dado fora do intervalo pedido.
func fatiarPeriodo(de, ate time.Time, porMes bool) []fatiaPeriodo {
	var out []fatiaPeriodo
	if !porMes {
		for d := de; !d.After(ate); d = d.AddDate(0, 0, 1) {
			out = append(out, fatiaPeriodo{d, d})
		}
		return out
	}
	for ini := de; !ini.After(ate); {
		fimMes := primeiroDoMes(ini).AddDate(0, 1, -1) // último dia do mês de `ini`
		if fimMes.After(ate) {
			fimMes = ate
		}
		out = append(out, fatiaPeriodo{ini, fimMes})
		ini = fimMes.AddDate(0, 0, 1)
	}
	return out
}

// fatiaJaTemDados — usado pelo modo "pular existentes". Olha as três tabelas de
// destino: basta uma ter linha no intervalo para a fatia contar como carregada.
//
// ⚠ Numa fatia MENSAL isso é grosseiro de propósito: um único dia carregado faz
// o mês inteiro ser pulado. É o comportamento certo para retomar um backfill
// interrompido (não refaz o que já passou), mas NÃO serve para tapar buraco no
// meio de um mês — nesse caso, rodar sem `pular_existentes` ou com `passo=dia`.
func fatiaJaTemDados(db *sql.DB, empresaID string, de, ate time.Time) bool {
	var existe bool
	err := db.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM vendas_faturadas
			 WHERE empresa_id=$1 AND data_faturamento BETWEEN $2 AND $3
			UNION ALL
			SELECT 1 FROM vendas_transmitidas
			 WHERE empresa_id=$1 AND data_transmissao BETWEEN $2 AND $3
			UNION ALL
			SELECT 1 FROM vendas_ccd
			 WHERE empresa_id=$1 AND data_evento BETWEEN $2 AND $3
		)`, empresaID, de, ate).Scan(&existe)
	if err != nil {
		// Na dúvida, NÃO pula: recarregar é barato e idempotente; deixar buraco
		// no histórico não é.
		log.Printf("[jc:carga] checagem de período existente falhou (%v) — vai recarregar %s..%s",
			err, de.Format("2006-01-02"), ate.Format("2006-01-02"))
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
func ExecutarCargaJCIntervalo(db *sql.DB, de, ate time.Time, pularExistentes bool, porMes bool) {
	empresaID := strings.TrimSpace(os.Getenv("JC_EMPRESA_ID"))
	inicio := time.Now()

	var resultados []*ResultadoExtracao
	var pulados []time.Time
	meses := map[aggMesYM]struct{}{}

	for _, fatia := range fatiarPeriodo(de, ate, porMes) {
		if pularExistentes && empresaID != "" && fatiaJaTemDados(db, empresaID, fatia.de, fatia.ate) {
			log.Printf("[jc:carga] %s já tem dados — pulado",
				(&ResultadoExtracao{DataRef: fatia.de, DataFim: fatia.ate}).Rotulo())
			pulados = append(pulados, fatia.de)
			continue
		}
		// skipRefresh=true: a reconsolidação sai do laço e roda UMA vez no fim.
		// Sem isso o backfill é quadrático — `upsert_aggs_mes` recalcula o MÊS
		// INTEIRO a cada import, então o custo sobe conforme os dias se acumulam.
		// Medido em 29/07: 2m23 no dia 01, 3m06 no 02, 3m59 no 03, e o total do
		// dia já em 7m43 no 04. Extrapolando daria mais de 4h para o mês.
		//
		// É seguro porque upsert_aggs_mes recomputa o mês inteiro de qualquer
		// forma — repetir o mesmo trabalho a cada fatia não produz resultado
		// melhor que fazer uma vez no fim.
		res := executarCargaJCPeriodo(db, fatia.de, fatia.ate, true)
		resultados = append(resultados, res)
		if res.Erro == nil && res.StatusImport == "done" {
			// Uma fatia mensal cai num mês só, mas o range livre pode cruzar —
			// marca todos os meses que ela toca.
			for m := primeiroDoMes(fatia.de); !m.After(fatia.ate); m = m.AddDate(0, 1, 0) {
				meses[aggMesYM{Ano: m.Year(), Mes: int(m.Month())}] = struct{}{}
			}
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

		// mv_fat_uf_mes — alimenta o filtro de UF. No import normal ela é
		// refeita pelo processImportJob; com skipRefresh=true isso NÃO acontece,
		// e eu havia esquecido de chamá-la aqui. Resultado do backfill de 06/08:
		// 19 meses de agregado prontos e a MV de UF vazia, deixando o filtro de
		// UF sem opção nenhuma no painel.
		refreshUFMV(db)

		// Invalida o cache DEPOIS da consolidação: invalidar antes deixaria a
		// janela em que uma request repovoaria o cache com agregado velho.
		for _, m := range lista {
			ym := m.Ano*100 + m.Mes
			invalidateBaseCacheMeses(empresaID, ym, ym)
			invalidateVendasPeriodoCacheMeses(empresaID, ym, ym)
			invalidateAggMesCacheMeses(empresaID, ym, ym)
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

	// Rotulo() em vez de Format: numa fatia mensal a linha precisa mostrar as
	// duas pontas, senão "01/07" daria a entender que só um dia foi importado.
	b.WriteString("Período                  Situação      Linhas       Tempo\n")
	b.WriteString("-----------------------  ------------  -----------  --------\n")
	for _, r := range res {
		situacao := "OK"
		switch {
		case r.Erro != nil:
			situacao = "FALHOU"
		case r.StatusImport == "sem_dados":
			situacao = "sem dados"
		}
		fmt.Fprintf(&b, "%-23s  %-12s  %11s  %8s\n",
			r.Rotulo(), situacao,
			milhar(r.LinhasImportad), r.Duracao().Round(time.Second))
	}

	// Erros detalhados no fim: quem só quer saber se deu certo lê o topo; quem
	// precisa agir lê aqui.
	if falhaN > 0 {
		b.WriteString("\n--- erros ---\n")
		for _, r := range res {
			if r.Erro != nil {
				fmt.Fprintf(&b, "%s: %s\n", r.Rotulo(), r.Erro.Error())
			}
		}
		b.WriteString("\nPeríodos com falha podem ser reprocessados:\n")
		b.WriteString("POST /api/v2/jc/carga?de=AAAA-MM-DD&ate=AAAA-MM-DD\n")
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
