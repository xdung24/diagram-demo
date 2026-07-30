package helper

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func WriteJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func SanitizeError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	msg = strings.ReplaceAll(msg, "\n", " ")
	msg = strings.ReplaceAll(msg, "\r", " ")
	msg = strings.ReplaceAll(msg, "\\", "/")
	return strings.TrimSpace(msg)
}

func SanitizeLog(input string) string {
	msg := strings.ReplaceAll(input, "\n", " ")
	msg = strings.ReplaceAll(msg, "\r", " ")
	msg = strings.ReplaceAll(msg, "\\", "/")
	return strings.TrimSpace(msg)
}

func summarizePayload(payload any) string {
	if payload == nil {
		return "<nil>"
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Sprintf("%v", payload)
	}
	return string(data)
}

func ResolvePublicRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return filepath.Join(".", "public")
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "public")); err == nil {
			return filepath.Join(dir, "public")
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return filepath.Join(dir, "public")
		}
		dir = parent
	}
}

// MarshalPayload converts a value into a JSON-serializable payload for the UI.
func MarshalPayload(v any) map[string]any {
	payload, err := json.Marshal(v)
	if err != nil {
		return map[string]any{"error": "failed to encode response"}
	}
	var out map[string]any
	_ = json.Unmarshal(payload, &out)
	return out
}
