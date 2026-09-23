package handlers

// farol_gamificacao_excel.go — exporta um extrato de Gamificação (já
// gerado, snapshot imutável — ver GamifExtratosHandler em
// farol_gamificacao.go) como planilha .xlsx com a logo da empresa. Pedido
// do Claudio 23/09/2026: "relatório de extrato por campanha, exportável
// pra Excel com a logo da empresa" — o documento de verdade que vai pra
// gestores/RH/jurídico, não só o CSV cru que já existia no frontend.

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
)

// extensaoImagem mapeia o mime salvo em companies.logo_mime pra uma das
// extensões que excelize.AddPictureFromBytes aceita. "" quando o mime não é
// suportado — o chamador simplesmente pula a imagem nesse caso.
func extensaoImagem(mime string) string {
	// excelize espera a extensão COM o ponto (".png", não "png") — confirmado
	// lendo supportedImageTypes em templates.go da própria lib.
	switch strings.ToLower(mime) {
	case "image/png":
		return ".png"
	case "image/jpeg", "image/jpg":
		return ".jpeg"
	case "image/gif":
		return ".gif"
	default:
		return ""
	}
}

// montarExtratoExcel gera o *excelize.File — separado do handler HTTP pra
// poder ser testado sem precisar de request/response.
func montarExtratoExcel(campanhaNome, industriaNome, dataInicio, dataFim string, geradoEm time.Time, geradoPor string, linhas []GamifExtratoLinha, logoData []byte, logoMime string) (*excelize.File, error) {
	f := excelize.NewFile()
	sheet := "Extrato"
	f.SetSheetName("Sheet1", sheet)

	// Linha 1-4 reservadas pro cabeçalho (logo + identificação da campanha);
	// a tabela de dados começa na linha 6, deixando a logo respirar.
	tituloStyle, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Size: 14}})
	subStyle, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Size: 10, Color: "64748b"}})

	f.SetCellValue(sheet, "C1", campanhaNome)
	f.SetCellStyle(sheet, "C1", "C1", tituloStyle)
	f.SetCellValue(sheet, "C2", fmt.Sprintf("%s · %s – %s", industriaNome, dataInicio, dataFim))
	f.SetCellStyle(sheet, "C2", "C2", subStyle)
	f.SetCellValue(sheet, "C3", fmt.Sprintf("Extrato gerado em %s por %s", geradoEm.Format("02/01/2006 15:04"), geradoPor))
	f.SetCellStyle(sheet, "C3", "C3", subStyle)
	f.SetCellValue(sheet, "C4", "Documento gerado automaticamente — snapshot imutável, não recalcula depois de gerado.")
	f.SetCellStyle(sheet, "C4", "C4", subStyle)

	if ext := extensaoImagem(logoMime); ext != "" && len(logoData) > 0 {
		if err := f.AddPictureFromBytes(sheet, "A1", &excelize.Picture{
			Extension: ext,
			File:      logoData,
			Format:    &excelize.GraphicOptions{AutoFit: true, AutoFitIgnoreAspect: false},
		}); err != nil {
			return nil, fmt.Errorf("inserir logo: %w", err)
		}
		f.SetRowHeight(sheet, 1, 60)
		f.SetColWidth(sheet, "A", "B", 12)
	}

	headerRow := 6
	headers := []string{"RCA", "Nome", "Nível", "% do objetivo", "Pontos", "Bônus (R$)", "Volume (desempate)"}
	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "FFFFFF", Size: 11},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"1e293b"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center"},
	})
	for j, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(j+1, headerRow)
		f.SetCellValue(sheet, cell, h)
		f.SetCellStyle(sheet, cell, cell, headerStyle)
	}

	altStyle, _ := f.NewStyle(&excelize.Style{Fill: excelize.Fill{Type: "pattern", Color: []string{"f8fafc"}, Pattern: 1}})
	// CustomNumFmt em vez do NumFmt embutido 44 (que é formato de moeda em
	// USD/accounting, sem jeito de trocar o símbolo) — aqui o valor já é
	// sempre R$, igual o resto do app (ver fmtBRL no frontend).
	bonusFmt := `"R$" #,##0.00`
	bonusStyle, _ := f.NewStyle(&excelize.Style{CustomNumFmt: &bonusFmt})
	nivelNome := map[string]string{"bronze": "Bronze", "prata": "Prata", "ouro": "Ouro", "diamante": "Diamante"}
	for i, l := range linhas {
		row := headerRow + 1 + i
		nivel := nivelNome[l.NivelPrincipal]
		if nivel == "" {
			nivel = "—"
		}
		valores := []any{l.CodRCA, l.NomeRCA, nivel, l.PercentualPrincipal / 100, l.PontosTotal, l.BonusTotal, l.VolumeDesempate}
		for j, v := range valores {
			cell, _ := excelize.CoordinatesToCellName(j+1, row)
			f.SetCellValue(sheet, cell, v)
			if i%2 == 1 {
				f.SetCellStyle(sheet, cell, cell, altStyle)
			}
		}
		cellPct, _ := excelize.CoordinatesToCellName(4, row)
		pctStyle, _ := f.NewStyle(&excelize.Style{NumFmt: 10}) // 0.00%
		f.SetCellStyle(sheet, cellPct, cellPct, pctStyle)
		cellBonus, _ := excelize.CoordinatesToCellName(6, row)
		f.SetCellStyle(sheet, cellBonus, cellBonus, bonusStyle)
	}

	widths := map[string]float64{"A": 14, "B": 34, "C": 12, "D": 14, "E": 10, "F": 14, "G": 16}
	for col, w := range widths {
		f.SetColWidth(sheet, col, col, w)
	}
	f.SetPanes(sheet, &excelize.Panes{Freeze: true, YSplit: headerRow, TopLeftCell: fmt.Sprintf("A%d", headerRow+1), ActivePane: "bottomLeft"})
	f.SetActiveSheet(0)
	return f, nil
}

