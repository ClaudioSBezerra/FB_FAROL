package handlers

// farol_metas_clientes_validos_csv.go — Importação de Clientes Válidos
// (Redes + RCA responsável) — Épico 3, Story 3.2, módulo Painel de Gestão
// de Metas por Indústria.
//
// Executa o modelo decidido na Story 1.4: lista escopada por vínculo +
// vigência (FR11), rede_nome livre (não FK), cod_rca só guardado (subir
// pra CRV/GGV é responsabilidade de quem consome, via JOIN com a hierarquia
// organizacional já existente). Mesmo padrão de atomicidade estrita da
// Story 3.1 (FR9): valida tudo, aplica tudo ou nada.
//
// Uma nova importação SUBSTITUI a lista anterior da mesma vigência (mesmo
// princípio do PUT-replace de farol_industrias.go) — a lista é sempre "o
// que vale agora pra esta vigência", não um acréscimo incremental.
//
// Formato CSV (';'): rede_nome;cnpj;cod_rca
// Rota: POST /api/farol/metas-clientes-validos-importar-csv?vinculo_id=&vigencia_id=

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

var cnpjSoDigitos = regexp.MustCompile(`^\d{14}$`)

type clienteValidoLinhaErro struct {
	Linha int    `json:"linha"`
	Erro  string `json:"erro"`
}

type clienteValidoRow struct {
	linha    int
	redeNome string
	cnpj     string
	codRCA   string
}

