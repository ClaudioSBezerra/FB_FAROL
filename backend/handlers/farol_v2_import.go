package handlers

// farol_v2_import.go — Importação CSV para vendas_importadas (novo Farol 2026).
//
// Padrão job-based async (mesmo do FB_APU01):
//   POST /api/v2/vendas/import  → lê arquivo, cria job, inicia worker goroutine, retorna {job_id}
//   GET  /api/v2/vendas/job/{id}          → status do job (polling a cada 2s)
//   POST /api/v2/vendas/job/{id}/cancel   → cancela job em andamento
//   GET  /api/v2/vendas/periodos          → lista períodos importados
//   DELETE /api/v2/vendas/clear           → remove período

import (
	"bufio"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/transform"
)

// importJobs guarda a função de cancelamento de cada job ativo.
// Keyed por job UUID; removido ao término (done/error/cancelled).
var importJobs sync.Map // string → context.CancelFunc

// Bucket genérico para hierarquia órfã (RCA/linha sem supervisor ou gerente).
// Código 99999999 (números repetidos) escolhido por não existir como
// vendedor/gerente real na base — agrupa os órfãos sob "NÃO IDENTIFICADO".
const (
	codNaoIdentificado  = "99999999"
	nomeNaoIdentificado = "NÃO IDENTIFICADO"
)

// ─── VendasImportHandler — POST /api/v2/vendas/import ───────────────────────

func VendasImportHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		if !RequireWrite(spCtx, w) {
			return
		}

		// ── Parâmetros ────────────────────────────────────────────────────────
		// ano/mes aceitos por compat (UI antiga envia) mas agora são apenas
		// FALLBACK para linhas sem data válida no CSV. A fonte de verdade é a
		// coluna DATA do CSV + PERIODO (que define a tabela destino).
		// tipo_base removido — comparativa agora é uma propriedade da QUERY
		// (range de datas escolhido), não do dado.
		fallbackAno, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("ano")))
		fallbackMes, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("mes")))
		skipRefreshStr := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("skip_refresh")))
		skipRefresh    := skipRefreshStr == "true" || skipRefreshStr == "1"

		// ── Ler arquivo — Fase B (jul/2026): salva em disco via io.Copy em
		// stream. Antes fazia io.ReadAll (RAM inteira, ~1 GB para CSVs grandes
		// de 400 fornecedores). Agora RAM constante ~4 MB (buffer do io.Copy).
		// Arquivo em disco é apagado no fim do processImportJob (defer os.Remove).
		//
		// Aceita CSV plain OU comprimido gzip: cliente pode subir .csv.gz
		// direto, poupando 5-10× de banda de upload. Detecção por magic bytes
		// (1F 8B), NÃO descomprime aqui — grava .gz no disco (economiza espaço),
		// o worker descomprime on-the-fly ao ler.
		_ = r.ParseMultipartForm(2048 << 20)
		var rawReader io.Reader
		formFile, _, ferr := r.FormFile("file")
		if ferr == nil {
			defer formFile.Close()
			rawReader = formFile
		} else {
			rawReader = r.Body
		}

		// Peek nos 2 primeiros bytes para detectar gzip magic (1F 8B).
		peekReader := bufio.NewReaderSize(rawReader, 4<<20) // 4MB buffer
		head, _ := peekReader.Peek(2)
		isGzip := len(head) == 2 && head[0] == 0x1F && head[1] == 0x8B

		// Diretório de imports temporários. Configurável via IMPORT_UPLOAD_DIR
		// (prod: Coolify mapeia volume; local: cai no TempDir do OS).
		// Cleanup é responsabilidade do processImportJob (defer os.Remove).
		uploadsDir := strings.TrimSpace(os.Getenv("IMPORT_UPLOAD_DIR"))
		if uploadsDir == "" {
			uploadsDir = filepath.Join(os.TempDir(), "farol-imports")
		}
		if mErr := os.MkdirAll(uploadsDir, 0755); mErr != nil {
			http.Error(w, `{"error":"falha ao criar dir uploads: `+mErr.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		suffix := ".csv"
		if isGzip {
			suffix = ".csv.gz"
		}
		tmpFile, cErr := os.CreateTemp(uploadsDir, "upload-*"+suffix)
		if cErr != nil {
			http.Error(w, `{"error":"falha ao criar arquivo temp: `+cErr.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		uploadedBytes, copyErr := io.Copy(tmpFile, peekReader)
		tmpFile.Close()
		if copyErr != nil || uploadedBytes == 0 {
			os.Remove(tmpFile.Name())
			http.Error(w, `{"error":"falha ao gravar arquivo em disco"}`, http.StatusBadRequest)
			return
		}
		uploadedPath := tmpFile.Name()

		log.Printf("[VendasImport] empresa=%s fallback=%d/%d arquivo=%dMB (%s) salvo em %s",
			spCtx.EmpresaID, fallbackAno, fallbackMes,
			int(uploadedBytes)/1024/1024,
			map[bool]string{true: "gzip", false: "plain"}[isGzip],
			filepath.Base(uploadedPath))

		// Estimativa de linhas (para progress %): baseada no tamanho.
		// Média empírica: linha CSV do ION VENDAS tem ~400 bytes. Se gzip,
		// aplica fator 10× (compressão típica). Só serve para o cálculo de
		// % — o total real vem do parse.
		estBytesPerLine := int64(400)
		multiplier := int64(1)
		if isGzip {
			multiplier = 10
		}
		estimatedRows := int((uploadedBytes * multiplier) / estBytesPerLine)

		// ── Cria job no banco ─────────────────────────────────────────────────
		// ano/mes do job representa o "balde de competência" do upload (mantido
		// para a tela de histórico de importações). Não afeta semântica dos dados.
		var jobID string
		if jErr := db.QueryRow(`
			INSERT INTO vendas_import_jobs
				(empresa_id, ano, mes, status, total_lines)
			VALUES ($1, $2, $3, 'pending', $4)
			RETURNING id`,
			spCtx.EmpresaID, fallbackAno, fallbackMes, estimatedRows,
		).Scan(&jobID); jErr != nil {
			os.Remove(uploadedPath)
			http.Error(w, `{"error":"erro ao criar job: `+jErr.Error()+`"}`, http.StatusInternalServerError)
			return
		}

		// ── Inicia worker goroutine ───────────────────────────────────────────
		ctx, cancel := context.WithCancel(context.Background())
		importJobs.Store(jobID, cancel)

		// Worker recebe path + flag gzip. Ele é responsável por abrir, ler em
		// stream, e apagar o arquivo do disco no fim (defer os.Remove).
		go processImportJob(ctx, db, jobID, uploadedPath, isGzip, spCtx, fallbackAno, fallbackMes, skipRefresh)

		// Retorna job_id imediatamente
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"job_id": jobID})
	}
}

// ─── processImportJob — worker goroutine ─────────────────────────────────────

const (
	copyChunkRows  = 100_000 // linhas por chunk de COPY (permite cancelar entre chunks)
	progressUpdate = 2 * time.Second
)

