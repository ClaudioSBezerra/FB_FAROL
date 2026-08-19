package handlers

// farol_ai.go — Assistente Virtual IA para o Farol de Vendas.
//
// POST /api/v2/farol/ai/query   — pergunta em linguagem natural → SQL → JSON
// POST /api/v2/farol/ai/export  — mesma query → arquivo Excel (.xlsx)

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"fb_farol/services"

	"github.com/xuri/excelize/v2"
)

// ─── Validação SQL ─────────────────────────────────────────────────────────────

var sqlDenyList = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bINSERT\b`),
	regexp.MustCompile(`(?i)\bUPDATE\b`),
	regexp.MustCompile(`(?i)\bDELETE\b`),
	regexp.MustCompile(`(?i)\bDROP\b`),
	regexp.MustCompile(`(?i)\bALTER\b`),
	regexp.MustCompile(`(?i)\bCREATE\b`),
	regexp.MustCompile(`(?i)\bTRUNCATE\b`),
	regexp.MustCompile(`(?i)\bGRANT\b`),
	regexp.MustCompile(`(?i)\bREVOKE\b`),
}

func validateFarolSQL(s string) error {
	for _, re := range sqlDenyList {
		if re.MatchString(s) {
			return fmt.Errorf("operação não permitida no SQL gerado")
		}
	}
	return nil
}

var reEmpresaPlaceholder = regexp.MustCompile(`'?__EMPRESA(?:_ID(?:__)?)?'?`)

// ─── Tipos ────────────────────────────────────────────────────────────────────

type aiQueryReq struct {
	Pergunta string `json:"pergunta"`
}

type aiQueryResp struct {
	Pergunta string                   `json:"pergunta"`
	SQL      string                   `json:"sql"`
	Columns  []string                 `json:"columns"`
	Rows     []map[string]interface{} `json:"rows"`
	RowCount int                      `json:"row_count"`
	Model    string                   `json:"model"`
}

// ─── Query Handler ────────────────────────────────────────────────────────────

func FarolAIQueryHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		// Personas com escopo (GGV/supervisor/RCA) não usam o text-to-SQL.
		//
		// Aqui a IA escreve o SQL: o recorte por gerente/supervisor teria de ser
		// costurado dentro de uma consulta que ela montou livremente, e nenhum
		// filtro colado por fora sobrevive a uma subquery, CTE ou UNION que o
		// modelo resolva gerar. A pergunta "quanto vendeu o gerente 350?" viraria
		// SQL sem restrição nenhuma. Enquanto o escopo não puder ser garantido
		// dentro do próprio SQL, isto fica com quem já pode ver a empresa inteira.
		if escopoDoUsuario(db, spCtx, "").restrito() {
			log.Printf("[farol:ai] acesso negado — persona=%s user=%s (text-to-SQL não garante escopo)",
				spCtx.TipoPersona, spCtx.UserID)
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}

		var req aiQueryReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Pergunta) == "" {
			http.Error(w, `{"error":"pergunta inválida ou ausente"}`, http.StatusBadRequest)
			return
		}

		finalSQL, model, err := generateAndPrepareSQL(db, spCtx.EmpresaID, req.Pergunta)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusUnprocessableEntity)
			return
		}

		cols, rows, err := executeQuery(db, finalSQL)
		if err != nil {
			// Uma tentativa de conserto: devolve o erro do Postgres ao modelo e
			// pede a correção. O texto-para-SQL erra em detalhe de sintaxe muito
			// mais do que em entendimento da pergunta — UNION com ORDER BY sem
			// parênteses, por exemplo — e esse é justamente o tipo de coisa que
			// a mensagem de erro descreve com precisão suficiente pra corrigir.
			//
			// Uma tentativa só, de propósito: se a segunda falhar, o problema não
			// é sintaxe e insistir só faz o usuário esperar mais pelo mesmo erro.
			log.Printf("[farol:ai] SQL falhou (%v) — tentando corrigir", err)
			corrigido, errFix := corrigirSQL(spCtx.EmpresaID, req.Pergunta, finalSQL, err)
			if errFix == nil {
				if cols2, rows2, err2 := executeQuery(db, corrigido); err2 == nil {
					log.Printf("[farol:ai] correção funcionou")
					finalSQL, cols, rows, err = corrigido, cols2, rows2, nil
				}
			}
		}
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q,"sql":%q}`, err.Error(), finalSQL), http.StatusBadRequest)
			return
		}

		json.NewEncoder(w).Encode(aiQueryResp{
			Pergunta: req.Pergunta,
			SQL:      finalSQL,
			Columns:  cols,
			Rows:     rows,
			RowCount: len(rows),
			Model:    model,
		})
	}
}

