package services

// csv_worker.go — Worker assíncrono de processamento de CSV
//
// Story 4.2 — Worker Assíncrono de CSV
//
// Padrão: goroutine + polling em sp_csv_jobs (status = 'pending').
// O worker roda em background desde o start do servidor e processa
// um job por vez (pode ser expandido para pool no futuro).
//
// Fluxo por job:
//   1. Marca job como 'processing'
//   2. Lê arquivo CSV do disco (file_path)
//   3. Faz parse linha a linha (separador ';', encoding UTF-8 BOM)
//   4. Insere em sp_enderecos (batch por transação)
//   5. Marca job como 'done' (ou 'failed' em caso de erro)
//
// Separadores CSV (Calibragem_WMS_v3.csv):
//   delimitador: ';'
//   decimal:     ',' (ex: "10,86" → 10.86)
//   encoding:    UTF-8 com BOM (bytes 0xEF 0xBB 0xBF removidos)

import (
	"bytes"
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// StartCSVWorker inicia o worker em background. Chamado via go StartCSVWorker(getDB) em main.go.
func StartCSVWorker(getDB func() *sql.DB) {
	go func() {
		log.Println("[CSVWorker] started")
		for {
			db := getDB()
			if db != nil {
				processNextJob(db)
			}
			time.Sleep(5 * time.Second)
		}
	}()
}

// processNextJob busca o próximo job pendente e processa.
func processNextJob(db *sql.DB) {
	// Seleciona e bloqueia atomicamente um job pendente (SKIP LOCKED evita concorrência)
	var jobID, filePath, empresaID string
	var cdID, filialID int
	err := db.QueryRow(`
		UPDATE farol.sp_csv_jobs
		SET status = 'processing', started_at = now()
		WHERE id = (
		  SELECT id FROM farol.sp_csv_jobs
		  WHERE status = 'pending'
		  ORDER BY created_at ASC
		  LIMIT 1
		  FOR UPDATE SKIP LOCKED
		)
		RETURNING id, file_path, empresa_id, cd_id, filial_id
	`).Scan(&jobID, &filePath, &empresaID, &cdID, &filialID)
	if err == sql.ErrNoRows {
		return // sem jobs pendentes
	}
	if err != nil {
		log.Printf("[CSVWorker] erro ao buscar job: %v", err)
		return
	}

	log.Printf("[CSVWorker] processando job %s (arquivo: %s)", jobID, filePath)

	linhasOk, linhasErro, err := parseAndInsertCSV(db, jobID, filePath, empresaID, filialID)

	if err != nil {
		log.Printf("[CSVWorker] job %s FAILED: %v", jobID, err)
		db.Exec(`
			UPDATE farol.sp_csv_jobs
			SET status = 'failed', erro_msg = $1, finished_at = now(),
			    linhas_ok = $2, linhas_erro = $3
			WHERE id = $4
		`, err.Error(), linhasOk, linhasErro, jobID)
		return
	}

	total := linhasOk + linhasErro
	db.Exec(`
		UPDATE farol.sp_csv_jobs
		SET status = 'done', finished_at = now(),
		    total_linhas = $1, linhas_ok = $2, linhas_erro = $3
		WHERE id = $4
	`, total, linhasOk, linhasErro, jobID)

	log.Printf("[CSVWorker] job %s concluído: %d ok / %d erros", jobID, linhasOk, linhasErro)
}

// csvCols agrupa os índices de colunas detectados pelo cabeçalho do CSV.
// Suporta múltiplos formatos WMS (com ou sem QTUNITCX).
type csvCols struct {
	codFilial, codEpto, departamento, codSec, secao int
	codProd, produto, embalagem, qtUnitCx, foraLinha int
	rua, predio, apto                               int
	capacidade, normaPalete, pontoRep               int
	participacao, acumulado                         int
	classeVenda, classeDias                         int
	giroDia, acesso90, qtMovPicking90               int
	qtDias, qtProd, qtProdCx                        int
	medVendaCx, medVendaDias, medDiasEst, medVendaCxAA int
}

// detectCols mapeia o cabeçalho CSV (case-insensitive) para índices de coluna.
// Colunas ausentes ficam com valor -1.
func detectCols(header []string) csvCols {
	idx := map[string]int{}
	for i, h := range header {
		idx[strings.ToUpper(strings.TrimSpace(h))] = i
	}
	get := func(name string) int {
		if i, ok := idx[name]; ok {
			return i
		}
		return -1
	}
	return csvCols{
		codFilial:    get("CODFILIAL"),
		codEpto:      get("CODEPTO"),
		departamento: get("DEPARTAMENTO"),
		codSec:       get("CODSEC"),
		secao:        get("SECAO"),
		codProd:      get("CODPROD"),
		produto:      get("PRODUTO"),
		embalagem:    get("EMBALAGEM"),
		qtUnitCx:     get("QTUNITCX"),   // opcional: ausente em alguns exports
		foraLinha:    get("FORALINHA"),
		rua:          get("RUA"),
		predio:       get("PREDIO"),
		apto:         get("APTO"),
		capacidade:      get("CAPACIDADE"),
		normaPalete:     get("NORMA_PALETE"),
		pontoRep:        get("PONTOREPOSICAO"),
		participacao:    get("PARTICIPACAO"),
		acumulado:       get("ACUMULADO"),
		classeVenda:     get("CLASSEVENDA"),
		classeDias:   get("CLASSEVENDA_DIAS"),
		giroDia:         get("QTGIRODIA_SISTEMA"),
		acesso90:        get("QTACESSO_PICKING_PERIODO_90"),
		qtMovPicking90:  get("QT_MOV_PICKING_90"),
		qtDias:          get("QT_DIAS"),
		qtProd:       get("QT_PROD"),
		qtProdCx:     get("QT_PROD_CX"),
		medVendaCx:   get("MED_VENDA_DIAS_CX"),
		medVendaDias: get("MED_VENDA_DIAS"),
		medDiasEst:   get("MED_DIAS_ESTOQUE"),
		medVendaCxAA: get("MED_VENDA_DIAS_CX_ANOANT_MESSEG"),
	}
}

// latin1ToUTF8 converte bytes Latin-1/Windows-1252 para UTF-8.
// Bytes 0x00-0x7F são idênticos em ambas as codificações.
// Bytes 0x80-0xFF mapeiam diretamente para U+0080..U+00FF em Latin-1,
// que em UTF-8 codificam como 2 bytes (0xC2/0xC3 + continuação).
func latin1ToUTF8(b []byte) []byte {
	out := make([]byte, 0, len(b)+len(b)/4)
	for _, c := range b {
		if c < 0x80 {
			out = append(out, c)
		} else {
			out = append(out, 0xC0|(c>>6), 0x80|(c&0x3F))
		}
	}
	return out
}

func parseAndInsertCSV(db *sql.DB, jobID, filePath, _ string, filialID int) (ok, erros int, err error) {
	rawBytes, err := os.ReadFile(filePath)
	if err != nil {
		return 0, 0, fmt.Errorf("arquivo não encontrado: %w", err)
	}
	// Se não for UTF-8 válido (ex: exportação Winthor em Windows-1252), converte
	if !utf8.Valid(rawBytes) {
		rawBytes = latin1ToUTF8(rawBytes)
	}

	r := csv.NewReader(bytes.NewReader(rawBytes))
	r.Comma = ';'
	r.LazyQuotes = true
	r.TrimLeadingSpace = true

	// Lê cabeçalho (primeira linha)
	header, err := r.Read()
	if err != nil {
		return 0, 0, fmt.Errorf("erro ao ler cabeçalho: %w", err)
	}
	// Remove BOM da primeira coluna do cabeçalho se presente
	if len(header) > 0 {
		header[0] = strings.TrimPrefix(header[0], "\xef\xbb\xbf")
	}

	// Detecta índices das colunas pelo nome (suporta qualquer ordem/formato WMS)
	cols := detectCols(header)
	if cols.codProd < 0 || cols.rua < 0 {
		return 0, 0, fmt.Errorf("cabeçalho inválido: colunas obrigatórias (CODPROD, RUA) não encontradas")
	}

	// Processa em batch de 500 linhas por transação
	const batchSize = 500
	var batch [][]string

	flushBatch := func() error {
		if len(batch) == 0 {
			return nil
		}
		tx, txErr := db.Begin()
		if txErr != nil {
			return txErr
		}
		defer tx.Rollback()

		stmt, stmtErr := tx.Prepare(`
			INSERT INTO farol.sp_enderecos (
				job_id, filial_id, cod_filial, codepto, departamento, codsec, secao,
				codprod, produto, embalagem, fora_linha, rua, predio, apto,
				capacidade, norma_palete, ponto_reposicao,
				participacao, acumulado,
				classe_venda, classe_venda_dias,
				qt_giro_dia, qt_acesso_90, qt_mov_picking_90, qt_dias, qt_prod, qt_prod_cx,
				med_venda_cx, med_venda_dias, med_dias_estoque, med_venda_cx_aa, unidade_master
			) VALUES (
				$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,
				$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,
				$28,$29,$30,$31,$32
			)
		`)
		if stmtErr != nil {
			return stmtErr
		}
		defer stmt.Close()

		for _, row := range batch {
			if len(row) < 10 {
				erros++
				continue
			}
			args := rowToArgs(jobID, filialID, row, cols)
			// Savepoint por linha: erro numa linha não aborta a transação inteira
			if _, spErr := tx.Exec("SAVEPOINT sp_row"); spErr != nil {
				return spErr
			}
			if _, execErr := stmt.Exec(args...); execErr != nil {
				log.Printf("[CSVWorker] erro na linha: %v", execErr)
				tx.Exec("ROLLBACK TO SAVEPOINT sp_row") //nolint
				erros++
				continue
			}
			tx.Exec("RELEASE SAVEPOINT sp_row") //nolint
			ok++
		}

		return tx.Commit()
	}

	for {
		row, readErr := r.Read()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			erros++
			continue
		}
		batch = append(batch, row)
		if len(batch) >= batchSize {
			if flushErr := flushBatch(); flushErr != nil {
				return ok, erros, flushErr
			}
			batch = batch[:0]
		}
	}

	if flushErr := flushBatch(); flushErr != nil {
		return ok, erros, flushErr
	}

	return ok, erros, nil
}