// parseDateBR aceita formatos dd/mm/yyyy, d/m/yyyy ou yyyy-mm-dd. Retorna zero
// time.Time se não conseguir parsear. Importador usa zero como sinal de "sem data".
func parseDateBR(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	// Tenta dd/mm/yyyy primeiro (formato do WinThor)
	for _, layout := range []string{"02/01/2006", "2/1/2006", "02/01/06", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

func processImportJob(ctx context.Context, db *sql.DB, jobID string,
	uploadedPath string, isGzip bool,
	spCtx *FarolContext, fallbackAno, fallbackMes int, skipRefresh bool) {

	// Cleanup do arquivo em disco — sempre roda, sucesso ou falha.
	defer func() {
		if uploadedPath != "" {
			if rmErr := os.Remove(uploadedPath); rmErr != nil {
				log.Printf("[ImportJob:%s] falha ao remover %s: %v", jobID, uploadedPath, rmErr)
			}
		}
	}()

	defer func() {
		importJobs.Delete(jobID)
		if rec := recover(); rec != nil {
			log.Printf("[ImportJob:%s] panic: %v", jobID, rec)
			db.Exec(`UPDATE vendas_import_jobs SET status='error', message=$1, atualizado_em=NOW() WHERE id=$2`,
				fmt.Sprintf("erro interno: %v", rec), jobID)
		}
	}()

	markStatus := func(status, msg string) {
		db.Exec(`UPDATE vendas_import_jobs SET status=$1, message=$2, atualizado_em=NOW() WHERE id=$3`,
			status, msg, jobID)
	}

	markStatus("processing", "")

	// ── Contador atômico de linhas processadas ────────────────────────────────
	var processed atomic.Int64

	// Goroutine de progresso: atualiza DB a cada 2s usando conexão separada do pool
	progCtx, stopProg := context.WithCancel(context.Background())
	defer stopProg()
	go func() {
		ticker := time.NewTicker(progressUpdate)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				n := processed.Load()
				// Lê total_lines do job para calcular %
				var total int64
				db.QueryRow(`SELECT total_lines FROM vendas_import_jobs WHERE id=$1`, jobID).Scan(&total)
				pct := 0
				if total > 0 && n > 0 {
					pct = int(n * 99 / total) // máx 99% até confirmar o commit
				}
				db.Exec(`UPDATE vendas_import_jobs SET importados=$1, progress=$2, atualizado_em=NOW() WHERE id=$3`,
					n, pct, jobID)
			case <-progCtx.Done():
				return
			}
		}
	}()

	// ── Parse CSV em stream (Fase B) ──────────────────────────────────────────
	// Pipeline: arquivo em disco → [gzip decoder se .gz] → bufio → strip BOM
	//   → charset auto-detect (UTF-8 raw | Windows-1252/Latin-1 via transform)
	//   → csv.Reader. RAM constante (~64 KB buffer), nada de rawBytes na heap.
	f, oErr := os.Open(uploadedPath)
	if oErr != nil {
		markStatus("error", "falha ao abrir arquivo temp: "+oErr.Error())
		return
	}
	defer f.Close()

	var streamReader io.Reader = f
	if isGzip {
		gz, gzErr := gzip.NewReader(f)
		if gzErr != nil {
			markStatus("error", "falha ao descomprimir gzip: "+gzErr.Error())
			return
		}
		defer gz.Close()
		streamReader = gz
	}

	bufReader := bufio.NewReaderSize(streamReader, 64<<10)

	// Strip UTF-8 BOM se presente.
	if bom, _ := bufReader.Peek(3); len(bom) == 3 && bom[0] == 0xEF && bom[1] == 0xBB && bom[2] == 0xBF {
		bufReader.Discard(3)
	}

	// Detecção de charset via peek: amostra 32 KB e testa UTF-8.
	// ION VENDAS costuma gerar Windows-1252; se UTF-8 for válido no header, mantém.
	sample, _ := bufReader.Peek(32 << 10)
	var csvSource io.Reader = bufReader
	if !utf8.Valid(sample) {
		csvSource = transform.NewReader(bufReader, charmap.Windows1252.NewDecoder())
		log.Printf("[ImportJob:%s] charset Windows-1252/Latin-1 detectado — convertendo em stream", jobID)
	}

	csvReader := csv.NewReader(csvSource)
	csvReader.Comma = ';'
	csvReader.LazyQuotes = true
	csvReader.TrimLeadingSpace = true
	csvReader.FieldsPerRecord = -1

	headerRow, err := csvReader.Read()
	if err != nil {
		markStatus("error", "falha ao ler cabeçalho CSV")
		return
	}

	norm := func(s string) string {
		s = strings.ToLower(strings.TrimSpace(s))
		s = strings.ReplaceAll(s, " ", "")
		s = strings.ReplaceAll(s, "_", "")
		return s
	}
	colMap := make(map[string]int, len(headerRow))
	for i, h := range headerRow {
		colMap[norm(h)] = i
	}
	col := func(def int, candidates ...string) int {
		for _, c := range candidates {
			if idx, ok := colMap[norm(c)]; ok {
				return idx
			}
		}
		return def
	}

	iCodGerente      := col(-1, "codgerente", "cod_gerente")
	iNomeGerente     := col(-1, "gerente", "nome_gerente")
	iCodSup          := col(-1, "codsupervisor", "cod_supervisor")
	iNomeSup         := col(-1, "supervisor", "nome_supervisor")
	iQtrcaSupervisor := col(-1, "qtrcasupervisor", "qtrca_supervisor")
	iCodRca          := col(-1, "codusur", "cod_rca", "codrca")
	iNomeRca         := col(-1, "rca", "nome_rca")
	iQtcliRca        := col(-1, "qtclirca", "qtcli_rca")
	iCodFornec       := col(-1, "codfornec", "cod_fornec")
	iNomeFornec      := col(-1, "fornecedor", "nome_fornec")
	iCodCli          := col(-1, "codcli", "cod_cli")
	iNomeCli         := col(-1, "cliente", "nome_cli")
	iCNPJ            := col(-1, "cnpj", "cnpj_cli", "cnpj_cliente")
	iUf              := col(-1, "uf")
	iEmpresa         := col(-1, "empresa")
	iCodProd         := col(-1, "codprod", "cod_prod")
	iNomeProd        := col(-1, "produto", "nome_prod")
	iEan             := col(-1, "ean", "codean", "cod_ean")
	// Campos puramente visuais — mig 168 (sem agregação, exibidos nos detalhes)
	iCodRamo         := col(-1, "codramo", "cod_ramo")
	iRamo            := col(-1, "ramo", "nome_ramo")
	iEmbalagem       := col(-1, "embalagem")
	iQtUnit          := col(-1, "qtunit", "qt_unit")
	iQtUnitCx        := col(-1, "qtunitcx", "qt_unit_cx", "qtunitcaixa")
	iCodBar          := col(-1, "codbar", "cod_bar", "codigobar")
	iQt              := col(-1, "qt", "quantidade")
	iPvenda          := col(-1, "pvenda", "valor", "vl_venda")
	iPlucro          := col(-1, "plucro", "lucro", "vl_lucro")
	iPeriodo         := col(-1, "periodo")
	iEstado          := col(-1, "estado")
	// Coluna ÚNICA de data — semântica dada pelo PERIODO/ESTADO:
	//   ESTADO=FATURADO/TRANSMITIDO → linhas vão pra vendas_faturadas/transmitidas
	//   ESTADO=CORTADO/CANCELADO/DEVOLVIDO → linhas vão pra vendas_ccd (mig 182)
	iData            := col(-1, "data", "data_processo", "dataprocesso", "dt", "data_movimento")

	// ── Colunas do NOVO LAYOUT (jul/2026) — opcionais, ficam vazias no CSV antigo.
	//   Departamento/Seção/Categoria: hierarquia do produto (Fase 2 usará no GRID).
	//   CodCliPrinc: cliente principal (rede) — Fase 2 usará em drill "Por Rede".
	//   Fantasia: nome fantasia do cliente (informativo).
	//   PvendaTotal: total já calculado (QT × PVENDA) — usa direto se presente.
	iCodDepto     := col(-1, "codepto", "cod_depto", "coddepto")
	iDepto        := col(-1, "departamento", "depto")
	iCodSec       := col(-1, "codsec", "cod_sec", "codsecao")
	iSecao        := col(-1, "secao", "sec")
	iCodCategoria := col(-1, "codcategoria", "cod_categoria")
	iCategoria    := col(-1, "categoria")
	iCodCliPrinc  := col(-1, "codcliprinc", "cod_cliprinc", "codcliprincipal")
	iFantasia     := col(-1, "fantasia", "nome_fantasia")
	iPvendaTotal  := col(-1, "pvendatotal", "pvenda_total", "valor_total", "vl_total", "pvendatot")

	// ── CONDVENDA (jul/2026) — CÓDIGO do tipo de venda (ex.: 1=Normal,
	// 5=Bonificação, 10=Transferência). Conforme a legenda oficial do ION VENDAS,
	// o novo layout traz DUAS colunas ao final: CONDVENDA (código) e
	// DESC_CONDVENDA (descrição). Gravamos SÓ o código em tipo_venda.
	// Detecção primária por nome (CONDVENDA); fallback por conteúdo (cabeçalho
	// contém "cond" e "venda", MAS não "desc" — para nunca pegar DESC_CONDVENDA).
	// Ausente → -1 → getField devolve "" → tipo_venda='' (compat CSV antigo).
	iTipoVenda := col(-1, "condvenda", "cond_venda", "tipovenda", "tipo_venda", "tipodevenda")
	if iTipoVenda < 0 {
		for i, h := range headerRow {
			n := norm(h)
			if strings.Contains(n, "cond") && strings.Contains(n, "venda") && !strings.Contains(n, "desc") {
				iTipoVenda = i
				break
			}
		}
	}
	// DESC_CONDVENDA — descrição oficial do ERP (ex.: "VENDA PADRAO",
	// "BONIFICACAO SIMPLES"). Usada como RÓTULO do dropdown (fonte da verdade;
	// cobre qualquer código). Ausente → '' → o dropdown cai no tipo_venda_label.
	iDescCondVenda := col(-1, "desccondvenda", "desc_condvenda", "descricaocondvenda")

	// Log dos cabeçalhos detectados para diagnóstico de mapeamento de colunas
	colLabel := func(idx int) string {
		if idx < 0 || idx >= len(headerRow) {
			return fmt.Sprintf("NÃO ENCONTRADO(idx=%d)", idx)
		}
		return fmt.Sprintf("%q (col %d)", headerRow[idx], idx)
	}
	log.Printf("[import:diag] colunas detectadas — qt=%s  pvenda=%s  pvenda_total=%s  plucro=%s  ean=%s  codCli=%s  codFornec=%s  periodo=%s  estado=%s  data=%s  qtrca_supervisor=%s",
		colLabel(iQt), colLabel(iPvenda), colLabel(iPvendaTotal), colLabel(iPlucro), colLabel(iEan),
		colLabel(iCodCli), colLabel(iCodFornec), colLabel(iPeriodo), colLabel(iEstado),
		colLabel(iData), colLabel(iQtrcaSupervisor))
	log.Printf("[import:diag] colunas do novo layout — depto=%s  secao=%s  categoria=%s  cliprinc=%s  fantasia=%s  tipo_venda=%s",
		colLabel(iCodDepto), colLabel(iCodSec), colLabel(iCodCategoria), colLabel(iCodCliPrinc), colLabel(iFantasia), colLabel(iTipoVenda))

	getField := func(row []string, idx int) string {
		if idx < 0 || idx >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[idx])
	}
	// parseNum detecta o separador decimal pelo ÚLTIMO separador presente, então
	// funciona tanto para ponto-decimal (ex.: "31.69" do WinThor) quanto para
	// pt-BR vírgula-decimal (ex.: "1.234.567,89"). NÃO assume formato fixo: tirar
	// o ponto cegamente inflava os valores (100x em 2 casas, 10x em 1 casa, etc.).
	parseNum := func(s string) float64 {
		s = strings.TrimSpace(s)
		if s == "" {
			return 0
		}
		lastDot := strings.LastIndex(s, ".")
		lastComma := strings.LastIndex(s, ",")
		switch {
		case lastComma > lastDot:
			// vírgula é o decimal (pt-BR): ponto é milhar → remove ponto, vírgula→ponto
			s = strings.ReplaceAll(s, ".", "")
			s = strings.ReplaceAll(s, ",", ".")
		case lastDot > lastComma:
			// ponto é o decimal (formato numérico): vírgula seria milhar → remove vírgula
			s = strings.ReplaceAll(s, ",", "")
		}
		v, _ := strconv.ParseFloat(s, 64)
		return v
	}
	parseInt3 := func(s string) int {
		cleaned := strings.Map(func(r rune) rune {
			if r >= '0' && r <= '9' {
				return r
			}
			return -1
		}, strings.TrimSpace(s))
		if cleaned == "" {
			return 0
		}
		v, _ := strconv.Atoi(cleaned)
		return v
	}
	// detectEvento — classifica a linha em um dos 5 eventos possíveis do novo
	// layout (jul/2026). Fallback é FATURADO para compat com CSVs antigos que
	// só tinham as duas opções TRANSMITIDO/FATURADO.
	//   FATURADO/TRANSMITIDO → vão pra vendas_faturadas/vendas_transmitidas
	//   CORTADO/CANCELADO/DEVOLVIDO → vão pra vendas_ccd (mig 182)
	detectEvento := func(periodo, estadoField string) string {
		e := strings.ToUpper(estadoField)
		p := strings.ToUpper(periodo)
		switch {
		case strings.Contains(e, "TRANS") || strings.Contains(p, "TRANS"):
			return "TRANSMITIDO"
		case strings.Contains(e, "CORT"):
			return "CORTADO"
		case strings.Contains(e, "CANCEL"):
			return "CANCELADO"
		case strings.Contains(e, "DEVOL"):
			return "DEVOLVIDO"
		default:
			return "FATURADO"
		}
	}

	// ── Lê todas as linhas do CSV em memória; rota cada linha para o buffer
	// correto conforme o ESTADO:
	//   FATURADO    → vendas_faturadas
	//   TRANSMITIDO → vendas_transmitidas
	//   CORTADO/CANCELADO/DEVOLVIDO → vendas_ccd (novo layout jul/2026)
	//
	// vals layout (38 colunas):
	//   0: empresa_id        1: data
	//   2: cod_gerente       3: nome_gerente
	//   4: cod_supervisor    5: nome_supervisor  6: qtrca_supervisor
	//   7: cod_rca           8: nome_rca         9: qtcli_rca
	//  10: cod_fornec        11: nome_fornec
	//  12: cod_cli           13: nome_cli        14: uf            15: empresa
	//  16: cod_prod          17: nome_prod       18: ean
	//  19: qt                20: pvenda (TOTAL)  21: plucro
	//  22: cnpj
	//  23: cod_ramo          24: ramo            ← visual cliente (mig 168)
	//  25: embalagem         26: qt_unit         27: qt_unit_cx    28: cod_bar ← visual produto (mig 168)
	//   -- NOVO LAYOUT jul/2026 (mig 181) --
	//  29: cod_depto         30: depto
	//  31: cod_sec           32: secao
	//  33: cod_categoria     34: categoria
	//  35: cod_cliprinc      36: fantasia
	//  37: pvenda_unit
	//
	// Campo `evento` é usado só para CCD (identifica qual dos 3 tipos:
	// CORTADO/CANCELADO/DEVOLVIDO); fat/trans deixam vazio.
	type vendaRaw struct {
		vals          [38]any
		evento        string
		tipoVenda     string // CONDVENDA — código (mig 187)
		tipoVendaDesc string // DESC_CONDVENDA — rótulo do ERP (mig 192)
	}
	var allFat   []vendaRaw // → vendas_faturadas
	var allTrans []vendaRaw // → vendas_transmitidas
	var allCCD   []vendaRaw // → vendas_ccd (novo layout jul/2026)
	diagSamples := 0
	skippedNoData := 0
	uniqueFatDates   := make(map[string]struct{})
	uniqueTransDates := make(map[string]struct{})
	uniqueCcdDates   := make(map[string]struct{})
	// Contagem de linhas por (ano,mes) — usada para detectar a COMPETÊNCIA do
	// arquivo pelos DADOS (mês dominante), em vez de confiar no nome do arquivo.
	mesContagem := make(map[[2]int]int)

	for {
		csvRow, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}
		codFornec := getField(csvRow, iCodFornec)
		codRca    := getField(csvRow, iCodRca)
		codCli    := getField(csvRow, iCodCli)
		if codFornec == "" && codRca == "" && codCli == "" {
			continue
		}
		periodo := getField(csvRow, iPeriodo)
		estadoF := getField(csvRow, iEstado)
		evento  := detectEvento(periodo, estadoF)

		// Data única — semântica dada pelo estado/tabela destino.
		dataProc := parseDateBR(getField(csvRow, iData))
		// Fallback: ano/mes da URL com dia=1 (compat com CSV antigo sem datas)
		if dataProc.IsZero() && fallbackAno > 0 && fallbackMes > 0 {
			dataProc = time.Date(fallbackAno, time.Month(fallbackMes), 1, 0, 0, 0, 0, time.UTC)
		}
		if dataProc.IsZero() {
			skippedNoData++
			continue
		}

		rawPvenda      := getField(csvRow, iPvenda)
		rawPvendaTotal := getField(csvRow, iPvendaTotal)
		rawPlucro      := getField(csvRow, iPlucro)
		rawQt          := getField(csvRow, iQt)
		if diagSamples < 5 {
			log.Printf("[import:diag] amostra %d — data=%s evento=%s pvenda_raw=%q→%.4f pvenda_total_raw=%q plucro_raw=%q→%.4f qt_raw=%q→%.4f cli=%s fornec=%s",
				diagSamples+1, dataProc.Format("2006-01-02"), evento,
				rawPvenda, parseNum(rawPvenda), rawPvendaTotal, rawPlucro, parseNum(rawPlucro), rawQt, parseNum(rawQt),
				codCli, codFornec)
			diagSamples++
		}

		// Normaliza hierarquia órfã: linha com RCA mas SEM supervisor (ou gerente)
		// some das views por equipe/gerência — a upsert_aggs_mes filtra
		// cod_supervisor<>'' / cod_gerente<>''. Em vez de perder a venda (e travar
		// o filtro com RCA órfão), atribui um bucket genérico "NÃO IDENTIFICADO"
		// (código 99999999, que não colide com vendedor/gerente real da base).
		codGer  := getField(csvRow, iCodGerente)
		nomeGer := getField(csvRow, iNomeGerente)
		codSup  := getField(csvRow, iCodSup)
		nomeSup := getField(csvRow, iNomeSup)
		nomeRca := getField(csvRow, iNomeRca)
		if codSup == "" {
			codSup, nomeSup = codNaoIdentificado, nomeNaoIdentificado
		}
		if codGer == "" {
			codGer, nomeGer = codNaoIdentificado, nomeNaoIdentificado
		}
		if codRca != "" && nomeRca == "" {
			nomeRca = "RCA " + codRca
		}

		var r vendaRaw
		r.vals[0]  = spCtx.EmpresaID
		r.vals[1]  = dataProc
		r.vals[2]  = codGer
		r.vals[3]  = nomeGer
		r.vals[4]  = codSup
		r.vals[5]  = nomeSup
		r.vals[6]  = parseInt3(getField(csvRow, iQtrcaSupervisor))
		r.vals[7]  = codRca
		r.vals[8]  = nomeRca
		r.vals[9]  = parseInt3(getField(csvRow, iQtcliRca))
		r.vals[10] = codFornec
		r.vals[11] = getField(csvRow, iNomeFornec)
		r.vals[12] = codCli
		r.vals[13] = getField(csvRow, iNomeCli)
		r.vals[14] = getField(csvRow, iUf)
		r.vals[15] = getField(csvRow, iEmpresa)
		r.vals[16] = getField(csvRow, iCodProd)
		r.vals[17] = getField(csvRow, iNomeProd)
		r.vals[18] = getField(csvRow, iEan)
		// pvenda_total: preferência pelo valor vindo do CSV (novo layout jul/2026);
		// fallback para PVENDA × QT (CSV antigo, quando iPvendaTotal=-1 ou vazio).
		// Grava pvenda como TOTAL para compat com todas as agg_*_mes/queries
		// SUM(pvenda). Grava pvenda_unit informativo (usado só em detalhes/futuro).
		//
		// plucro: no novo layout está marcado como "NÃO DEFINIDO"; se vier 0 ou
		// vazio → plucro=0. Se vier valor (CSV antigo com %), calcula como antes
		// (pvendaTotal × pct / 100) para preservar histórico.
		qtVal := parseNum(rawQt)
		pvendaUnit := parseNum(rawPvenda)
		var pvendaTotal float64
		if rawPvendaTotal != "" {
			pvendaTotal = parseNum(rawPvendaTotal)
		} else {
			pvendaTotal = pvendaUnit * qtVal
		}
		plucroPct := parseNum(rawPlucro)
		plucroValor := 0.0
		if plucroPct != 0 {
			plucroValor = pvendaTotal * plucroPct / 100.0
		}
		r.vals[19] = qtVal
		r.vals[20] = pvendaTotal
		r.vals[21] = plucroValor
		r.vals[22] = getField(csvRow, iCNPJ)
		// Campos visuais (mig 168) — só gravamos, não usamos em agregados
		r.vals[23] = getField(csvRow, iCodRamo)
		r.vals[24] = getField(csvRow, iRamo)
		r.vals[25] = getField(csvRow, iEmbalagem)
		r.vals[26] = parseNum(getField(csvRow, iQtUnit))
		r.vals[27] = parseNum(getField(csvRow, iQtUnitCx))
		r.vals[28] = getField(csvRow, iCodBar)
		// Novo layout jul/2026 (mig 181) — hierarquia de produto + rede + fantasia
		r.vals[29] = getField(csvRow, iCodDepto)
		r.vals[30] = getField(csvRow, iDepto)
		r.vals[31] = getField(csvRow, iCodSec)
		r.vals[32] = getField(csvRow, iSecao)
		r.vals[33] = getField(csvRow, iCodCategoria)
		r.vals[34] = getField(csvRow, iCategoria)
		r.vals[35] = getField(csvRow, iCodCliPrinc)
		r.vals[36] = getField(csvRow, iFantasia)
		r.vals[37] = pvendaUnit // preserva o unitário original do CSV (informativo)
		r.evento        = evento
		r.tipoVenda     = getField(csvRow, iTipoVenda)     // CONDVENDA (código); '' se ausente
		r.tipoVendaDesc = getField(csvRow, iDescCondVenda) // DESC_CONDVENDA; '' se ausente

		dKey := dataProc.Format("2006-01-02")
		mesContagem[[2]int{dataProc.Year(), int(dataProc.Month())}]++
		switch evento {
		case "TRANSMITIDO":
			uniqueTransDates[dKey] = struct{}{}
			allTrans = append(allTrans, r)
		case "CORTADO", "CANCELADO", "DEVOLVIDO":
			uniqueCcdDates[dKey] = struct{}{}
			allCCD = append(allCCD, r)
		default: // FATURADO
			uniqueFatDates[dKey] = struct{}{}
			allFat = append(allFat, r)
		}
	}

	// Competência do job = mês DOMINANTE pelos dados (não pelo nome do arquivo).
	// O usuário pode nomear o arquivo de qualquer forma; a verdade é a coluna DATA.
	if len(mesContagem) > 0 {
		bestAno, bestMes, bestN := 0, 0, -1
		for ym, n := range mesContagem {
			// desempate: maior contagem; se empatar, mês mais recente
			if n > bestN || (n == bestN && (ym[0] > bestAno || (ym[0] == bestAno && ym[1] > bestMes))) {
				bestAno, bestMes, bestN = ym[0], ym[1], n
			}
		}
		if bestAno > 0 {
			if _, e := db.Exec(`UPDATE vendas_import_jobs SET ano=$1, mes=$2 WHERE id=$3`,
				bestAno, bestMes, jobID); e != nil {
				log.Printf("[ImportJob:%s] update competência ERRO: %v", jobID, e)
			} else {
				log.Printf("[ImportJob:%s] competência detectada pelos dados: %04d-%02d (%d linhas)",
					jobID, bestAno, bestMes, bestN)
			}
		}
		// P2.1 — marca TODOS os meses tocados como pendentes de consolidação.
		// A RefreshViews (chamada ao fim do lote de imports) consolida só estes.
		for ym := range mesContagem {
			if _, e := db.Exec(`INSERT INTO farol.consolidacao_pendente (empresa_id, ano, mes)
				VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`, spCtx.EmpresaID, ym[0], ym[1]); e != nil {
				log.Printf("[ImportJob:%s] marcar pendente %04d-%02d ERRO: %v", jobID, ym[0], ym[1], e)
			}
		}
	}
	if skippedNoData > 0 {
		log.Printf("[import:diag] %d linhas puladas — sem data válida (coluna DATA ausente e sem fallback)", skippedNoData)
	}
	log.Printf("[import:diag] roteamento: %d linhas → vendas_faturadas, %d linhas → vendas_transmitidas, %d linhas → vendas_ccd", len(allFat), len(allTrans), len(allCCD))

	// Parse terminou — arquivo em disco não é mais necessário. Apaga já em
	// vez de esperar o defer (que só roda após COPY + refresh de views, ~10min
	// pra arquivos grandes). Libera ~100MB de disco no meio do job, o que
	// ajuda quando entram vários imports em sequência.
	// Fecha explicitamente antes de remover (defer f.Close continua no fim).
	f.Close()
	if rmErr := os.Remove(uploadedPath); rmErr == nil {
		log.Printf("[ImportJob:%s] arquivo temp %s removido após parse", jobID, filepath.Base(uploadedPath))
		uploadedPath = "" // sinaliza pro defer não tentar remover de novo
	} else {
		log.Printf("[ImportJob:%s] falha ao remover %s após parse: %v (defer tentará novamente)", jobID, uploadedPath, rmErr)
	}

	// Dedup defensivo: o ION VENDAS exporta a mesma NF múltiplas vezes (uma por
	// RCA cuja carteira inclui o cliente). Sem chave de NF para deduplicar
	// semanticamente, usamos a tupla de negócio (data, cnpj|cli, prod, qt, pvenda)
	// — colisão real é patológica (1 cliente comprando idêntico, mesmo dia, 2x+
	// é raríssimo). Mantém a PRIMEIRA ocorrência (ordem do CSV); descarta o resto.
	type dedupKey struct {
		data    string
		cliCnpj string
		codProd string
		qt      float64
		pvenda  float64
	}
	dedupSlice := func(in []vendaRaw, label string) []vendaRaw {
		if len(in) == 0 {
			return in
		}
		seen := make(map[dedupKey]struct{}, len(in))
		out := make([]vendaRaw, 0, len(in))
		descartadas := 0
		for _, r := range in {
			cnpj, _ := r.vals[22].(string)
			cli, _ := r.vals[12].(string)
			key := dedupKey{
				data:    r.vals[1].(time.Time).Format("2006-01-02"),
				cliCnpj: cnpj + "|" + cli, // cnpj é primário, cli é fallback se cnpj vazio
				codProd: r.vals[16].(string),
				qt:      r.vals[19].(float64),
				pvenda:  r.vals[20].(float64),
			}
			if _, dup := seen[key]; dup {
				descartadas++
				continue
			}
			seen[key] = struct{}{}
			out = append(out, r)
		}
		if descartadas > 0 {
			log.Printf("[import:dedup] %s — %d brutas → %d únicas (descartadas %d duplicatas inter-RCA, fator %.2fx)",
				label, len(in), len(out), descartadas, float64(len(in))/float64(len(out)))
		} else {
			log.Printf("[import:dedup] %s — %d linhas, sem duplicatas", label, len(in))
		}
		return out
	}
	// Total bruto ANTES do dedup — é o que o campo "importados" do job vai
	// mostrar na UI. Reflete quantas linhas do CSV o parser efetivamente
	// entendeu, sem deduzir as duplicatas inter-RCA que o ION VENDAS gera
	// (mesma NF replicada por cada RCA cuja carteira inclui o cliente).
	// Se mostrássemos só o pós-dedup, o gestor pensaria que o arquivo teve
	// perda — mas foi o próprio ION que replicou.
	brutoImportadas := len(allFat) + len(allTrans) + len(allCCD)

	allFat = dedupSlice(allFat, "vendas_faturadas")
	allTrans = dedupSlice(allTrans, "vendas_transmitidas")
	allCCD = dedupSlice(allCCD, "vendas_ccd")

	if len(allFat) == 0 && len(allTrans) == 0 && len(allCCD) == 0 {
		markStatus("error", "nenhuma linha válida encontrada no arquivo")
		return
	}

	// ── Transação única: DELETE + COPY de AMBOS os fluxos atomicamente ────────
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		if ctx.Err() != nil {
			markStatus("cancelled", "cancelado pelo usuário")
		} else {
			markStatus("error", "erro ao iniciar transação: "+err.Error())
		}
		return
	}

	// Colunas comuns (idênticas para vendas_faturadas e vendas_transmitidas
	// exceto pelo nome da coluna de data). Mantemos o mesmo vals layout pros
	// dois COPYs.
	copyCols := []string{
		"empresa_id", "", // [1] é a data; preenchemos por fluxo
		"cod_gerente", "nome_gerente", "cod_supervisor", "nome_supervisor", "qtrca_supervisor",
		"cod_rca", "nome_rca", "qtcli_rca",
		"cod_fornec", "nome_fornec",
		"cod_cli", "nome_cli", "uf", "empresa",
		"cod_prod", "nome_prod", "ean",
		"qt", "pvenda", "plucro",
		"cnpj",
		"cod_ramo", "ramo",                     // visual cliente (mig 168)
		"embalagem", "qt_unit", "qt_unit_cx", "cod_bar", // visual produto (mig 168)
		// Novo layout jul/2026 (mig 181) — 9 colunas adicionais
		"cod_depto", "depto",
		"cod_sec", "secao",
		"cod_categoria", "categoria",
		"cod_cliprinc", "fantasia",
		"pvenda_unit",
	}
	// Colunas de vendas_ccd (mig 182): mesmas de fat/trans + `tipo_venda` (mig 189)
	// + `evento` no final. Como o COPY exige alinhamento posicional entre `vals` e
	// cols, tratamos CCD num processFlow separado que anexa tipo_venda e evento
	// como últimos args (nessa ordem).
	copyColsCcd := append(append([]string(nil), copyCols...), "tipo_venda", "evento")

	// extraCols/extraVals: colunas adicionais gravadas só em alguns fluxos (ex.:
	// tipo_venda + desc_condvenda no faturado). Vazio → COPY usa só o vals layout.
	processFlow := func(tableName, dateColName string, dates map[string]struct{}, rows []vendaRaw, extraCols []string, extraVals func(vendaRaw) []any) error {
		// DELETE atômico pelos dias presentes no CSV — preserva dias anteriores
		// não incluídos neste upload (cliente sobe vendas diárias).
		dateList := make([]string, 0, len(dates))
		for d := range dates {
			dateList = append(dateList, d)
		}
		if len(dateList) > 0 {
			_, dErr := tx.ExecContext(ctx,
				fmt.Sprintf(`DELETE FROM %s WHERE empresa_id=$1 AND %s = ANY($2::date[])`,
					tableName, dateColName),
				spCtx.EmpresaID, pq.Array(dateList),
			)
			if dErr != nil {
				return fmt.Errorf("DELETE %s: %w", tableName, dErr)
			}
			log.Printf("[ImportJob:%s] %s — DELETE prévio cobriu %d dia(s): %v",
				jobID, tableName, len(dateList), dateList)
		}

		// COPY em chunks; cancelamento checado entre chunks.
		cols := append([]string(nil), copyCols...)
		cols[1] = dateColName
		cols = append(cols, extraCols...)
		for start := 0; start < len(rows); start += copyChunkRows {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			end := start + copyChunkRows
			if end > len(rows) {
				end = len(rows)
			}
			chunk := rows[start:end]

			stmt, sErr := tx.PrepareContext(ctx, pq.CopyIn(tableName, cols...))
			if sErr != nil {
				return fmt.Errorf("PREPARE COPY %s: %w", tableName, sErr)
			}
			for i := range chunk {
				var eErr error
				if len(extraCols) > 0 {
					args := append(append([]any(nil), chunk[i].vals[:]...), extraVals(chunk[i])...)
					_, eErr = stmt.Exec(args...)
				} else {
					_, eErr = stmt.Exec(chunk[i].vals[:]...)
				}
				if eErr != nil {
					stmt.Close()
					return fmt.Errorf("enfileirar %s: %w", tableName, eErr)
				}
				processed.Add(1)
			}
			if _, fErr := stmt.Exec(); fErr != nil { // flush
				stmt.Close()
				return fmt.Errorf("flush %s: %w", tableName, fErr)
			}
			stmt.Close()
		}
		return nil
	}

	// processFlowCcd — variante para vendas_ccd. Diferença: anexa o campo
	// `evento` (CORTADO/CANCELADO/DEVOLVIDO) como argumento adicional no COPY.
	// Reutiliza a estrutura de DELETE prévio + chunks + PREPARE COPY.
	processFlowCcd := func(dates map[string]struct{}, rows []vendaRaw) error {
		dateList := make([]string, 0, len(dates))
		for d := range dates {
			dateList = append(dateList, d)
		}
		if len(dateList) > 0 {
			_, dErr := tx.ExecContext(ctx,
				`DELETE FROM vendas_ccd WHERE empresa_id=$1 AND data_evento = ANY($2::date[])`,
				spCtx.EmpresaID, pq.Array(dateList),
			)
			if dErr != nil {
				return fmt.Errorf("DELETE vendas_ccd: %w", dErr)
			}
			log.Printf("[ImportJob:%s] vendas_ccd — DELETE prévio cobriu %d dia(s): %v",
				jobID, len(dateList), dateList)
		}

		cols := append([]string(nil), copyColsCcd...)
		cols[1] = "data_evento"
		for start := 0; start < len(rows); start += copyChunkRows {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			end := start + copyChunkRows
			if end > len(rows) {
				end = len(rows)
			}
			chunk := rows[start:end]

			stmt, sErr := tx.PrepareContext(ctx, pq.CopyIn("vendas_ccd", cols...))
			if sErr != nil {
				return fmt.Errorf("PREPARE COPY vendas_ccd: %w", sErr)
			}
			for i := range chunk {
				// Args = vals[0..37] + tipo_venda + evento (alinha com copyColsCcd)
				args := append(append([]any(nil), chunk[i].vals[:]...), chunk[i].tipoVenda, chunk[i].evento)
				if _, eErr := stmt.Exec(args...); eErr != nil {
					stmt.Close()
					return fmt.Errorf("enfileirar vendas_ccd: %w", eErr)
				}
				processed.Add(1)
			}
			if _, fErr := stmt.Exec(); fErr != nil {
				stmt.Close()
				return fmt.Errorf("flush vendas_ccd: %w", fErr)
			}
			stmt.Close()
		}
		return nil
	}

	if err = processFlow("vendas_faturadas", "data_faturamento", uniqueFatDates, allFat,
		[]string{"tipo_venda", "desc_condvenda"},
		func(r vendaRaw) []any { return []any{r.tipoVenda, r.tipoVendaDesc} }); err != nil {
		tx.Rollback()
		if ctx.Err() != nil {
			markStatus("cancelled", "cancelado pelo usuário")
		} else {
			markStatus("error", err.Error())
		}
		return
	}
	if err = processFlow("vendas_transmitidas", "data_transmissao", uniqueTransDates, allTrans, nil, nil); err != nil {
		tx.Rollback()
		if ctx.Err() != nil {
			markStatus("cancelled", "cancelado pelo usuário")
		} else {
			markStatus("error", err.Error())
		}
		return
	}
	if err = processFlowCcd(uniqueCcdDates, allCCD); err != nil {
		tx.Rollback()
		if ctx.Err() != nil {
			markStatus("cancelled", "cancelado pelo usuário")
		} else {
			markStatus("error", err.Error())
		}
		return
	}

	if err = tx.Commit(); err != nil {
		if ctx.Err() != nil {
			markStatus("cancelled", "cancelado pelo usuário")
		} else {
			markStatus("error", "erro ao confirmar importação: "+err.Error())
		}
		return
	}

	// "importados" = total BRUTO lido do CSV (inclui as duplicatas inter-RCA
	// que o dedup depois descartou). É esse número que sobe na UI e que o
	// gestor compara com wc -l do arquivo.
	// `processed.Load()` reflete só o que foi efetivamente COPIADO pós-dedup
	// — útil pro log de auditoria, não pra UI.
	importados := brutoImportadas
	copiadas := int(processed.Load())
	log.Printf("[ImportJob:%s] concluído: %d linhas brutas do CSV (%d após dedup via COPY)",
		jobID, importados, copiadas)

	if !skipRefresh {
		// Sinaliza "Consolidando..." e faz REFRESH da view materializada.
		// CONCURRENTLY não bloqueia leituras concorrentes (requer unique index, criado na migration 141).
		db.Exec(`UPDATE vendas_import_jobs
			SET progress=91, message='Consolidando dados...', atualizado_em=NOW()
			WHERE id=$1`, jobID)

		log.Printf("[farol:view] ImportJob=%s refresh carteiras + upsert_aggs_mes", jobID)

		// REFRESH só das 2 carteiras (~ms). Tudo o resto é upsert_aggs_mes.
		for _, mv := range []string{"farol.mv_fat_carteira_rca", "farol.mv_trans_carteira_rca"} {
			if _, err := db.Exec(`REFRESH MATERIALIZED VIEW CONCURRENTLY ` + mv); err != nil {
				if _, err2 := db.Exec(`REFRESH MATERIALIZED VIEW ` + mv); err2 != nil {
					log.Printf("[farol:view] ImportJob=%s REFRESH %s ERRO: %v", jobID, mv, err2)
				}
			}
			db.Exec(`ANALYZE ` + mv)
		}

		// Popula tabelas agg_*_mes para os meses presentes neste arquivo.
		tAgg := time.Now()
		mesesTocados := make(map[[2]int]struct{})
		for k := range uniqueFatDates {
			if t, e := time.Parse("2006-01-02", k); e == nil {
				mesesTocados[[2]int{t.Year(), int(t.Month())}] = struct{}{}
			}
		}
		for k := range uniqueTransDates {
			if t, e := time.Parse("2006-01-02", k); e == nil {
				mesesTocados[[2]int{t.Year(), int(t.Month())}] = struct{}{}
			}
		}
		anosTocados := make(map[int]struct{})
		for ym := range mesesTocados {
			anosTocados[ym[0]] = struct{}{}
		}
		for ano := range anosTocados {
			if _, err := db.Exec(`SELECT farol.create_agg_year_partitions($1)`, ano); err != nil {
				log.Printf("[farol:agg] ImportJob=%s create_agg_year_partitions(%d) ERRO: %v", jobID, ano, err)
			}
		}
		var meses []aggMesYM
		for ym := range mesesTocados {
			meses = append(meses, aggMesYM{Ano: ym[0], Mes: ym[1]})
		}
		upsertAggsMesParallel(db, spCtx.EmpresaID, meses, 4)
		invalidateBaseCache(spCtx.EmpresaID)          // dados mudaram → limpa cache da base
		invalidateVendasPeriodoCache(spCtx.EmpresaID) // limpa cache Q1 de ranges diários
		invalidateBICache(spCtx.EmpresaID)            // painel BI serve resposta pronta do cache
		log.Printf("[farol:agg] ImportJob=%s UPSERT total (%d meses) em %v",
			jobID, len(mesesTocados), time.Since(tAgg))
	} else {
		log.Printf("[farol:view] ImportJob=%s skip_refresh=true — MVs e agg_mes adiados para consolidação final", jobID)
	}

	db.Exec(`UPDATE vendas_import_jobs
		SET status='done', progress=100, importados=$1, message='', atualizado_em=NOW()
		WHERE id=$2`, importados, jobID)

	// Criação de usuários em background — não bloqueia.
	// Usa fallbackAno/fallbackMes (vindo da URL) como "competência do upload"
	// para fins de sincronização de cadastros — mantém a semântica antiga.
	go func() {
		criados, err := syncUsuariosFromImport(db, spCtx, fallbackAno, fallbackMes)
		if err != nil {
			log.Printf("[SyncUsuarios] erro: %v", err)
		} else {
			log.Printf("[SyncUsuarios] %d criados", criados)
		}
	}()
}