// MetasClientesValidosImportarCSVHandler — POST .../metas-clientes-validos-importar-csv
func MetasClientesValidosImportarCSVHandler(db *sql.DB) http.HandlerFunc {
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

		vinculoID, err1 := strconv.Atoi(r.URL.Query().Get("vinculo_id"))
		vigenciaID, err2 := strconv.Atoi(r.URL.Query().Get("vigencia_id"))
		if err1 != nil || err2 != nil {
			http.Error(w, `{"error":"parâmetros vinculo_id e vigencia_id (query string) são obrigatórios"}`, http.StatusBadRequest)
			return
		}

		var vigenciaVinculoID int
		var vigenciaStatus string
		err := db.QueryRow(`SELECT vinculo_id, status FROM farol.metas_vigencias WHERE id = $1 AND empresa_id = $2`, vigenciaID, spCtx.EmpresaID).Scan(&vigenciaVinculoID, &vigenciaStatus)
		if err == sql.ErrNoRows {
			http.Error(w, `{"error":"vigencia_id não encontrada"}`, http.StatusBadRequest)
			return
		} else if err != nil {
			http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
			return
		}
		if vigenciaVinculoID != vinculoID {
			http.Error(w, `{"error":"vigencia_id não pertence ao vinculo_id informado"}`, http.StatusBadRequest)
			return
		}
		if vigenciaStatus == "fechada" {
			http.Error(w, `{"error":"vigência fechada — lista de Clientes Válidos não pode ser reimportada (congelamento FR13/FR17)"}`, http.StatusForbidden)
			return
		}

		rawBytes, err := lerArquivoCSV(r)
		if err != nil {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		csvReader := csv.NewReader(strings.NewReader(string(rawBytes)))
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
		for _, c := range []string{"rede_nome", "cnpj", "cod_rca"} {
			if _, ok := colIdx[c]; !ok {
				http.Error(w, `{"error":"coluna obrigatória ausente no CSV: `+c+`"}`, http.StatusBadRequest)
				return
			}
		}

		var (
			rows       []clienteValidoRow
			erros      []clienteValidoLinhaErro
			linhaAtual = 1
		)
		cnpjsVistos := map[string]int{} // cnpj -> primeira linha vista (detecta duplicata dentro do próprio arquivo)
		for {
			record, err := csvReader.Read()
			if err == io.EOF {
				break
			}
			linhaAtual++
			if err != nil {
				erros = append(erros, clienteValidoLinhaErro{Linha: linhaAtual, Erro: "linha malformada: " + err.Error()})
				continue
			}
			get := func(col string) string {
				if idx, ok := colIdx[col]; ok && idx < len(record) {
					return strings.TrimSpace(record[idx])
				}
				return ""
			}
			row := clienteValidoRow{linha: linhaAtual, redeNome: get("rede_nome"), cnpj: get("cnpj"), codRCA: get("cod_rca")}
			linhaErro := false

			if row.redeNome == "" {
				erros = append(erros, clienteValidoLinhaErro{Linha: linhaAtual, Erro: "rede_nome é obrigatório"})
				linhaErro = true
			}
			cnpjLimpo := regexp.MustCompile(`\D`).ReplaceAllString(row.cnpj, "")
			if !cnpjSoDigitos.MatchString(cnpjLimpo) {
				erros = append(erros, clienteValidoLinhaErro{Linha: linhaAtual, Erro: "cnpj inválido (precisa ter 14 dígitos): " + row.cnpj})
				linhaErro = true
			}
			row.cnpj = cnpjLimpo
			// FR11: todo CNPJ deve ter RCA vinculado — regra de qualidade de
			// dado validada AQUI na importação, não tratada como fallback depois.
			if row.codRCA == "" {
				erros = append(erros, clienteValidoLinhaErro{Linha: linhaAtual, Erro: fmt.Sprintf("CNPJ %s sem RCA vinculado — todo CNPJ deve ter RCA (FR11)", row.cnpj)})
				linhaErro = true
			}
			if !linhaErro {
				if primeira, dup := cnpjsVistos[row.cnpj]; dup {
					erros = append(erros, clienteValidoLinhaErro{Linha: linhaAtual, Erro: fmt.Sprintf("CNPJ %s duplicado no arquivo (já aparece na linha %d)", row.cnpj, primeira)})
					linhaErro = true
				} else {
					cnpjsVistos[row.cnpj] = linhaAtual
				}
			}
			if !linhaErro {
				rows = append(rows, row)
			}
		}

		if len(rows) == 0 && len(erros) == 0 {
			http.Error(w, `{"error":"CSV vazio — nenhuma linha de dado encontrada"}`, http.StatusBadRequest)
			return
		}
		if len(erros) > 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]any{"erros": erros, "linhas_com_erro": len(erros)})
			return
		}

		tx, err := db.Begin()
		if err != nil {
			http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()

		if _, err := tx.Exec(`DELETE FROM farol.metas_clientes_validos WHERE vigencia_id = $1 AND empresa_id = $2`, vigenciaID, spCtx.EmpresaID); err != nil {
			http.Error(w, `{"error":"database error ao limpar lista anterior"}`, http.StatusInternalServerError)
			return
		}
		for _, row := range rows {
			if _, err := tx.Exec(`
				INSERT INTO farol.metas_clientes_validos (empresa_id, vinculo_id, vigencia_id, rede_nome, cnpj, cod_rca)
				VALUES ($1, $2, $3, $4, $5, $6)
			`, spCtx.EmpresaID, vinculoID, vigenciaID, row.redeNome, row.cnpj, row.codRCA); err != nil {
				http.Error(w, `{"error":"database error: `+err.Error()+`"}`, http.StatusInternalServerError)
				return
			}
		}

		if err := writeAuditLogTx(tx, spCtx.EmpresaID, spCtx.UserID, "metas_clientes_validos", strconv.Itoa(vigenciaID), "importar_csv", map[string]any{
			"vinculo_id": vinculoID, "vigencia_id": vigenciaID, "linhas": len(rows),
		}); err != nil {
			http.Error(w, `{"error":"erro ao gravar auditoria"}`, http.StatusInternalServerError)
			return
		}
		if err := tx.Commit(); err != nil {
			http.Error(w, `{"error":"commit error"}`, http.StatusInternalServerError)
			return
		}
		log.Printf("MetasClientesValidos: %d linhas importadas (vinculo=%d, vigencia=%d) empresa %s por %s", len(rows), vinculoID, vigenciaID, spCtx.EmpresaID, spCtx.UserID)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "clientes_importados": len(rows)})
	}
}

// MetasClientesValidosHandler — GET .../metas-clientes-validos?vigencia_id=
func MetasClientesValidosHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		vigenciaID := r.URL.Query().Get("vigencia_id")
		if vigenciaID == "" {
			http.Error(w, "vigencia_id é obrigatório", http.StatusBadRequest)
			return
		}
		rows, err := db.Query(`
			SELECT rede_nome, cnpj, cod_rca FROM farol.metas_clientes_validos
			WHERE vigencia_id = $1 AND empresa_id = $2
			ORDER BY rede_nome, cnpj
		`, vigenciaID, spCtx.EmpresaID)
		if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		type item struct {
			RedeNome string `json:"rede_nome"`
			CNPJ     string `json:"cnpj"`
			CodRCA   string `json:"cod_rca"`
		}
		lista := []item{}
		for rows.Next() {
			var it item
			if err := rows.Scan(&it.RedeNome, &it.CNPJ, &it.CodRCA); err != nil {
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
			lista = append(lista, it)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(lista)
	}
}
