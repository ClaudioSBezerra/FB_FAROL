package handlers

// farol_metas_itens_validos_csv_test.go — cobre a I/O Matrix da Story 3.3
// (_bmad-output/implementation-artifacts/3-3-importacao-itens-validos.md).
//
// A coluna tipo_embalagem (e os testes que a cobriam — validação de
// domínio, normalização case-insensitive) foi REMOVIDA em 2026-09-04
// (migration 225, orientação do Heverton): a regra de quantidade mínima do
// Sortimento agora é resolvida pelo motor de apuração via cadastro de
// produto (embalagem/qt_unit_cx, já importado na carga diária de vendas),
// não mais por um campo pedido de novo no CSV mensal de Itens Válidos.

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

func itensValidosImportReq(empresaID, userID, vinculoID, vigenciaID string, csvContent string) *http.Request {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "itens.csv")
	fw.Write([]byte(csvContent))
	mw.Close()

	url := fmt.Sprintf("/api/farol/metas-itens-validos-importar-csv?vinculo_id=%s&vigencia_id=%s", vinculoID, vigenciaID)
	r := httptest.NewRequest(http.MethodPost, url, &buf)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	ctx := context.WithValue(r.Context(), SpContextKey, &FarolContext{
		UserID: userID, SpRole: "gestor_geral", EmpresaID: empresaID, AllFiliais: true,
	})
	return r.WithContext(ctx)
}

func contarItensValidos(t *testing.T, db *sql.DB, vigenciaID int) int {
	t.Helper()
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM farol.metas_itens_validos WHERE vigencia_id = $1`, vigenciaID).Scan(&n)
	return n
}

func TestMetasItensValidos_ImportarLoteValido(t *testing.T) {
	db, empresaID := biTestDB(t)
	userID := tipoMetricaTestUserID(t, db)
	vinculoID, cleanup := criarVinculoFixture(t, db, empresaID, "TIV Valido")
	t.Cleanup(cleanup)
	vigenciaID := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-01-01", "2026-01-31")

	// EAN 789001 mapeado em 2 cod_prod diferentes (variantes) — cobre o AC
	// "um EAN pode ter mais de uma variante mapeada" (FR12), inclusive mais
	// de 2 (BASE EANS real da JC mostra itens com vários códigos JC).
	csvContent := "ean;cod_prod\n789001;PROD-A-UN\n789001;PROD-A-CX\n789002;PROD-B\n"
	w := httptest.NewRecorder()
	MetasItensValidosImportarCSVHandler(db)(w, itensValidosImportReq(empresaID, userID, fmt.Sprint(vinculoID), fmt.Sprint(vigenciaID), csvContent))
	if w.Code != http.StatusOK {
		t.Fatalf("import válido → status %d, body=%s", w.Code, w.Body.String())
	}
	if n := contarItensValidos(t, db, vigenciaID); n != 3 {
		t.Errorf("esperava 3 itens válidos, veio %d", n)
	}
}

func TestMetasItensValidos_EANOuCodProdVazio_400(t *testing.T) {
	db, empresaID := biTestDB(t)
	userID := tipoMetricaTestUserID(t, db)
	vinculoID, cleanup := criarVinculoFixture(t, db, empresaID, "TIV Vazio")
	t.Cleanup(cleanup)
	vigenciaID := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-02-01", "2026-02-28")

	csvContent := "ean;cod_prod\n789001;PROD-A\n;PROD-B\n"
	w := httptest.NewRecorder()
	MetasItensValidosImportarCSVHandler(db)(w, itensValidosImportReq(empresaID, userID, fmt.Sprint(vinculoID), fmt.Sprint(vigenciaID), csvContent))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("ean vazio → status %d, want 400, body=%s", w.Code, w.Body.String())
	}
	if n := contarItensValidos(t, db, vigenciaID); n != 0 {
		t.Errorf("FR9 violado: deveria ter 0 itens (lote todo rejeitado), veio %d", n)
	}
}

func TestMetasItensValidos_CombinacaoDuplicada_400(t *testing.T) {
	db, empresaID := biTestDB(t)
	userID := tipoMetricaTestUserID(t, db)
	vinculoID, cleanup := criarVinculoFixture(t, db, empresaID, "TIV Duplicado")
	t.Cleanup(cleanup)
	vigenciaID := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-04-01", "2026-04-30")

	csvContent := "ean;cod_prod\n789001;PROD-A\n789001;PROD-A\n"
	w := httptest.NewRecorder()
	MetasItensValidosImportarCSVHandler(db)(w, itensValidosImportReq(empresaID, userID, fmt.Sprint(vinculoID), fmt.Sprint(vigenciaID), csvContent))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("combinação EAN+cod_prod duplicada → status %d, want 400, body=%s", w.Code, w.Body.String())
	}
}

func TestMetasItensValidos_VigenciaFechada_403(t *testing.T) {
	db, empresaID := biTestDB(t)
	userID := tipoMetricaTestUserID(t, db)
	vinculoID, cleanup := criarVinculoFixture(t, db, empresaID, "TIV Fechada")
	t.Cleanup(cleanup)
	vigenciaID := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-05-01", "2026-05-31")
	db.Exec(`UPDATE farol.metas_vigencias SET status = 'fechada' WHERE id = $1`, vigenciaID)

	w := httptest.NewRecorder()
	MetasItensValidosImportarCSVHandler(db)(w, itensValidosImportReq(empresaID, userID, fmt.Sprint(vinculoID), fmt.Sprint(vigenciaID),
		"ean;cod_prod\n789001;PROD-A\n"))
	if w.Code != http.StatusForbidden {
		t.Fatalf("import numa vigência fechada → status %d, want 403, body=%s", w.Code, w.Body.String())
	}
}
