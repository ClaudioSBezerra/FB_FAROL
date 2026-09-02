package handlers

// farol_metas_public_test.go — cobre a I/O Matrix da Story 6.1
// (_bmad-output/implementation-artifacts/6-1-painel-mobile-link-publico.md).
//
// O teste central (TestMetasPublicPainel_EscopoPorRCA_NaoVazaOutrasRedes)
// é de SEGURANÇA: confirma que o painel público de um RCA não mostra
// Redes de outro RCA — é sem login, então esse isolamento é o que impede
// um link vazado de expor dado de fora do escopo pretendido.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMetasPublicVinculos_ResolvePorCNPJ(t *testing.T) {
	db, empresaID := biTestDB(t)
	var cnpjEmpresa string
	if err := db.QueryRow(`SELECT regexp_replace(cnpj, '[^0-9]', '', 'g') FROM companies WHERE id = $1`, empresaID).Scan(&cnpjEmpresa); err != nil || cnpjEmpresa == "" {
		t.Skip("empresa de teste sem CNPJ cadastrado — teste pulado")
	}
	vinculoID, cleanup := criarVinculoComFormula(t, empresaID, "TPUB Vinculos", "cobertura_rede", "rede",
		[]ParametroSchemaDTO{{Key: "limiar_valor_medio", Label: "Limiar (R$)", Type: "number"}},
		map[string]any{"limiar_valor_medio": 100.0})
	t.Cleanup(cleanup)
	_ = vinculoID

	url := fmt.Sprintf("/api/farol/public/metas-vinculos?cnpj=%s", cnpjEmpresa)
	w := httptest.NewRecorder()
	MetasPublicVinculosHandler(db)(w, httptest.NewRequest(http.MethodGet, url, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("GET público → status %d, body=%s", w.Code, w.Body.String())
	}
	var lista []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &lista); err != nil {
		t.Fatalf("decode: %v", err)
	}
	achado := false
	for _, v := range lista {
		if int(v["id"].(float64)) == vinculoID {
			achado = true
		}
	}
	if !achado {
		t.Errorf("vínculo criado não apareceu na lista pública: %+v", lista)
	}
}

