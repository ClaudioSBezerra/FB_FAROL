package handlers

// farol_metas_congelamento_test.go — cobre a I/O Matrix da Story 4.3
// (_bmad-output/implementation-artifacts/4-3-congelamento-mes-fechado.md).
//
// O teste mais importante (TestObterOuCongelar_FluxoCompleto) simula o
// cenário real do FR17/NFR3 de ponta a ponta: calcula com a vigência
// aberta (ao vivo), fecha, calcula de novo (congela), MUDA o dado bruto de
// vendas (simulando "a base de vendas mudou depois"), calcula uma terceira
// vez (deve continuar retornando o valor CONGELADO, não o novo), e só
// depois de um reprocessamento manual explícito é que o valor muda.

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestObterOuCongelar_VigenciaAberta_SempreAoVivo(t *testing.T) {
	db, empresaID := biTestDB(t)
	vinculoID, cleanup := criarVinculoComFormula(t, empresaID, "TCONG Aberta", "cobertura_rede", "rede",
		[]ParametroSchemaDTO{{Key: "limiar_valor_medio", Label: "Limiar (R$)", Type: "number"}},
		map[string]any{"limiar_valor_medio": 100.0})
	t.Cleanup(cleanup)
	vigenciaID := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-09-01", "2026-09-30")

	loja := "99999999000101"
	t.Cleanup(func() { limparVendasFaturadasFixture(t, empresaID, []string{loja}) })
	inserirClienteValidoFixture(t, empresaID, vinculoID, vigenciaID, "REDE ABERTA", loja, "TCALC-RCA9")
	inserirVendaFaturadaFixture(t, empresaID, loja, "PROD1", "TCALC-RCA9", "1", 1000, 1, "2026-09-05")

	r1, err := obterOuCongelarRealizado(db, empresaID, vinculoID, vigenciaID, "faturado", "rede")
	if err != nil {
		t.Fatalf("1ª chamada: %v", err)
	}
	if r1.Redes[0].Valor != 1000 {
		t.Fatalf("esperava 1000, veio %.2f", r1.Redes[0].Valor)
	}

	// muda o dado (vigência ainda aberta)
	inserirVendaFaturadaFixture(t, empresaID, loja, "PROD1", "TCALC-RCA9", "1", 500, 1, "2026-09-10")

	r2, err := obterOuCongelarRealizado(db, empresaID, vinculoID, vigenciaID, "faturado", "rede")
	if err != nil {
		t.Fatalf("2ª chamada: %v", err)
	}
	if r2.Redes[0].Valor != 1500 {
		t.Errorf("vigência aberta deveria refletir o dado novo ao vivo — esperava 1500, veio %.2f", r2.Redes[0].Valor)
	}

	var snapshots int
	db.QueryRow(`SELECT COUNT(*) FROM farol.metas_realizados_snapshot WHERE vigencia_id = $1`, vigenciaID).Scan(&snapshots)
	if snapshots != 0 {
		t.Errorf("vigência aberta não deveria gravar snapshot nenhum, veio %d", snapshots)
	}
}

