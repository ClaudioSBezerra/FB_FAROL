package handlers

// farol_metas_ppas_csv.go — Importação de PPAs (Épico 7 addendum, Story 7.3,
// ver _bmad-output/planning-artifacts/epics.md)
//
// PPA = agrupamento de produto ("família de itens/EANs", termo do
// documento-fonte da Unilever) usado só pelo Sortimento Numérica — modelo
// de dados PRÓPRIO (farol.metas_ppas, migration 248), NÃO reaproveita a
// lista de Itens Válidos (farol_metas_itens_validos_csv.go, FR12) usada
// pelo Sortimento por Rede.
//
// Formato CSV (';'): cod_prod;ppa_nome;ean;embalagem;regiao;ae;bu
// Colunas obrigatórias (não-vazias): cod_prod, ppa_nome. ean/embalagem/
// regiao/ae/bu são informativas (não usadas pelo cálculo — ver
// contarPPAsPositivados em farol_metas_calculo_numerica.go, que lê
// embalagem/qt_unit_cx direto da venda bruta, mesmo padrão do Sortimento
// por Rede) — capturadas porque vêm no arquivo real da JC e podem virar
// relevantes depois (Questão em aberto #4 do PRD, colunas Região/AE/BU).
//
// Um cod_prod pertence a EXATAMENTE 1 PPA (UNIQUE vigencia_id+cod_prod);
// múltiplos cod_prod podem apontar pro mesmo PPA (várias embalagens da
// mesma família).
//
// Atômico (tudo-ou-nada, FR9/FR28) — mesmo racional de
// farol_metas_clientes_numericas_csv.go: módulo novo, sem histórico de
// sujeira de dado que justifique importação parcial.
//
// Rota: POST /api/farol/metas-ppas-importar-csv?vinculo_id=&vigencia_id=

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

type ppaLinhaErro struct {
	Linha int    `json:"linha"`
	Erro  string `json:"erro"`
}

type ppaRow struct {
	linha     int
	codProd   string
	ppaNome   string
	ean       string
	embalagem string
	regiao    string
	ae        string
	bu        string
}

// MetasPPAsImportarCSVHandler — POST .../metas-ppas-importar-csv
func MetasPPAsImportarCSVHandler(db *sql.DB) http.HandlerFunc {
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
			http.Error(w, `{"error":"vigência fechada — lista de PPAs não pode ser reimportada (congelamento FR13/FR17)"}`, http.StatusForbidden)
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
		colunasObrigatorias := []string{"cod_prod", "ppa_nome"}
		for _, c := range colunasObrigatorias {
			if _, ok := colIdx[c]; !ok {
				http.Error(w, `{"error":"coluna obrigatória ausente no CSV: `+c+`"}`, http.StatusBadRequest)
				return
			}
		}

		var (
			rows       []ppaRow
			erros      []ppaLinhaErro
			linhaAtual = 1
		)
		codProdsVistos := map[string]int{}
		for {
			record, err := csvReader.Read()
			if err == io.EOF {
				break
			}
			linhaAtual++
			if err != nil {
				erros = append(erros, ppaLinhaErro{Linha: linhaAtual, Erro: "linha malformada: " + err.Error()})
				continue
			}
			get := func(col string) string {
				if idx, ok := colIdx[col]; ok && idx < len(record) {
					return strings.TrimSpace(record[idx])
				}
				return ""
			}
			row := ppaRow{
				linha: linhaAtual, codProd: get("cod_prod"), ppaNome: get("ppa_nome"),
				ean: get("ean"), embalagem: get("embalagem"),
				regiao: get("regiao"), ae: get("ae"), bu: get("bu"),
			}

			if row.codProd == "" {
				erros = append(erros, ppaLinhaErro{Linha: linhaAtual, Erro: "cod_prod é obrigatório"})
				continue
			}
			if row.ppaNome == "" {
				erros = append(erros, ppaLinhaErro{Linha: linhaAtual, Erro: fmt.Sprintf("cod_prod %s sem ppa_nome — todo produto precisa pertencer a um PPA", row.codProd)})
				continue
			}
			if primeira, dup := codProdsVistos[row.codProd]; dup {
				erros = append(erros, ppaLinhaErro{Linha: linhaAtual, Erro: fmt.Sprintf("cod_prod %s duplicado no arquivo (já aparece na linha %d)", row.codProd, primeira)})
				continue
			}
			codProdsVistos[row.codProd] = linhaAtual
			rows = append(rows, row)
		}

		if len(erros) > 0 {
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

		if _, err := tx.Exec(`DELETE FROM farol.metas_ppas WHERE vigencia_id = $1 AND empresa_id = $2`, vigenciaID, spCtx.EmpresaID); err != nil {
			http.Error(w, `{"error":"database error ao limpar lista anterior"}`, http.StatusInternalServerError)
			return
		}
		for _, row := range rows {
			if _, err := tx.Exec(`
				INSERT INTO farol.metas_ppas (empresa_id, vinculo_id, vigencia_id, cod_prod, ppa_nome, ean, embalagem, regiao, ae, bu)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			`, spCtx.EmpresaID, vinculoID, vigenciaID, row.codProd, row.ppaNome, row.ean, row.embalagem, row.regiao, row.ae, row.bu); err != nil {
				http.Error(w, `{"error":"database error: `+err.Error()+`"}`, http.StatusInternalServerError)
				return
			}
		}

		if err := writeAuditLogTx(tx, spCtx.EmpresaID, spCtx.UserID, "metas_ppas", strconv.Itoa(vigenciaID), "importar_csv", map[string]any{
			"vinculo_id": vinculoID, "vigencia_id": vigenciaID, "linhas": len(rows),
		}); err != nil {
			http.Error(w, `{"error":"erro ao gravar auditoria"}`, http.StatusInternalServerError)
			return
		}
		if err := tx.Commit(); err != nil {
			http.Error(w, `{"error":"commit error"}`, http.StatusInternalServerError)
			return
		}
		log.Printf("MetasPPAs: %d linhas importadas (vinculo=%d, vigencia=%d) empresa %s por %s",
			len(rows), vinculoID, vigenciaID, spCtx.EmpresaID, spCtx.UserID)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "ppas_importados": len(rows)})
	}
}

// MetasPPAsHandler — GET .../metas-ppas?vigencia_id=
func MetasPPAsHandler(db *sql.DB) http.HandlerFunc {
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
			SELECT cod_prod, ppa_nome, ean, embalagem, regiao, ae, bu
			FROM farol.metas_ppas
			WHERE vigencia_id = $1 AND empresa_id = $2
			ORDER BY ppa_nome, cod_prod
		`, vigenciaID, spCtx.EmpresaID)
		if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		type item struct {
			CodProd   string `json:"cod_prod"`
			PPANome   string `json:"ppa_nome"`
			EAN       string `json:"ean"`
			Embalagem string `json:"embalagem"`
			Regiao    string `json:"regiao"`
			AE        string `json:"ae"`
			BU        string `json:"bu"`
		}
		lista := []item{}
		for rows.Next() {
			var it item
			if err := rows.Scan(&it.CodProd, &it.PPANome, &it.EAN, &it.Embalagem, &it.Regiao, &it.AE, &it.BU); err != nil {
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
			lista = append(lista, it)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(lista)
	}
}
