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

// ExecutarCargaJC roda o ciclo completo para uma data e devolve o resultado.
// Nunca entra em pânico: qualquer falha vira res.Erro e VAI para o e-mail —
// silêncio é o pior desfecho possível numa carga automática, porque ninguém
// descobre que o painel parou de atualizar até alguém estranhar um número.
func ExecutarCargaJC(db *sql.DB, dataRef time.Time) *ResultadoExtracao {
	res := &ResultadoExtracao{DataRef: dataRef, Inicio: time.Now()}
	defer func() {
		res.Fim = time.Now()
		if r := recover(); r != nil {
			res.Erro = fmt.Errorf("pânico durante a carga: %v", r)
		}
		enviarResumoJC(res)
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
		dataRef.Year(), int(dataRef.Month()), false)
	res.DuracaoImport = time.Since(tImp)

	// Status final vem do banco, não do que achamos que aconteceu.
	var status string
	var processadas sql.NullInt64
	var erroMsg sql.NullString
	if err := db.QueryRow(`
		SELECT status, processed_lines, error_message
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

	log.Printf("[jc:carga] dia=%s CONCLUÍDO status=%s lidas=%d importadas=%d em %v",
		dataRef.Format("2006-01-02"), status, res.LinhasLidas, res.LinhasImportad, res.Duracao())
	return res
}

func destinatariosJC() []string {
	raw := strings.TrimSpace(os.Getenv("JC_EXTRACAO_EMAILS"))
	if raw == "" {
		raw = jcDestinatariosPadrao
	}
	var out []string
	for _, e := range strings.Split(raw, ",") {
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