// ─── Export Excel Handler ────────────────────────────────────────────────────

func FarolAIExportHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		// Personas com escopo (GGV/supervisor/RCA) não usam o text-to-SQL.
		//
		// Aqui a IA escreve o SQL: o recorte por gerente/supervisor teria de ser
		// costurado dentro de uma consulta que ela montou livremente, e nenhum
		// filtro colado por fora sobrevive a uma subquery, CTE ou UNION que o
		// modelo resolva gerar. A pergunta "quanto vendeu o gerente 350?" viraria
		// SQL sem restrição nenhuma. Enquanto o escopo não puder ser garantido
		// dentro do próprio SQL, isto fica com quem já pode ver a empresa inteira.
		if escopoDoUsuario(db, spCtx, "").restrito() {
			log.Printf("[farol:ai] acesso negado — persona=%s user=%s (text-to-SQL não garante escopo)",
				spCtx.TipoPersona, spCtx.UserID)
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}

		var req aiQueryReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Pergunta) == "" {
			http.Error(w, `{"error":"pergunta inválida ou ausente"}`, http.StatusBadRequest)
			return
		}

		finalSQL, _, err := generateAndPrepareSQL(db, spCtx.EmpresaID, req.Pergunta)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusUnprocessableEntity)
			return
		}

		cols, rows, err := executeQuery(db, finalSQL)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
			return
		}

		xlsx, err := buildExcel(req.Pergunta, finalSQL, cols, rows)
		if err != nil {
			http.Error(w, `{"error":"falha ao gerar Excel"}`, http.StatusInternalServerError)
			return
		}

		filename := fmt.Sprintf("farol_consulta_%s.xlsx", time.Now().Format("20060102_150405"))
		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
		xlsx.Write(w)
	}
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func generateAndPrepareSQL(db *sql.DB, empresaID, pergunta string) (string, string, error) {
	ai := services.NewZAIClient()
	if !ai.IsAvailable() {
		return "", "", fmt.Errorf("assistente IA não configurado (ZAI_API_KEY ausente)")
	}

	result, err := ai.Ask(
		services.FarolTextToSQLSystem,
		services.BuildFarolSQLPrompt(pergunta),
		"", 2048,
	)
	if err != nil {
		return "", "", fmt.Errorf("erro na IA: %v", err)
	}

	rawSQL, err := services.ExtractFarolSQL(result.Text)
	if err != nil {
		return "", "", fmt.Errorf("IA não retornou SQL válido — tente reformular a pergunta")
	}

	if err := validateFarolSQL(rawSQL); err != nil {
		return "", "", err
	}

	// Injeta empresa_id real no lugar do placeholder
	finalSQL := reEmpresaPlaceholder.ReplaceAllString(rawSQL, "'"+empresaID+"'")
	if strings.Contains(finalSQL, "__EMPRESA") {
		return "", "", fmt.Errorf("SQL gerado contém placeholder não resolvido — tente reformular")
	}

	finalSQL = garantirLimite(finalSQL)

	return finalSQL, result.Model, nil
}

// corrigirSQL — segunda passada no modelo com o SQL que falhou e o erro do
// banco. Passa pelas MESMAS validações da primeira: uma resposta de correção é
// tão não-confiável quanto a original, e nada garante que ela não venha com um
// DELETE dentro.
// garantirLimite — teto de linhas quando a IA esquece o LIMIT.
//
// O ponto e vírgula importa: concatenar "\nLIMIT 200" depois dele produz um
// segundo comando órfão e a query inteira falha com erro de sintaxe. Nenhum
// exemplo do prompt terminava sem LIMIT, então isso nunca disparou — até o
// exemplo de ROW_NUMBER, que naturalmente não tem.
func garantirLimite(q string) string {
	if strings.Contains(strings.ToUpper(q), "LIMIT") {
		return q
	}
	q = strings.TrimRight(q, " \t\r\n")
	q = strings.TrimSuffix(q, ";")
	return q + "\nLIMIT 200;"
}

