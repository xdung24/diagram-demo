package server

import (
	"bufio"
	"bytes"
	"io/fs"
	"log"
	"net"
	"net/http"
	"path/filepath"
	"time"
)

type statusPageResponseWriter struct {
	http.ResponseWriter
	status    int
	wroteHead bool
	body      bytes.Buffer
}

func (w *statusPageResponseWriter) WriteHeader(status int) {
	if w.wroteHead {
		return
	}
	w.status = status
	w.wroteHead = true
}

func (w *statusPageResponseWriter) Write(p []byte) (int, error) {
	if !w.wroteHead {
		w.WriteHeader(http.StatusOK)
	}
	return w.body.Write(p)
}

func (w *statusPageResponseWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *statusPageResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := w.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, http.ErrNotSupported
}

func (w *statusPageResponseWriter) Push(target string, opts *http.PushOptions) error {
	if pusher, ok := w.ResponseWriter.(http.Pusher); ok {
		return pusher.Push(target, opts)
	}
	return http.ErrNotSupported
}

func statusPageMiddleware(next http.Handler, publicFS fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rw := &statusPageResponseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)

		status := rw.status
		contentType := rw.Header().Get("Content-Type")
		if contentType != "application/json" {
			if status == http.StatusUnauthorized || status == http.StatusForbidden || status == http.StatusNotFound {
				writeStatusPage(w, status, publicFS)
				return
			}
		}

		if !rw.wroteHead {
			rw.WriteHeader(http.StatusOK)
		}
		w.WriteHeader(status)
		_, _ = w.Write(rw.body.Bytes())
	})
}

func writeStatusPage(w http.ResponseWriter, status int, publicFS fs.FS) {
	var file string
	switch status {
	case http.StatusUnauthorized:
		file = "401.html"
	case http.StatusForbidden:
		file = "403.html"
	case http.StatusNotFound:
		file = "404.html"
	default:
		http.Error(w, http.StatusText(status), status)
		return
	}

	data, err := fs.ReadFile(publicFS, filepath.ToSlash(filepath.Join("public", file)))
	if err != nil {
		http.Error(w, http.StatusText(status), status)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}

func loggingMiddleware(next http.Handler) http.Handler {

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		wrapped := &CustomResponseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(wrapped, r)
		log.Printf("%s %s %s %d %s", r.RemoteAddr, r.Method, r.URL.Path, wrapped.status, time.Since(start))
	})
}

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

func (rw *CustomResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := rw.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, http.ErrNotSupported
}

func (rw *CustomResponseWriter) Push(target string, opts *http.PushOptions) error {
	if pusher, ok := rw.ResponseWriter.(http.Pusher); ok {
		return pusher.Push(target, opts)
	}
	return http.ErrNotSupported
}
