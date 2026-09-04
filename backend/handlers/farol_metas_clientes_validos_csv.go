package handlers

// farol_metas_clientes_validos_csv.go — Importação de Clientes Válidos
// (Redes + hierarquia GGV/CRV/RCA responsável) — Épico 3, Story 3.2, módulo
// Painel de Gestão de Metas por Indústria.
//
// Formato reformulado em 2026-09-04 (orientação direta do Heverton +
// confronto com o modelo real da JC, aba "BASE DE LOJAS" do arquivo "Unico
// Acompanhamento Ponderadas Unilever HC_V1.xlsx"): a lista mensal não traz
// só "rede_nome" livre e "cod_rca" (formato original da Story 3.2) — ela
// traz o COD PRINC (chave real da Rede — Claudio confirmou "Rede é
// COD_PRINC, inclusive podem ter clientes que são redes apontando para ele
// mesmo") e o trio GGV/CRV/RCA (código+nome) já resolvido por CNPJ. Ver
// migration 224 pro racional completo da mudança de schema.
//
// Mesmo padrão de atomicidade estrita da Story 3.1 (FR9): valida tudo,
// aplica tudo ou nada. Uma nova importação SUBSTITUI a lista anterior da
// mesma vigência (mesmo princípio do PUT-replace de farol_industrias.go).
//
// Formato CSV (';'): cnpj;cod_princ;razao;fantasia;cod_ggv;nome_ggv;cod_crv;nome_crv;cod_rca;nome_rca
// Colunas obrigatórias (não-vazias): cnpj, cod_princ, cod_ggv, cod_crv, cod_rca
// (são as chaves usadas pro rollup GGV→CRV→RCA→Rede — sem elas o painel não
// tem como agrupar). razao/fantasia/nome_ggv/nome_crv/nome_rca são só
// rótulo de exibição — podem vir vazios sem travar a importação.
//
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
var digitosApenas = regexp.MustCompile(`\D`)

type clienteValidoLinhaErro struct {
	Linha int    `json:"linha"`
	Erro  string `json:"erro"`
}

type clienteValidoRow struct {
	linha    int
	cnpj     string
	codPrinc string
	razao    string
	fantasia string
	codGGV   string
	nomeGGV  string
	codCRV   string
	nomeCRV  string
	codRCA   string
	nomeRCA  string
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
		colunasObrigatorias := []string{"cnpj", "cod_princ", "cod_ggv", "cod_crv", "cod_rca"}
		for _, c := range colunasObrigatorias {
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
			row := clienteValidoRow{
				linha: linhaAtual, cnpj: get("cnpj"), codPrinc: get("cod_princ"),
				razao: get("razao"), fantasia: get("fantasia"),
				codGGV: get("cod_ggv"), nomeGGV: get("nome_ggv"),
				codCRV: get("cod_crv"), nomeCRV: get("nome_crv"),
				codRCA: get("cod_rca"), nomeRCA: get("nome_rca"),
			}
			linhaErro := false

			cnpjLimpo := digitosApenas.ReplaceAllString(row.cnpj, "")
			if !cnpjSoDigitos.MatchString(cnpjLimpo) {
				erros = append(erros, clienteValidoLinhaErro{Linha: linhaAtual, Erro: "cnpj inválido (precisa ter 14 dígitos): " + row.cnpj})
				linhaErro = true
			}
			row.cnpj = cnpjLimpo
			// Rede = COD_PRINC (decisão 2026-09-04) — uma Rede pode ter 1 CNPJ
			// só, apontando pra si mesma (COD_PRINC == COD_CL do próprio
			// cliente), mas o campo em si nunca pode faltar.
			if row.codPrinc == "" {
				erros = append(erros, clienteValidoLinhaErro{Linha: linhaAtual, Erro: "cod_princ é obrigatório"})
				linhaErro = true
			}
			// FR11 (ampliado): todo CNPJ precisa vir com o trio GGV/CRV/RCA já
			// resolvido — não é mais derivado por JOIN na hierarquia de vendas
			// (ver racional na migration 224), então precisa vir completo aqui.
			if row.codGGV == "" {
				erros = append(erros, clienteValidoLinhaErro{Linha: linhaAtual, Erro: fmt.Sprintf("CNPJ %s sem cod_ggv — todo CNPJ deve ter GGV vinculado", row.cnpj)})
				linhaErro = true
			}
			if row.codCRV == "" {
				erros = append(erros, clienteValidoLinhaErro{Linha: linhaAtual, Erro: fmt.Sprintf("CNPJ %s sem cod_crv — todo CNPJ deve ter CRV vinculado", row.cnpj)})
				linhaErro = true
			}
			if row.codRCA == "" {
				erros = append(erros, clienteValidoLinhaErro{Linha: linhaAtual, Erro: fmt.Sprintf("CNPJ %s sem cod_rca — todo CNPJ deve ter RCA vinculado (FR11)", row.cnpj)})
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
				INSERT INTO farol.metas_clientes_validos
					(empresa_id, vinculo_id, vigencia_id, cnpj, cod_princ, razao, fantasia, cod_ggv, nome_ggv, cod_crv, nome_crv, cod_rca, nome_rca)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
			`, spCtx.EmpresaID, vinculoID, vigenciaID, row.cnpj, row.codPrinc, row.razao, row.fantasia,
				row.codGGV, row.nomeGGV, row.codCRV, row.nomeCRV, row.codRCA, row.nomeRCA); err != nil {
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
			SELECT cnpj, cod_princ, razao, fantasia, cod_ggv, nome_ggv, cod_crv, nome_crv, cod_rca, nome_rca
			FROM farol.metas_clientes_validos
			WHERE vigencia_id = $1 AND empresa_id = $2
			ORDER BY cod_princ, cnpj
		`, vigenciaID, spCtx.EmpresaID)
		if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		type item struct {
			CNPJ     string `json:"cnpj"`
			CodPrinc string `json:"cod_princ"`
			Razao    string `json:"razao"`
			Fantasia string `json:"fantasia"`
			CodGGV   string `json:"cod_ggv"`
			NomeGGV  string `json:"nome_ggv"`
			CodCRV   string `json:"cod_crv"`
			NomeCRV  string `json:"nome_crv"`
			CodRCA   string `json:"cod_rca"`
			NomeRCA  string `json:"nome_rca"`
		}
		lista := []item{}
		for rows.Next() {
			var it item
			if err := rows.Scan(&it.CNPJ, &it.CodPrinc, &it.Razao, &it.Fantasia, &it.CodGGV, &it.NomeGGV, &it.CodCRV, &it.NomeCRV, &it.CodRCA, &it.NomeRCA); err != nil {
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
			lista = append(lista, it)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(lista)
	}
}
