package handlers

// farol_mcp_auth_test.go — testa authMiddleware/tokensValidosFarolJC sem
// banco nem rede: dois tokens de classes diferentes (agente direto vs.
// Gateway FB_CEREBRO) devem funcionar independentemente, e revogar um não
// pode afetar o outro (AD-6 da espinha de arquitetura de 16/09/2026).

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthMiddlewareAceitaQualquerTokenDaLista(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	h := authMiddleware([]string{"token-agente", "token-gateway"}, next)

	for _, tok := range []string{"token-agente", "token-gateway"} {
		req := httptest.NewRequest("GET", "/x", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("token %q: status = %d, want 200", tok, rec.Code)
		}
	}
}

func TestAuthMiddlewareRejeitaTokenDesconhecido(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	h := authMiddleware([]string{"token-agente"}, next)

	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Authorization", "Bearer outro-qualquer")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestAuthMiddlewareSemHeader(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	h := authMiddleware([]string{"token-agente"}, next)

	req := httptest.NewRequest("GET", "/x", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestTokensValidosFarolJC(t *testing.T) {
	t.Setenv("FAROL_MCP_TOKEN", "a")
	t.Setenv("FAROL_GATEWAY_TOKEN", "b")
	got := tokensValidosFarolJC()
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("tokensValidosFarolJC() = %v, want [a b]", got)
	}
}

func TestTokensValidosFarolJCSoUmConfigurado(t *testing.T) {
	t.Setenv("FAROL_MCP_TOKEN", "a")
	t.Setenv("FAROL_GATEWAY_TOKEN", "")
	got := tokensValidosFarolJC()
	if len(got) != 1 || got[0] != "a" {
		t.Errorf("tokensValidosFarolJC() = %v, want [a]", got)
	}
}
