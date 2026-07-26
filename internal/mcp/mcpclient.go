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
type McpClient struct {
	mu       sync.RWMutex
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	stdout   *bufio.Reader
	stderr   io.Writer
	closed   bool
	seq      int64
	tools    []Tool
	initDone bool
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
		cmd:    cmd,
		stdin:  stdin,
		stdout: bufio.NewReader(stdout),
		stderr: &stderrWriter{},
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
	if err := c.sendRequest(ctx, "initialize", map[string]any{
		"protocolVersion": "2025-03-26",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "diagram-demo-server", "version": "1.0.0"},
	}); err != nil {
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
	var out struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text,omitempty"`
			Data string `json:"data,omitempty"`
		} `json:"content"`
		IsError bool `json:"isError,omitempty"`
	}
	if err := c.callMethod(ctx, "tools/call", map[string]any{"name": name, "arguments": args}, &out); err != nil {
		return nil, err
	}
	if out.IsError {
		return nil, fmt.Errorf("tool %s returned error", name)
	}
	result := map[string]any{"content": out.Content}
	return result, nil
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
