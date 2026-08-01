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

// responseWriter
type responseWriter struct {
	http.ResponseWriter
	status    int
	wroteHead bool
	body      bytes.Buffer
}

func (w *responseWriter) WriteHeader(status int) {
	if w.wroteHead {
		return
	}
	w.status = status
	w.wroteHead = true
}

func (w *responseWriter) Write(p []byte) (int, error) {
	if !w.wroteHead {
		w.WriteHeader(http.StatusOK)
	}
	return w.body.Write(p)
}

func (w *responseWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := w.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, http.ErrNotSupported
}

func (w *responseWriter) Push(target string, opts *http.PushOptions) error {
	if pusher, ok := w.ResponseWriter.(http.Pusher); ok {
		return pusher.Push(target, opts)
	}
	return http.ErrNotSupported
}

func ResponseWriter(next http.Handler, publicFS fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Streaming endpoints (SSE) must write directly to the client;
		// buffering here would hold the response until the handler returns,
		// which never happens for a long-lived stream.
		if r.URL.Path == "/api/logs/stream" {
			next.ServeHTTP(w, r)
			return
		}

		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
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

// logWriter is a wrapper around http.ResponseWriter that captures the request and write logging information.
type logWriter struct {
	http.ResponseWriter
	status int
}

func (rw *logWriter) WriteHeader(status int) {
	rw.status = status
	rw.ResponseWriter.WriteHeader(status)
}

func (rw *logWriter) Flush() {
	if flusher, ok := rw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (rw *logWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := rw.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, http.ErrNotSupported
}

func (rw *logWriter) Push(target string, opts *http.PushOptions) error {
	if pusher, ok := rw.ResponseWriter.(http.Pusher); ok {
		return pusher.Push(target, opts)
	}
	return http.ErrNotSupported
}

func LogWriter(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Hide /health request from logs
		if r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		wrapped := &logWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(wrapped, r)
		log.Printf("%s %s %s %d %s", r.RemoteAddr, r.Method, r.URL.Path, wrapped.status, time.Since(start))
	})
}
