package handlers

// farol_metas_import_csv.go — Importação de metas via CSV (Épico 3, Story
// 3.1, módulo Painel de Gestão de Metas por Indústria)
//
// Diferente de ObjetivosImportHandler (import de vendas, alto volume,
// batch/streaming SSE), este é síncrono e ESTRITAMENTE atômico — FR9 exige
// "sem aplicar parcialmente um lote com erro". Volume aqui é baixo (metas
// por vínculo/faixa/vigência, dezenas de linhas, não milhões), então uma
// única transação pro arquivo inteiro é a escolha certa: valida tudo
// primeiro, só então aplica tudo, ou nada.
//
// Formato CSV (';' delimitado, mesmo padrão de ObjetivosImportHandler):
//   vinculo_id;data_inicio;data_fim;faixa;valor_meta
// Linhas com o mesmo vinculo_id+data_inicio+data_fim agrupam na MESMA
// vigência (várias faixas por vigência, mesmo modelo da Story 2.2).
//
// Rota: POST /api/farol/metas-vinculos-importar-csv (gestor_geral)

import (
	"bytes"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"
)

type csvMetaLinhaErro struct {
	Linha int    `json:"linha"`
	Erro  string `json:"erro"`
}

type csvMetaRow struct {
	linha      int
	vinculoID  int
	dataInicio string
	dataFim    string
	faixa      int
	valorMeta  float64
}

