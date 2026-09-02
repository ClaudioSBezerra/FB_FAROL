package handlers

// farol_metas_import_csv_test.go — cobre a I/O Matrix da Story 3.1
// (_bmad-output/implementation-artifacts/3-1-importacao-metas-csv.md).
//
// O AC central (FR9) é "tudo ou nada": se qualquer linha tiver erro,
// NENHUMA linha do lote é aplicada — mesmo as que estariam corretas
// sozinhas. É isso que os testes abaixo verificam de verdade (checando o
// banco depois, não só o código HTTP).

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

func csvImportReq(empresaID, userID, csvContent string) *http.Request {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "metas.csv")
	fw.Write([]byte(csvContent))
	mw.Close()

	r := httptest.NewRequest(http.MethodPost, "/api/farol/metas-vinculos-importar-csv", &buf)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	ctx := context.WithValue(r.Context(), SpContextKey, &FarolContext{
		UserID: userID, SpRole: "gestor_geral", EmpresaID: empresaID, AllFiliais: true,
	})
	return r.WithContext(ctx)
}

func contarVigencias(t *testing.T, db *sql.DB, vinculoID int) int {
	t.Helper()
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM farol.metas_vigencias WHERE vinculo_id = $1`, vinculoID).Scan(&n)
	return n
}

func TestMetasImportarCSV_LoteValido(t *testing.T) {
	db, empresaID := biTestDB(t)
	userID := tipoMetricaTestUserID(t, db)
	vinculoID, cleanup := criarVinculoFixture(t, db, empresaID, "TCSV Valido")
	t.Cleanup(cleanup)

	csvContent := fmt.Sprintf("vinculo_id;data_inicio;data_fim;faixa;valor_meta\n%d;2026-01-01;2026-03-31;3;78\n%d;2026-01-01;2026-03-31;2;85\n%d;2026-01-01;2026-03-31;1;91\n",
		vinculoID, vinculoID, vinculoID)

	w := httptest.NewRecorder()
	MetasImportarCSVHandler(db)(w, csvImportReq(empresaID, userID, csvContent))
	if w.Code != http.StatusOK {
		t.Fatalf("import válido → status %d, body=%s", w.Code, w.Body.String())
	}
	if n := contarVigencias(t, db, vinculoID); n != 1 {
		t.Errorf("esperava 1 vigência (3 faixas agrupadas), veio %d", n)
	}

	var faixas int
	db.QueryRow(`SELECT COUNT(*) FROM farol.metas_faixas mf JOIN farol.metas_vigencias mv ON mv.id = mf.vigencia_id WHERE mv.vinculo_id = $1`, vinculoID).Scan(&faixas)
	if faixas != 3 {
		t.Errorf("esperava 3 faixas, veio %d", faixas)
	}
}

// TestMetasImportarCSV_LinhaComErro_NadaAplicado é o teste central do FR9:
// um CSV com 2 vigências válidas pra vínculos DIFERENTES + 1 linha inválida
// não aplica NADA — nem as vigências que estariam corretas sozinhas.
func TestMetasImportarCSV_LinhaComErro_NadaAplicado(t *testing.T) {
	db, empresaID := biTestDB(t)
	userID := tipoMetricaTestUserID(t, db)
	vinculoA, cleanupA := criarVinculoFixture(t, db, empresaID, "TCSV ErroA")
	defer cleanupA()
	vinculoB, cleanupB := criarVinculoFixture(t, db, empresaID, "TCSV ErroB")
	defer cleanupB()

	csvContent := fmt.Sprintf("vinculo_id;data_inicio;data_fim;faixa;valor_meta\n%d;2026-01-01;2026-01-31;1;100\nABC;2026-01-01;2026-01-31;1;200\n%d;2026-02-01;2026-02-28;1;300\n",
		vinculoA, vinculoB)

	w := httptest.NewRecorder()
	MetasImportarCSVHandler(db)(w, csvImportReq(empresaID, userID, csvContent))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("import com linha inválida → status %d, want 400, body=%s", w.Code, w.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["erros"] == nil {
		t.Errorf("resposta deveria listar os erros: %s", w.Body.String())
	}

	if n := contarVigencias(t, db, vinculoA); n != 0 {
		t.Errorf("FR9 violado: vínculo A (linha válida) tem %d vigência(s), deveria ter 0 — lote deveria ser tudo-ou-nada", n)
	}
	if n := contarVigencias(t, db, vinculoB); n != 0 {
		t.Errorf("FR9 violado: vínculo B (linha válida) tem %d vigência(s), deveria ter 0 — lote deveria ser tudo-ou-nada", n)
	}
}

func TestMetasImportarCSV_VinculoInexistente_400(t *testing.T) {
	db, empresaID := biTestDB(t)
	userID := tipoMetricaTestUserID(t, db)

	csvContent := "vinculo_id;data_inicio;data_fim;faixa;valor_meta\n999999999;2026-01-01;2026-01-31;1;100\n"
	w := httptest.NewRecorder()
	MetasImportarCSVHandler(db)(w, csvImportReq(empresaID, userID, csvContent))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("import com vinculo_id inexistente → status %d, want 400, body=%s", w.Code, w.Body.String())
	}
}

func TestMetasImportarCSV_ColunaObrigatoriaAusente_400(t *testing.T) {
	db, empresaID := biTestDB(t)
	userID := tipoMetricaTestUserID(t, db)

	csvContent := "vinculo_id;faixa;valor_meta\n1;1;100\n" // faltam data_inicio/data_fim
	w := httptest.NewRecorder()
	MetasImportarCSVHandler(db)(w, csvImportReq(empresaID, userID, csvContent))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("CSV sem colunas obrigatórias → status %d, want 400, body=%s", w.Code, w.Body.String())
	}
}

// TestMetasImportarCSV_SobreposicaoComVigenciaFechada_409 cobre a Story 3.4
// (FR13/snapshot congelado): mesmo a importação de metas (Story 3.1, que
// cria vigências NOVAS) não consegue criar nada que se sobreponha a uma
// vigência já fechada — a constraint EXCLUDE protege independente de qual
// handler tenta escrever.
func TestMetasImportarCSV_SobreposicaoComVigenciaFechada_409(t *testing.T) {
	db, empresaID := biTestDB(t)
	userID := tipoMetricaTestUserID(t, db)
	vinculoID, cleanup := criarVinculoFixture(t, db, empresaID, "TCSV Fechada")
	t.Cleanup(cleanup)

	vigenciaID := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-01-01", "2026-01-31")
	db.Exec(`UPDATE farol.metas_vigencias SET status = 'fechada' WHERE id = $1`, vigenciaID)

	csvContent := fmt.Sprintf("vinculo_id;data_inicio;data_fim;faixa;valor_meta\n%d;2026-01-01;2026-01-31;1;999\n", vinculoID)
	w := httptest.NewRecorder()
	MetasImportarCSVHandler(db)(w, csvImportReq(empresaID, userID, csvContent))
	if w.Code != http.StatusConflict {
		t.Fatalf("CSV tentando recriar vigência sobre uma já fechada → status %d, want 409, body=%s", w.Code, w.Body.String())
	}
	if n := contarVigencias(t, db, vinculoID); n != 1 {
		t.Errorf("deveria continuar existindo só a vigência fechada original (1), veio %d", n)
	}
}

func TestMetasImportarCSV_SobreposicaoComVigenciaExistente_409(t *testing.T) {
	db, empresaID := biTestDB(t)
	userID := tipoMetricaTestUserID(t, db)
	vinculoID, cleanup := criarVinculoFixture(t, db, empresaID, "TCSV Overlap")
	t.Cleanup(cleanup)

	// vigência existente jan
	w0 := httptest.NewRecorder()
	MetasVigenciasHandler(db)(w0, vigenciaReq(http.MethodPost, "/api/farol/metas-vigencias", empresaID, userID, VigenciaRequest{
		VinculoID: vinculoID, DataInicio: "2026-01-01", DataFim: "2026-01-31", Faixas: []FaixaDTO{{Faixa: 1, ValorMeta: 100}},
	}))
	if w0.Code != http.StatusCreated {
		t.Fatalf("setup: status %d", w0.Code)
	}

	csvContent := fmt.Sprintf("vinculo_id;data_inicio;data_fim;faixa;valor_meta\n%d;2026-01-15;2026-02-15;1;999\n", vinculoID)
	w := httptest.NewRecorder()
	MetasImportarCSVHandler(db)(w, csvImportReq(empresaID, userID, csvContent))
	if w.Code != http.StatusConflict {
		t.Fatalf("CSV sobrepondo vigência existente → status %d, want 409, body=%s", w.Code, w.Body.String())
	}
}
