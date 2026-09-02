package handlers

// farol_metas_vigencias_test.go — cobre a I/O Matrix da Story 2.2
// (_bmad-output/implementation-artifacts/2-2-metas-faixa-vigencia.md).

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

func vigenciaReq(method, url, empresaID, userID string, body any) *http.Request {
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

// criarVinculoFixture monta indústria + tipo de métrica + vínculo direto no
// banco (mais rápido que passar pelos handlers) e retorna o vinculo_id.
func criarVinculoFixture(t *testing.T, db *sql.DB, empresaID, prefixo string) (vinculoID int, cleanup func()) {
	t.Helper()
	industriaID := criarIndustriaFixture(t, db, empresaID, prefixo+" Industria")
	tipoID := criarTipoMetricaFixture(t, db, empresaID, prefixo+" Tipo", "rede",
		[]ParametroSchemaDTO{{Key: "x", Label: "X", Type: "number"}})
	db.Exec(`DELETE FROM farol.metas_vinculos WHERE empresa_id = $1 AND industria_id = $2 AND tipo_metrica_id = $3`, empresaID, industriaID, tipoID)
	var id int
	if err := db.QueryRow(`
		INSERT INTO farol.metas_vinculos (empresa_id, industria_id, tipo_metrica_id, parametros_valores)
		VALUES ($1, $2, $3, '{"x":1}') RETURNING id
	`, empresaID, industriaID, tipoID).Scan(&id); err != nil {
		t.Fatalf("criar fixture de vínculo: %v", err)
	}
	return id, func() {
		db.Exec(`DELETE FROM farol.metas_vinculos WHERE id = $1`, id)
		db.Exec(`DELETE FROM farol.industrias WHERE id = $1`, industriaID)
		db.Exec(`DELETE FROM farol.tipos_metrica WHERE id = $1`, tipoID)
	}
}

func TestMetasVigencias_CriarComFaixas(t *testing.T) {
	db, empresaID := biTestDB(t)
	userID := tipoMetricaTestUserID(t, db)
	vinculoID, cleanup := criarVinculoFixture(t, db, empresaID, "TVIG Criar")
	t.Cleanup(cleanup)

	body := VigenciaRequest{
		VinculoID: vinculoID, DataInicio: "2026-01-01", DataFim: "2026-03-31",
		Faixas: []FaixaDTO{{Faixa: 3, ValorMeta: 78}, {Faixa: 2, ValorMeta: 85}, {Faixa: 1, ValorMeta: 91}},
	}
	w := httptest.NewRecorder()
	MetasVigenciasHandler(db)(w, vigenciaReq(http.MethodPost, "/api/farol/metas-vigencias", empresaID, userID, body))
	if w.Code != http.StatusCreated {
		t.Fatalf("POST → status %d, body=%s", w.Code, w.Body.String())
	}

	w2 := httptest.NewRecorder()
	MetasVigenciasHandler(db)(w2, vigenciaReq(http.MethodGet, fmt.Sprintf("/api/farol/metas-vigencias?vinculo_id=%d", vinculoID), empresaID, userID, nil))
	var lista []VigenciaResponse
	json.Unmarshal(w2.Body.Bytes(), &lista)
	if len(lista) != 1 {
		t.Fatalf("esperava 1 vigência, veio %d: %+v", len(lista), lista)
	}
	if len(lista[0].Faixas) != 3 || lista[0].Status != "aberta" {
		t.Errorf("faixas/status incorretos: %+v", lista[0])
	}
}

// TestMetasVigencias_MultiplasVigenciasMesmoVinculo cobre o AC central da
// Story 2.2: um vínculo pode ter várias vigências ao longo do ano, cada uma
// com valores diferentes, sem uma sobrescrever a outra.
func TestMetasVigencias_MultiplasVigenciasMesmoVinculo(t *testing.T) {
	db, empresaID := biTestDB(t)
	userID := tipoMetricaTestUserID(t, db)
	vinculoID, cleanup := criarVinculoFixture(t, db, empresaID, "TVIG Multiplas")
	t.Cleanup(cleanup)

	w1 := httptest.NewRecorder()
	MetasVigenciasHandler(db)(w1, vigenciaReq(http.MethodPost, "/api/farol/metas-vigencias", empresaID, userID, VigenciaRequest{
		VinculoID: vinculoID, DataInicio: "2026-01-01", DataFim: "2026-03-31",
		Faixas: []FaixaDTO{{Faixa: 1, ValorMeta: 100}},
	}))
	if w1.Code != http.StatusCreated {
		t.Fatalf("POST jan-mar → status %d, body=%s", w1.Code, w1.Body.String())
	}

	w2 := httptest.NewRecorder()
	MetasVigenciasHandler(db)(w2, vigenciaReq(http.MethodPost, "/api/farol/metas-vigencias", empresaID, userID, VigenciaRequest{
		VinculoID: vinculoID, DataInicio: "2026-04-01", DataFim: "2026-06-30",
		Faixas: []FaixaDTO{{Faixa: 1, ValorMeta: 200}},
	}))
	if w2.Code != http.StatusCreated {
		t.Fatalf("POST abr-jun → status %d, body=%s", w2.Code, w2.Body.String())
	}

	w3 := httptest.NewRecorder()
	MetasVigenciasHandler(db)(w3, vigenciaReq(http.MethodGet, fmt.Sprintf("/api/farol/metas-vigencias?vinculo_id=%d", vinculoID), empresaID, userID, nil))
	var lista []VigenciaResponse
	json.Unmarshal(w3.Body.Bytes(), &lista)
	if len(lista) != 2 {
		t.Fatalf("esperava 2 vigências distintas, veio %d: %+v", len(lista), lista)
	}
}

func TestMetasVigencias_Sobreposicao_Conflito409(t *testing.T) {
	db, empresaID := biTestDB(t)
	userID := tipoMetricaTestUserID(t, db)
	vinculoID, cleanup := criarVinculoFixture(t, db, empresaID, "TVIG Sobreposicao")
	t.Cleanup(cleanup)

	w1 := httptest.NewRecorder()
	MetasVigenciasHandler(db)(w1, vigenciaReq(http.MethodPost, "/api/farol/metas-vigencias", empresaID, userID, VigenciaRequest{
		VinculoID: vinculoID, DataInicio: "2026-01-01", DataFim: "2026-03-31",
		Faixas: []FaixaDTO{{Faixa: 1, ValorMeta: 100}},
	}))
	if w1.Code != http.StatusCreated {
		t.Fatalf("setup: status %d, body=%s", w1.Code, w1.Body.String())
	}

	w2 := httptest.NewRecorder()
	MetasVigenciasHandler(db)(w2, vigenciaReq(http.MethodPost, "/api/farol/metas-vigencias", empresaID, userID, VigenciaRequest{
		VinculoID: vinculoID, DataInicio: "2026-03-01", DataFim: "2026-05-31", // sobrepõe março
		Faixas: []FaixaDTO{{Faixa: 1, ValorMeta: 200}},
	}))
	if w2.Code != http.StatusConflict {
		t.Fatalf("POST sobreposto → status %d, want 409, body=%s", w2.Code, w2.Body.String())
	}
}

// TestMetasVigencias_FecharBloqueiaEdicao cobre o congelamento (FR17): uma
// vez fechada, a vigência não pode mais ser editada por esta tela.
func TestMetasVigencias_FecharBloqueiaEdicao(t *testing.T) {
	db, empresaID := biTestDB(t)
	userID := tipoMetricaTestUserID(t, db)
	vinculoID, cleanup := criarVinculoFixture(t, db, empresaID, "TVIG Fechar")
	t.Cleanup(cleanup)

	wc := httptest.NewRecorder()
	MetasVigenciasHandler(db)(wc, vigenciaReq(http.MethodPost, "/api/farol/metas-vigencias", empresaID, userID, VigenciaRequest{
		VinculoID: vinculoID, DataInicio: "2026-01-01", DataFim: "2026-01-31",
		Faixas: []FaixaDTO{{Faixa: 1, ValorMeta: 100}},
	}))
	var created map[string]int
	json.Unmarshal(wc.Body.Bytes(), &created)
	id := created["id"]

	wf := httptest.NewRecorder()
	MetaVigenciaItemHandler(db)(wf, vigenciaReq(http.MethodPost, fmt.Sprintf("/api/farol/metas-vigencias/%d/fechar", id), empresaID, userID, nil))
	if wf.Code != http.StatusOK {
		t.Fatalf("POST fechar → status %d, body=%s", wf.Code, wf.Body.String())
	}

	wp := httptest.NewRecorder()
	MetaVigenciaItemHandler(db)(wp, vigenciaReq(http.MethodPut, fmt.Sprintf("/api/farol/metas-vigencias/%d", id), empresaID, userID, VigenciaRequest{
		DataInicio: "2026-01-01", DataFim: "2026-01-31", Faixas: []FaixaDTO{{Faixa: 1, ValorMeta: 999}},
	}))
	if wp.Code != http.StatusForbidden {
		t.Fatalf("PUT numa vigência fechada → status %d, want 403, body=%s", wp.Code, wp.Body.String())
	}

	wd := httptest.NewRecorder()
	MetaVigenciaItemHandler(db)(wd, vigenciaReq(http.MethodDelete, fmt.Sprintf("/api/farol/metas-vigencias/%d", id), empresaID, userID, nil))
	if wd.Code != http.StatusForbidden {
		t.Fatalf("DELETE numa vigência fechada → status %d, want 403, body=%s", wd.Code, wd.Body.String())
	}
}

func TestMetasVigencias_FaixaVazia_400(t *testing.T) {
	db, empresaID := biTestDB(t)
	userID := tipoMetricaTestUserID(t, db)
	vinculoID, cleanup := criarVinculoFixture(t, db, empresaID, "TVIG SemFaixa")
	t.Cleanup(cleanup)

	w := httptest.NewRecorder()
	MetasVigenciasHandler(db)(w, vigenciaReq(http.MethodPost, "/api/farol/metas-vigencias", empresaID, userID, VigenciaRequest{
		VinculoID: vinculoID, DataInicio: "2026-01-01", DataFim: "2026-01-31", Faixas: []FaixaDTO{},
	}))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("POST sem faixa → status %d, want 400, body=%s", w.Code, w.Body.String())
	}
}