// ─── VendasJobHandler — GET/POST /api/v2/vendas/job/{id}[/cancel] ────────────

func VendasJobHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		// Extrai jobID e ação do path: /api/v2/vendas/job/{id}[/cancel]
		path := strings.TrimPrefix(r.URL.Path, "/api/v2/vendas/job/")
		parts := strings.SplitN(path, "/", 2)
		jobID := parts[0]
		action := ""
		if len(parts) > 1 {
			action = parts[1]
		}

		if jobID == "" {
			http.Error(w, `{"error":"job_id obrigatório"}`, http.StatusBadRequest)
			return
		}

		// ── Cancel ───────────────────────────────────────────────────────────
		if action == "cancel" && r.Method == http.MethodPost {
			// Verifica que o job pertence a esta empresa
			var empID string
			err := db.QueryRow(`SELECT empresa_id FROM vendas_import_jobs WHERE id=$1`, jobID).Scan(&empID)
			if err != nil || empID != spCtx.EmpresaID {
				http.Error(w, `{"error":"job não encontrado"}`, http.StatusNotFound)
				return
			}
			if fn, ok := importJobs.Load(jobID); ok {
				fn.(context.CancelFunc)()
			}
			// Garante status = cancelled mesmo se a goroutine já terminou
			db.Exec(`UPDATE vendas_import_jobs
				SET status='cancelled', message='Cancelado pelo usuário', atualizado_em=NOW()
				WHERE id=$1 AND status IN ('pending','processing')`, jobID)
			json.NewEncoder(w).Encode(map[string]bool{"cancelled": true})
			return
		}

		// ── Status ───────────────────────────────────────────────────────────
		if r.Method == http.MethodGet {
			var job struct {
				ID         string `json:"id"`
				Ano        int    `json:"ano"`
				Mes        int    `json:"mes"`
				Status     string `json:"status"`
				Progress   int    `json:"progress"`
				TotalLines int    `json:"total_lines"`
				Importados int    `json:"importados"`
				Message    string `json:"message"`
			}
			err := db.QueryRow(`
				SELECT id, ano, mes, status, progress, total_lines, importados, message
				FROM vendas_import_jobs
				WHERE id=$1 AND empresa_id=$2`,
				jobID, spCtx.EmpresaID,
			).Scan(&job.ID, &job.Ano, &job.Mes,
				&job.Status, &job.Progress, &job.TotalLines, &job.Importados, &job.Message)
			if err != nil {
				http.Error(w, `{"error":"job não encontrado"}`, http.StatusNotFound)
				return
			}
			json.NewEncoder(w).Encode(job)
			return
		}

		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// ─── syncUsuariosFromImport ───────────────────────────────────────────────────