// GamifExtratoExcelHandler — GET /api/farol/gamif-extratos-excel?id=
func GamifExtratoExcelHandler(db *sql.DB) http.HandlerFunc {
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
		extratoID, err := strconv.Atoi(r.URL.Query().Get("id"))
		if err != nil {
			http.Error(w, "id é obrigatório", http.StatusBadRequest)
			return
		}

		var campanhaID int
		var geradoEm time.Time
		var geradoPor string
		var linhasJSON []byte
		err = db.QueryRow(`
			SELECT campanha_id, gerado_em, gerado_por, linhas
			FROM farol.gamif_extratos WHERE id = $1 AND empresa_id = $2
		`, extratoID, spCtx.EmpresaID).Scan(&campanhaID, &geradoEm, &geradoPor, &linhasJSON)
		if err == sql.ErrNoRows {
			http.Error(w, "Extrato não encontrado", http.StatusNotFound)
			return
		} else if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}

		var campanhaNome, industriaNome, dataInicio, dataFim string
		if err := db.QueryRow(`
			SELECT c.nome, i.nome, c.data_inicio::text, c.data_fim::text
			FROM farol.gamif_campanhas c JOIN farol.industrias i ON i.id = c.industria_id
			WHERE c.id = $1 AND c.empresa_id = $2
		`, campanhaID, spCtx.EmpresaID).Scan(&campanhaNome, &industriaNome, &dataInicio, &dataFim); err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}

		var linhas []GamifExtratoLinha
		if err := json.Unmarshal(linhasJSON, &linhas); err != nil {
			http.Error(w, "Erro ao ler linhas do extrato", http.StatusInternalServerError)
			return
		}

		logoData, logoMime, err := buscarLogoEmpresa(db, spCtx.EmpresaID)
		if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}

		xlsx, err := montarExtratoExcel(campanhaNome, industriaNome, dataInicio, dataFim, geradoEm, geradoPor, linhas, logoData, logoMime)
		if err != nil {
			http.Error(w, "Erro ao gerar Excel: "+err.Error(), http.StatusInternalServerError)
			return
		}

		filename := fmt.Sprintf("extrato_%s_%s.xlsx", sanitizeFilename(campanhaNome), geradoEm.Format("20060102_1504"))
		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
		xlsx.Write(w)
	}
}
