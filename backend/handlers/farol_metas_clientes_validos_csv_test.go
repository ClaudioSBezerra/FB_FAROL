package handlers

// farol_metas_clientes_validos_csv_test.go — cobre a I/O Matrix da Story 3.2
// (_bmad-output/implementation-artifacts/3-2-importacao-clientes-validos.md).
//
// Formato do CSV atualizado em 2026-09-04 (orientação do Heverton — ver
// migration 224 e farol_metas_clientes_validos_csv.go): cabeçalho completo
// cnpj;cod_princ;razao;fantasia;cod_ggv;nome_ggv;cod_crv;nome_crv;cod_rca;nome_rca.

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

const clientesValidosHeader = "cnpj;cod_princ;razao;fantasia;cod_ggv;nome_ggv;cod_crv;nome_crv;cod_rca;nome_rca"

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

	csvContent := clientesValidosHeader + "\n" +
		"11222333000181;REDEMAIS;REDE MAIS LTDA;REDE MAIS;G1;GGV UM;C1;CRV UM;RCA001;RCA UM\n" +
		"11222333000182;REDEMAIS;REDE MAIS LTDA;REDE MAIS;G1;GGV UM;C1;CRV UM;RCA001;RCA UM\n" +
		"11222333000183;REDEBOM;REDE BOM LTDA;REDE BOM;G1;GGV UM;C1;CRV UM;RCA002;RCA DOIS\n"
	w := httptest.NewRecorder()
	MetasClientesValidosImportarCSVHandler(db)(w, clientesValidosImportReq(empresaID, userID, fmt.Sprint(vinculoID), fmt.Sprint(vigenciaID), csvContent))
	if w.Code != http.StatusOK {
		t.Fatalf("import válido → status %d, body=%s", w.Code, w.Body.String())
	}
	if n := contarClientesValidos(t, db, vigenciaID); n != 3 {
		t.Errorf("esperava 3 clientes válidos, veio %d", n)
	}
}

// TestMetasClientesValidos_CNPJSemRCA_ImportaParcialComAviso — FR11 (todo
// CNPJ precisa de RCA) continua valendo por LINHA, mas desde 08/09/2026
// (decisão do Heverton, pra aprovar o MVP com a planilha real da JC, que tem
// pendência conhecida de RCA faltando em algumas linhas) uma linha inválida
// não trava mais o lote inteiro (era FR9/Story 3.1) — vira só um aviso, e o
// resto do arquivo entra normalmente.
func TestMetasClientesValidos_CNPJSemRCA_ImportaParcialComAviso(t *testing.T) {
	db, empresaID := biTestDB(t)
	userID := tipoMetricaTestUserID(t, db)
	vinculoID, cleanup := criarVinculoFixture(t, db, empresaID, "TCV SemRCA")
	t.Cleanup(cleanup)
	vigenciaID := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-02-01", "2026-02-28")

	csvContent := clientesValidosHeader + "\n" +
		"11222333000181;REDEMAIS;;;G1;;C1;;RCA001;\n" +
		"11222333000182;REDEMAIS;;;G1;;C1;;;\n"
	w := httptest.NewRecorder()
	MetasClientesValidosImportarCSVHandler(db)(w, clientesValidosImportReq(empresaID, userID, fmt.Sprint(vinculoID), fmt.Sprint(vigenciaID), csvContent))
	if w.Code != http.StatusOK {
		t.Fatalf("1 linha válida + 1 sem RCA → status %d, want 200 (importação parcial), body=%s", w.Code, w.Body.String())
	}
	if n := contarClientesValidos(t, db, vigenciaID); n != 1 {
		t.Errorf("esperava 1 cliente importado (a linha com RCA), veio %d", n)
	}
	var resp map[string]json.RawMessage
	json.Unmarshal(w.Body.Bytes(), &resp)
	if _, ok := resp["avisos"]; !ok {
		t.Errorf("resposta deveria listar a linha sem RCA em 'avisos': %s", w.Body.String())
	}
}

// TestMetasClientesValidos_TodasLinhasInvalidas_400NadaApagado — se NENHUMA
// linha for válida, ainda recusa de vez (não apaga a lista antiga da
// vigência sem nenhum substituto).
func TestMetasClientesValidos_TodasLinhasInvalidas_400NadaApagado(t *testing.T) {
	db, empresaID := biTestDB(t)
	userID := tipoMetricaTestUserID(t, db)
	vinculoID, cleanup := criarVinculoFixture(t, db, empresaID, "TCV TodasInvalidas")
	t.Cleanup(cleanup)
	vigenciaID := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-08-01", "2026-08-31")

	csvContent := clientesValidosHeader + "\n" +
		"11222333000181;REDEMAIS;;;G1;;C1;;;\n" +
		"11222333000182;REDEMAIS;;;;;C1;;RCA001;\n"
	w := httptest.NewRecorder()
	MetasClientesValidosImportarCSVHandler(db)(w, clientesValidosImportReq(empresaID, userID, fmt.Sprint(vinculoID), fmt.Sprint(vigenciaID), csvContent))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("nenhuma linha válida → status %d, want 400, body=%s", w.Code, w.Body.String())
	}
	if n := contarClientesValidos(t, db, vigenciaID); n != 0 {
		t.Errorf("esperava 0 clientes (nada válido pra importar), veio %d", n)
	}
}

