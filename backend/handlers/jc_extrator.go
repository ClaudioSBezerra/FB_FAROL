// jc_extrator.go — carga automática diária a partir do Oracle do grupo JC.
//
// MODELO PULL: nós conectamos no Oracle deles (banco paralelo 26ai, rede
// liberada por allowlist do nosso IP) e lemos a view consolidada. Não há agente
// rodando lá, não há VPN, e não há POST de entrada — por isso a "Fase 1" do
// plano original (tabela api_tokens + auth Bearer) saiu do escopo: quem conecta
// somos nós e a escrita é interna.
//
// FLUXO: SELECT das 40 colunas do dia → CSV gzip em disco → processImportJob
// (mesmo caminho do upload manual, já idempotente: apaga o dia antes do COPY) →
// e-mail de resumo.
//
// Env:
//
//	JC_ORACLE_HOST     default 201.48.119.197 (o de menor jitter dos dois liberados)
//	JC_ORACLE_PORT     default 1521
//	JC_ORACLE_SERVICE  default cdb1  (é o CDB root; PDB1 existe mas não tem o usuário)
//	JC_ORACLE_USER     obrigatório
//	JC_ORACLE_PASS     obrigatório
//	JC_ORACLE_OBJETO   default IAUSER.COMPRAS_FAROL_VW (sinônimo → IAADMIN.*)
//	JC_EMPRESA_ID      obrigatório — empresa do FAROL que recebe a carga
//	JC_EXTRACAO_HORA   default 06:30 (HH:MM, horário de Brasília)
//	JC_EXTRACAO_EMAILS default os dois destinatários combinados, separados por vírgula
package handlers

import (
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	go_ora "github.com/sijms/go-ora/v2"
)

// colunasJC — as 40 colunas da view, na ordem exata que o Keslley montou.
// Conferidas uma a uma contra o mapeamento do importador (farol_v2_import.go):
// batem todas. Vão explicitadas no SELECT (em vez de `SELECT *`) para que a
// ordem do CSV não dependa da definição da view — se ele reordenar lá, aqui
// continua igual.
var colunasJC = []string{
	"DATA", "PERIODO", "ESTADO",
	"CODGERENTE", "GERENTE",
	"CODSUPERVISOR", "SUPERVISOR", "QTRCA_SUPERVISOR",
	"CODUSUR", "RCA", "QTCLI_RCA",
	"CODFORNEC", "FORNECEDOR",
	"CODEPTO", "DEPARTAMENTO",
	"CODSEC", "SECAO",
	"CODCATEGORIA", "CATEGORIA",
	"CODCLIPRINC", "CODCLI", "CLIENTE", "FANTASIA",
	"CODRAMO", "RAMO", "CNPJ", "UF", "EMPRESA",
	"CODPROD", "PRODUTO", "EAN", "EMBALAGEM",
	"QTUNIT", "QTUNITCX", "QT",
	"PVENDA", "PVENDA_TOTAL", "PLUCRO",
	"CONDVENDA", "DESC_CONDVENDA",
}

// delimitadorCSVJC — PONTO-E-VÍRGULA, não vírgula.
//
// O importador lê com `csvReader.Comma = ';'` (farol_v2_import.go:297), padrão
// brasileiro que o ION VENDAS gera. Na 1ª versão usei o default do
// encoding/csv (vírgula) e o resultado foi silencioso e total: cada linha virou
// um campo único, o mapeamento de cabeçalho não achou coluna nenhuma, e as
// 152.599 linhas extraídas foram rejeitadas com "nenhuma linha válida
// encontrada no arquivo". A extração parecia perfeita nos logs.
const delimitadorCSVJC = ';'

// novoEscritorCSVJC centraliza a configuração do CSV para que ela seja testável
// sem precisar do Oracle — é o que faltava para pegar o erro acima no build.
func novoEscritorCSVJC(w io.Writer) *csv.Writer {
	c := csv.NewWriter(w)
	c.Comma = delimitadorCSVJC
	return c
}

// ResultadoExtracao — tudo que o e-mail de resumo precisa relatar.
type ResultadoExtracao struct {
	DataRef time.Time
	// DataFim — última data do intervalo (inclusiva). Igual a DataRef na carga
	// diária; no backfill mensal marca o fim da fatia.
	DataFim        time.Time
	Inicio         time.Time
	Fim            time.Time
	LinhasLidas    int
	PorEstado      map[string]int
	BytesCSV       int64
	DuracaoQuery   time.Duration
	DuracaoImport  time.Duration
	JobID          string
	StatusImport   string
	LinhasImportad int
	Erro           error
}

