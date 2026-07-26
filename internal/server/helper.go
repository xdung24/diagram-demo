package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func sanitizeError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	msg = strings.ReplaceAll(msg, "\n", " ")
	msg = strings.ReplaceAll(msg, "\r", " ")
	msg = strings.ReplaceAll(msg, "\\", "/")
	return strings.TrimSpace(msg)
}

func sanitizeLog(input string) string {
	return sanitizeError(fmt.Errorf("%s", input))
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
