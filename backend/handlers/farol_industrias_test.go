package handlers

// farol_industrias_test.go — cobre a I/O Matrix de spec-cadastro-industria.md.
//
// Testes de integração: exigem DATABASE_URL (ver biTestDB em
// farol_bi_api_test.go). Cada teste cria e remove seus próprios dados,
// isolados por nome de indústria único (prefixo TIND).

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// industriaReq monta uma request já autenticada, injetando o FarolContext
// direto — mesmo padrão de biReq.
func industriaReq(method, url, empresaID string, body any) *http.Request {
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	r := httptest.NewRequest(method, url, &buf)
	ctx := context.WithValue(r.Context(), SpContextKey, &FarolContext{
		UserID: "teste", SpRole: "gestor_filial", EmpresaID: empresaID, AllFiliais: true,
	})
	return r.WithContext(ctx)
}

func limparIndustriaFixture(t *testing.T, db *sql.DB, empresaID, nome string) {
	t.Helper()
	if _, err := db.Exec(`DELETE FROM farol.industrias WHERE empresa_id = $1 AND nome = $2`, empresaID, nome); err != nil {
		t.Fatalf("limpar fixture de indústria %s: %v", nome, err)
	}
}

func TestIndustrias_CriarComFornecedores(t *testing.T) {
	db, empresaID := biTestDB(t)
	nome := "TIND UNILEVER HC"
	limparIndustriaFixture(t, db, empresaID, nome)
	t.Cleanup(func() { limparIndustriaFixture(t, db, empresaID, nome) })

	body := IndustriaRequest{
		Nome: nome,
		Fornecedores: []IndustriaFornecedorDTO{
			{CodFornec: "99991", Rotulo: "MTZ/MS/BA"},
			{CodFornec: "99992"},
		},
	}
	w := httptest.NewRecorder()
	IndustriasHandler(db)(w, industriaReq(http.MethodPost, "/api/farol/industrias", empresaID, body))
	if w.Code != http.StatusCreated {
		t.Fatalf("POST → status %d, body=%s", w.Code, w.Body.String())
	}

	w2 := httptest.NewRecorder()
	IndustriasHandler(db)(w2, industriaReq(http.MethodGet, "/api/farol/industrias", empresaID, nil))
	var lista []IndustriaResponse
	if err := json.Unmarshal(w2.Body.Bytes(), &lista); err != nil {
		t.Fatalf("decode: %v", err)
	}
	var achada *IndustriaResponse
	for i := range lista {
		if lista[i].Nome == nome {
			achada = &lista[i]
		}
	}
	if achada == nil {
		t.Fatalf("indústria %s não apareceu na lista: %+v", nome, lista)
	}
	if len(achada.Fornecedores) != 2 {
		t.Errorf("esperava 2 fornecedores vinculados, veio %d: %+v", len(achada.Fornecedores), achada.Fornecedores)
	}
}

func TestIndustrias_CodFornecJaMapeado_Conflito409(t *testing.T) {
	db, empresaID := biTestDB(t)
	nomeA := "TIND A CONFLITO"
	nomeB := "TIND B CONFLITO"
	for _, n := range []string{nomeA, nomeB} {
		limparIndustriaFixture(t, db, empresaID, n)
	}
	t.Cleanup(func() {
		for _, n := range []string{nomeA, nomeB} {
			limparIndustriaFixture(t, db, empresaID, n)
		}
	})

	wA := httptest.NewRecorder()
	IndustriasHandler(db)(wA, industriaReq(http.MethodPost, "/api/farol/industrias", empresaID,
		IndustriaRequest{Nome: nomeA, Fornecedores: []IndustriaFornecedorDTO{{CodFornec: "99001"}}}))
	if wA.Code != http.StatusCreated {
		t.Fatalf("setup indústria A: status %d, body=%s", wA.Code, wA.Body.String())
	}

	wB := httptest.NewRecorder()
	IndustriasHandler(db)(wB, industriaReq(http.MethodPost, "/api/farol/industrias", empresaID,
		IndustriaRequest{Nome: nomeB, Fornecedores: []IndustriaFornecedorDTO{{CodFornec: "99001"}}}))
	if wB.Code != http.StatusConflict {
		t.Fatalf("POST reusando cod_fornec de outra indústria → status %d, want 409, body=%s", wB.Code, wB.Body.String())
	}
	if !bytes.Contains(wB.Body.Bytes(), []byte(nomeA)) {
		t.Errorf("mensagem de conflito não cita a indústria conflitante (%s): %s", nomeA, wB.Body.String())
	}
}

func TestIndustrias_NomeDuplicado_Conflito409(t *testing.T) {
	db, empresaID := biTestDB(t)
	nome := "TIND NOME DUPLICADO"
	limparIndustriaFixture(t, db, empresaID, nome)
	t.Cleanup(func() { limparIndustriaFixture(t, db, empresaID, nome) })

	w1 := httptest.NewRecorder()
	IndustriasHandler(db)(w1, industriaReq(http.MethodPost, "/api/farol/industrias", empresaID, IndustriaRequest{Nome: nome}))
	if w1.Code != http.StatusCreated {
		t.Fatalf("setup: status %d, body=%s", w1.Code, w1.Body.String())
	}

	w2 := httptest.NewRecorder()
	IndustriasHandler(db)(w2, industriaReq(http.MethodPost, "/api/farol/industrias", empresaID, IndustriaRequest{Nome: nome}))
	if w2.Code != http.StatusConflict {
		t.Fatalf("POST com nome duplicado → status %d, want 409, body=%s", w2.Code, w2.Body.String())
	}
}