func syncUsuariosFromImport(db *sql.DB, spCtx *FarolContext, ano, mes int) (int, error) {
	type entrada struct {
		tipo string
		cod  string
		nome string
	}

	empresaShort := spCtx.EmpresaID
	if len(empresaShort) > 8 {
		empresaShort = empresaShort[:8]
	}

	var pessoas []entrada
	queries := []struct {
		tipo    string
		colCod  string
		colNome string
	}{
		{"ggv", "cod_gerente", "nome_gerente"},
		{"supervisor", "cod_supervisor", "nome_supervisor"},
		{"rca", "cod_rca", "nome_rca"},
	}
	for _, q := range queries {
		// UNION das duas tabelas — gerentes/supervisores/RCAs aparecem em
		// ambas (faturado e transmitido) e queremos cadastrar todos.
		qSQL := fmt.Sprintf(`
			SELECT DISTINCT cod, nome FROM (
			    SELECT %s AS cod, %s AS nome FROM vendas_faturadas
			     WHERE empresa_id=$1
			       AND EXTRACT(YEAR FROM data_faturamento)::int=$2
			       AND EXTRACT(MONTH FROM data_faturamento)::int=$3
			       AND %s != '' AND %s != ''
			    UNION
			    SELECT %s AS cod, %s AS nome FROM vendas_transmitidas
			     WHERE empresa_id=$1
			       AND EXTRACT(YEAR FROM data_transmissao)::int=$2
			       AND EXTRACT(MONTH FROM data_transmissao)::int=$3
			       AND %s != '' AND %s != ''
			) u`,
			q.colCod, q.colNome, q.colCod, q.colNome,
			q.colCod, q.colNome, q.colCod, q.colNome)
		rows, err := db.Query(qSQL, spCtx.EmpresaID, ano, mes)
		if err != nil {
			continue
		}
		for rows.Next() {
			var cod, nome string
			if scanErr := rows.Scan(&cod, &nome); scanErr == nil && cod != "" {
				pessoas = append(pessoas, entrada{q.tipo, cod, nome})
			}
		}
		rows.Close()
	}
	if len(pessoas) == 0 {
		return 0, nil
	}

	var envID sql.NullString
	_ = db.QueryRow(`SELECT environment_id FROM user_environments WHERE user_id=$1 LIMIT 1`,
		spCtx.UserID).Scan(&envID)

	trialEndsAt := time.Date(2099, 12, 31, 0, 0, 0, 0, time.UTC)
	criados := 0

	for _, p := range pessoas {
		codSafe := strings.ToLower(strings.Map(func(r rune) rune {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
				return r
			}
			return '-'
		}, p.cod))
		email := fmt.Sprintf("%s.%s@%s.farol.local", p.tipo, codSafe, empresaShort)

		// bcrypt cost 10 (vs auth.go cost 14) — auto-generated accounts only.
		// Runs sequentially for 200+ RCAs; cost 14 ≈ 2s each → 400s total.
		hashBytes, herr := bcrypt.GenerateFromPassword([]byte("Farol@"+p.cod), 10)
		if herr != nil {
			continue
		}
		hash := string(hashBytes)

		var userID string
		err := db.QueryRow(`
			INSERT INTO users (email, password_hash, full_name, trial_ends_at, is_verified, role, sp_role, tipo_persona, cod_referencia)
			VALUES ($1, $2, $3, $4, TRUE, 'user', 'somente_leitura', $5, $6)
			ON CONFLICT (email) DO NOTHING
			RETURNING id`,
			email, hash, p.nome, trialEndsAt, p.tipo, p.cod,
		).Scan(&userID)
		if err == sql.ErrNoRows {
			continue // email já existia — OK, ignora
		}
		if err != nil {
			log.Printf("[SyncUsuarios] falha ao criar %s (%s): %v", email, p.nome, err)
			continue
		}

		if envID.Valid && envID.String != "" {
			_, _ = db.Exec(`
				INSERT INTO user_environments (user_id, environment_id, role, preferred_company_id)
				VALUES ($1, $2, 'user', $3) ON CONFLICT DO NOTHING`,
				userID, envID.String, spCtx.EmpresaID)
		}
		_, _ = db.Exec(`
			INSERT INTO farol.sp_user_filiais (user_id, empresa_id, filial_id, all_filiais)
			VALUES ($1, $2, NULL, TRUE) ON CONFLICT DO NOTHING`,
			userID, spCtx.EmpresaID)

		criados++
		log.Printf("[SyncUsuarios] criado: %s %s (%s)", p.tipo, p.nome, email)
	}

	return criados, nil
}

