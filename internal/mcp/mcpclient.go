package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const (
	jsonRPCVersion = "2.0"
	defaultTimeout = 45 * time.Second
)

// Tool describes an MCP tool exposed by the server.
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"inputSchema,omitempty"`
}

// McpClient manages a child process running the diagram MCP server over stdio.
const defaultGlobalInstructions = "instructions://global"

type McpClient struct {
	mu              sync.RWMutex
	cmd             *exec.Cmd
	stdin           io.WriteCloser
	stdout          *bufio.Reader
	stderr          io.Writer
	closed          bool
	seq             int64
	tools           []Tool
	initDone        bool
	instructionsURI string
}

// NewMcpClient starts the MCP server binary as a stdio child process.
func NewMcpClient(ctx context.Context, binary string, args ...string) (*McpClient, error) {
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Stderr = &stderrWriter{}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	c := &McpClient{
		cmd:             cmd,
		stdin:           stdin,
		stdout:          bufio.NewReader(stdout),
		stderr:          &stderrWriter{},
		instructionsURI: defaultGlobalInstructions,
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start child process: %w", err)
	}

	if err := c.initialize(ctx); err != nil {
		_ = c.Close()
		return nil, err
	}

	return c, nil
}

// Initialize performs MCP initialization and tool discovery.
func (c *McpClient) initialize(ctx context.Context) error {
	initParams := map[string]any{
		"protocolVersion": "2025-03-26",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "diagram-demo-server", "version": "1.0.0"},
	}
	if c.instructionsURI != "" {
		initParams["instructions"] = []string{c.instructionsURI}
	}
	if err := c.sendRequest(ctx, "initialize", initParams); err != nil {
		return err
	}

	if err := c.sendNotification(ctx, "notifications/initialized", map[string]any{}); err != nil {
		return err
	}

	tools, err := c.ListTools(ctx)
	if err != nil {
		return fmt.Errorf("list tools: %w", err)
	}
	c.mu.Lock()
	c.tools = tools
	c.initDone = true
	c.mu.Unlock()
	return nil
}

// ListTools fetches the available MCP tools from the server.
func (c *McpClient) ListTools(ctx context.Context) ([]Tool, error) {
	var out struct {
		Tools []Tool `json:"tools"`
	}
	if err := c.callMethod(ctx, "tools/list", nil, &out); err != nil {
		return nil, err
	}
	return out.Tools, nil
}

// CallTool executes an MCP tool and returns the parsed response payload.
func (c *McpClient) CallTool(ctx context.Context, name string, args map[string]any) (map[string]any, error) {
	callArgs := make(map[string]any, len(args)+1)
	for k, v := range args {
		callArgs[k] = v
	}
	if c.instructionsURI != "" {
		if _, ok := callArgs["instructions"]; !ok {
			callArgs["instructions"] = []string{c.instructionsURI}
		}
	}
	var out map[string]any
	if err := c.callMethod(ctx, "tools/call", map[string]any{"name": name, "arguments": callArgs}, &out); err != nil {
		return nil, err
	}
	if isError, ok := out["isError"].(bool); ok && isError {
		msg := ""
		if m, ok := out["message"].(string); ok && m != "" {
			msg = m
		}
		if errVal, ok := out["error"].(string); ok && errVal != "" {
			msg = errVal
		}
		if errMap, ok := out["error"].(map[string]any); ok {
			if m, ok := errMap["message"].(string); ok && m != "" {
				msg = m
			}
		}
		if msg != "" {
			return nil, fmt.Errorf("tool %s returned error: %s", name, msg)
		}
		return nil, fmt.Errorf("tool %s returned error", name)
	}
	content, contentExists := out["content"]
	if !contentExists || content == nil {
		if structured, ok := out["structuredContent"]; ok {
			out["content"] = structured
		}
	}
	return out, nil
}

// GenerateDiagram calls the server's generate_diagram tool.
func (c *McpClient) GenerateDiagram(ctx context.Context, content, outputPath string) (map[string]any, error) {
	return c.CallTool(ctx, "generate_diagram", map[string]any{
		"content":    content,
		"outputPath": outputPath,
	})
}

// ValidateDiagram calls the server's validate_diagram tool.
func (c *McpClient) ValidateDiagram(ctx context.Context, content, path string) (map[string]any, error) {
	args := make(map[string]any)
	if content != "" {
		args["content"] = content
	}
	if path != "" {
		args["path"] = path
	}
	return c.CallTool(ctx, "validate_diagram", args)
}

// ParseDiagram calls the server's parse_diagram tool.
func (c *McpClient) ParseDiagram(ctx context.Context, content, path string) (map[string]any, error) {
	args := make(map[string]any)
	if content != "" {
		args["content"] = content
	}
	if path != "" {
		args["path"] = path
	}
	return c.CallTool(ctx, "parse_diagram", args)
}

// SuggestDiagramType calls the server's suggest_diagram_type tool.
func (c *McpClient) SuggestDiagramType(ctx context.Context, requirement string) (map[string]any, error) {
	if requirement == "" {
		return nil, fmt.Errorf("requirement is required")
	}
	return c.CallTool(ctx, "suggest_diagram_type", map[string]any{"requirement": requirement})
}

// Tools returns the cached tool list.
func (c *McpClient) Tools() []Tool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return append([]Tool(nil), c.tools...)
}

// Close terminates the child process.
func (c *McpClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true

	if c.stdin != nil {
		_ = c.stdin.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
		_ = c.cmd.Wait()
	}
	return nil
}

func (c *McpClient) callMethod(ctx context.Context, method string, params map[string]any, out any) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	if err := c.sendRequest(ctx, method, params); err != nil {
		return err
	}
	resp, err := c.readResponse(ctx)
	if err != nil {
		return err
	}
	if resp.Error != nil {
		return fmt.Errorf("mcp %s: %s", method, resp.Error.Message)
	}
	if out != nil {
		if err := json.Unmarshal(resp.Result, out); err != nil {
			return fmt.Errorf("decode result: %w", err)
		}
	}
	return nil
}

func (c *McpClient) sendRequest(ctx context.Context, method string, params map[string]any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return fmt.Errorf("client closed")
	}

	c.seq++
	msg := map[string]any{
		"jsonrpc": jsonRPCVersion,
		"id":      c.seq,
		"method":  method,
		"params":  params,
	}
	payload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}
	payload = append(payload, '\n')
	if _, err := c.stdin.Write(payload); err != nil {
		return fmt.Errorf("write request: %w", err)
	}
	return nil
}

func (c *McpClient) sendNotification(ctx context.Context, method string, params map[string]any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return fmt.Errorf("client closed")
	}
	msg := map[string]any{
		"jsonrpc": jsonRPCVersion,
		"method":  method,
		"params":  params,
	}
	payload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal notification: %w", err)
	}
	payload = append(payload, '\n')
	if _, err := c.stdin.Write(payload); err != nil {
		return fmt.Errorf("write notification: %w", err)
	}
	return nil
}

func (c *McpClient) readResponse(ctx context.Context) (*jsonRPCResponse, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	line, err := c.stdout.ReadString('\n')
	if err != nil {
		if err == io.EOF {
			return nil, fmt.Errorf("server closed connection")
		}
		return nil, fmt.Errorf("read response: %w", err)
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return c.readResponse(ctx)
	}
	var resp jsonRPCResponse
	if err := json.Unmarshal([]byte(line), &resp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &resp, nil
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type stderrWriter struct{}

func (w *stderrWriter) Write(p []byte) (int, error) {
	return len(p), nil
}