func TestMetasPublicVinculos_CNPJInexistente_404(t *testing.T) {
	db, _ := biTestDB(t)
	w := httptest.NewRecorder()
	MetasPublicVinculosHandler(db)(w, httptest.NewRequest(http.MethodGet, "/api/farol/public/metas-vinculos?cnpj=00000000000199", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("CNPJ inexistente → status %d, want 404, body=%s", w.Code, w.Body.String())
	}
}

// TestMetasPublicPainel_EscopoPorRCA_NaoVazaOutrasRedes é o teste central
// desta story: o painel público de um RCA só mostra as Redes DELE, mesmo
// que existam outras Redes no mesmo vínculo/vigência atendidas por outro
// RCA. Sem login, esse isolamento é a única proteção contra um link
// compartilhado vazar dado de fora do escopo.
func TestMetasPublicPainel_EscopoPorRCA_NaoVazaOutrasRedes(t *testing.T) {
	db, empresaID := biTestDB(t)
	var cnpjEmpresa string
	if err := db.QueryRow(`SELECT regexp_replace(cnpj, '[^0-9]', '', 'g') FROM companies WHERE id = $1`, empresaID).Scan(&cnpjEmpresa); err != nil || cnpjEmpresa == "" {
		t.Skip("empresa de teste sem CNPJ cadastrado — teste pulado")
	}
	vinculoID, cleanup := criarVinculoComFormula(t, empresaID, "TPUB Escopo", "cobertura_rede", "rede",
		[]ParametroSchemaDTO{{Key: "limiar_valor_medio", Label: "Limiar (R$)", Type: "number"}},
		map[string]any{"limiar_valor_medio": 100.0})
	t.Cleanup(cleanup)
	vigenciaID := criarVigenciaFixture(t, db, empresaID, vinculoID, "2027-01-01", "2027-01-31")
	db.Exec(`INSERT INTO farol.metas_faixas (empresa_id, vigencia_id, faixa, valor_meta) VALUES ($1,$2,1,10)`, empresaID, vigenciaID)

	lojaA, lojaB := "80808080000101", "80808080000102"
	t.Cleanup(func() { limparVendasFaturadasFixture(t, empresaID, []string{lojaA, lojaB}) })
	inserirClienteValidoFixture(t, empresaID, vinculoID, vigenciaID, "REDE RCA-A", lojaA, "TCALC-RCAPUBA")
	inserirClienteValidoFixture(t, empresaID, vinculoID, vigenciaID, "REDE RCA-B", lojaB, "TCALC-RCAPUBB")
	inserirVendaFaturadaFixture(t, empresaID, lojaA, "PROD1", "TCALC-RCAPUBA", "1", 500, 1, "2027-01-10")
	inserirVendaFaturadaFixture(t, empresaID, lojaB, "PROD1", "TCALC-RCAPUBB", "1", 500, 1, "2027-01-10")

	url := fmt.Sprintf("/api/farol/public/metas-painel?cnpj=%s&scope=rca&cod=TCALC-RCAPUBA&vinculo_id=%d&vigencia_id=%d&fluxo=faturado", cnpjEmpresa, vinculoID, vigenciaID)
	w := httptest.NewRecorder()
	MetasPublicPainelHandler(db)(w, httptest.NewRequest(http.MethodGet, url, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("GET painel público → status %d, body=%s", w.Code, w.Body.String())
	}
	var resp PainelResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Realizado.Redes) != 1 {
		t.Fatalf("escopo do RCA-A deveria ter só 1 Rede, veio %d: %+v", len(resp.Realizado.Redes), resp.Realizado.Redes)
	}
	if resp.Realizado.Redes[0].RedeNome != "REDE RCA-A" {
		t.Errorf("VAZAMENTO: painel do RCA-A mostrou a Rede %q (deveria ser só REDE RCA-A)", resp.Realizado.Redes[0].RedeNome)
	}
}

// TestMetasPublicPainel_RecortesRespeitaEscopo cobre a Story 6.2 (FR21/
// FR23 no mobile): recortes de tempo funcionam no painel público, e
// continuam respeitando o isolamento de escopo (não vazam Rede de outro
// RCA em nenhum recorte).
func TestMetasPublicPainel_RecortesRespeitaEscopo(t *testing.T) {
	db, empresaID := biTestDB(t)
	var cnpjEmpresa string
	if err := db.QueryRow(`SELECT regexp_replace(cnpj, '[^0-9]', '', 'g') FROM companies WHERE id = $1`, empresaID).Scan(&cnpjEmpresa); err != nil || cnpjEmpresa == "" {
		t.Skip("empresa de teste sem CNPJ cadastrado — teste pulado")
	}
	vinculoID, cleanup := criarVinculoComFormula(t, empresaID, "TPUB Recortes", "cobertura_rede", "rede",
		[]ParametroSchemaDTO{{Key: "limiar_valor_medio", Label: "Limiar (R$)", Type: "number"}},
		map[string]any{"limiar_valor_medio": 100.0})
	t.Cleanup(cleanup)

	hoje := time.Now()
	inicio := hoje.AddDate(0, 0, -20)
	fim := hoje.AddDate(0, 0, 20)
	vigenciaID := criarVigenciaFixture(t, db, empresaID, vinculoID, inicio.Format("2006-01-02"), fim.Format("2006-01-02"))

	lojaA, lojaB := "90909090000101", "90909090000102"
	t.Cleanup(func() { limparVendasFaturadasFixture(t, empresaID, []string{lojaA, lojaB}) })
	inserirClienteValidoFixture(t, empresaID, vinculoID, vigenciaID, "REDE A", lojaA, "TCALC-RCARECPUBA")
	inserirClienteValidoFixture(t, empresaID, vinculoID, vigenciaID, "REDE B", lojaB, "TCALC-RCARECPUBB")
	inserirVendaFaturadaFixture(t, empresaID, lojaA, "PROD1", "TCALC-RCARECPUBA", "1", 200, 1, hoje.Format("2006-01-02"))
	inserirVendaFaturadaFixture(t, empresaID, lojaB, "PROD1", "TCALC-RCARECPUBB", "1", 200, 1, hoje.Format("2006-01-02"))

	url := fmt.Sprintf("/api/farol/public/metas-painel?cnpj=%s&scope=rca&cod=TCALC-RCARECPUBA&vinculo_id=%d&vigencia_id=%d&fluxo=faturado&recortes=1", cnpjEmpresa, vinculoID, vigenciaID)
	w := httptest.NewRecorder()
	MetasPublicPainelHandler(db)(w, httptest.NewRequest(http.MethodGet, url, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("GET → status %d, body=%s", w.Code, w.Body.String())
	}
	var resp PainelResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	for _, rec := range []string{"dia_anterior", "semana", "mes", "ano_corrente"} {
		if resp.Recortes[rec] == nil {
			t.Errorf("recorte %q ausente", rec)
			continue
		}
		for _, r := range resp.Recortes[rec].Redes {
			if r.RedeNome != "REDE A" {
				t.Errorf("VAZAMENTO no recorte %q: apareceu %q (deveria ser só REDE A)", rec, r.RedeNome)
			}
		}
	}
	// "semana" inclui hoje — RCA-A deveria ter Realizado > 0 (só a própria Rede).
	if resp.Recortes["semana"].RealizadoTotal != 1 {
		t.Errorf("semana: RealizadoTotal = %.0f, want 1 (só REDE A, coberta)", resp.Recortes["semana"].RealizadoTotal)
	}
}

func TestMetasPublicPainel_ParametrosObrigatorios_400(t *testing.T) {
	db, _ := biTestDB(t)
	w := httptest.NewRecorder()
	MetasPublicPainelHandler(db)(w, httptest.NewRequest(http.MethodGet, "/api/farol/public/metas-painel?cnpj=123", nil))
	if w.Code != http.StatusBadRequest && w.Code != http.StatusNotFound {
		t.Fatalf("sem scope/cod/vinculo_id/vigencia_id → status %d, want 400 ou 404, body=%s", w.Code, w.Body.String())
	}
}