// Rotulo — como o período aparece em log e e-mail. Um dia mostra a data; um
// intervalo mostra as duas pontas, para não dar a impressão de que a fatia
// inteira é de um dia só.
func (r *ResultadoExtracao) Rotulo() string {
	if r.DataFim.IsZero() || r.DataFim.Equal(r.DataRef) {
		return r.DataRef.Format("02/01/2006")
	}
	return r.DataRef.Format("02/01/2006") + " a " + r.DataFim.Format("02/01/2006")
}

// Duracao só é válida depois que o Fim foi preenchido (no defer de
// ExecutarCargaJC). Chamada antes disso, subtrairia de um time.Time zerado e
// estouraria o int64 — foi o que produziu "-2562047h47m16s" no log da 1ª carga
// bem-sucedida. O fallback protege quem chamar cedo demais.
func (r *ResultadoExtracao) Duracao() time.Duration {
	if r.Fim.IsZero() {
		return time.Since(r.Inicio)
	}
	return r.Fim.Sub(r.Inicio)
}

func envJC(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

// dsnJC monta a URL de conexão. go-ora v2 — a v3 NÃO conecta neste servidor
// (morre na negociação de protocolo antes da autenticação, testado em 29/07).
func dsnJC() (string, error) {
	user := envJC("JC_ORACLE_USER", "")
	pass := envJC("JC_ORACLE_PASS", "")
	if user == "" || pass == "" {
		return "", fmt.Errorf("JC_ORACLE_USER/JC_ORACLE_PASS não configurados")
	}
	porta, err := strconv.Atoi(envJC("JC_ORACLE_PORT", "1521"))
	if err != nil {
		porta = 1521
	}
	return go_ora.BuildUrl(
		envJC("JC_ORACLE_HOST", "201.48.119.197"), porta,
		envJC("JC_ORACLE_SERVICE", "cdb1"), user, pass, nil), nil
}

// valorCSV converte o que o driver devolveu para o texto que o importador espera.
//
// O importador tolera bastante (parseNum aceita ponto OU vírgula decimal;
// parseDateBR aceita dd/mm/aaaa e aaaa-mm-dd), então o objetivo aqui é só não
// introduzir ruído: nada de notação científica em float, nada de timestamp com
// hora numa coluna de data.
func valorCSV(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case time.Time:
		return t.Format("2006-01-02")
	case []byte:
		return string(t)
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64) // 'f' evita 1.234e+06
	case float32:
		return strconv.FormatFloat(float64(t), 'f', -1, 64)
	case int64:
		return strconv.FormatInt(t, 10)
	case bool:
		if t {
			return "1"
		}
		return "0"
	default:
		return fmt.Sprint(t)
	}
}

// ExtrairDiaJC — atalho para um dia só (carga diária).
func ExtrairDiaJC(ctx context.Context, dataRef time.Time, res *ResultadoExtracao) (string, error) {
	return ExtrairPeriodoJC(ctx, dataRef, dataRef, res)
}

