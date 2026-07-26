package service

import (
	"context"
	"encoding/json"
	"fmt"
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
	}
	return svc, nil
}

// ListTools returns the MCP tools exposed by the server.
func (s *Service) ListTools(ctx context.Context) ([]mcp.Tool, error) {
	return s.mcpclient.ListTools(ctx)
}

// Render invokes an MCP tool with diagram code and params.
func (s *Service) Render(ctx context.Context, toolName string, code string, params map[string]any) (map[string]any, error) {
	args := mergeArgs(params, code)
	res, err := s.mcpclient.CallTool(ctx, toolName, args)
	if err != nil {
		return nil, fmt.Errorf("rendering failed: %s", sanitizeError(err))
	}
	return res, nil
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
	args := make(map[string]any, len(params)+3)
	for k, v := range params {
		args[k] = v
	}
	if code != "" {
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
