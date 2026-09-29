package handlers

// farol_metas_clientes_numericas_csv_test.go — cobre a Story 7.2 do Épico 7
// addendum (ver epics.md): import de Clientes Numéricas, formato sem Rede.

import (
	"bytes"
	"context"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

const clientesNumericasHeader = "cnpj;cod_cl;classificacao_pdv;razao;fantasia;cod_ggv;nome_ggv;cod_crv;nome_crv;cod_rca;nome_rca"

func clientesNumericasImportReq(empresaID, userID, vinculoID, vigenciaID string, csvContent string) *http.Request {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "numericas.csv")
	fw.Write([]byte(csvContent))
	mw.Close()

	url := fmt.Sprintf("/api/farol/metas-clientes-numericas-importar-csv?vinculo_id=%s&vigencia_id=%s", vinculoID, vigenciaID)
	r := httptest.NewRequest(http.MethodPost, url, &buf)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	ctx := context.WithValue(r.Context(), SpContextKey, &FarolContext{
		UserID: userID, SpRole: "gestor_geral", EmpresaID: empresaID, AllFiliais: true,
	})
	return r.WithContext(ctx)
}

func TestMetasClientesNumericas_ImportarLoteValido(t *testing.T) {
	db, empresaID := biTestDB(t)
	userID := tipoMetricaTestUserID(t, db)
	vinculoID, cleanup := criarVinculoFixture(t, db, empresaID, "TCN Valido")
	t.Cleanup(cleanup)
	vigenciaID := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-01-01", "2026-01-31")

	csvContent := clientesNumericasHeader + "\n" +
		"11222333000181;182920;Num. A;CLIENTE UM LTDA;CLIENTE UM;1;GGV UM;100;CRV UM;RCA001;RCA UM\n" +
		"11222333000182;183769;Num. B;CLIENTE DOIS LTDA;CLIENTE DOIS;1;GGV UM;100;CRV UM;RCA001;RCA UM\n" +
		"11222333000183;184614;Num. C;CLIENTE TRES LTDA;CLIENTE TRES;1;GGV UM;100;CRV UM;RCA002;RCA DOIS\n"
	w := httptest.NewRecorder()
	MetasClientesNumericasImportarCSVHandler(db)(w, clientesNumericasImportReq(empresaID, userID, fmt.Sprint(vinculoID), fmt.Sprint(vigenciaID), csvContent))
	if w.Code != http.StatusOK {
		t.Fatalf("import válido → status %d, body=%s", w.Code, w.Body.String())
	}
	var n int
	db.QueryRow(`SELECT count(*) FROM farol.metas_clientes_numericas WHERE vigencia_id = $1`, vigenciaID).Scan(&n)
	if n != 3 {
		t.Errorf("esperava 3 clientes numéricas, veio %d", n)
	}
}

func TestMetasClientesNumericas_ClassificacaoInvalida_400NadaImportado(t *testing.T) {
	db, empresaID := biTestDB(t)
	userID := tipoMetricaTestUserID(t, db)
	vinculoID, cleanup := criarVinculoFixture(t, db, empresaID, "TCN ClasseInvalida")
	t.Cleanup(cleanup)
	vigenciaID := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-02-01", "2026-02-28")

	// 1ª linha válida, 2ª com classificação fora do enum ("Num. D" não
	// existe) — atômico (Story 7.2/FR27): nenhuma linha entra.
	csvContent := clientesNumericasHeader + "\n" +
		"11222333000181;182920;Num. A;CLIENTE UM LTDA;CLIENTE UM;1;GGV UM;100;CRV UM;RCA001;RCA UM\n" +
		"11222333000182;183769;Num. D;CLIENTE DOIS LTDA;CLIENTE DOIS;1;GGV UM;100;CRV UM;RCA001;RCA UM\n"
	w := httptest.NewRecorder()
	MetasClientesNumericasImportarCSVHandler(db)(w, clientesNumericasImportReq(empresaID, userID, fmt.Sprint(vinculoID), fmt.Sprint(vigenciaID), csvContent))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("classificação inválida → status %d, want 400. body=%s", w.Code, w.Body.String())
	}
	var n int
	db.QueryRow(`SELECT count(*) FROM farol.metas_clientes_numericas WHERE vigencia_id = $1`, vigenciaID).Scan(&n)
	if n != 0 {
		t.Errorf("esperava 0 clientes importados (lote atômico), veio %d", n)
	}
}