// ExtrairPeriodoJC lê o intervalo [de..ate] (INCLUSIVO nas duas pontas) numa
// ÚNICA consulta e grava o CSV gzip em disco. Devolve o caminho.
//
// SEMPRE filtra por DATA. Sem o predicado a view não responde — ela é view
// COMUM sobre tabelas de 28M+20M linhas, e o otimizador monta o join inteiro
// antes da primeira linha (medido: estourou 20s até para ROWNUM<=1).
//
// POR QUE ACEITAR INTERVALO: a consulta custa ~1 MINUTO FIXO, independente do
// volume — medido em 29/07, o dia 19/07 com 694 linhas levou os mesmos 59s que
// o dia 14 com 154 mil. Esse minuto é o join se montando, não transferência de
// dado. Puxar um mês de uma vez paga o minuto UMA vez em lugar de 30. Num
// backfill de 2025 até hoje (576 dias) a diferença é ~29h contra ~3h.
//
// O importador aceita arquivo com vários dias: agrupa as datas presentes e faz
// o DELETE prévio de cada uma, então a idempotência continua valendo.
func ExtrairPeriodoJC(ctx context.Context, de, ate time.Time, res *ResultadoExtracao) (string, error) {
	dsn, err := dsnJC()
	if err != nil {
		return "", err
	}
	objeto := envJC("JC_ORACLE_OBJETO", "IAUSER.COMPRAS_FAROL_VW")

	conn, err := sql.Open("oracle", dsn)
	if err != nil {
		return "", fmt.Errorf("abrir conexão Oracle: %w", err)
	}
	defer conn.Close()

	pingCtx, cancelPing := context.WithTimeout(ctx, 30*time.Second)
	defer cancelPing()
	if err := conn.PingContext(pingCtx); err != nil {
		return "", fmt.Errorf("conectar no Oracle da JC: %w", err)
	}

	ini := time.Date(de.Year(), de.Month(), de.Day(), 0, 0, 0, 0, time.UTC)
	// `ate` é inclusivo para quem chama; no SQL vira `< ate+1dia` para não
	// depender de a coluna DATA ter ou não componente de hora.
	fim := time.Date(ate.Year(), ate.Month(), ate.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1)

	q := fmt.Sprintf("SELECT %s FROM %s WHERE DATA >= :1 AND DATA < :2",
		strings.Join(colunasJC, ", "), objeto)

	t0 := time.Now()
	rows, err := conn.QueryContext(ctx, q, ini, fim)
	if err != nil {
		return "", fmt.Errorf("consultar %s: %w", objeto, err)
	}
	defer rows.Close()
	res.DuracaoQuery = time.Since(t0)

	uploadsDir := strings.TrimSpace(os.Getenv("IMPORT_UPLOAD_DIR"))
	if uploadsDir == "" {
		uploadsDir = filepath.Join(os.TempDir(), "farol-imports")
	}
	if err := os.MkdirAll(uploadsDir, 0755); err != nil {
		return "", fmt.Errorf("criar dir de uploads: %w", err)
	}
	// Nome carrega o intervalo: num backfill vários arquivos coexistem no
	// diretório enquanto são processados, e "jc-2026-07-01.csv.gz" para um mês
	// inteiro seria enganoso se alguém for investigar um import.
	rotulo := ini.Format("2006-01-02")
	if !fim.AddDate(0, 0, -1).Equal(ini) {
		rotulo += "_a_" + fim.AddDate(0, 0, -1).Format("2006-01-02")
	}
	arquivo := filepath.Join(uploadsDir, fmt.Sprintf("jc-%s.csv.gz", rotulo))
	f, err := os.Create(arquivo)
	if err != nil {
		return "", fmt.Errorf("criar CSV: %w", err)
	}

	gz := gzip.NewWriter(f)
	w := novoEscritorCSVJC(gz)

	// Fecha na ordem inversa e propaga erro — CSV truncado por falha de escrita
	// seria importado como "dia com menos venda", que é pior que erro explícito.
	fecharTudo := func() error {
		w.Flush()
		if err := w.Error(); err != nil {
			gz.Close()
			f.Close()
			return err
		}
		if err := gz.Close(); err != nil {
			f.Close()
			return err
		}
		return f.Close()
	}

	if err := w.Write(colunasJC); err != nil {
		fecharTudo()
		os.Remove(arquivo)
		return "", fmt.Errorf("escrever cabeçalho: %w", err)
	}

	res.PorEstado = map[string]int{}
	iEstado := indiceDe(colunasJC, "ESTADO")

	valores := make([]any, len(colunasJC))
	ponteiros := make([]any, len(colunasJC))
	for i := range valores {
		ponteiros[i] = &valores[i]
	}
	linha := make([]string, len(colunasJC))

	for rows.Next() {
		if err := rows.Scan(ponteiros...); err != nil {
			fecharTudo()
			os.Remove(arquivo)
			return "", fmt.Errorf("ler linha %d: %w", res.LinhasLidas+1, err)
		}
		for i, v := range valores {
			linha[i] = valorCSV(v)
		}
		if err := w.Write(linha); err != nil {
			fecharTudo()
			os.Remove(arquivo)
			return "", fmt.Errorf("escrever linha %d: %w", res.LinhasLidas+1, err)
		}
		res.LinhasLidas++
		if iEstado >= 0 {
			res.PorEstado[linha[iEstado]]++
		}
	}
	// rows.Err() DEPOIS do loop: uma conexão que cai no meio termina o Next()
	// silenciosamente, e sem esta checagem gravaríamos um CSV parcial como se
	// fosse o dia inteiro.
	if err := rows.Err(); err != nil {
		fecharTudo()
		os.Remove(arquivo)
		return "", fmt.Errorf("leitura interrompida após %d linhas: %w", res.LinhasLidas, err)
	}
	if err := fecharTudo(); err != nil {
		os.Remove(arquivo)
		return "", fmt.Errorf("finalizar CSV: %w", err)
	}

	if st, err := os.Stat(arquivo); err == nil {
		res.BytesCSV = st.Size()
	}
	log.Printf("[jc:extrator] %s → %d linhas, %s gzip, query em %v",
		rotulo, res.LinhasLidas, tamanhoLegivel(res.BytesCSV), res.DuracaoQuery)
	return arquivo, nil
}

func indiceDe(s []string, alvo string) int {
	for i, v := range s {
		if v == alvo {
			return i
		}
	}
	return -1
}

func tamanhoLegivel(b int64) string {
	switch {
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(b)/(1<<10))
	}
	return fmt.Sprintf("%d B", b)
}