// ─── VendasPeriodosHandler — GET /api/v2/vendas/periodos ────────────────────

type v2PeriodoItem struct {
	Ano   int    `json:"ano"`
	Mes   int    `json:"mes"`
	Label string `json:"label"`
	Total int    `json:"total"`
}

func VendasPeriodosHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		// UNION das duas tabelas — período é determinado pela data (ano/mes
		// extraídos de data_faturamento ou data_transmissao). Soma de FAT+TRANS
		// dá o total de linhas importadas naquele mês.
		rows, err := db.Query(`
			SELECT ano, mes, SUM(total)::int AS total
			  FROM (
			    SELECT EXTRACT(YEAR  FROM data_faturamento)::int AS ano,
			           EXTRACT(MONTH FROM data_faturamento)::int AS mes,
			           COUNT(*) AS total
			      FROM vendas_faturadas
			     WHERE empresa_id = $1
			     GROUP BY ano, mes
			    UNION ALL
			    SELECT EXTRACT(YEAR  FROM data_transmissao)::int AS ano,
			           EXTRACT(MONTH FROM data_transmissao)::int AS mes,
			           COUNT(*) AS total
			      FROM vendas_transmitidas
			     WHERE empresa_id = $1
			     GROUP BY ano, mes
			  ) u
			 GROUP BY ano, mes
			 ORDER BY ano DESC, mes DESC
		`, spCtx.EmpresaID)
		if err != nil {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		items := []v2PeriodoItem{}
		for rows.Next() {
			var it v2PeriodoItem
			if rows.Scan(&it.Ano, &it.Mes, &it.Total) == nil {
				it.Label = fmtMesAno(it.Mes, it.Ano)
				items = append(items, it)
			}
		}
		json.NewEncoder(w).Encode(items)
	}
}

