package logging

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCustomResponseWriterImplementsFlusher(t *testing.T) {
	rr := httptest.NewRecorder()
	rw := &CustomResponseWriter{ResponseWriter: rr, status: http.StatusOK}

	if _, ok := any(rw).(http.Flusher); !ok {
		t.Fatal("CustomResponseWriter should implement http.Flusher")
	}

	rw.Flush()
}
