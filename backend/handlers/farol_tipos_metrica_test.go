package handlers

// farol_tipos_metrica_test.go — cobre a I/O Matrix da Story 1.1
// (_bmad-output/implementation-artifacts/1-1-cadastro-tipo-metrica.md).
//
// Testes de integração: exigem DATABASE_URL (ver biTestDB em
// farol_bi_api_test.go). Cada teste cria e remove seus próprios dados,
// isolados por nome de Tipo de Métrica único (prefixo TTM).

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

// tipoMetricaTestUserID busca um user_id real — writeAuditLogTx faz
// $2::uuid contra farol.sp_audit_log.user_id (FK pra users), então o
// literal "teste" usado em industriaReq/biReq não serve aqui (só funciona
// pra handlers que nunca chamam auditoria).
func tipoMetricaTestUserID(t *testing.T, db *sql.DB) string {
	t.Helper()
	var userID string
	if err := db.QueryRow(`SELECT id::text FROM users ORDER BY created_at LIMIT 1`).Scan(&userID); err != nil {
		t.Skipf("nenhum usuário na base (%v) — teste pulado", err)
	}
	return userID
}

// tipoMetricaReq monta uma request autenticada com sp_role=gestor_geral
// (nível mínimo exigido pelas rotas de Tipos de Métrica, diferente de
// gestor_filial usado pelo precedente de Indústrias).
func tipoMetricaReq(method, url, empresaID, userID string, body any) *http.Request {
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	r := httptest.NewRequest(method, url, &buf)
	ctx := context.WithValue(r.Context(), SpContextKey, &FarolContext{
		UserID: userID, SpRole: "gestor_geral", EmpresaID: empresaID, AllFiliais: true,
	})
	return r.WithContext(ctx)
}

func limparTipoMetricaFixture(t *testing.T, db *sql.DB, empresaID, nome string) {
	t.Helper()
	if _, err := db.Exec(`DELETE FROM farol.tipos_metrica WHERE empresa_id = $1 AND nome = $2`, empresaID, nome); err != nil {
		t.Fatalf("limpar fixture de Tipo de Métrica %s: %v", nome, err)
	}
}

func TestTiposMetrica_CriarCoberturaPorRede(t *testing.T) {
	db, empresaID := biTestDB(t)
	userID := tipoMetricaTestUserID(t, db)
	nome := "TTM Cobertura por Rede"
	limparTipoMetricaFixture(t, db, empresaID, nome)
	t.Cleanup(func() { limparTipoMetricaFixture(t, db, empresaID, nome) })

	body := TipoMetricaRequest{
		Nome:           nome,
		Descricao:      "Média de compra por loja acima de um limiar em R$",
		NivelAgregacao: "rede",
		ParametrosSchema: []ParametroSchemaDTO{
			{Key: "limiar_valor", Label: "Limiar de valor médio (R$)", Type: "number"},
		},
	}
	w := httptest.NewRecorder()
	TiposMetricaHandler(db)(w, tipoMetricaReq(http.MethodPost, "/api/farol/tipos-metrica", empresaID, userID, body))
	if w.Code != http.StatusCreated {
		t.Fatalf("POST → status %d, body=%s", w.Code, w.Body.String())
	}

	w2 := httptest.NewRecorder()
	TiposMetricaHandler(db)(w2, tipoMetricaReq(http.MethodGet, "/api/farol/tipos-metrica", empresaID, userID, nil))
	var lista []TipoMetricaResponse
	if err := json.Unmarshal(w2.Body.Bytes(), &lista); err != nil {
		t.Fatalf("decode: %v", err)
	}
	var achado *TipoMetricaResponse
	for i := range lista {
		if lista[i].Nome == nome {
			achado = &lista[i]
		}
	}
	if achado == nil {
		t.Fatalf("Tipo de Métrica %s não apareceu na lista: %+v", nome, lista)
	}
	if achado.NivelAgregacao != "rede" || len(achado.ParametrosSchema) != 1 || achado.ParametrosSchema[0].Key != "limiar_valor" {
		t.Errorf("dados persistidos não batem com o esperado: %+v", achado)
	}
}

