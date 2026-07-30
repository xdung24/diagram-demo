package diagram

import (
	"os"
	"strings"
)

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
