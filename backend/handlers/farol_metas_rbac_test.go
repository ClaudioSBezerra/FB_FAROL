package handlers

// farol_metas_rbac_test.go — cobre a I/O Matrix da Story 5.4
// (_bmad-output/implementation-artifacts/5-4-papel-acesso-separado.md).
//
// Story de auditoria: confirma, de uma vez só, que TODO endpoint de
// escrita do módulo (Épicos 1-3) rejeita um usuário somente_leitura
// (NFR2), e que os endpoints de VISUALIZAÇÃO (Épico 5) aceitam esse mesmo
// papel — a separação "edição restrita, visualização ampla" precisa valer
// pro módulo inteiro, não handler a handler.
//
// A checagem real de RBAC no caminho de produção acontece em 2 camadas:
// FarolAuthMiddleware (nível de rota, main.go — não exercitado aqui, isso
// é teste de handler) + a checagem interna hasSpRole em cada handler de
// escrita (defesa em profundidade — É isso que este teste verifica
// diretamente). Os 2 handlers de leitura pura de listas admin
// (MetasClientesValidosHandler/MetasItensValidosHandler) não têm checagem
// interna — dependem só do gate de rota (mesmo padrão de outros GETs
// admin no projeto, ex. ObjetivosImportHandler) — não testados aqui por
// não serem exercitáveis sem o middleware real.

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// somenteLeituraReq monta uma request com sp_role=somente_leitura — o
// nível mínimo do FarolContext, sem nenhum privilégio de escrita.
func somenteLeituraReq(method, url, empresaID string) *http.Request {
	r := httptest.NewRequest(method, url, nil)
	ctx := context.WithValue(r.Context(), SpContextKey, &FarolContext{
		UserID: "teste", SpRole: "somente_leitura", EmpresaID: empresaID, AllFiliais: true,
	})
	return r.WithContext(ctx)
}

func TestRBAC_SomenteLeitura_TodosEndpointsDeEscrita403(t *testing.T) {
	db, empresaID := biTestDB(t)

	casos := []struct {
		nome    string
		handler http.HandlerFunc
		method  string
		url     string
	}{
		{"TiposMetricaHandler POST", TiposMetricaHandler(db), http.MethodPost, "/api/farol/tipos-metrica"},
		{"TipoMetricaItemHandler PUT", TipoMetricaItemHandler(db), http.MethodPut, "/api/farol/tipos-metrica/1"},
		{"TipoMetricaItemHandler DELETE", TipoMetricaItemHandler(db), http.MethodDelete, "/api/farol/tipos-metrica/1"},
		{"MetasVinculosHandler POST", MetasVinculosHandler(db), http.MethodPost, "/api/farol/metas-vinculos"},
		{"MetaVinculoItemHandler PUT", MetaVinculoItemHandler(db), http.MethodPut, "/api/farol/metas-vinculos/1"},
		{"MetaVinculoItemHandler DELETE", MetaVinculoItemHandler(db), http.MethodDelete, "/api/farol/metas-vinculos/1"},
		{"MetasVigenciasHandler POST", MetasVigenciasHandler(db), http.MethodPost, "/api/farol/metas-vigencias"},
		{"MetaVigenciaItemHandler PUT", MetaVigenciaItemHandler(db), http.MethodPut, "/api/farol/metas-vigencias/1"},
		{"MetaVigenciaItemHandler DELETE", MetaVigenciaItemHandler(db), http.MethodDelete, "/api/farol/metas-vigencias/1"},
		{"MetaVigenciaItemHandler POST fechar", MetaVigenciaItemHandler(db), http.MethodPost, "/api/farol/metas-vigencias/1/fechar"},
		{"MetasImportarCSVHandler POST", MetasImportarCSVHandler(db), http.MethodPost, "/api/farol/metas-vinculos-importar-csv"},
		{"MetasClientesValidosImportarCSVHandler POST", MetasClientesValidosImportarCSVHandler(db), http.MethodPost, "/api/farol/metas-clientes-validos-importar-csv?vinculo_id=1&vigencia_id=1"},
		{"MetasItensValidosImportarCSVHandler POST", MetasItensValidosImportarCSVHandler(db), http.MethodPost, "/api/farol/metas-itens-validos-importar-csv?vinculo_id=1&vigencia_id=1"},
		{"MetasRealizadoReprocessarHandler POST", MetasRealizadoReprocessarHandler(db), http.MethodPost, "/api/farol/metas-realizado/reprocessar?vinculo_id=1&vigencia_id=1"},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			w := httptest.NewRecorder()
			c.handler(w, somenteLeituraReq(c.method, c.url, empresaID))
			if w.Code != http.StatusForbidden {
				t.Errorf("%s com somente_leitura → status %d, want 403, body=%s", c.nome, w.Code, w.Body.String())
			}
		})
	}
}

// TestRBAC_SomenteLeitura_EndpointsDeVisualizacaoFuncionam confirma o outro
// lado da separação do NFR2: quem só tem somente_leitura CONSEGUE ver o
// painel (Épico 5), mesmo não conseguindo editar nada.
func TestRBAC_SomenteLeitura_EndpointsDeVisualizacaoFuncionam(t *testing.T) {
	db, empresaID := biTestDB(t)
	vinculoID, cleanup := criarVinculoComFormula(t, empresaID, "TRBAC Visualizacao", "cobertura_rede", "rede",
		[]ParametroSchemaDTO{{Key: "limiar_valor_medio", Label: "Limiar (R$)", Type: "number"}},
		map[string]any{"limiar_valor_medio": 100.0})
	t.Cleanup(cleanup)
	vigenciaID := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-12-01", "2026-12-31")

	loja := "70707070000101"
	t.Cleanup(func() { limparVendasFaturadasFixture(t, empresaID, []string{loja}) })
	inserirClienteValidoFixture(t, empresaID, vinculoID, vigenciaID, "REDE RBAC", loja, "TCALC-RCARBAC")

	urlRealizado := fmt.Sprintf("/api/farol/metas-realizado?vinculo_id=%d&vigencia_id=%d", vinculoID, vigenciaID)
	wr := httptest.NewRecorder()
	MetasRealizadoHandler(db)(wr, somenteLeituraReq(http.MethodGet, urlRealizado, empresaID))
	if wr.Code != http.StatusOK {
		t.Errorf("GET %s com somente_leitura → status %d, want 200, body=%s", urlRealizado, wr.Code, wr.Body.String())
	}

	urlPainel := fmt.Sprintf("/api/farol/metas-painel?vinculo_id=%d&vigencia_id=%d", vinculoID, vigenciaID)
	wp := httptest.NewRecorder()
	MetasPainelHandler(db)(wp, somenteLeituraReq(http.MethodGet, urlPainel, empresaID))
	if wp.Code != http.StatusOK {
		t.Errorf("GET %s com somente_leitura → status %d, want 200, body=%s", urlPainel, wp.Code, wp.Body.String())
	}
}
