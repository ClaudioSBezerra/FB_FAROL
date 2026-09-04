package handlers

// farol_metas_itens_validos_csv.go — Importação de Itens Válidos (EAN) —
// Épico 3, Story 3.3, módulo Painel de Gestão de Metas por Indústria.
//
// Mesmo modelo/padrão de atomicidade das Stories 3.1/3.2: escopo por
// vinculo_id/vigencia_id (query string), validação completa antes de
// qualquer escrita, reimportação substitui a lista da vigência, vigência
// fechada bloqueia.
//
// Formato CSV (';'): ean;cod_prod
// Um EAN pode aparecer em várias linhas (mapeando pra cod_prod diferentes —
// variantes/embalagens do mesmo item; sem limite de quantas — BASE EANS da
// JC já mostra itens com mais de 2 códigos JC pro mesmo EAN).
//
// A coluna tipo_embalagem que existia aqui até 2026-09-04 foi REMOVIDA
// (migration 225) — orientação direta do Heverton: essa informação nunca
// deveria ter sido pedida à JC no CSV mensal, ela já existe no cadastro de
// produto importado todo dia na carga de vendas (embalagem/qt_unit_cx,
// migration 168). O motor de apuração (farol_metas_calculo.go) resolve
// isso via JOIN com vendas_faturadas/transmitidas, não mais por aqui.
//
// Rota: POST /api/farol/metas-itens-validos-importar-csv?vinculo_id=&vigencia_id=

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

type itemValidoLinhaErro struct {
	Linha int    `json:"linha"`
	Erro  string `json:"erro"`
}

type itemValidoRow struct {
	linha   int
	ean     string
	codProd string
}

// MetasItensValidosImportarCSVHandler — POST .../metas-itens-validos-importar-csv
func MetasItensValidosImportarCSVHandler(db *sql.DB) http.HandlerFunc {
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
			http.Error(w, `{"error":"vigência fechada — lista de Itens Válidos não pode ser reimportada (congelamento FR13/FR17)"}`, http.StatusForbidden)
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
		for _, c := range []string{"ean", "cod_prod"} {
			if _, ok := colIdx[c]; !ok {
				http.Error(w, `{"error":"coluna obrigatória ausente no CSV: `+c+`"}`, http.StatusBadRequest)
				return
			}
		}

		var (
			rows       []itemValidoRow
			erros      []itemValidoLinhaErro
			linhaAtual = 1
		)
		combinacoesVistas := map[string]int{} // "ean|codProd" -> primeira linha
		for {
			record, err := csvReader.Read()
			if err == io.EOF {
				break
			}
			linhaAtual++
			if err != nil {
				erros = append(erros, itemValidoLinhaErro{Linha: linhaAtual, Erro: "linha malformada: " + err.Error()})
				continue
			}
			get := func(col string) string {
				if idx, ok := colIdx[col]; ok && idx < len(record) {
					return strings.TrimSpace(record[idx])
				}
				return ""
			}
			row := itemValidoRow{
				linha: linhaAtual, ean: get("ean"), codProd: get("cod_prod"),
			}
			linhaErro := false

			if row.ean == "" {
				erros = append(erros, itemValidoLinhaErro{Linha: linhaAtual, Erro: "ean é obrigatório"})
				linhaErro = true
			}
			if row.codProd == "" {
				erros = append(erros, itemValidoLinhaErro{Linha: linhaAtual, Erro: "cod_prod é obrigatório"})
				linhaErro = true
			}
			if !linhaErro {
				chave := row.ean + "|" + row.codProd
				if primeira, dup := combinacoesVistas[chave]; dup {
					erros = append(erros, itemValidoLinhaErro{Linha: linhaAtual, Erro: fmt.Sprintf("combinação EAN %s + cod_prod %s duplicada no arquivo (já aparece na linha %d)", row.ean, row.codProd, primeira)})
					linhaErro = true
				} else {
					combinacoesVistas[chave] = linhaAtual
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

		if _, err := tx.Exec(`DELETE FROM farol.metas_itens_validos WHERE vigencia_id = $1 AND empresa_id = $2`, vigenciaID, spCtx.EmpresaID); err != nil {
			http.Error(w, `{"error":"database error ao limpar lista anterior"}`, http.StatusInternalServerError)
			return
		}
		for _, row := range rows {
			if _, err := tx.Exec(`
				INSERT INTO farol.metas_itens_validos (empresa_id, vinculo_id, vigencia_id, ean, cod_prod)
				VALUES ($1, $2, $3, $4, $5)
			`, spCtx.EmpresaID, vinculoID, vigenciaID, row.ean, row.codProd); err != nil {
				http.Error(w, `{"error":"database error: `+err.Error()+`"}`, http.StatusInternalServerError)
				return
			}
		}

		if err := writeAuditLogTx(tx, spCtx.EmpresaID, spCtx.UserID, "metas_itens_validos", strconv.Itoa(vigenciaID), "importar_csv", map[string]any{
			"vinculo_id": vinculoID, "vigencia_id": vigenciaID, "linhas": len(rows),
		}); err != nil {
			http.Error(w, `{"error":"erro ao gravar auditoria"}`, http.StatusInternalServerError)
			return
		}
		if err := tx.Commit(); err != nil {
			http.Error(w, `{"error":"commit error"}`, http.StatusInternalServerError)
			return
		}
		log.Printf("MetasItensValidos: %d linhas importadas (vinculo=%d, vigencia=%d) empresa %s por %s", len(rows), vinculoID, vigenciaID, spCtx.EmpresaID, spCtx.UserID)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "itens_importados": len(rows)})
	}
}

// MetasItensValidosHandler — GET .../metas-itens-validos?vigencia_id=
func MetasItensValidosHandler(db *sql.DB) http.HandlerFunc {
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
			SELECT ean, cod_prod FROM farol.metas_itens_validos
			WHERE vigencia_id = $1 AND empresa_id = $2
			ORDER BY ean, cod_prod
		`, vigenciaID, spCtx.EmpresaID)
		if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		type item struct {
			EAN     string `json:"ean"`
			CodProd string `json:"cod_prod"`
		}
		lista := []item{}
		for rows.Next() {
			var it item
			if err := rows.Scan(&it.EAN, &it.CodProd); err != nil {
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
			lista = append(lista, it)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(lista)
	}
}