// TestTiposMetrica_CriarTipoHipotetico_SemAlterarSchema é o teste de
// generalidade do FR1 (prd.md linha 70): um Tipo de Métrica com nível de
// agregação e parâmetro totalmente diferentes de Cobertura/Sortimento
// precisa caber no mesmo endpoint/tabela, sem migration adicional.
func TestTiposMetrica_CriarTipoHipotetico_SemAlterarSchema(t *testing.T) {
	db, empresaID := biTestDB(t)
	userID := tipoMetricaTestUserID(t, db)
	nome := "TTM Frequência de Visita por Cliente"
	limparTipoMetricaFixture(t, db, empresaID, nome)
	t.Cleanup(func() { limparTipoMetricaFixture(t, db, empresaID, nome) })

	body := TipoMetricaRequest{
		Nome:           nome,
		NivelAgregacao: "cliente", // nível diferente de "rede" (Cobertura/Sortimento)
		ParametrosSchema: []ParametroSchemaDTO{
			{Key: "min_visitas", Label: "Nº mínimo de visitas no período", Type: "integer"},
		},
	}
	w := httptest.NewRecorder()
	TiposMetricaHandler(db)(w, tipoMetricaReq(http.MethodPost, "/api/farol/tipos-metrica", empresaID, userID, body))
	if w.Code != http.StatusCreated {
		t.Fatalf("POST de tipo hipotético → status %d, body=%s (a genericidade do modelo falhou)", w.Code, w.Body.String())
	}

	w2 := httptest.NewRecorder()
	TiposMetricaHandler(db)(w2, tipoMetricaReq(http.MethodGet, "/api/farol/tipos-metrica", empresaID, userID, nil))
	var lista []TipoMetricaResponse
	json.Unmarshal(w2.Body.Bytes(), &lista)
	var achado *TipoMetricaResponse
	for i := range lista {
		if lista[i].Nome == nome {
			achado = &lista[i]
		}
	}
	if achado == nil {
		t.Fatalf("tipo hipotético não apareceu na lista: %+v", lista)
	}
	if achado.NivelAgregacao != "cliente" || len(achado.ParametrosSchema) != 1 || achado.ParametrosSchema[0].Key != "min_visitas" {
		t.Errorf("shape diferente não foi preservado corretamente: %+v", achado)
	}
}

func TestTiposMetrica_NomeDuplicado_Conflito409(t *testing.T) {
	db, empresaID := biTestDB(t)
	userID := tipoMetricaTestUserID(t, db)
	nome := "TTM Nome Duplicado"
	limparTipoMetricaFixture(t, db, empresaID, nome)
	t.Cleanup(func() { limparTipoMetricaFixture(t, db, empresaID, nome) })

	reqBody := TipoMetricaRequest{
		Nome: nome, NivelAgregacao: "rede",
		ParametrosSchema: []ParametroSchemaDTO{{Key: "x", Label: "X", Type: "number"}},
	}

	w1 := httptest.NewRecorder()
	TiposMetricaHandler(db)(w1, tipoMetricaReq(http.MethodPost, "/api/farol/tipos-metrica", empresaID, userID, reqBody))
	if w1.Code != http.StatusCreated {
		t.Fatalf("setup: status %d, body=%s", w1.Code, w1.Body.String())
	}

	w2 := httptest.NewRecorder()
	TiposMetricaHandler(db)(w2, tipoMetricaReq(http.MethodPost, "/api/farol/tipos-metrica", empresaID, userID, reqBody))
	if w2.Code != http.StatusConflict {
		t.Fatalf("POST com nome duplicado → status %d, want 409, body=%s", w2.Code, w2.Body.String())
	}
}

func TestTiposMetrica_ParametrosSchemaVazio_400(t *testing.T) {
	db, empresaID := biTestDB(t)
	userID := tipoMetricaTestUserID(t, db)
	nome := "TTM Sem Parametros"
	limparTipoMetricaFixture(t, db, empresaID, nome)
	t.Cleanup(func() { limparTipoMetricaFixture(t, db, empresaID, nome) })

	w := httptest.NewRecorder()
	TiposMetricaHandler(db)(w, tipoMetricaReq(http.MethodPost, "/api/farol/tipos-metrica", empresaID, userID, TipoMetricaRequest{
		Nome: nome, NivelAgregacao: "rede", ParametrosSchema: []ParametroSchemaDTO{},
	}))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("POST sem parâmetros → status %d, want 400, body=%s", w.Code, w.Body.String())
	}
}

func TestTiposMetrica_RequerGestorGeral_403(t *testing.T) {
	db, empresaID := biTestDB(t)
	nome := "TTM Sem Permissao"
	limparTipoMetricaFixture(t, db, empresaID, nome)
	t.Cleanup(func() { limparTipoMetricaFixture(t, db, empresaID, nome) })

	r := httptest.NewRequest(http.MethodPost, "/api/farol/tipos-metrica", bytes.NewReader(mustJSON(TipoMetricaRequest{
		Nome: nome, NivelAgregacao: "rede",
		ParametrosSchema: []ParametroSchemaDTO{{Key: "x", Label: "X", Type: "number"}},
	})))
	ctx := context.WithValue(r.Context(), SpContextKey, &FarolContext{
		UserID: "teste", SpRole: "gestor_filial", EmpresaID: empresaID, AllFiliais: true,
	})
	r = r.WithContext(ctx)

	w := httptest.NewRecorder()
	TiposMetricaHandler(db)(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("POST com sp_role=gestor_filial → status %d, want 403, body=%s", w.Code, w.Body.String())
	}
}

