package handlers

// farol_jc_email_test.go — testa a allowlist de /objetivos-industria-email
// sem banco nem rede: mesma trava do FB_CEREBRO (EMAILS_PERMITIDOS),
// decisão de 16/09/2026 de manter o padrão entre os dois serviços.

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEmailsPermitidosFarolFormatos(t *testing.T) {
	t.Setenv("EMAILS_PERMITIDOS", "  Claudio@x.com , @Ferreiracosta.com.br,, ")
	got := emailsPermitidos()
	want := []string{"claudio@x.com", "@ferreiracosta.com.br"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestEmailPermitidoFarolExatoEDominio(t *testing.T) {
	permitidos := []string{"claudio@x.com", "@ferreiracosta.com.br"}
	if !emailPermitido("Claudio@X.com", permitidos) {
		t.Error("esperava permitido: e-mail exato, case-insensitive")
	}
	if !emailPermitido("qualquer.pessoa@ferreiracosta.com.br", permitidos) {
		t.Error("esperava permitido: domínio liberado")
	}
	if emailPermitido("outro@gmail.com", permitidos) {
		t.Error("esperava NÃO permitido: fora da lista")
	}
	if emailPermitido("x@naoferreiracosta.com.br", permitidos) {
		t.Error("domínio parecido não deveria bypassar o sufixo real")
	}
}

// TestRotaObjetivosIndustriaEmailValidacoes cobre só os caminhos que a
// allowlist/validação de parâmetros barra ANTES de precisar de banco —
// por isso getDB nunca é de fato chamado nestes 3 casos.
func TestRotaObjetivosIndustriaEmailValidacoes(t *testing.T) {
	mux := http.NewServeMux()
	getDB := func() *sql.DB { return nil }
	registrarRotaEmailObjetivosIndustria(mux, getDB, "empresa-x")

	casos := []struct {
		nome           string
		envAllowlist   string
		query          string
		statusEsperado int
	}{
		{
			nome:           "faltam parametros obrigatorios",
			envAllowlist:   "@x.com",
			query:          "?industria=UNILEVER+HC",
			statusEsperado: http.StatusBadRequest,
		},
		{
			nome:           "sem allowlist configurada",
			envAllowlist:   "",
			query:          "?industria=UNILEVER+HC&periodo=2026-08&email=a@x.com",
			statusEsperado: http.StatusServiceUnavailable,
		},
		{
			nome:           "destinatario nao autorizado",
			envAllowlist:   "permitido@x.com",
			query:          "?industria=UNILEVER+HC&periodo=2026-08&email=outro@x.com",
			statusEsperado: http.StatusForbidden,
		},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			t.Setenv("EMAILS_PERMITIDOS", c.envAllowlist)
			r := httptest.NewRequest(http.MethodGet, "/objetivos-industria-email"+c.query, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, r)
			if rec.Code != c.statusEsperado {
				t.Errorf("status = %d, want %d (body=%s)", rec.Code, c.statusEsperado, rec.Body.String())
			}
		})
	}
}