// ─── VendasClearHandler — DELETE /api/v2/vendas/clear ───────────────────────

func VendasClearHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		if !RequireWrite(spCtx, w) {
			return
		}

		// Intervalo de datas — alinhado à nova granularidade diária.
		//   ?data_inicio=YYYY-MM-DD&data_fim=YYYY-MM-DD  → apaga o intervalo
		//   sem parâmetros                               → apaga a base inteira
		// Para apagar 1 dia, passar mesma data nos dois campos.
		dataInicio := strings.TrimSpace(r.URL.Query().Get("data_inicio"))
		dataFim    := strings.TrimSpace(r.URL.Query().Get("data_fim"))
		validRange := dataInicio != "" && dataFim != ""

		// Apaga em AMBAS as tabelas — cada CSV importado povoou as duas.
		var totalAffected int64
		execBoth := func(qFat, qTrans string, args ...any) error {
			r1, e1 := db.Exec(qFat, args...)
			if e1 != nil {
				return e1
			}
			n1, _ := r1.RowsAffected()
			r2, e2 := db.Exec(qTrans, args...)
			if e2 != nil {
				return e2
			}
			n2, _ := r2.RowsAffected()
			totalAffected = n1 + n2
			return nil
		}

		var err error
		switch {
		case validRange:
			err = execBoth(
				`DELETE FROM vendas_faturadas
				  WHERE empresa_id=$1 AND data_faturamento BETWEEN $2::date AND $3::date`,
				`DELETE FROM vendas_transmitidas
				  WHERE empresa_id=$1 AND data_transmissao BETWEEN $2::date AND $3::date`,
				spCtx.EmpresaID, dataInicio, dataFim,
			)
		default:
			// Base inteira da empresa ("Limpar tudo").
			err = execBoth(
				`DELETE FROM vendas_faturadas    WHERE empresa_id=$1`,
				`DELETE FROM vendas_transmitidas WHERE empresa_id=$1`,
				spCtx.EmpresaID,
			)
		}
		if err != nil {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		n := totalAffected

		// Reconstrói as views materializadas para o painel refletir a limpeza —
		// sem isso o dashboard continua mostrando os dados apagados (views = stale).
		if rerr := refreshAllFarolViews(db); rerr != nil {
			log.Printf("[VendasClear] delete OK (%d linhas) mas REFRESH falhou: %v", n, rerr)
		}
		invalidateBICache(spCtx.EmpresaID) // senão o BI segue exibindo o que foi apagado
		json.NewEncoder(w).Encode(map[string]any{"deleted": n})
	}
}

