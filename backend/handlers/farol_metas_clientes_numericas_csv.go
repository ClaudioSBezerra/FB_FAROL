package handlers

// farol_metas_clientes_numericas_csv.go — Importação de Clientes Numéricas
// (Épico 7 addendum, Story 7.2, ver _bmad-output/planning-artifacts/epics.md)
//
// Formato DIFERENTE de Clientes Válidos (farol_metas_clientes_validos_csv.go,
// FR11): CNPJ individual com Classificação PDV, SEM cod_princ/Rede (FR24,
// FR27) — a Numérica não tem conceito de Rede. GGV/CRV/RCA vêm PRONTOS do
// arquivo da JC (denormalizados), não resolvidos por JOIN com a hierarquia
// organizacional do Farol como Clientes Válidos faz.
//
// Diferente da importação de Clientes Válidos (que desde 08/09/2026 aceita
// lote PARCIAL, linha com erro vira só aviso — ver farol_metas_clientes_validos_csv.go),
// esta é ATÔMICA (tudo-ou-nada, FR9/FR27): a Numérica é módulo novo, sem
// histórico real de sujeira de dado que justifique a mesma flexibilização —
// revisitar se a JC mandar um arquivo real com linhas problemáticas.
//
// Formato CSV (';'): cnpj;cod_cl;classificacao_pdv;razao;fantasia;cod_ggv;nome_ggv;cod_crv;nome_crv;cod_rca;nome_rca
// Colunas obrigatórias (não-vazias): cnpj, classificacao_pdv (precisa ser
// exatamente "Num. A", "Num. B" ou "Num. C" — mesmo texto da planilha da JC,
// CHECK constraint na migration 248).
//
// Uma nova importação SUBSTITUI a lista anterior da mesma vigência (mesmo
// princípio do PUT-replace de farol_industrias.go e de Clientes Válidos).
//
// Rota: POST /api/farol/metas-clientes-numericas-importar-csv?vinculo_id=&vigencia_id=

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
)

var classificacoesPDVValidas = map[string]bool{"Num. A": true, "Num. B": true, "Num. C": true}

type clienteNumericaLinhaErro struct {
	Linha int    `json:"linha"`
	Erro  string `json:"erro"`
}

type clienteNumericaRow struct {
	linha         int
	cnpj          string
	codCl         string
	classificacao string
	razao         string
	fantasia      string
	codGGV        string
	nomeGGV       string
	codCRV        string
	nomeCRV       string
	codRCA        string
	nomeRCA       string
}

