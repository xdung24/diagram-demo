package logging

import (
	"log"
	"net/http"
	"time"
)

type CustomResponseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *CustomResponseWriter) WriteHeader(status int) {
	rw.status = status
	rw.ResponseWriter.WriteHeader(status)
}

func (rw *CustomResponseWriter) Flush() {
	if flusher, ok := rw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func LoggingMiddleware(next http.Handler) http.Handler {

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		wrapped := &CustomResponseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(wrapped, r)
		log.Printf("%s %s %s %d %s", r.RemoteAddr, r.Method, r.URL.Path, wrapped.status, time.Since(start))
	})
}
