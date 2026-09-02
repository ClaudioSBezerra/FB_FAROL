package handlers

// farol_metas_itens_validos_csv_test.go — cobre a I/O Matrix da Story 3.3
// (_bmad-output/implementation-artifacts/3-3-importacao-itens-validos.md).

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

	// EAN 789001 mapeado em 2 embalagens diferentes (variantes) — cobre o
	// AC "um EAN pode ter mais de uma variante/embalagem mapeada" (FR12).
	csvContent := "ean;cod_prod;tipo_embalagem\n789001;PROD-A-UN;UN\n789001;PROD-A-CX;CX\n789002;PROD-B;DISPLAY\n"
	w := httptest.NewRecorder()
	MetasItensValidosImportarCSVHandler(db)(w, itensValidosImportReq(empresaID, userID, fmt.Sprint(vinculoID), fmt.Sprint(vigenciaID), csvContent))
	if w.Code != http.StatusOK {
		t.Fatalf("import válido → status %d, body=%s", w.Code, w.Body.String())
	}
	if n := contarItensValidos(t, db, vigenciaID); n != 3 {
		t.Errorf("esperava 3 itens válidos, veio %d", n)
	}
}

// TestMetasItensValidos_EmbalagemInvalida_FR12 cobre o AC central do FR12:
// tipo_embalagem é obrigatório e decide a regra de quantidade mínima do
// Sortimento — linha sem embalagem válida rejeita o lote inteiro (FR9).
func TestMetasItensValidos_EmbalagemInvalida_FR12(t *testing.T) {
	db, empresaID := biTestDB(t)
	userID := tipoMetricaTestUserID(t, db)
	vinculoID, cleanup := criarVinculoFixture(t, db, empresaID, "TIV EmbInvalida")
	t.Cleanup(cleanup)
	vigenciaID := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-02-01", "2026-02-28")

	csvContent := "ean;cod_prod;tipo_embalagem\n789001;PROD-A;UN\n789002;PROD-B;GARRAFA\n"
	w := httptest.NewRecorder()
	MetasItensValidosImportarCSVHandler(db)(w, itensValidosImportReq(empresaID, userID, fmt.Sprint(vinculoID), fmt.Sprint(vigenciaID), csvContent))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("embalagem inválida → status %d, want 400, body=%s", w.Code, w.Body.String())
	}
	if n := contarItensValidos(t, db, vigenciaID); n != 0 {
		t.Errorf("FR9 violado: deveria ter 0 itens (lote todo rejeitado), veio %d", n)
	}
}

func TestMetasItensValidos_EmbalagemCaseInsensitive(t *testing.T) {
	db, empresaID := biTestDB(t)
	userID := tipoMetricaTestUserID(t, db)
	vinculoID, cleanup := criarVinculoFixture(t, db, empresaID, "TIV CaseInsensitive")
	t.Cleanup(cleanup)
	vigenciaID := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-03-01", "2026-03-31")

	csvContent := "ean;cod_prod;tipo_embalagem\n789001;PROD-A;un\n"
	w := httptest.NewRecorder()
	MetasItensValidosImportarCSVHandler(db)(w, itensValidosImportReq(empresaID, userID, fmt.Sprint(vinculoID), fmt.Sprint(vigenciaID), csvContent))
	if w.Code != http.StatusOK {
		t.Fatalf("embalagem minúscula deveria ser normalizada → status %d, body=%s", w.Code, w.Body.String())
	}
}

func TestMetasItensValidos_CombinacaoDuplicada_400(t *testing.T) {
	db, empresaID := biTestDB(t)
	userID := tipoMetricaTestUserID(t, db)
	vinculoID, cleanup := criarVinculoFixture(t, db, empresaID, "TIV Duplicado")
	t.Cleanup(cleanup)
	vigenciaID := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-04-01", "2026-04-30")

	csvContent := "ean;cod_prod;tipo_embalagem\n789001;PROD-A;UN\n789001;PROD-A;CX\n"
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
		"ean;cod_prod;tipo_embalagem\n789001;PROD-A;UN\n"))
	if w.Code != http.StatusForbidden {
		t.Fatalf("import numa vigência fechada → status %d, want 403, body=%s", w.Code, w.Body.String())
	}
}
