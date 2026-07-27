package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/xdung24/diagram-demo/internal/ai"
	"github.com/xdung24/diagram-demo/internal/logging"
	"github.com/xdung24/diagram-demo/internal/mcp"
)

// Service holds the MCP client, log stream, and LLM integration for the demo server.
type Service struct {
	mcpclient *mcp.McpClient
	llmclient *ai.AIClient
	logStream *logging.Stream
	binary    string
}

// New creates a service and starts the MCP child process.
func New(ctx context.Context, binary string, args ...string) (*Service, error) {
	mcpclient, err := mcp.NewMcpClient(ctx, binary, args...)
	if err != nil {
		return nil, err
	}

	svc := &Service{
		mcpclient: mcpclient,
		llmclient: ai.NewClient(),
		logStream: logging.New(),
		binary:    binary,
	}
	return svc, nil
}

// ListTools returns the MCP tools exposed by the server.
func (s *Service) ListTools(ctx context.Context) ([]mcp.Tool, error) {
	return s.mcpclient.ListTools(ctx)
}

// Render invokes an MCP tool with diagram code and params.
func (s *Service) Render(ctx context.Context, toolName string, code string, params map[string]any) (map[string]any, error) {
	toolName = normalizeToolName(toolName)
	if toolName == "generate_diagram" {
		return s.renderMermaidToSVG(ctx, code, params)
	}

	args := mergeArgs(params, code)
	if isGenerateTool(toolName) && getOutputPath(args) == "" {
		args["outputPath"] = filepath.Join(os.TempDir(), "diagram.mmd")
	}
	res, err := s.mcpclient.CallTool(ctx, toolName, args)
	if err != nil {
		return nil, fmt.Errorf("rendering failed: %s", sanitizeError(err))
	}
	return enrichRenderResult(res), nil
}

func (s *Service) renderMermaidToSVG(ctx context.Context, code string, params map[string]any) (map[string]any, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("current working directory: %w", err)
	}
	publicRoot := filepath.Join(wd, "public")
	outDir := filepath.Join(publicRoot, "diagram")
	if dir, ok := params["outputDir"].(string); ok && dir != "" {
		if filepath.IsAbs(dir) {
			outDir = filepath.Clean(dir)
		} else {
			outDir = filepath.Clean(filepath.Join(".", dir))
		}
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, fmt.Errorf("create diagram output dir: %w", err)
	}

	name := fmt.Sprintf("diagram-%d", time.Now().UnixNano())
	if n, ok := params["name"].(string); ok && n != "" {
		name = sanitizeFileName(n)
	}

	svgPath := filepath.Join(outDir, name+".svg")
	if p, ok := params["outputPath"].(string); ok && p != "" {
		cleanPath := filepath.Clean(p)
		if filepath.IsAbs(cleanPath) {
			svgPath = cleanPath
		} else {
			svgPath = filepath.Join(".", cleanPath)
		}
		if filepath.Ext(svgPath) == "" {
			svgPath += ".svg"
		}
	}
	if err := os.MkdirAll(filepath.Dir(svgPath), 0o755); err != nil {
		return nil, fmt.Errorf("create svg output dir: %w", err)
	}

	mmdPath := filepath.Join(filepath.Dir(svgPath), strings.TrimSuffix(filepath.Base(svgPath), filepath.Ext(svgPath))+".mmd")
	if err := os.WriteFile(mmdPath, []byte(code), 0o644); err != nil {
		return nil, fmt.Errorf("write mmd file: %w", err)
	}

	cmd := exec.CommandContext(ctx, s.binary, "render", "-i", mmdPath, "-f", "svg", "-o", svgPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("diagram-mcp render failed: %s: %s", err, strings.TrimSpace(string(out)))
	}

	svgURL := filepath.ToSlash(svgPath)
	if rel, err := filepath.Rel(publicRoot, svgPath); err == nil && !strings.HasPrefix(rel, "..") {
		svgURL = "/" + filepath.ToSlash(rel)
	}
	mmdURL := filepath.ToSlash(mmdPath)
	if rel, err := filepath.Rel(publicRoot, mmdPath); err == nil && !strings.HasPrefix(rel, "..") {
		mmdURL = "/" + filepath.ToSlash(rel)
	}

	return map[string]any{
		"svgPath": svgURL,
		"mmdPath": mmdURL,
	}, nil
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

// Generate asks Mermaid MCP to generate the diagram.
func (s *Service) Generate(ctx context.Context, prompt string, toolName string, params map[string]any) (map[string]any, error) {
	res, err := s.llmclient.GenerateDiagramCode(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("generating failed: %s", sanitizeError(err))
	}
	return map[string]any{"generatedCode": res}, nil
}

// Logs returns the log stream used by the HTTP layer.
func (s *Service) Logs() *logging.Stream {
	return s.logStream
}

// Close shuts down the child process and associated resources.
func (s *Service) Close() error {
	return s.mcpclient.Close()
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

// NewContext creates a request-scoped context with timeout.
func NewContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 45*time.Second)
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