// TestObterOuCongelar_FluxoCompleto é o teste central da Story 4.3 — prova
// o congelamento de ponta a ponta, incluindo o caso que mais importa: dado
// mudando DEPOIS do fechamento não deveria mover o número já congelado.
func TestObterOuCongelar_FluxoCompleto(t *testing.T) {
	db, empresaID := biTestDB(t)
	userID := tipoMetricaTestUserID(t, db)
	vinculoID, cleanup := criarVinculoComFormula(t, empresaID, "TCONG Completo", "cobertura_rede", "rede",
		[]ParametroSchemaDTO{{Key: "limiar_valor_medio", Label: "Limiar (R$)", Type: "number"}},
		map[string]any{"limiar_valor_medio": 100.0})
	t.Cleanup(cleanup)
	vigenciaID := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-10-01", "2026-10-31")

	loja := "10101010000101"
	t.Cleanup(func() { limparVendasFaturadasFixture(t, empresaID, []string{loja}) })
	inserirClienteValidoFixture(t, empresaID, vinculoID, vigenciaID, "REDE CONGELA", loja, "TCALC-RCA10")
	inserirVendaFaturadaFixture(t, empresaID, loja, "PROD1", "TCALC-RCA10", "1", 1000, 1, "2026-10-05")

	// 1. fecha a vigência
	db.Exec(`UPDATE farol.metas_vigencias SET status = 'fechada' WHERE id = $1`, vigenciaID)

	// 2. primeiro acesso pós-fechamento — congela em 1000
	r1, err := obterOuCongelarRealizado(db, empresaID, vinculoID, vigenciaID, "faturado", "rede")
	if err != nil {
		t.Fatalf("1º acesso pós-fechamento: %v", err)
	}
	if r1.Redes[0].Valor != 1000 {
		t.Fatalf("esperava congelar em 1000, veio %.2f", r1.Redes[0].Valor)
	}
	var snapshotsAposCongelar int
	db.QueryRow(`SELECT COUNT(*) FROM farol.metas_realizados_snapshot WHERE vigencia_id = $1`, vigenciaID).Scan(&snapshotsAposCongelar)
	if snapshotsAposCongelar != 1 {
		t.Fatalf("esperava 1 snapshot criado, veio %d", snapshotsAposCongelar)
	}

	// 3. o dado de vendas MUDA depois do fechamento (planilha do fornecedor
	// corrigida, reimportação, etc.)
	inserirVendaFaturadaFixture(t, empresaID, loja, "PROD1", "TCALC-RCA10", "1", 99999, 1, "2026-10-20")

	// 4. NFR3/FR17: o valor congelado NÃO deve mudar sozinho
	r2, err := obterOuCongelarRealizado(db, empresaID, vinculoID, vigenciaID, "faturado", "rede")
	if err != nil {
		t.Fatalf("2º acesso pós-mudança de dado: %v", err)
	}
	if r2.Redes[0].Valor != 1000 {
		t.Errorf("FR17 violado: valor congelado mudou sozinho de 1000 pra %.2f depois do dado de vendas mudar", r2.Redes[0].Valor)
	}

	// 5. só reprocessamento manual explícito muda o congelado
	w := httptest.NewRecorder()
	url := fmt.Sprintf("/api/farol/metas-realizado/reprocessar?vinculo_id=%d&vigencia_id=%d&fluxo=faturado&nivel=rede", vinculoID, vigenciaID)
	req := metaVinculoReq(http.MethodPost, url, empresaID, userID, nil)
	MetasRealizadoReprocessarHandler(db)(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("POST reprocessar → status %d, body=%s", w.Code, w.Body.String())
	}

	r3, err := obterOuCongelarRealizado(db, empresaID, vinculoID, vigenciaID, "faturado", "rede")
	if err != nil {
		t.Fatalf("3º acesso pós-reprocessamento: %v", err)
	}
	if r3.Redes[0].Valor != 100999 { // 1000 + 99999
		t.Errorf("depois do reprocessamento manual, esperava 100999, veio %.2f", r3.Redes[0].Valor)
	}

	// auditoria do reprocessamento
	var acao string
	db.QueryRow(`SELECT acao FROM farol.sp_audit_log WHERE entidade = 'metas_realizados_snapshot' AND entidade_id = $1 ORDER BY created_at DESC LIMIT 1`, fmt.Sprint(vigenciaID)).Scan(&acao)
	if acao != "reprocessar_manual" {
		t.Errorf("reprocessamento manual deveria gravar auditoria, acao=%q", acao)
	}
}

func TestMetasRealizadoReprocessar_RequerGestorGeral_403(t *testing.T) {
	db, empresaID := biTestDB(t)
	vinculoID, cleanup := criarVinculoComFormula(t, empresaID, "TCONG SemPermissao", "cobertura_rede", "rede",
		[]ParametroSchemaDTO{{Key: "limiar_valor_medio", Label: "Limiar (R$)", Type: "number"}},
		map[string]any{"limiar_valor_medio": 100.0})
	t.Cleanup(cleanup)
	vigenciaID := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-11-01", "2026-11-30")

	url := fmt.Sprintf("/api/farol/metas-realizado/reprocessar?vinculo_id=%d&vigencia_id=%d", vinculoID, vigenciaID)
	r := httptest.NewRequest(http.MethodPost, url, nil)
	ctx := context.WithValue(r.Context(), SpContextKey, &FarolContext{
		UserID: "teste", SpRole: "gestor_filial", EmpresaID: empresaID, AllFiliais: true,
	})
	r = r.WithContext(ctx)

	w := httptest.NewRecorder()
	MetasRealizadoReprocessarHandler(db)(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("reprocessar com gestor_filial → status %d, want 403", w.Code)
	}
}
