package handlers

// farol_metas_painel_test.go — cobre a I/O Matrix da Story 5.1
// (_bmad-output/implementation-artifacts/5-1-painel-indicadores-oficiais.md).

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func painelReq(url, empresaID, userID string) *http.Request {
	return metaVinculoReq(http.MethodGet, url, empresaID, userID, nil)
}

// TestMetasPainel_DeltaExplicito é o teste central da Story 5.1 (FR19a): o
// painel destaca quanto falta pra bater a PRÓXIMA faixa, não só realizado e
// meta lado a lado — reproduz o cenário real Unilever (Faixas 78/85/91).
func TestMetasPainel_DeltaExplicito(t *testing.T) {
	db, empresaID := biTestDB(t)
	userID := tipoMetricaTestUserID(t, db)
	vinculoID, cleanup := criarVinculoComFormula(t, empresaID, "TPAINEL Delta", "cobertura_rede", "rede",
		[]ParametroSchemaDTO{{Key: "limiar_valor_medio", Label: "Limiar (R$)", Type: "number"}},
		map[string]any{"limiar_valor_medio": 100.0})
	t.Cleanup(cleanup)
	vigenciaID := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-01-01", "2026-01-31")
	// Faixas em escala pequena (proporcional ao exemplo real Unilever
	// 78/85/91), pra não precisar de dezenas de fixtures de venda no teste.
	db.Exec(`INSERT INTO farol.metas_faixas (empresa_id, vigencia_id, faixa, valor_meta) VALUES
		($1,$2,3,3), ($1,$2,2,4), ($1,$2,1,5)`, empresaID, vigenciaID)

	// 3 Redes cobertas — bateu Faixa 3 (3), não bateu Faixa 2 (4) ainda
	var cnpjs []string
	for i := 0; i < 3; i++ {
		cnpj := fmt.Sprintf("%014d", 20000000000000+i)
		cnpjs = append(cnpjs, cnpj)
		inserirClienteValidoFixture(t, empresaID, vinculoID, vigenciaID, fmt.Sprintf("REDE %d", i), cnpj, "TCALC-RCAP")
		inserirVendaFaturadaFixture(t, empresaID, cnpj, "PROD1", "TCALC-RCAP", "1", 1000, 1, "2026-01-10")
	}
	t.Cleanup(func() { limparVendasFaturadasFixture(t, empresaID, cnpjs) })

	url := fmt.Sprintf("/api/farol/metas-painel?vinculo_id=%d&vigencia_id=%d&fluxo=faturado&nivel=rede", vinculoID, vigenciaID)
	w := httptest.NewRecorder()
	MetasPainelHandler(db)(w, painelReq(url, empresaID, userID))
	if w.Code != http.StatusOK {
		t.Fatalf("GET painel → status %d, body=%s", w.Code, w.Body.String())
	}

	var resp PainelResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Realizado.RealizadoTotal != 3 {
		t.Fatalf("RealizadoTotal = %.0f, want 3", resp.Realizado.RealizadoTotal)
	}
	if resp.FaixaAtual == nil || resp.FaixaAtual.Faixa != 3 {
		t.Fatalf("FaixaAtual deveria ser a Faixa 3 (meta=3), veio %+v", resp.FaixaAtual)
	}
	if resp.ProximaFaixa == nil || resp.ProximaFaixa.Faixa != 2 {
		t.Fatalf("ProximaFaixa deveria ser a Faixa 2 (meta=4), veio %+v", resp.ProximaFaixa)
	}
	if resp.Delta != 1 { // 4 - 3
		t.Errorf("Delta = %.0f, want 1 (falta 1 Rede pra bater a Faixa 2)", resp.Delta)
	}
}

