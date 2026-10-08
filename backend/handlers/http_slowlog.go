package handlers

import (
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// SlowRequestLog registra toda requisição /api que passa do limiar
// (FAROL_SLOW_REQ_MS, padrão 1000 ms) com rota, status, duração total e
// tamanho da resposta. Os logs por handler só medem o cálculo de alguns
// endpoints; em 08/10/2026 uma espera de minutos na tela não apareceu em
// lugar nenhum deles.
func SlowRequestLog(next http.Handler) http.Handler {
	limiar := 1000 * time.Millisecond
	if v, err := strconv.Atoi(strings.TrimSpace(os.Getenv("FAROL_SLOW_REQ_MS"))); err == nil && v >= 0 {
		limiar = time.Duration(v) * time.Millisecond
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		t0 := time.Now()
		rec := &respostaMedida{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		if d := time.Since(t0); d >= limiar {
			log.Printf("[http:lento] %s %s%s → %d em %v (%d bytes, 1º byte em %v)",
				r.Method, r.URL.Path, queryParaLog(r.URL.Query()), rec.status, d.Round(time.Millisecond),
				rec.bytes, rec.primeiroByte.Sub(t0).Round(time.Millisecond))
		}
	})
}

type respostaMedida struct {
	http.ResponseWriter
	status       int
	bytes        int64
	primeiroByte time.Time
}

func (r *respostaMedida) WriteHeader(code int) {
	r.status = code
	if r.primeiroByte.IsZero() {
		r.primeiroByte = time.Now()
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *respostaMedida) Write(b []byte) (int, error) {
	if r.primeiroByte.IsZero() {
		r.primeiroByte = time.Now()
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytes += int64(n)
	return n, err
}

// Flush mantém o streaming (SSE em objetivos.go) funcionando através do wrapper.
func (r *respostaMedida) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (r *respostaMedida) Unwrap() http.ResponseWriter { return r.ResponseWriter }

func queryParaLog(q url.Values) string {
	if len(q) == 0 {
		return ""
	}
	limpo := url.Values{}
	for k, vs := range q {
		kl := strings.ToLower(k)
		if strings.Contains(kl, "token") || strings.Contains(kl, "senha") || strings.Contains(kl, "password") ||
			strings.Contains(kl, "secret") || strings.Contains(kl, "key") {
			limpo.Set(k, "***")
			continue
		}
		limpo[k] = vs
	}
	s := "?" + limpo.Encode()
	if len(s) > 300 {
		s = s[:300] + "…"
	}
	return s
}
