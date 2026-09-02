package handlers

// farol_metas_vinculos_test.go — cobre a I/O Matrix da Story 2.1
// (_bmad-output/implementation-artifacts/2-1-vinculo-industria-tipo-metrica.md).
//
// Testes de integração: exigem DATABASE_URL (ver biTestDB). Cada teste cria
// e remove seus próprios dados, isolados por prefixo TMV (Tipo/indústria de
// Métrica-Vínculo).

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

func metaVinculoReq(method, url, empresaID, userID string, body any) *http.Request {
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

// criarIndustriaFixture/criarTipoMetricaFixture criam pré-requisitos de FK
// direto no banco (mais rápido que passar pelos handlers de novo) e
// retornam o ID criado.
func criarIndustriaFixture(t *testing.T, db *sql.DB, empresaID, nome string) int {
	t.Helper()
	db.Exec(`DELETE FROM farol.industrias WHERE empresa_id = $1 AND nome = $2`, empresaID, nome)
	var id int
	if err := db.QueryRow(`INSERT INTO farol.industrias (empresa_id, nome) VALUES ($1, $2) RETURNING id`, empresaID, nome).Scan(&id); err != nil {
		t.Fatalf("criar fixture de indústria %s: %v", nome, err)
	}
	return id
}

func criarTipoMetricaFixture(t *testing.T, db *sql.DB, empresaID, nome, nivel string, schema []ParametroSchemaDTO) int {
	t.Helper()
	db.Exec(`DELETE FROM farol.tipos_metrica WHERE empresa_id = $1 AND nome = $2`, empresaID, nome)
	schemaJSON, _ := json.Marshal(schema)
	var id int
	if err := db.QueryRow(`
		INSERT INTO farol.tipos_metrica (empresa_id, nome, nivel_agregacao, parametros_schema)
		VALUES ($1, $2, $3, $4) RETURNING id
	`, empresaID, nome, nivel, schemaJSON).Scan(&id); err != nil {
		t.Fatalf("criar fixture de Tipo de Métrica %s: %v", nome, err)
	}
	return id
}

func limparMetaVinculoFixture(t *testing.T, db *sql.DB, empresaID string, industriaID, tipoMetricaID int) {
	t.Helper()
	db.Exec(`DELETE FROM farol.metas_vinculos WHERE empresa_id = $1 AND industria_id = $2 AND tipo_metrica_id = $3`, empresaID, industriaID, tipoMetricaID)
}

func TestMetasVinculos_CriarComParametros(t *testing.T) {
	db, empresaID := biTestDB(t)
	userID := tipoMetricaTestUserID(t, db)
	industriaID := criarIndustriaFixture(t, db, empresaID, "TMV Unilever HC")
	tipoID := criarTipoMetricaFixture(t, db, empresaID, "TMV Cobertura por Rede", "rede",
		[]ParametroSchemaDTO{{Key: "limiar_valor_medio", Label: "Limiar (R$)", Type: "number"}})
	t.Cleanup(func() {
		limparMetaVinculoFixture(t, db, empresaID, industriaID, tipoID)
		db.Exec(`DELETE FROM farol.industrias WHERE id = $1`, industriaID)
		db.Exec(`DELETE FROM farol.tipos_metrica WHERE id = $1`, tipoID)
	})

	body := MetaVinculoRequest{
		IndustriaID: industriaID, TipoMetricaID: tipoID,
		ParametrosValores: map[string]any{"limiar_valor_medio": 9100},
	}
	w := httptest.NewRecorder()
	MetasVinculosHandler(db)(w, metaVinculoReq(http.MethodPost, "/api/farol/metas-vinculos", empresaID, userID, body))
	if w.Code != http.StatusCreated {
		t.Fatalf("POST → status %d, body=%s", w.Code, w.Body.String())
	}

	w2 := httptest.NewRecorder()
	MetasVinculosHandler(db)(w2, metaVinculoReq(http.MethodGet, "/api/farol/metas-vinculos", empresaID, userID, nil))
	var lista []MetaVinculoResponse
	json.Unmarshal(w2.Body.Bytes(), &lista)
	var achado *MetaVinculoResponse
	for i := range lista {
		if lista[i].IndustriaID == industriaID {
			achado = &lista[i]
		}
	}
	if achado == nil {
		t.Fatalf("vínculo não apareceu na lista: %+v", lista)
	}
	if achado.IndustriaNome != "TMV Unilever HC" || achado.TipoMetricaNome != "TMV Cobertura por Rede" {
		t.Errorf("JOIN de nomes incorreto: %+v", achado)
	}
	if v, ok := achado.ParametrosValores["limiar_valor_medio"]; !ok || fmt.Sprintf("%v", v) != "9100" {
		t.Errorf("parametros_valores não persistiu corretamente: %+v", achado.ParametrosValores)
	}
}

func TestMetasVinculos_ParametroObrigatorioFaltando_400(t *testing.T) {
	db, empresaID := biTestDB(t)
	userID := tipoMetricaTestUserID(t, db)
	industriaID := criarIndustriaFixture(t, db, empresaID, "TMV Sem Parametro")
	tipoID := criarTipoMetricaFixture(t, db, empresaID, "TMV Tipo Exige Parametro", "rede",
		[]ParametroSchemaDTO{{Key: "limiar_valor_medio", Label: "Limiar (R$)", Type: "number"}})
	t.Cleanup(func() {
		limparMetaVinculoFixture(t, db, empresaID, industriaID, tipoID)
		db.Exec(`DELETE FROM farol.industrias WHERE id = $1`, industriaID)
		db.Exec(`DELETE FROM farol.tipos_metrica WHERE id = $1`, tipoID)
	})

	w := httptest.NewRecorder()
	MetasVinculosHandler(db)(w, metaVinculoReq(http.MethodPost, "/api/farol/metas-vinculos", empresaID, userID, MetaVinculoRequest{
		IndustriaID: industriaID, TipoMetricaID: tipoID, ParametrosValores: map[string]any{},
	}))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("POST sem parâmetro obrigatório → status %d, want 400, body=%s", w.Code, w.Body.String())
	}
}

