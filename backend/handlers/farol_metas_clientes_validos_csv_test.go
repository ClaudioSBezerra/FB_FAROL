package handlers

// farol_metas_clientes_validos_csv_test.go — cobre a I/O Matrix da Story 3.2
// (_bmad-output/implementation-artifacts/3-2-importacao-clientes-validos.md).

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

func clientesValidosImportReq(empresaID, userID, vinculoID, vigenciaID string, csvContent string) *http.Request {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "clientes.csv")
	fw.Write([]byte(csvContent))
	mw.Close()

	url := fmt.Sprintf("/api/farol/metas-clientes-validos-importar-csv?vinculo_id=%s&vigencia_id=%s", vinculoID, vigenciaID)
	r := httptest.NewRequest(http.MethodPost, url, &buf)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	ctx := context.WithValue(r.Context(), SpContextKey, &FarolContext{
		UserID: userID, SpRole: "gestor_geral", EmpresaID: empresaID, AllFiliais: true,
	})
	return r.WithContext(ctx)
}

// criarVigenciaFixture cria uma vigência aberta pro vínculo dado, retorna o id.
func criarVigenciaFixture(t *testing.T, db *sql.DB, empresaID string, vinculoID int, inicio, fim string) int {
	t.Helper()
	var id int
	if err := db.QueryRow(`
		INSERT INTO farol.metas_vigencias (empresa_id, vinculo_id, data_inicio, data_fim) VALUES ($1, $2, $3, $4) RETURNING id
	`, empresaID, vinculoID, inicio, fim).Scan(&id); err != nil {
		t.Fatalf("criar fixture de vigência: %v", err)
	}
	return id
}

