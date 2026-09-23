package handlers

// farol_gamificacao_jc_email_test.go — cobre o resumo diário de
// Gamificação pro CEO José Costa (pedido do Claudio 23/09/2026: "enviar
// ao CEO todos os dias um resumo... do ranking por campanha"), mesmo
// padrão de farol_jc_email_test.go.

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRotaGamifResumoEmailValidacoes cobre os caminhos que a validação de
// parâmetro/allowlist barra ANTES de precisar de banco.
func TestRotaGamifResumoEmailValidacoes(t *testing.T) {
	mux := http.NewServeMux()
	getDB := func() *sql.DB { return nil }
	registrarRotasGamificacaoJC(mux, getDB, "empresa-x")

	casos := []struct {
		nome           string
		envAllowlist   string
		query          string
		statusEsperado int
	}{
		{nome: "falta email", envAllowlist: "@x.com", query: "", statusEsperado: http.StatusBadRequest},
		{nome: "sem allowlist configurada", envAllowlist: "", query: "?email=a@x.com", statusEsperado: http.StatusServiceUnavailable},
		{nome: "destinatario nao autorizado", envAllowlist: "permitido@x.com", query: "?email=outro@x.com", statusEsperado: http.StatusForbidden},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			t.Setenv("EMAILS_PERMITIDOS", c.envAllowlist)
			r := httptest.NewRequest(http.MethodGet, "/gamif-resumo-email"+c.query, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, r)
			if rec.Code != c.statusEsperado {
				t.Errorf("status = %d, want %d (body=%s)", rec.Code, c.statusEsperado, rec.Body.String())
			}
		})
	}
}

// TestListarResumoGamificacaoAtivas_SoIncluiCampanhaAtiva — a campanha
// encerrada não deveria aparecer no resumo diário, mesmo tendo pontuação
// registrada (achado real: o resumo é pra campanha em andamento, uma
// campanha encerrada já foi paga/fechada).
func TestListarResumoGamificacaoAtivas_SoIncluiCampanhaAtiva(t *testing.T) {
	db, empresaID := biTestDB(t)

	vinculoID, cleanup := criarVinculoComFormula(t, empresaID, "TGAM ResumoJC", "cobertura_rede", "rede",
		[]ParametroSchemaDTO{{Key: "limiar_valor_medio", Label: "Limiar", Type: "number"}},
		map[string]any{"limiar_valor_medio": 100.0})
	t.Cleanup(cleanup)
	vigenciaID := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-08-01", "2026-08-31")

	cnpjAtiva := "80000000001201"
	t.Cleanup(func() {
		db.Exec(`DELETE FROM vendas_faturadas WHERE empresa_id = $1 AND cod_rca = 'TGAM-RCA-RESUMO'`, empresaID)
	})
	inserirClienteValidoFixture(t, empresaID, vinculoID, vigenciaID, "REDE RESUMO", cnpjAtiva, "TGAM-RCA-RESUMO")
	inserirVendaFaturadaFixture(t, empresaID, cnpjAtiva, "PRODRESUMO", "TGAM-RCA-RESUMO", "1", 150, 1, "2026-08-10")

	var industriaID int
	db.QueryRow(`SELECT industria_id FROM farol.metas_vinculos WHERE id = $1`, vinculoID).Scan(&industriaID)

	campanhaAtivaID := criarGamifCampanhaFixture(t, empresaID, industriaID, "2026-08-01", "2026-08-31")
	criarGamifRegraFixture(t, campanhaAtivaID, "cobertura_atingida", vinculoID, vigenciaID, nil, 0, 10, 300)
	if err := CalcularPontuacaoCampanha(db, empresaID, campanhaAtivaID); err != nil {
		t.Fatalf("CalcularPontuacaoCampanha: %v", err)
	}

	campanhaEncerradaID := criarGamifCampanhaFixture(t, empresaID, industriaID, "2026-07-01", "2026-07-31")
	db.Exec(`UPDATE farol.gamif_campanhas SET status = 'encerrada' WHERE id = $1`, campanhaEncerradaID)

	campanhas, err := listarResumoGamificacaoAtivas(db, empresaID)
	if err != nil {
		t.Fatalf("listarResumoGamificacaoAtivas: %v", err)
	}
	var achouAtiva, achouEncerrada bool
	for _, c := range campanhas {
		if c.CampanhaID == campanhaAtivaID {
			achouAtiva = true
			if c.TotalRCAs != 1 || len(c.Ranking) != 1 || c.Ranking[0].CodRCA != "TGAM-RCA-RESUMO" {
				t.Errorf("campanha ativa no resumo = %+v, want 1 RCA (TGAM-RCA-RESUMO) pontuando", c)
			}
		}
		if c.CampanhaID == campanhaEncerradaID {
			achouEncerrada = true
		}
	}
	if !achouAtiva {
		t.Errorf("campanha ativa não apareceu no resumo: %+v", campanhas)
	}
	if achouEncerrada {
		t.Errorf("campanha ENCERRADA não deveria aparecer no resumo diário: %+v", campanhas)
	}

	// O HTML final precisa citar a campanha — é o que vai na tela do
	// José Costa; se sumir daqui, sumiu do e-mail. `campanhas` aqui já
	// só tem a ativa (filtrada na própria query), então esta é a única
	// checada.
	_, _, htmlBody := construirEmailResumoGamificacao(campanhas)
	for _, c := range campanhas {
		if !strings.Contains(htmlBody, esc(c.Nome)) {
			t.Errorf("HTML não contém o nome da campanha (%s)", c.Nome)
		}
	}
}