// TestMetasVinculos_ReusoDeTipoPorDuasIndustrias fecha o AC que ficou
// pendente da Story 1.3 (FR3): o mesmo Tipo de Métrica vinculado a 2
// indústrias diferentes, cada uma com seu próprio valor de parâmetro,
// calculando de forma independente.
func TestMetasVinculos_ReusoDeTipoPorDuasIndustrias(t *testing.T) {
	db, empresaID := biTestDB(t)
	userID := tipoMetricaTestUserID(t, db)
	industriaHC := criarIndustriaFixture(t, db, empresaID, "TMV Unilever HC Reuso")
	industriaFoods := criarIndustriaFixture(t, db, empresaID, "TMV Unilever Foods Reuso")
	tipoID := criarTipoMetricaFixture(t, db, empresaID, "TMV Cobertura Reuso", "rede",
		[]ParametroSchemaDTO{{Key: "limiar_valor_medio", Label: "Limiar (R$)", Type: "number"}})
	t.Cleanup(func() {
		limparMetaVinculoFixture(t, db, empresaID, industriaHC, tipoID)
		limparMetaVinculoFixture(t, db, empresaID, industriaFoods, tipoID)
		db.Exec(`DELETE FROM farol.industrias WHERE id IN ($1, $2)`, industriaHC, industriaFoods)
		db.Exec(`DELETE FROM farol.tipos_metrica WHERE id = $1`, tipoID)
	})

	wHC := httptest.NewRecorder()
	MetasVinculosHandler(db)(wHC, metaVinculoReq(http.MethodPost, "/api/farol/metas-vinculos", empresaID, userID, MetaVinculoRequest{
		IndustriaID: industriaHC, TipoMetricaID: tipoID, ParametrosValores: map[string]any{"limiar_valor_medio": 9100},
	}))
	if wHC.Code != http.StatusCreated {
		t.Fatalf("POST vínculo HC → status %d, body=%s", wHC.Code, wHC.Body.String())
	}

	wFoods := httptest.NewRecorder()
	MetasVinculosHandler(db)(wFoods, metaVinculoReq(http.MethodPost, "/api/farol/metas-vinculos", empresaID, userID, MetaVinculoRequest{
		IndustriaID: industriaFoods, TipoMetricaID: tipoID, ParametrosValores: map[string]any{"limiar_valor_medio": 1500},
	}))
	if wFoods.Code != http.StatusCreated {
		t.Fatalf("POST vínculo Foods (mesmo Tipo de Métrica) → status %d, body=%s", wFoods.Code, wFoods.Body.String())
	}

	w2 := httptest.NewRecorder()
	MetasVinculosHandler(db)(w2, metaVinculoReq(http.MethodGet, "/api/farol/metas-vinculos", empresaID, userID, nil))
	var lista []MetaVinculoResponse
	json.Unmarshal(w2.Body.Bytes(), &lista)
	var vHC, vFoods *MetaVinculoResponse
	for i := range lista {
		if lista[i].IndustriaID == industriaHC {
			vHC = &lista[i]
		}
		if lista[i].IndustriaID == industriaFoods {
			vFoods = &lista[i]
		}
	}
	if vHC == nil || vFoods == nil {
		t.Fatalf("os dois vínculos deveriam existir independentemente: %+v", lista)
	}
	if fmt.Sprintf("%v", vHC.ParametrosValores["limiar_valor_medio"]) == fmt.Sprintf("%v", vFoods.ParametrosValores["limiar_valor_medio"]) {
		t.Errorf("os dois vínculos deveriam ter limiares independentes, ambos vieram %v", vHC.ParametrosValores["limiar_valor_medio"])
	}
	if vHC.TipoMetricaID != vFoods.TipoMetricaID {
		t.Errorf("os dois vínculos deveriam apontar pro MESMO Tipo de Métrica (reuso, FR3)")
	}
}