// MetasClientesNumericasImportarCSVHandler — POST .../metas-clientes-numericas-importar-csv
func MetasClientesNumericasImportarCSVHandler(db *sql.DB) http.HandlerFunc {
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
			http.Error(w, `{"error":"vigência fechada — lista de Clientes Numéricas não pode ser reimportada (congelamento FR13/FR17)"}`, http.StatusForbidden)
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
		colunasObrigatorias := []string{"cnpj", "classificacao_pdv"}
		for _, c := range colunasObrigatorias {
			if _, ok := colIdx[c]; !ok {
				http.Error(w, `{"error":"coluna obrigatória ausente no CSV: `+c+`"}`, http.StatusBadRequest)
				return
			}
		}

		var (
			rows       []clienteNumericaRow
			erros      []clienteNumericaLinhaErro
			linhaAtual = 1
		)
		cnpjsVistos := map[string]int{}
		for {
			record, err := csvReader.Read()
			if err == io.EOF {
				break
			}
			linhaAtual++
			if err != nil {
				erros = append(erros, clienteNumericaLinhaErro{Linha: linhaAtual, Erro: "linha malformada: " + err.Error()})
				continue
			}
			get := func(col string) string {
				if idx, ok := colIdx[col]; ok && idx < len(record) {
					return strings.TrimSpace(record[idx])
				}
				return ""
			}
			row := clienteNumericaRow{
				linha: linhaAtual, cnpj: get("cnpj"), codCl: get("cod_cl"), classificacao: get("classificacao_pdv"),
				razao: get("razao"), fantasia: get("fantasia"),
				codGGV: get("cod_ggv"), nomeGGV: get("nome_ggv"),
				codCRV: get("cod_crv"), nomeCRV: get("nome_crv"),
				codRCA: get("cod_rca"), nomeRCA: get("nome_rca"),
			}

			cnpjLimpo := digitosApenas.ReplaceAllString(row.cnpj, "")
			if !cnpjSoDigitos.MatchString(cnpjLimpo) {
				erros = append(erros, clienteNumericaLinhaErro{Linha: linhaAtual, Erro: "cnpj inválido (precisa ter 14 dígitos): " + row.cnpj})
				continue
			}
			row.cnpj = cnpjLimpo
			if !classificacoesPDVValidas[row.classificacao] {
				erros = append(erros, clienteNumericaLinhaErro{Linha: linhaAtual, Erro: fmt.Sprintf("CNPJ %s com classificacao_pdv inválida: %q (use exatamente \"Num. A\", \"Num. B\" ou \"Num. C\")", row.cnpj, row.classificacao)})
				continue
			}
			if primeira, dup := cnpjsVistos[row.cnpj]; dup {
				erros = append(erros, clienteNumericaLinhaErro{Linha: linhaAtual, Erro: fmt.Sprintf("CNPJ %s duplicado no arquivo (já aparece na linha %d)", row.cnpj, primeira)})
				continue
			}
			cnpjsVistos[row.cnpj] = linhaAtual
			rows = append(rows, row)
		}

		if len(erros) > 0 {
			// Atômico (FR9/FR27, ver cabeçalho) — diferente de Clientes
			// Válidos, nenhuma linha é aplicada se QUALQUER uma tiver erro.
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]any{"erros": erros, "linhas_com_erro": len(erros)})
			return
		}
		if len(rows) == 0 {
			http.Error(w, `{"error":"CSV vazio — nenhuma linha de dado encontrada"}`, http.StatusBadRequest)
			return
		}

		tx, err := db.Begin()
		if err != nil {
			http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()

		if _, err := tx.Exec(`DELETE FROM farol.metas_clientes_numericas WHERE vigencia_id = $1 AND empresa_id = $2`, vigenciaID, spCtx.EmpresaID); err != nil {
			http.Error(w, `{"error":"database error ao limpar lista anterior"}`, http.StatusInternalServerError)
			return
		}
		for _, row := range rows {
			if _, err := tx.Exec(`
				INSERT INTO farol.metas_clientes_numericas
					(empresa_id, vinculo_id, vigencia_id, cnpj, cod_cl, classificacao_pdv, razao, fantasia, cod_ggv, nome_ggv, cod_crv, nome_crv, cod_rca, nome_rca)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
			`, spCtx.EmpresaID, vinculoID, vigenciaID, row.cnpj, row.codCl, row.classificacao, row.razao, row.fantasia,
				row.codGGV, row.nomeGGV, row.codCRV, row.nomeCRV, row.codRCA, row.nomeRCA); err != nil {
				http.Error(w, `{"error":"database error: `+err.Error()+`"}`, http.StatusInternalServerError)
				return
			}
		}

		if err := writeAuditLogTx(tx, spCtx.EmpresaID, spCtx.UserID, "metas_clientes_numericas", strconv.Itoa(vigenciaID), "importar_csv", map[string]any{
			"vinculo_id": vinculoID, "vigencia_id": vigenciaID, "linhas": len(rows),
		}); err != nil {
			http.Error(w, `{"error":"erro ao gravar auditoria"}`, http.StatusInternalServerError)
			return
		}
		if err := tx.Commit(); err != nil {
			http.Error(w, `{"error":"commit error"}`, http.StatusInternalServerError)
			return
		}
		log.Printf("MetasClientesNumericas: %d linhas importadas (vinculo=%d, vigencia=%d) empresa %s por %s",
			len(rows), vinculoID, vigenciaID, spCtx.EmpresaID, spCtx.UserID)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "clientes_importados": len(rows)})
	}
}

// MetasClientesNumericasHandler — GET .../metas-clientes-numericas?vigencia_id=
func MetasClientesNumericasHandler(db *sql.DB) http.HandlerFunc {
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
			SELECT cnpj, cod_cl, classificacao_pdv, razao, fantasia, cod_ggv, nome_ggv, cod_crv, nome_crv, cod_rca, nome_rca
			FROM farol.metas_clientes_numericas
			WHERE vigencia_id = $1 AND empresa_id = $2
			ORDER BY classificacao_pdv, cnpj
		`, vigenciaID, spCtx.EmpresaID)
		if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		type item struct {
			CNPJ          string `json:"cnpj"`
			CodCl         string `json:"cod_cl"`
			Classificacao string `json:"classificacao_pdv"`
			Razao         string `json:"razao"`
			Fantasia      string `json:"fantasia"`
			CodGGV        string `json:"cod_ggv"`
			NomeGGV       string `json:"nome_ggv"`
			CodCRV        string `json:"cod_crv"`
			NomeCRV       string `json:"nome_crv"`
			CodRCA        string `json:"cod_rca"`
			NomeRCA       string `json:"nome_rca"`
		}
		lista := []item{}
		for rows.Next() {
			var it item
			if err := rows.Scan(&it.CNPJ, &it.CodCl, &it.Classificacao, &it.Razao, &it.Fantasia, &it.CodGGV, &it.NomeGGV, &it.CodCRV, &it.NomeCRV, &it.CodRCA, &it.NomeRCA); err != nil {
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
			lista = append(lista, it)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(lista)
	}
}
