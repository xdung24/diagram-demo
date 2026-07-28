package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

type testWriteCloser struct {
	*bytes.Buffer
}

func (w *testWriteCloser) Close() error { return nil }

func TestSendRequestWritesJSONRPCRequest(t *testing.T) {
	payload := &bytes.Buffer{}
	c := &McpClient{
		stdin:  &testWriteCloser{Buffer: payload},
		stdout: bufio.NewReader(strings.NewReader("")),
	}

	if err := c.sendRequest(context.Background(), "tools/list", map[string]any{"name": "demo"}); err != nil {
		t.Fatalf("sendRequest returned error: %v", err)
	}

	if c.seq != 1 {
		t.Fatalf("expected seq to be 1, got %d", c.seq)
	}

	var msg map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(payload.Bytes()), &msg); err != nil {
		t.Fatalf("unmarshal request payload: %v", err)
	}

	if msg["method"] != "tools/list" {
		t.Fatalf("expected method tools/list, got %v", msg["method"])
	}

	params, ok := msg["params"].(map[string]any)
	if !ok {
		t.Fatalf("expected params object, got %#v", msg["params"])
	}
	if params["name"] != "demo" {
		t.Fatalf("expected forwarded params, got %#v", params)
	}
}

func TestSendNotificationWritesJSONRPCNotification(t *testing.T) {
	payload := &bytes.Buffer{}
	c := &McpClient{
		stdin:  &testWriteCloser{Buffer: payload},
		stdout: bufio.NewReader(strings.NewReader("")),
	}

	if err := c.sendNotification(context.Background(), "notifications/initialized", map[string]any{}); err != nil {
		t.Fatalf("sendNotification returned error: %v", err)
	}

	var msg map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(payload.Bytes()), &msg); err != nil {
		t.Fatalf("unmarshal notification payload: %v", err)
	}

	if msg["method"] != "notifications/initialized" {
		t.Fatalf("expected notification method, got %v", msg["method"])
	}
	if _, ok := msg["id"]; ok {
		t.Fatal("did not expect notification to include an id")
	}
}

func TestReadResponseParsesJSONRPCResult(t *testing.T) {
	c := &McpClient{
		stdout: bufio.NewReader(strings.NewReader("\n{\"jsonrpc\":\"2.0\",\"id\":7,\"result\":{\"ok\":true}}\n")),
	}

	resp, err := c.readResponse(context.Background())
	if err != nil {
		t.Fatalf("readResponse returned error: %v", err)
	}
	if resp.ID != 7 {
		t.Fatalf("expected id 7, got %d", resp.ID)
	}

	var out map[string]any
	if err := json.Unmarshal(resp.Result, &out); err != nil {
		t.Fatalf("unmarshal response result: %v", err)
	}
	if out["ok"] != true {
		t.Fatalf("expected decoded result to contain ok=true, got %#v", out)
	}
}

func TestCallToolIncludesGlobalInstructions(t *testing.T) {
	payload := &bytes.Buffer{}
	c := &McpClient{
		stdin:           &testWriteCloser{Buffer: payload},
		stdout:          bufio.NewReader(strings.NewReader("{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"ok\":true}}\n")),
		instructionsURI: "instructions://global",
	}

	_, err := c.CallTool(context.Background(), "generate_diagram", map[string]any{"content": "graph TD"})
	if err != nil {
		t.Fatalf("CallTool returned unexpected error: %v", err)
	}

	var msg map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(payload.Bytes()), &msg); err != nil {
		t.Fatalf("unmarshal request payload: %v", err)
	}

	params, ok := msg["params"].(map[string]any)
	if !ok {
		t.Fatalf("expected params object, got %#v", msg["params"])
	}

	arguments, ok := params["arguments"].(map[string]any)
	if !ok {
		t.Fatalf("expected arguments object, got %#v", params["arguments"])
	}

	instructions, ok := arguments["instructions"].([]any)
	if !ok {
		t.Fatalf("expected instructions array, got %#v", arguments["instructions"])
	}
	if !reflect.DeepEqual(instructions, []any{"instructions://global"}) {
		t.Fatalf("expected global instructions URI, got %#v", instructions)
	}
}

func TestCallToolReturnsErrorForToolError(t *testing.T) {
	payload := &bytes.Buffer{}
	c := &McpClient{
		stdin:  &testWriteCloser{Buffer: payload},
		stdout: bufio.NewReader(strings.NewReader("{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"isError\":true,\"message\":\"boom\"}}\n")),
	}

	_, err := c.CallTool(context.Background(), "generate_diagram", map[string]any{"content": "graph TD"})
	if err == nil {
		t.Fatal("expected CallTool to return an error for tool errors")
	}
	if !strings.Contains(err.Error(), "tool generate_diagram returned error") {
		t.Fatalf("expected wrapped tool error, got %v", err)
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected tool error message to be preserved, got %v", err)
	}
}
