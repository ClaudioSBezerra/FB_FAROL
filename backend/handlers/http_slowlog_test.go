package handlers

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestSlowRequestLog(t *testing.T) {
	t.Setenv("FAROL_SLOW_REQ_MS", "20")
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)
	h := SlowRequestLog(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/lento" {
			time.Sleep(40 * time.Millisecond)
		}
		w.WriteHeader(http.StatusTeapot)
		w.Write([]byte("abc"))
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/rapido", nil))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/static/lento", nil))
	if buf.Len() != 0 {
		t.Fatalf("não devia logar: %s", buf.String())
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/api/lento?view=V01&token=segredo", nil))
	out := buf.String()
	if rr.Code != http.StatusTeapot || rr.Body.String() != "abc" {
		t.Fatalf("resposta alterada: %d %q", rr.Code, rr.Body.String())
	}
	for _, quer := range []string{"[http:lento] GET /api/lento", "view=V01", "→ 418", "(3 bytes"} {
		if !strings.Contains(out, quer) {
			t.Errorf("faltou %q em %q", quer, out)
		}
	}
	if strings.Contains(out, "segredo") {
		t.Errorf("token vazou no log: %q", out)
	}
}