func TestMetasClientesValidos_CNPJInvalido_400(t *testing.T) {
	db, empresaID := biTestDB(t)
	userID := tipoMetricaTestUserID(t, db)
	vinculoID, cleanup := criarVinculoFixture(t, db, empresaID, "TCV CNPJInvalido")
	t.Cleanup(cleanup)
	vigenciaID := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-03-01", "2026-03-31")

	csvContent := clientesValidosHeader + "\n" + "123;REDEMAIS;;;G1;;C1;;RCA001;\n"
	w := httptest.NewRecorder()
	MetasClientesValidosImportarCSVHandler(db)(w, clientesValidosImportReq(empresaID, userID, fmt.Sprint(vinculoID), fmt.Sprint(vigenciaID), csvContent))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("CNPJ inválido → status %d, want 400, body=%s", w.Code, w.Body.String())
	}
}

func TestMetasClientesValidos_CNPJSemGGVOuCRV_400(t *testing.T) {
	db, empresaID := biTestDB(t)
	userID := tipoMetricaTestUserID(t, db)
	vinculoID, cleanup := criarVinculoFixture(t, db, empresaID, "TCV SemGGV")
	t.Cleanup(cleanup)
	vigenciaID := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-07-01", "2026-07-31")

	// cod_ggv e cod_crv vazios — hierarquia agora vem do CSV (migration 224),
	// não é mais derivada por JOIN em vendas, então precisa vir completa.
	csvContent := clientesValidosHeader + "\n" + "11222333000181;REDEMAIS;;;;;;;RCA001;\n"
	w := httptest.NewRecorder()
	MetasClientesValidosImportarCSVHandler(db)(w, clientesValidosImportReq(empresaID, userID, fmt.Sprint(vinculoID), fmt.Sprint(vigenciaID), csvContent))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("CNPJ sem GGV/CRV → status %d, want 400, body=%s", w.Code, w.Body.String())
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
		clientesValidosHeader+"\n"+
			"11222333000181;REDEA;;;G1;;C1;;RCA001;\n"+
			"11222333000182;REDEA;;;G1;;C1;;RCA001;\n"))
	if w1.Code != http.StatusOK {
		t.Fatalf("import 1: status %d, body=%s", w1.Code, w1.Body.String())
	}

	w2 := httptest.NewRecorder()
	MetasClientesValidosImportarCSVHandler(db)(w2, clientesValidosImportReq(empresaID, userID, fmt.Sprint(vinculoID), fmt.Sprint(vigenciaID),
		clientesValidosHeader+"\n"+"11222333000183;REDEB;;;G1;;C1;;RCA002;\n"))
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
		clientesValidosHeader+"\n"+"11222333000181;REDEA;;;G1;;C1;;RCA001;\n"))
	if w.Code != http.StatusForbidden {
		t.Fatalf("import numa vigência fechada → status %d, want 403, body=%s", w.Code, w.Body.String())
	}
}

// TestMetasClientesValidos_CNPJDuplicadoNoArquivo_ImportaPrimeiraOcorrencia —
// desde 08/09/2026, duplicata também é só aviso: a PRIMEIRA ocorrência do
// CNPJ entra, as seguintes ficam de fora (com aviso), em vez de rejeitar o
// arquivo inteiro.
func TestMetasClientesValidos_CNPJDuplicadoNoArquivo_ImportaPrimeiraOcorrencia(t *testing.T) {
	db, empresaID := biTestDB(t)
	userID := tipoMetricaTestUserID(t, db)
	vinculoID, cleanup := criarVinculoFixture(t, db, empresaID, "TCV Duplicado")
	t.Cleanup(cleanup)
	vigenciaID := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-06-01", "2026-06-30")

	csvContent := clientesValidosHeader + "\n" +
		"11222333000181;REDEA;;;G1;;C1;;RCA001;\n" +
		"11222333000181;REDEB;;;G1;;C1;;RCA002;\n"
	w := httptest.NewRecorder()
	MetasClientesValidosImportarCSVHandler(db)(w, clientesValidosImportReq(empresaID, userID, fmt.Sprint(vinculoID), fmt.Sprint(vigenciaID), csvContent))
	if w.Code != http.StatusOK {
		t.Fatalf("CNPJ duplicado no mesmo arquivo → status %d, want 200 (1ª ocorrência importada), body=%s", w.Code, w.Body.String())
	}
	if n := contarClientesValidos(t, db, vigenciaID); n != 1 {
		t.Errorf("esperava 1 cliente importado (a 1ª ocorrência do CNPJ duplicado), veio %d", n)
	}
	var resp map[string]json.RawMessage
	json.Unmarshal(w.Body.Bytes(), &resp)
	if _, ok := resp["avisos"]; !ok {
		t.Errorf("resposta deveria listar a duplicata em 'avisos': %s", w.Body.String())
	}
}
