package handlers

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"
)

// gzipResponseWriter envolve um http.ResponseWriter para escrever gzip
// quando o cliente aceita. Usa nível 5 (balance entre CPU e taxa).
type gzipResponseWriter struct {
	http.ResponseWriter
	gz *gzip.Writer
}

func (g *gzipResponseWriter) Write(b []byte) (int, error) {
	return g.gz.Write(b)
}

// Hijack/Flush ficam fora porque os handlers do Farol não usam SSE/WS.

// GzipMiddleware comprime respostas JSON quando o cliente aceita gzip.
// Aplicado seletivamente nas rotas de listagem que retornam grandes payloads.
func GzipMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next(w, r)
			return
		}
		gz, err := gzip.NewWriterLevel(w, 5)
		if err != nil {
			next(w, r)
			return
		}
		defer gz.Close()

		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Vary", "Accept-Encoding")
		// Content-Length seria errado após compressão — remove.
		w.Header().Del("Content-Length")

		gw := &gzipResponseWriter{ResponseWriter: w, gz: gz}
		next(gw, r)
	}
}

// noopWriter para silenciar lints.
var _ io.Writer = (*gzipResponseWriter)(nil)