func contarClientesValidos(t *testing.T, db *sql.DB, vigenciaID int) int {
	t.Helper()
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM farol.metas_clientes_validos WHERE vigencia_id = $1`, vigenciaID).Scan(&n)
	return n
}

func TestMetasClientesValidos_ImportarLoteValido(t *testing.T) {
	db, empresaID := biTestDB(t)
	userID := tipoMetricaTestUserID(t, db)
	vinculoID, cleanup := criarVinculoFixture(t, db, empresaID, "TCV Valido")
	t.Cleanup(cleanup)
	vigenciaID := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-01-01", "2026-01-31")

	csvContent := "rede_nome;cnpj;cod_rca\nREDE MAIS;11222333000181;RCA001\nREDE MAIS;11222333000182;RCA001\nREDE BOM;11222333000183;RCA002\n"
	w := httptest.NewRecorder()
	MetasClientesValidosImportarCSVHandler(db)(w, clientesValidosImportReq(empresaID, userID, fmt.Sprint(vinculoID), fmt.Sprint(vigenciaID), csvContent))
	if w.Code != http.StatusOK {
		t.Fatalf("import válido → status %d, body=%s", w.Code, w.Body.String())
	}
	if n := contarClientesValidos(t, db, vigenciaID); n != 3 {
		t.Errorf("esperava 3 clientes válidos, veio %d", n)
	}
}

// TestMetasClientesValidos_CNPJSemRCA_FR11 é o teste central do FR11: todo
// CNPJ deve ter RCA vinculado — linha sem RCA rejeita o lote inteiro (FR9).
func TestMetasClientesValidos_CNPJSemRCA_FR11(t *testing.T) {
	db, empresaID := biTestDB(t)
	userID := tipoMetricaTestUserID(t, db)
	vinculoID, cleanup := criarVinculoFixture(t, db, empresaID, "TCV SemRCA")
	t.Cleanup(cleanup)
	vigenciaID := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-02-01", "2026-02-28")

	csvContent := "rede_nome;cnpj;cod_rca\nREDE MAIS;11222333000181;RCA001\nREDE MAIS;11222333000182;\n"
	w := httptest.NewRecorder()
	MetasClientesValidosImportarCSVHandler(db)(w, clientesValidosImportReq(empresaID, userID, fmt.Sprint(vinculoID), fmt.Sprint(vigenciaID), csvContent))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("CNPJ sem RCA → status %d, want 400, body=%s", w.Code, w.Body.String())
	}
	if n := contarClientesValidos(t, db, vigenciaID); n != 0 {
		t.Errorf("FR9 violado: deveria ter 0 clientes (lote todo rejeitado), veio %d", n)
	}
}

func TestMetasClientesValidos_CNPJInvalido_400(t *testing.T) {
	db, empresaID := biTestDB(t)
	userID := tipoMetricaTestUserID(t, db)
	vinculoID, cleanup := criarVinculoFixture(t, db, empresaID, "TCV CNPJInvalido")
	t.Cleanup(cleanup)
	vigenciaID := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-03-01", "2026-03-31")

	csvContent := "rede_nome;cnpj;cod_rca\nREDE MAIS;123;RCA001\n"
	w := httptest.NewRecorder()
	MetasClientesValidosImportarCSVHandler(db)(w, clientesValidosImportReq(empresaID, userID, fmt.Sprint(vinculoID), fmt.Sprint(vigenciaID), csvContent))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("CNPJ inválido → status %d, want 400, body=%s", w.Code, w.Body.String())
	}
}

func TestMetasClientesValidos_ReimportacaoSubstituiLista(t *testing.T) {
	db, empresaID := biTestDB(t)
	userID := tipoMetricaTestUserID(t, db)
	vinculoID, cleanup := criarVinculoFixture(t, db, empresaID, "TCV Substitui")
	t.Cleanup(cleanup)
	vigenciaID := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-04-01", "2026-04-30")

	w1 := httptest.NewRecorder()
	MetasClientesValidosImportarCSVHandler(db)(w1, clientesValidosImportReq(empresaID, userID, fmt.Sprint(vinculoID), fmt.Sprint(vigenciaID),
		"rede_nome;cnpj;cod_rca\nREDE A;11222333000181;RCA001\nREDE A;11222333000182;RCA001\n"))
	if w1.Code != http.StatusOK {
		t.Fatalf("import 1: status %d, body=%s", w1.Code, w1.Body.String())
	}

	w2 := httptest.NewRecorder()
	MetasClientesValidosImportarCSVHandler(db)(w2, clientesValidosImportReq(empresaID, userID, fmt.Sprint(vinculoID), fmt.Sprint(vigenciaID),
		"rede_nome;cnpj;cod_rca\nREDE B;11222333000183;RCA002\n"))
	if w2.Code != http.StatusOK {
		t.Fatalf("import 2: status %d, body=%s", w2.Code, w2.Body.String())
	}

	if n := contarClientesValidos(t, db, vigenciaID); n != 1 {
		t.Errorf("reimportação deveria SUBSTITUIR a lista anterior (esperava 1, veio %d)", n)
	}
}

func TestMetasClientesValidos_VigenciaFechada_403(t *testing.T) {
	db, empresaID := biTestDB(t)
	userID := tipoMetricaTestUserID(t, db)
	vinculoID, cleanup := criarVinculoFixture(t, db, empresaID, "TCV Fechada")
	t.Cleanup(cleanup)
	vigenciaID := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-05-01", "2026-05-31")
	db.Exec(`UPDATE farol.metas_vigencias SET status = 'fechada' WHERE id = $1`, vigenciaID)

	w := httptest.NewRecorder()
	MetasClientesValidosImportarCSVHandler(db)(w, clientesValidosImportReq(empresaID, userID, fmt.Sprint(vinculoID), fmt.Sprint(vigenciaID),
		"rede_nome;cnpj;cod_rca\nREDE A;11222333000181;RCA001\n"))
	if w.Code != http.StatusForbidden {
		t.Fatalf("import numa vigência fechada → status %d, want 403, body=%s", w.Code, w.Body.String())
	}
}

func TestMetasClientesValidos_CNPJDuplicadoNoArquivo_400(t *testing.T) {
	db, empresaID := biTestDB(t)
	userID := tipoMetricaTestUserID(t, db)
	vinculoID, cleanup := criarVinculoFixture(t, db, empresaID, "TCV Duplicado")
	t.Cleanup(cleanup)
	vigenciaID := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-06-01", "2026-06-30")

	csvContent := "rede_nome;cnpj;cod_rca\nREDE A;11222333000181;RCA001\nREDE B;11222333000181;RCA002\n"
	w := httptest.NewRecorder()
	MetasClientesValidosImportarCSVHandler(db)(w, clientesValidosImportReq(empresaID, userID, fmt.Sprint(vinculoID), fmt.Sprint(vigenciaID), csvContent))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("CNPJ duplicado no mesmo arquivo → status %d, want 400, body=%s", w.Code, w.Body.String())
	}
	var resp map[string]json.RawMessage
	json.Unmarshal(w.Body.Bytes(), &resp)
	if _, ok := resp["erros"]; !ok {
		t.Errorf("resposta deveria listar o erro de duplicata: %s", w.Body.String())
	}
}
