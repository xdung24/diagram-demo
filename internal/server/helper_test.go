package server

import (
	"strings"
	"testing"
)

func TestSummarizePayload(t *testing.T) {
	payload := map[string]any{
		"content": []any{
			map[string]any{"type": "text", "text": "hello world"},
		},
	}

	got := summarizePayload(payload)
	if !strings.Contains(got, `"content"`) {
		t.Fatalf("expected payload summary to include content, got %q", got)
	}
	if !strings.Contains(got, `"hello world"`) {
		t.Fatalf("expected payload summary to include text content, got %q", got)
	}
}