func TestIndustrias_PutSubstituiConjuntoDeFornecedores(t *testing.T) {
	db, empresaID := biTestDB(t)
	nome := "TIND REPLACE"
	limparIndustriaFixture(t, db, empresaID, nome)
	t.Cleanup(func() { limparIndustriaFixture(t, db, empresaID, nome) })

	wc := httptest.NewRecorder()
	IndustriasHandler(db)(wc, industriaReq(http.MethodPost, "/api/farol/industrias", empresaID, IndustriaRequest{
		Nome: nome,
		Fornecedores: []IndustriaFornecedorDTO{
			{CodFornec: "11111"}, {CodFornec: "22222"}, {CodFornec: "33333"},
		},
	}))
	if wc.Code != http.StatusCreated {
		t.Fatalf("setup: status %d, body=%s", wc.Code, wc.Body.String())
	}
	var created map[string]int
	json.Unmarshal(wc.Body.Bytes(), &created)
	id := created["id"]

	wp := httptest.NewRecorder()
	IndustriaItemHandler(db)(wp, industriaReq(http.MethodPut, fmt.Sprintf("/api/farol/industrias/%d", id), empresaID, IndustriaRequest{
		Nome:         nome,
		Fornecedores: []IndustriaFornecedorDTO{{CodFornec: "44444"}, {CodFornec: "55555"}},
	}))
	if wp.Code != http.StatusOK {
		t.Fatalf("PUT → status %d, body=%s", wp.Code, wp.Body.String())
	}

	wg := httptest.NewRecorder()
	IndustriasHandler(db)(wg, industriaReq(http.MethodGet, "/api/farol/industrias", empresaID, nil))
	var lista []IndustriaResponse
	json.Unmarshal(wg.Body.Bytes(), &lista)
	var achada *IndustriaResponse
	for i := range lista {
		if lista[i].ID == id {
			achada = &lista[i]
		}
	}
	if achada == nil {
		t.Fatalf("indústria %d não encontrada após PUT", id)
	}
	codigos := map[string]bool{}
	for _, f := range achada.Fornecedores {
		codigos[f.CodFornec] = true
	}
	if len(codigos) != 2 || !codigos["44444"] || !codigos["55555"] {
		t.Errorf("esperava só [44444, 55555] após replace, veio %+v", achada.Fornecedores)
	}
}

func TestIndustrias_Delete_RemoveEmCascata(t *testing.T) {
	db, empresaID := biTestDB(t)
	nome := "TIND DELETE CASCADE"
	limparIndustriaFixture(t, db, empresaID, nome)
	t.Cleanup(func() { limparIndustriaFixture(t, db, empresaID, nome) })

	wc := httptest.NewRecorder()
	IndustriasHandler(db)(wc, industriaReq(http.MethodPost, "/api/farol/industrias", empresaID, IndustriaRequest{
		Nome: nome, Fornecedores: []IndustriaFornecedorDTO{{CodFornec: "77777"}},
	}))
	var created map[string]int
	json.Unmarshal(wc.Body.Bytes(), &created)
	id := created["id"]

	wd := httptest.NewRecorder()
	IndustriaItemHandler(db)(wd, industriaReq(http.MethodDelete, fmt.Sprintf("/api/farol/industrias/%d", id), empresaID, nil))
	if wd.Code != http.StatusOK {
		t.Fatalf("DELETE → status %d, body=%s", wd.Code, wd.Body.String())
	}

	var restam int
	db.QueryRow(`SELECT COUNT(*) FROM farol.industria_fornecedores WHERE industria_id = $1`, id).Scan(&restam)
	if restam != 0 {
		t.Errorf("esperava 0 fornecedores vinculados após DELETE (cascade), veio %d", restam)
	}
}

func TestIndustrias_IsolamentoEntreEmpresas_404(t *testing.T) {
	db, empresaID := biTestDB(t)
	outraEmpresaID := "00000000-0000-0000-0000-000000000000"
	nome := "TIND ISOLAMENTO"
	limparIndustriaFixture(t, db, empresaID, nome)
	t.Cleanup(func() { limparIndustriaFixture(t, db, empresaID, nome) })

	wc := httptest.NewRecorder()
	IndustriasHandler(db)(wc, industriaReq(http.MethodPost, "/api/farol/industrias", empresaID, IndustriaRequest{Nome: nome}))
	var created map[string]int
	json.Unmarshal(wc.Body.Bytes(), &created)
	id := created["id"]

	wp := httptest.NewRecorder()
	IndustriaItemHandler(db)(wp, industriaReq(http.MethodPut, fmt.Sprintf("/api/farol/industrias/%d", id), outraEmpresaID, IndustriaRequest{Nome: "hack"}))
	if wp.Code != http.StatusNotFound {
		t.Errorf("PUT de outra empresa → status %d, want 404, body=%s", wp.Code, wp.Body.String())
	}

	wd := httptest.NewRecorder()
	IndustriaItemHandler(db)(wd, industriaReq(http.MethodDelete, fmt.Sprintf("/api/farol/industrias/%d", id), outraEmpresaID, nil))
	if wd.Code != http.StatusNotFound {
		t.Errorf("DELETE de outra empresa → status %d, want 404, body=%s", wd.Code, wd.Body.String())
	}
}
