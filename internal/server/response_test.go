package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteStatusPage(t *testing.T) {
	publicFS := os.DirFS(filepath.Join("..", ".."))

	tests := []struct {
		name       string
		statusCode int
		wantBody   string
	}{
		{name: "unauthorized", statusCode: http.StatusUnauthorized, wantBody: "Unauthorized"},
		{name: "forbidden", statusCode: http.StatusForbidden, wantBody: "Forbidden"},
		{name: "not found", statusCode: http.StatusNotFound, wantBody: "Not Found"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			writeStatusPage(rr, tt.statusCode, publicFS)

			if rr.Code != tt.statusCode {
				t.Fatalf("expected status %d, got %d", tt.statusCode, rr.Code)
			}
			if !strings.Contains(rr.Body.String(), tt.wantBody) {
				t.Fatalf("expected response body to contain %q, got %q", tt.wantBody, rr.Body.String())
			}
		})
	}
}

func TestCustomResponseWriterImplementsFlusher(t *testing.T) {
	rr := httptest.NewRecorder()
	rw := &CustomResponseWriter{ResponseWriter: rr, status: http.StatusOK}

	if _, ok := any(rw).(http.Flusher); !ok {
		t.Fatal("CustomResponseWriter should implement http.Flusher")
	}

	rw.Flush()
}