func corrigirSQL(empresaID, pergunta, sqlRuim string, erroBanco error) (string, error) {
	ai := services.NewZAIClient()
	if !ai.IsAvailable() {
		return "", fmt.Errorf("IA indisponível")
	}

	result, err := ai.Ask(
		services.FarolTextToSQLSystem,
		services.BuildFarolSQLFixPrompt(pergunta, sqlRuim, erroBanco.Error()),
		"", 2048,
	)
	if err != nil {
		return "", err
	}

	rawSQL, err := services.ExtractFarolSQL(result.Text)
	if err != nil {
		return "", err
	}
	if err := validateFarolSQL(rawSQL); err != nil {
		return "", err
	}

	fixed := reEmpresaPlaceholder.ReplaceAllString(rawSQL, "'"+empresaID+"'")
	if strings.Contains(fixed, "__EMPRESA") {
		return "", fmt.Errorf("placeholder não resolvido")
	}
	fixed = garantirLimite(fixed)
	return fixed, nil
}

func executeQuery(db *sql.DB, sqlStr string) ([]string, []map[string]interface{}, error) {
	rows, err := db.Query(sqlStr)
	if err != nil {
		return nil, nil, fmt.Errorf("erro ao executar consulta: %v", err)
	}
	defer rows.Close()

	cols, _ := rows.Columns()
	var result []map[string]interface{}
	for rows.Next() {
		vals := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			continue
		}
		row := make(map[string]interface{})
		for i, col := range cols {
			if b, ok := vals[i].([]byte); ok {
				row[col] = string(b)
			} else {
				row[col] = vals[i]
			}
		}
		result = append(result, row)
	}
	if result == nil {
		result = []map[string]interface{}{}
	}
	return cols, result, nil
}

func buildExcel(pergunta, sqlUsado string, cols []string, rows []map[string]interface{}) (*excelize.File, error) {
	f := excelize.NewFile()

	// ── Aba de dados ──────────────────────────────────────────────────────────
	sheet := "Dados"
	f.SetSheetName("Sheet1", sheet)

	// Estilos
	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "FFFFFF", Size: 11},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"1e293b"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center"},
		Border: []excelize.Border{
			{Type: "bottom", Color: "94a3b8", Style: 1},
		},
	})
	altStyle, _ := f.NewStyle(&excelize.Style{
		Fill: excelize.Fill{Type: "pattern", Color: []string{"f8fafc"}, Pattern: 1},
	})

	// Cabeçalho
	for j, col := range cols {
		cell, _ := excelize.CoordinatesToCellName(j+1, 1)
		f.SetCellValue(sheet, cell, strings.ToUpper(col))
		f.SetCellStyle(sheet, cell, cell, headerStyle)
	}

	// Linhas de dados
	for i, row := range rows {
		for j, col := range cols {
			cell, _ := excelize.CoordinatesToCellName(j+1, i+2)
			f.SetCellValue(sheet, cell, fmt.Sprintf("%v", row[col]))
			if i%2 == 1 {
				f.SetCellStyle(sheet, cell, cell, altStyle)
			}
		}
	}

	// Ajuste automático de largura de colunas
	for j, col := range cols {
		colLetter, _ := excelize.ColumnNumberToName(j + 1)
		width := max(len(col)+4, 12)
		if width > 40 {
			width = 40
		}
		f.SetColWidth(sheet, colLetter, colLetter, float64(width))
	}

	// Freeze header row
	f.SetPanes(sheet, &excelize.Panes{
		Freeze:      true,
		YSplit:      1,
		TopLeftCell: "A2",
		ActivePane:  "bottomLeft",
	})

	// ── Aba de informações ────────────────────────────────────────────────────
	infoSheet := "Informações"
	f.NewSheet(infoSheet)

	infoLabelStyle, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"f1f5f9"}, Pattern: 1},
	})

	infos := [][]string{
		{"Pergunta", pergunta},
		{"Gerado em", time.Now().Format("02/01/2006 15:04:05")},
		{"Total de linhas", fmt.Sprintf("%d", len(rows))},
		{"SQL utilizado", sqlUsado},
	}
	for i, info := range infos {
		cellA, _ := excelize.CoordinatesToCellName(1, i+1)
		cellB, _ := excelize.CoordinatesToCellName(2, i+1)
		f.SetCellValue(infoSheet, cellA, info[0])
		f.SetCellValue(infoSheet, cellB, info[1])
		f.SetCellStyle(infoSheet, cellA, cellA, infoLabelStyle)
	}
	f.SetColWidth(infoSheet, "A", "A", 20)
	f.SetColWidth(infoSheet, "B", "B", 80)

	f.SetActiveSheet(0)
	return f, nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