// ─── IndustriasConfigHandler — GET/POST /api/v2/industrias ──────────────────

type industriaConfig struct {
	CodFornec            string  `json:"cod_fornec"`
	NomeFornec           string  `json:"nome_fornec"`
	ProgramaDistribuicao string  `json:"programa_distribuicao"`
	TravaMinima          float64 `json:"trava_minima_qt"`
}

func IndustriasConfigHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		switch r.Method {
		case http.MethodGet:
			rows, err := db.Query(`
				SELECT cod_fornec, nome_fornec, programa_distribuicao, trava_minima_qt
				FROM industrias_config WHERE empresa_id=$1 ORDER BY nome_fornec
			`, spCtx.EmpresaID)
			if err != nil {
				http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
				return
			}
			defer rows.Close()
			items := []industriaConfig{}
			for rows.Next() {
				var it industriaConfig
				if rows.Scan(&it.CodFornec, &it.NomeFornec, &it.ProgramaDistribuicao, &it.TravaMinima) == nil {
					items = append(items, it)
				}
			}
			json.NewEncoder(w).Encode(items)

		case http.MethodPut, http.MethodPost:
			if !RequireWrite(spCtx, w) {
				return
			}
			var it industriaConfig
			if err := json.NewDecoder(r.Body).Decode(&it); err != nil || it.CodFornec == "" {
				http.Error(w, `{"error":"payload inválido"}`, http.StatusBadRequest)
				return
			}
			if it.TravaMinima < 0 {
				it.TravaMinima = 1
			}
			_, err := db.Exec(`
				INSERT INTO industrias_config
					(empresa_id, cod_fornec, nome_fornec, programa_distribuicao, trava_minima_qt)
				VALUES ($1,$2,$3,$4,$5)
				ON CONFLICT (empresa_id, cod_fornec) DO UPDATE SET
					nome_fornec=EXCLUDED.nome_fornec,
					programa_distribuicao=EXCLUDED.programa_distribuicao,
					trava_minima_qt=EXCLUDED.trava_minima_qt,
					atualizado_em=NOW()
			`, spCtx.EmpresaID, it.CodFornec, it.NomeFornec, it.ProgramaDistribuicao, it.TravaMinima)
			if err != nil {
				http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"ok": true})

		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	}
}

// ─── Util ─────────────────────────────────────────────────────────────────────

var mesNomes = []string{"", "Jan", "Fev", "Mar", "Abr", "Mai", "Jun", "Jul", "Ago", "Set", "Out", "Nov", "Dez"}

func fmtMesAno(mes, ano int) string {
	if mes < 1 || mes > 12 {
		return fmt.Sprintf("%d", ano)
	}
	return fmt.Sprintf("%s/%d", mesNomes[mes], ano)
}
