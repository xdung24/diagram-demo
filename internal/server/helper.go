package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
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

func resolvePublicRoot() string {
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

func sanitizeFileName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, " ", "-")
	return strings.Map(func(r rune) rune {
		if r == '-' || r == '_' || r == '.' || r == '+' || r == ' ' {
			return r
		}
		if r >= '0' && r <= '9' {
			return r
		}
		if r >= 'a' && r <= 'z' {
			return r
		}
		if r >= 'A' && r <= 'Z' {
			return r
		}
		return -1
	}, name)
}

func slugifyTitle(title string) string {
	text := strings.TrimSpace(strings.ToLower(title))
	if text == "" {
		return "diagram"
	}

	var builder strings.Builder
	prevDash := false
	for _, r := range text {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			builder.WriteRune(r)
			prevDash = false
		default:
			if builder.Len() > 0 && !prevDash {
				builder.WriteRune('-')
				prevDash = true
			}
		}
	}
	return strings.Trim(builder.String(), "-")
}

func normalizeToolName(toolName string) string {
	if toolName == "generate_mermaid_diagram" {
		return "generate_diagram"
	}
	return toolName
}

func isGenerateTool(toolName string) bool {
	return toolName == "generate_diagram" || (strings.HasPrefix(toolName, "generate_") && strings.HasSuffix(toolName, "_diagram"))
}

func getOutputPath(args map[string]any) string {
	if v, ok := args["outputPath"].(string); ok && v != "" {
		return v
	}
	return ""
}

func enrichRenderResult(res map[string]any) map[string]any {
	if content, ok := res["content"]; ok {
		if content == nil {
			return res
		}
		if m, ok := content.(map[string]any); ok {
			if svgPath, ok := m["svgPath"].(string); ok && svgPath != "" {
				if svgData, err := os.ReadFile(svgPath); err == nil {
					res["svg"] = string(svgData)
					res["content"] = string(svgData)
				}
			}
		}
	}
	return res
}

func mergeArgs(params map[string]any, code string) map[string]any {
	args := make(map[string]any, len(params)+4)
	for k, v := range params {
		args[k] = v
	}
	if code != "" {
		args["content"] = code
		args["code"] = code
		args["diagramCode"] = code
		args["input"] = code
		args["prompt"] = code
	}
	return args
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