func TestTiposMetrica_Editar_GravaAuditLog(t *testing.T) {
	db, empresaID := biTestDB(t)
	userID := tipoMetricaTestUserID(t, db)
	nome := "TTM Editar Audit"
	limparTipoMetricaFixture(t, db, empresaID, nome)
	t.Cleanup(func() { limparTipoMetricaFixture(t, db, empresaID, nome) })

	wc := httptest.NewRecorder()
	TiposMetricaHandler(db)(wc, tipoMetricaReq(http.MethodPost, "/api/farol/tipos-metrica", empresaID, userID, TipoMetricaRequest{
		Nome: nome, NivelAgregacao: "rede",
		ParametrosSchema: []ParametroSchemaDTO{{Key: "limiar_valor", Label: "Limiar (R$)", Type: "number"}},
	}))
	if wc.Code != http.StatusCreated {
		t.Fatalf("setup: status %d, body=%s", wc.Code, wc.Body.String())
	}
	var created map[string]int
	json.Unmarshal(wc.Body.Bytes(), &created)
	id := created["id"]

	wp := httptest.NewRecorder()
	TipoMetricaItemHandler(db)(wp, tipoMetricaReq(http.MethodPut, fmt.Sprintf("/api/farol/tipos-metrica/%d", id), empresaID, userID, TipoMetricaRequest{
		Nome: nome, NivelAgregacao: "rede",
		ParametrosSchema: []ParametroSchemaDTO{{Key: "limiar_valor", Label: "Limiar (R$) v2", Type: "number"}},
	}))
	if wp.Code != http.StatusOK {
		t.Fatalf("PUT → status %d, body=%s", wp.Code, wp.Body.String())
	}

	var acao string
	var payload []byte
	err := db.QueryRow(`
		SELECT acao, payload FROM farol.sp_audit_log
		WHERE entidade = 'tipos_metrica' AND entidade_id = $1
		ORDER BY created_at DESC LIMIT 1
	`, fmt.Sprintf("%d", id)).Scan(&acao, &payload)
	if err != nil {
		t.Fatalf("audit log não encontrado pra tipos_metrica/%d: %v", id, err)
	}
	if acao != "editar" {
		t.Errorf("acao = %q, want \"editar\"", acao)
	}
	if !bytes.Contains(payload, []byte("antes")) || !bytes.Contains(payload, []byte("depois")) {
		t.Errorf("payload de auditoria não contém antes/depois: %s", payload)
	}
}

func TestTiposMetrica_IsolamentoEntreEmpresas_404(t *testing.T) {
	db, empresaID := biTestDB(t)
	userID := tipoMetricaTestUserID(t, db)
	outraEmpresaID := "00000000-0000-0000-0000-000000000000"
	nome := "TTM Isolamento"
	limparTipoMetricaFixture(t, db, empresaID, nome)
	t.Cleanup(func() { limparTipoMetricaFixture(t, db, empresaID, nome) })

	wc := httptest.NewRecorder()
	TiposMetricaHandler(db)(wc, tipoMetricaReq(http.MethodPost, "/api/farol/tipos-metrica", empresaID, userID, TipoMetricaRequest{
		Nome: nome, NivelAgregacao: "rede",
		ParametrosSchema: []ParametroSchemaDTO{{Key: "x", Label: "X", Type: "number"}},
	}))
	var created map[string]int
	json.Unmarshal(wc.Body.Bytes(), &created)
	id := created["id"]

	wp := httptest.NewRecorder()
	TipoMetricaItemHandler(db)(wp, tipoMetricaReq(http.MethodPut, fmt.Sprintf("/api/farol/tipos-metrica/%d", id), outraEmpresaID, userID, TipoMetricaRequest{
		Nome: "hack", NivelAgregacao: "rede",
		ParametrosSchema: []ParametroSchemaDTO{{Key: "x", Label: "X", Type: "number"}},
	}))
	if wp.Code != http.StatusNotFound {
		t.Errorf("PUT de outra empresa → status %d, want 404, body=%s", wp.Code, wp.Body.String())
	}

	wd := httptest.NewRecorder()
	TipoMetricaItemHandler(db)(wd, tipoMetricaReq(http.MethodDelete, fmt.Sprintf("/api/farol/tipos-metrica/%d", id), outraEmpresaID, userID, nil))
	if wd.Code != http.StatusNotFound {
		t.Errorf("DELETE de outra empresa → status %d, want 404, body=%s", wd.Code, wd.Body.String())
	}
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