// rowToArgs converte uma linha CSV nos argumentos do prepared statement.
// cols contém os índices detectados dinamicamente pelo cabeçalho.
func rowToArgs(jobID string, filialID int, row []string, cols csvCols) []any {
	get := func(i int) string {
		if i >= 0 && i < len(row) {
			return strings.TrimSpace(row[i])
		}
		return ""
	}
	parseInt := func(i int) *int {
		s := strings.TrimSpace(get(i))
		if s == "" {
			return nil
		}
		v, err := strconv.Atoi(s)
		if err != nil {
			return nil
		}
		return &v
	}
	parseFloat := func(i int) *float64 {
		s := strings.TrimSpace(strings.ReplaceAll(get(i), ",", "."))
		if s == "" {
			return nil
		}
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return nil
		}
		return &v
	}

	codFilialVal := filialID
	if cf := parseInt(cols.codFilial); cf != nil {
		codFilialVal = *cf
	}

	foraLinha := strings.EqualFold(get(cols.foraLinha), "S")
	classeVenda := get(cols.classeVenda)
	if len(classeVenda) > 1 {
		classeVenda = classeVenda[:1]
	}

	return []any{
		jobID,                         // $1  job_id
		filialID,                      // $2  filial_id
		codFilialVal,                  // $3  cod_filial
		parseInt(cols.codEpto),        // $4  codepto
		get(cols.departamento),        // $5  departamento
		parseInt(cols.codSec),         // $6  codsec
		get(cols.secao),               // $7  secao
		parseInt(cols.codProd),        // $8  codprod
		get(cols.produto),             // $9  produto
		get(cols.embalagem),           // $10 embalagem
		foraLinha,                     // $11 fora_linha
		parseInt(cols.rua),            // $12 rua
		parseInt(cols.predio),         // $13 predio
		parseInt(cols.apto),           // $14 apto
		parseInt(cols.capacidade),     // $15 capacidade
		parseInt(cols.normaPalete),    // $16 norma_palete
		parseInt(cols.pontoRep),       // $17 ponto_reposicao
		parseFloat(cols.participacao), // $18 participacao  (% participação nas vendas)
		parseFloat(cols.acumulado),    // $19 acumulado     (% acumulada — corte ABC)
		nilIfEmpty(classeVenda),       // $20 classe_venda
		parseInt(cols.classeDias),     // $21 classe_venda_dias
		parseFloat(cols.giroDia),      // $22 qt_giro_dia
		parseInt(cols.acesso90),       // $23 qt_acesso_90
		parseInt(cols.qtMovPicking90), // $24 qt_mov_picking_90 (peças/cx movimentadas em 90 dias)
		parseInt(cols.qtDias),         // $25 qt_dias
		parseInt(cols.qtProd),         // $26 qt_prod
		parseInt(cols.qtProdCx),       // $27 qt_prod_cx
		parseFloat(cols.medVendaCx),   // $28 med_venda_cx
		parseFloat(cols.medVendaDias), // $29 med_venda_dias
		parseFloat(cols.medDiasEst),   // $30 med_dias_estoque
		parseFloat(cols.medVendaCxAA), // $31 med_venda_cx_aa
		parseInt(cols.qtUnitCx),       // $32 unidade_master (QTUNITCX; nil quando ausente)
	}
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