// TestMetasVinculos_RecorteOrganizacional cobre a Story 2.3 (FR5): recorte
// parcial (UF + GGVs) persiste e volta corretamente na resposta.
func TestMetasVinculos_RecorteOrganizacional(t *testing.T) {
	db, empresaID := biTestDB(t)
	userID := tipoMetricaTestUserID(t, db)
	industriaID := criarIndustriaFixture(t, db, empresaID, "TMV Recorte")
	tipoID := criarTipoMetricaFixture(t, db, empresaID, "TMV Tipo Recorte", "rede",
		[]ParametroSchemaDTO{{Key: "x", Label: "X", Type: "number"}})
	t.Cleanup(func() {
		limparMetaVinculoFixture(t, db, empresaID, industriaID, tipoID)
		db.Exec(`DELETE FROM farol.industrias WHERE id = $1`, industriaID)
		db.Exec(`DELETE FROM farol.tipos_metrica WHERE id = $1`, tipoID)
	})

	wc := httptest.NewRecorder()
	MetasVinculosHandler(db)(wc, metaVinculoReq(http.MethodPost, "/api/farol/metas-vinculos", empresaID, userID, MetaVinculoRequest{
		IndustriaID: industriaID, TipoMetricaID: tipoID, ParametrosValores: map[string]any{"x": 1},
		RecorteUF: "GO", RecorteGGVs: []string{"GO", "GO FOOD", "V7"},
	}))
	if wc.Code != http.StatusCreated {
		t.Fatalf("POST com recorte → status %d, body=%s", wc.Code, wc.Body.String())
	}

	w2 := httptest.NewRecorder()
	MetasVinculosHandler(db)(w2, metaVinculoReq(http.MethodGet, "/api/farol/metas-vinculos", empresaID, userID, nil))
	var lista []MetaVinculoResponse
	json.Unmarshal(w2.Body.Bytes(), &lista)
	var achado *MetaVinculoResponse
	for i := range lista {
		if lista[i].IndustriaID == industriaID {
			achado = &lista[i]
		}
	}
	if achado == nil {
		t.Fatalf("vínculo não encontrado")
	}
	if achado.RecorteUF != "GO" || len(achado.RecorteGGVs) != 3 {
		t.Errorf("recorte não persistiu corretamente: uf=%q ggvs=%v", achado.RecorteUF, achado.RecorteGGVs)
	}
}