// MetasImportarCSVHandler — POST /api/farol/metas-vinculos-importar-csv
func MetasImportarCSVHandler(db *sql.DB) http.HandlerFunc {
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
		if !hasSpRole(spCtx.SpRole, "gestor_geral") {
			http.Error(w, `{"error":"Forbidden: gestor_geral necessário"}`, http.StatusForbidden)
			return
		}

		rawBytes, err := lerArquivoCSV(r)
		if err != nil {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
			return
		}

		csvReader := csv.NewReader(bytes.NewReader(rawBytes))
		csvReader.Comma = ';'
		csvReader.LazyQuotes = true
		csvReader.TrimLeadingSpace = true
		csvReader.FieldsPerRecord = -1

		headerRow, err := csvReader.Read()
		if err != nil {
			http.Error(w, `{"error":"falha ao ler cabeçalho CSV"}`, http.StatusBadRequest)
			return
		}
		norm := func(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
		colIdx := map[string]int{}
		for i, h := range headerRow {
			colIdx[norm(h)] = i
		}
		required := []string{"vinculo_id", "data_inicio", "data_fim", "faixa", "valor_meta"}
		for _, c := range required {
			if _, ok := colIdx[c]; !ok {
				http.Error(w, `{"error":"coluna obrigatória ausente no CSV: `+c+`"}`, http.StatusBadRequest)
				return
			}
		}

		var (
			rows       []csvMetaRow
			erros      []csvMetaLinhaErro
			linhaAtual = 1 // cabeçalho já consumido = linha 1
		)
		for {
			record, err := csvReader.Read()
			if err == io.EOF {
				break
			}
			linhaAtual++
			if err != nil {
				erros = append(erros, csvMetaLinhaErro{Linha: linhaAtual, Erro: "linha malformada: " + err.Error()})
				continue
			}
			get := func(col string) string {
				if idx, ok := colIdx[col]; ok && idx < len(record) {
					return strings.TrimSpace(record[idx])
				}
				return ""
			}
			row := csvMetaRow{linha: linhaAtual}
			linhaErro := false

			vinculoID, errV := strconv.Atoi(get("vinculo_id"))
			if errV != nil || vinculoID <= 0 {
				erros = append(erros, csvMetaLinhaErro{Linha: linhaAtual, Erro: "vinculo_id inválido: " + get("vinculo_id")})
				linhaErro = true
			}
			row.vinculoID = vinculoID

			row.dataInicio = get("data_inicio")
			row.dataFim = get("data_fim")
			if row.dataInicio == "" || row.dataFim == "" {
				erros = append(erros, csvMetaLinhaErro{Linha: linhaAtual, Erro: "data_inicio e data_fim são obrigatórias"})
				linhaErro = true
			}

			faixa, errF := strconv.Atoi(get("faixa"))
			if errF != nil || faixa <= 0 {
				erros = append(erros, csvMetaLinhaErro{Linha: linhaAtual, Erro: "faixa inválida: " + get("faixa")})
				linhaErro = true
			}
			row.faixa = faixa

			valorMeta, errVM := strconv.ParseFloat(strings.ReplaceAll(get("valor_meta"), ",", "."), 64)
			if errVM != nil {
				erros = append(erros, csvMetaLinhaErro{Linha: linhaAtual, Erro: "valor_meta inválido: " + get("valor_meta")})
				linhaErro = true
			}
			row.valorMeta = valorMeta

			if !linhaErro {
				rows = append(rows, row)
			}
		}

		if len(rows) == 0 && len(erros) == 0 {
			http.Error(w, `{"error":"CSV vazio — nenhuma linha de dado encontrada"}`, http.StatusBadRequest)
			return
		}

		// Valida vinculo_id contra o banco (empresa-scoped) — ainda faseamento
		// de validação, nenhuma escrita ainda.
		vinculosVistos := map[int]bool{}
		for _, row := range rows {
			vinculosVistos[row.vinculoID] = true
		}
		vinculosValidos := map[int]bool{}
		for vID := range vinculosVistos {
			var existe bool
			db.QueryRow(`SELECT EXISTS(SELECT 1 FROM farol.metas_vinculos WHERE id = $1 AND empresa_id = $2)`, vID, spCtx.EmpresaID).Scan(&existe)
			vinculosValidos[vID] = existe
		}
		for _, row := range rows {
			if !vinculosValidos[row.vinculoID] {
				erros = append(erros, csvMetaLinhaErro{Linha: row.linha, Erro: fmt.Sprintf("vinculo_id %d não encontrado", row.vinculoID)})
			}
		}

		// FR9: qualquer erro em qualquer linha invalida o lote inteiro — nada é
		// aplicado, mesmo as linhas que estariam corretas sozinhas.
		if len(erros) > 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]any{"erros": erros, "linhas_com_erro": len(erros)})
			return
		}

		// Agrupa por vigência (vinculo_id + data_inicio + data_fim) — várias
		// linhas de faixa viram uma única vigência (mesmo modelo da Story 2.2).
		type chaveVigencia struct {
			vinculoID  int
			dataInicio string
			dataFim    string
		}
		ordem := []chaveVigencia{}
		grupos := map[chaveVigencia][]FaixaDTO{}
		for _, row := range rows {
			k := chaveVigencia{row.vinculoID, row.dataInicio, row.dataFim}
			if _, ok := grupos[k]; !ok {
				ordem = append(ordem, k)
			}
			grupos[k] = append(grupos[k], FaixaDTO{Faixa: row.faixa, ValorMeta: row.valorMeta})
		}
		for k, faixas := range grupos {
			if msg := validarVigenciaRequest(VigenciaRequest{VinculoID: k.vinculoID, DataInicio: k.dataInicio, DataFim: k.dataFim, Faixas: faixas}); msg != "" {
				erros = append(erros, csvMetaLinhaErro{Linha: 0, Erro: fmt.Sprintf("vínculo %d, %s..%s: %s", k.vinculoID, k.dataInicio, k.dataFim, msg)})
			}
		}
		if len(erros) > 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]any{"erros": erros, "linhas_com_erro": len(erros)})
			return
		}

		// ── Tudo validado — aplica o lote inteiro numa única transação ────────
		tx, err := db.Begin()
		if err != nil {
			http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()

		vigenciasCriadas := 0
		for _, k := range ordem {
			id, err := inserirVigenciaTx(tx, spCtx.EmpresaID, k.vinculoID, k.dataInicio, k.dataFim, grupos[k])
			if err != nil {
				msg := "Database error: " + err.Error()
				if strings.Contains(err.Error(), "ex_farol_metas_vigencias_sem_overlap") {
					msg = fmt.Sprintf("vínculo %d: vigência %s..%s se sobrepõe a uma já existente", k.vinculoID, k.dataInicio, k.dataFim)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusConflict)
				json.NewEncoder(w).Encode(map[string]any{"erros": []csvMetaLinhaErro{{Linha: 0, Erro: msg}}})
				return
			}
			if err := writeAuditLogTx(tx, spCtx.EmpresaID, spCtx.UserID, "metas_vigencias", strconv.Itoa(id), "criar_via_csv", map[string]any{
				"vinculo_id": k.vinculoID, "data_inicio": k.dataInicio, "data_fim": k.dataFim, "faixas": grupos[k],
			}); err != nil {
				http.Error(w, `{"error":"erro ao gravar auditoria"}`, http.StatusInternalServerError)
				return
			}
			vigenciasCriadas++
		}

		if err := tx.Commit(); err != nil {
			http.Error(w, `{"error":"commit error"}`, http.StatusInternalServerError)
			return
		}
		log.Printf("MetasImportarCSV: %d vigências criadas (%d linhas) empresa %s por %s", vigenciasCriadas, len(rows), spCtx.EmpresaID, spCtx.UserID)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "vigencias_criadas": vigenciasCriadas, "linhas_processadas": len(rows)})
	}
}

// lerArquivoCSV lê o upload (multipart ou body direto), remove BOM UTF-8 e
// corrige encoding Latin-1/Windows-1252 — mesmo tratamento de
// ObjetivosImportHandler, reaproveitado aqui pra consistência.
func lerArquivoCSV(r *http.Request) ([]byte, error) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		// fallback pro body direto
	}
	var reader io.Reader
	file, _, ferr := r.FormFile("file")
	if ferr == nil {
		defer file.Close()
		reader = file
	} else {
		reader = r.Body
	}
	rawBytes, err := io.ReadAll(reader)
	if err != nil || len(rawBytes) == 0 {
		return nil, fmt.Errorf("falha ao ler arquivo")
	}
	if len(rawBytes) >= 3 && rawBytes[0] == 0xEF && rawBytes[1] == 0xBB && rawBytes[2] == 0xBF {
		rawBytes = rawBytes[3:]
	}
	if !utf8.Valid(rawBytes) {
		out := make([]byte, 0, len(rawBytes)*2)
		for _, b := range rawBytes {
			if b < 0x80 {
				out = append(out, b)
			} else {
				var buf [4]byte
				n := utf8.EncodeRune(buf[:], rune(b))
				out = append(out, buf[:n]...)
			}
		}
		rawBytes = out
	}
	return rawBytes, nil
}