// TestMetasPainel_AlternanciaFluxo cobre a Story 5.2 (FR20): trocar de
// fluxo no painel recalcula Realizado/delta corretamente, sem perder o
// nível de drill-down pedido na mesma consulta.
func TestMetasPainel_AlternanciaFluxo(t *testing.T) {
	db, empresaID := biTestDB(t)
	userID := tipoMetricaTestUserID(t, db)
	vinculoID, cleanup := criarVinculoComFormula(t, empresaID, "TPAINEL Fluxo", "cobertura_rede", "rede",
		[]ParametroSchemaDTO{{Key: "limiar_valor_medio", Label: "Limiar (R$)", Type: "number"}},
		map[string]any{"limiar_valor_medio": 100.0})
	t.Cleanup(cleanup)
	vigenciaID := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-03-01", "2026-03-31")
	db.Exec(`INSERT INTO farol.metas_faixas (empresa_id, vigencia_id, faixa, valor_meta) VALUES ($1,$2,1,1)`, empresaID, vigenciaID)

	loja := "40404040000101"
	t.Cleanup(func() {
		limparVendasFaturadasFixture(t, empresaID, []string{loja})
		limparVendasTransmitidasFixture(t, empresaID, []string{loja})
	})
	inserirClienteValidoFixture(t, empresaID, vinculoID, vigenciaID, "REDE FLUXO", loja, "TCALC-RCAF")
	inserirVendaFaturadaFixture(t, empresaID, loja, "PROD1", "TCALC-RCAF", "1", 1000, 1, "2026-03-05")
	// sem venda transmitida — fluxo transmitido deve dar 0, não vazar o faturado

	urlFat := fmt.Sprintf("/api/farol/metas-painel?vinculo_id=%d&vigencia_id=%d&fluxo=faturado&nivel=rca", vinculoID, vigenciaID)
	wFat := httptest.NewRecorder()
	MetasPainelHandler(db)(wFat, painelReq(urlFat, empresaID, userID))
	var respFat PainelResponse
	json.Unmarshal(wFat.Body.Bytes(), &respFat)
	if respFat.Realizado.Nivel != "rca" {
		t.Errorf("nível deveria continuar 'rca' com fluxo=faturado, veio %q", respFat.Realizado.Nivel)
	}
	if respFat.Realizado.RealizadoTotal != 1 {
		t.Fatalf("fluxo faturado: RealizadoTotal = %.0f, want 1", respFat.Realizado.RealizadoTotal)
	}

	urlTrans := fmt.Sprintf("/api/farol/metas-painel?vinculo_id=%d&vigencia_id=%d&fluxo=transmitido&nivel=rca", vinculoID, vigenciaID)
	wTrans := httptest.NewRecorder()
	MetasPainelHandler(db)(wTrans, painelReq(urlTrans, empresaID, userID))
	var respTrans PainelResponse
	json.Unmarshal(wTrans.Body.Bytes(), &respTrans)
	if respTrans.Realizado.Nivel != "rca" {
		t.Errorf("nível deveria continuar 'rca' com fluxo=transmitido, veio %q", respTrans.Realizado.Nivel)
	}
	if respTrans.Realizado.RealizadoTotal != 0 {
		t.Errorf("fluxo transmitido: RealizadoTotal = %.0f, want 0 (sem venda transmitida, não deveria herdar o faturado)", respTrans.Realizado.RealizadoTotal)
	}
}

func TestMetasPainel_TodasFaixasAtingidas_DeltaZero(t *testing.T) {
	db, empresaID := biTestDB(t)
	userID := tipoMetricaTestUserID(t, db)
	vinculoID, cleanup := criarVinculoComFormula(t, empresaID, "TPAINEL TudoBatido", "cobertura_rede", "rede",
		[]ParametroSchemaDTO{{Key: "limiar_valor_medio", Label: "Limiar (R$)", Type: "number"}},
		map[string]any{"limiar_valor_medio": 100.0})
	t.Cleanup(cleanup)
	vigenciaID := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-02-01", "2026-02-28")
	db.Exec(`INSERT INTO farol.metas_faixas (empresa_id, vigencia_id, faixa, valor_meta) VALUES ($1,$2,1,1)`, empresaID, vigenciaID)

	loja := "30303030000101"
	t.Cleanup(func() { limparVendasFaturadasFixture(t, empresaID, []string{loja}) })
	inserirClienteValidoFixture(t, empresaID, vinculoID, vigenciaID, "REDE UNICA", loja, "TCALC-RCAP2")
	inserirVendaFaturadaFixture(t, empresaID, loja, "PROD1", "TCALC-RCAP2", "1", 1000, 1, "2026-02-10")

	url := fmt.Sprintf("/api/farol/metas-painel?vinculo_id=%d&vigencia_id=%d", vinculoID, vigenciaID)
	w := httptest.NewRecorder()
	MetasPainelHandler(db)(w, painelReq(url, empresaID, userID))
	if w.Code != http.StatusOK {
		t.Fatalf("GET painel → status %d, body=%s", w.Code, w.Body.String())
	}
	var resp PainelResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.ProximaFaixa != nil {
		t.Errorf("todas as faixas batidas — ProximaFaixa deveria ser nil, veio %+v", resp.ProximaFaixa)
	}
	if resp.Delta != 0 {
		t.Errorf("Delta = %.2f, want 0 (já bateu tudo)", resp.Delta)
	}
}