func TestMetasVinculos_MesmaIndustriaMesmoTipo_Conflito409(t *testing.T) {
	db, empresaID := biTestDB(t)
	userID := tipoMetricaTestUserID(t, db)
	industriaID := criarIndustriaFixture(t, db, empresaID, "TMV Duplicado")
	tipoID := criarTipoMetricaFixture(t, db, empresaID, "TMV Tipo Duplicado", "rede",
		[]ParametroSchemaDTO{{Key: "x", Label: "X", Type: "number"}})
	t.Cleanup(func() {
		limparMetaVinculoFixture(t, db, empresaID, industriaID, tipoID)
		db.Exec(`DELETE FROM farol.industrias WHERE id = $1`, industriaID)
		db.Exec(`DELETE FROM farol.tipos_metrica WHERE id = $1`, tipoID)
	})

	reqBody := MetaVinculoRequest{IndustriaID: industriaID, TipoMetricaID: tipoID, ParametrosValores: map[string]any{"x": 1}}
	w1 := httptest.NewRecorder()
	MetasVinculosHandler(db)(w1, metaVinculoReq(http.MethodPost, "/api/farol/metas-vinculos", empresaID, userID, reqBody))
	if w1.Code != http.StatusCreated {
		t.Fatalf("setup: status %d, body=%s", w1.Code, w1.Body.String())
	}

	w2 := httptest.NewRecorder()
	MetasVinculosHandler(db)(w2, metaVinculoReq(http.MethodPost, "/api/farol/metas-vinculos", empresaID, userID, reqBody))
	if w2.Code != http.StatusConflict {
		t.Fatalf("POST duplicado → status %d, want 409, body=%s", w2.Code, w2.Body.String())
	}
}

func TestMetasVinculos_IsolamentoEntreEmpresas_404(t *testing.T) {
	db, empresaID := biTestDB(t)
	userID := tipoMetricaTestUserID(t, db)
	outraEmpresaID := "00000000-0000-0000-0000-000000000000"
	industriaID := criarIndustriaFixture(t, db, empresaID, "TMV Isolamento")
	tipoID := criarTipoMetricaFixture(t, db, empresaID, "TMV Tipo Isolamento", "rede",
		[]ParametroSchemaDTO{{Key: "x", Label: "X", Type: "number"}})
	t.Cleanup(func() {
		limparMetaVinculoFixture(t, db, empresaID, industriaID, tipoID)
		db.Exec(`DELETE FROM farol.industrias WHERE id = $1`, industriaID)
		db.Exec(`DELETE FROM farol.tipos_metrica WHERE id = $1`, tipoID)
	})

	wc := httptest.NewRecorder()
	MetasVinculosHandler(db)(wc, metaVinculoReq(http.MethodPost, "/api/farol/metas-vinculos", empresaID, userID, MetaVinculoRequest{
		IndustriaID: industriaID, TipoMetricaID: tipoID, ParametrosValores: map[string]any{"x": 1},
	}))
	var created map[string]int
	json.Unmarshal(wc.Body.Bytes(), &created)
	id := created["id"]

	wd := httptest.NewRecorder()
	MetaVinculoItemHandler(db)(wd, metaVinculoReq(http.MethodDelete, fmt.Sprintf("/api/farol/metas-vinculos/%d", id), outraEmpresaID, userID, nil))
	if wd.Code != http.StatusNotFound {
		t.Errorf("DELETE de outra empresa → status %d, want 404, body=%s", wd.Code, wd.Body.String())
	}
}
