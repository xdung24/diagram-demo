package server

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
	"unsafe"
)

func TestLogsStreamEndpointFlusher(t *testing.T) {
	svc := &Service{}
	v := reflect.ValueOf(svc).Elem().FieldByName("logStream")
	if !v.IsValid() {
		t.Fatal("logStream field not found")
	}
	reflect.NewAt(v.Type(), unsafe.Pointer(v.UnsafeAddr())).Elem().Set(reflect.ValueOf(NewLogStream()))
	h := CreateHttpServer(nil, svc)
	ts := httptest.NewServer(h)
	defer ts.Close()

	client := ts.Client()
	client.Timeout = 2 * time.Second
	resp, err := client.Get(ts.URL + "/api/logs/stream")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	reader := bufio.NewReader(resp.Body)
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if line == "" {
		t.Fatal("expected a line in response")
	}
}
