package handlers

// farol_gamificacao_excel_test.go — cobre a exportação em .xlsx do extrato
// de Gamificação (pedido do Claudio 23/09/2026: "relatório de extrato por
// campanha, exportável para Excel com a logo da empresa").

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
)

// pngMinusculoFixture — 1x1 PNG transparente válido, só pra exercitar
// AddPictureFromBytes sem depender de um arquivo de logo real no disco.
var pngMinusculoFixture = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
	0x89, 0x00, 0x00, 0x00, 0x0a, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae,
	0x42, 0x60, 0x82,
}

func TestMontarExtratoExcel_ComLogoEDadosCorretos(t *testing.T) {
	linhas := []GamifExtratoLinha{
		{CodRCA: "1001", NomeRCA: "Fulano de Tal", PontosTotal: 10, BonusTotal: 300, VolumeDesempate: 15, NivelPrincipal: "Ouro", PercentualPrincipal: 100},
		{CodRCA: "1002", NomeRCA: "Beltrano da Silva", PontosTotal: 3, BonusTotal: 90, VolumeDesempate: 7, NivelPrincipal: "Bronze", PercentualPrincipal: 66.67},
	}
	geradoEm := time.Date(2026, 9, 23, 14, 30, 0, 0, time.UTC)

	f, err := montarExtratoExcel("Campanha Teste", "UNILEVER FOOD", "2026-08-01", "2026-09-30", geradoEm, "admin-teste", linhas, pngMinusculoFixture, "image/png")
	if err != nil {
		t.Fatalf("montarExtratoExcel: %v", err)
	}

	sheet := "Extrato"
	titulo, _ := f.GetCellValue(sheet, "C1")
	if titulo != "Campanha Teste" {
		t.Errorf("título = %q, want %q", titulo, "Campanha Teste")
	}

	rcaCell, _ := f.GetCellValue(sheet, "A7")
	if rcaCell != "1001" {
		t.Errorf("A7 (RCA da 1ª linha de dados) = %q, want 1001", rcaCell)
	}
	nomeCell, _ := f.GetCellValue(sheet, "B7")
	if nomeCell != "Fulano de Tal" {
		t.Errorf("B7 = %q, want Fulano de Tal", nomeCell)
	}
	nivelCell, _ := f.GetCellValue(sheet, "C7")
	if nivelCell != "Ouro" {
		t.Errorf("C7 (nível) = %q, want Ouro", nivelCell)
	}
	bonusCell, _ := f.GetCellValue(sheet, "F8")
	if bonusCell != "R$ 90.00" {
		t.Errorf("F8 (bônus da 2ª linha, formatado como moeda) = %q, want R$ 90.00", bonusCell)
	}

	pics, err := f.GetPictures(sheet, "A1")
	if err != nil {
		t.Fatalf("GetPictures: %v", err)
	}
	if len(pics) != 1 {
		t.Errorf("logo inserida em A1 = %d imagens, want 1", len(pics))
	}
}

func TestMontarExtratoExcel_SemLogo_NaoQuebra(t *testing.T) {
	geradoEm := time.Date(2026, 9, 23, 14, 30, 0, 0, time.UTC)
	f, err := montarExtratoExcel("Sem Logo", "DIAGEO", "2026-08-01", "2026-09-30", geradoEm, "admin-teste", nil, nil, "")
	if err != nil {
		t.Fatalf("montarExtratoExcel sem logo: %v", err)
	}
	pics, _ := f.GetPictures("Extrato", "A1")
	if len(pics) != 0 {
		t.Errorf("sem logo_data, não deveria inserir imagem — achou %d", len(pics))
	}
}

// TestGamifExtratoExcelHandler_ServeXlsxValido — ponta a ponta via HTTP:
// gera um extrato de verdade (mesmo fluxo do TestGamifExtratosHandler),
// baixa o .xlsx pelo novo endpoint e confirma que o arquivo devolvido abre
// como planilha válida com o conteúdo esperado.
func TestGamifExtratoExcelHandler_ServeXlsxValido(t *testing.T) {
	db, empresaID := biTestDB(t)

	vinculoID, cleanup := criarVinculoComFormula(t, empresaID, "TGAM ExcelExtrato", "cobertura_rede", "rede",
		[]ParametroSchemaDTO{{Key: "limiar_valor_medio", Label: "Limiar", Type: "number"}},
		map[string]any{"limiar_valor_medio": 100.0})
	t.Cleanup(cleanup)
	vigenciaID := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-08-01", "2026-08-31")

	cnpjUnico := "80000000001001"
	t.Cleanup(func() {
		db.Exec(`DELETE FROM vendas_faturadas WHERE empresa_id = $1 AND cod_rca = 'TGAM-RCA-XLS'`, empresaID)
	})
	inserirClienteValidoFixture(t, empresaID, vinculoID, vigenciaID, "REDE XLS", cnpjUnico, "TGAM-RCA-XLS")
	inserirVendaFaturadaFixture(t, empresaID, cnpjUnico, "PRODXLS", "TGAM-RCA-XLS", "1", 150, 1, "2026-08-10")

	var industriaID int
	db.QueryRow(`SELECT industria_id FROM farol.metas_vinculos WHERE id = $1`, vinculoID).Scan(&industriaID)
	campanhaID := criarGamifCampanhaFixture(t, empresaID, industriaID, "2026-08-01", "2026-08-31")
	criarGamifRegraFixture(t, campanhaID, "cobertura_atingida", vinculoID, vigenciaID, nil, 0, 10, 300)
	t.Cleanup(func() { db.Exec(`DELETE FROM farol.gamif_extratos WHERE campanha_id = $1`, campanhaID) })

	extratosHandler := GamifExtratosHandler(db)
	reqGerar := gamifReq(http.MethodPost, "/api/farol/gamif-extratos?campanha_id="+strconv.Itoa(campanhaID), empresaID, "teste", nil)
	wGerar := httptest.NewRecorder()
	extratosHandler(wGerar, reqGerar)
	if wGerar.Code != http.StatusCreated {
		t.Fatalf("gerar extrato: status = %d, body = %s", wGerar.Code, wGerar.Body.String())
	}
	var extrato GamifExtratoResponse
	json.Unmarshal(wGerar.Body.Bytes(), &extrato)

	excelHandler := GamifExtratoExcelHandler(db)
	reqExcel := gamifReq(http.MethodGet, "/api/farol/gamif-extratos-excel?id="+strconv.Itoa(extrato.ID), empresaID, "teste", nil)
	wExcel := httptest.NewRecorder()
	excelHandler(wExcel, reqExcel)
	if wExcel.Code != http.StatusOK {
		t.Fatalf("baixar excel: status = %d, body = %s", wExcel.Code, wExcel.Body.String())
	}
	if ct := wExcel.Header().Get("Content-Type"); ct != "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" {
		t.Errorf("Content-Type = %q", ct)
	}
	if cd := wExcel.Header().Get("Content-Disposition"); cd == "" {
		t.Errorf("Content-Disposition ausente — o navegador não saberia baixar com nome de arquivo")
	}

	f, err := excelize.OpenReader(wExcel.Body)
	if err != nil {
		t.Fatalf("o corpo da resposta não é um .xlsx válido: %v", err)
	}
	rcaCell, _ := f.GetCellValue("Extrato", "A7")
	if rcaCell != "TGAM-RCA-XLS" {
		t.Errorf("A7 = %q, want TGAM-RCA-XLS", rcaCell)
	}
}
