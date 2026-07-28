package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/xdung24/diagram-demo/internal/ai"
	"github.com/xdung24/diagram-demo/internal/mcp"
)

// Service holds the MCP client, log stream, and LLM integration for the demo server.
type Diagram struct {
	Title     string    `json:"title"`
	Slug      string    `json:"slug"`
	Prompt    string    `json:"prompt"`
	Code      string    `json:"code"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Service struct {
	mcpclient *mcp.McpClient
	llmclient *ai.AIClient
	logStream *Stream
	binary    string
	diagrams  map[string]Diagram
	mu        sync.RWMutex
}

// New creates a service and starts the MCP child process.
func NewDiagramService(ctx context.Context, binary string, args ...string) (*Service, error) {
	mcpclient, err := mcp.NewMcpClient(ctx, binary, args...)
	if err != nil {
		return nil, err
	}

	svc := &Service{
		mcpclient: mcpclient,
		llmclient: ai.NewClient(),
		logStream: NewLogStream(),
		binary:    binary,
		diagrams:  make(map[string]Diagram),
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
	publicRoot := resolvePublicRoot()
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

// Generate asks Mermaid MCP to generate the diagram.
func (s *Service) Generate(ctx context.Context, prompt string, toolName string, params map[string]any) (map[string]any, error) {
	res, err := s.llmclient.GenerateDiagramCode(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("generating failed: %s", sanitizeError(err))
	}
	return map[string]any{"generatedCode": res}, nil
}

// Logs returns the log stream used by the HTTP layer.
func (s *Service) Logs() *Stream {
	return s.logStream
}

func (s *Service) ensureStore() {
	if s == nil {
		return
	}
	if s.diagrams == nil {
		s.diagrams = make(map[string]Diagram)
	}
}

func (s *Service) ListDiagrams() []Diagram {
	if s == nil {
		return nil
	}
	s.ensureStore()
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]Diagram, 0, len(s.diagrams))
	for _, item := range s.diagrams {
		items = append(items, item)
	}
	return items
}

func (s *Service) CreateDiagram(input Diagram) (Diagram, error) {
	if s == nil {
		return Diagram{}, fmt.Errorf("service unavailable")
	}
	s.ensureStore()
	s.mu.Lock()
	input.CreatedAt = time.Now().UTC()
	input.UpdatedAt = input.CreatedAt
	input.Title = strings.TrimSpace(input.Title)
	if input.Title == "" {
		input.Title = "Untitled diagram"
	}
	input.Prompt = strings.TrimSpace(input.Prompt)
	input.Code = strings.TrimSpace(input.Code)
	input.Slug = slugifyTitle(input.Title)
	if err := s.persistDiagramFolder(input, ""); err != nil {
		return Diagram{}, err
	}
	s.diagrams[input.Slug] = input
	defer s.mu.Unlock()
	createdItem := s.diagrams[input.Slug]
	return createdItem, nil
}

func (s *Service) GetDiagram(slug string) (Diagram, error) {
	if s == nil {
		return Diagram{}, fmt.Errorf("service unavailable")
	}
	s.ensureStore()
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.diagrams[slug]
	if !ok {
		return Diagram{}, fmt.Errorf("diagram %s not found", slug)
	}
	return item, nil
}

func (s *Service) UpdateDiagram(slug string, input Diagram) (Diagram, error) {
	if s == nil {
		return Diagram{}, fmt.Errorf("service unavailable")
	}
	s.ensureStore()
	s.mu.Lock()
	defer s.mu.Unlock()

	current, ok := s.diagrams[slug]
	if !ok {
		return Diagram{}, fmt.Errorf("diagram %s not found", slug)
	}

	previousSlug := current.Slug
	current.Title = strings.TrimSpace(input.Title)
	if current.Title == "" {
		current.Title = "Untitled diagram"
	}
	current.Prompt = strings.TrimSpace(input.Prompt)
	current.Code = strings.TrimSpace(input.Code)
	current.Slug = slugifyTitle(current.Title)
	current.UpdatedAt = time.Now().UTC()

	if err := s.persistDiagramFolder(current, previousSlug); err != nil {
		return Diagram{}, err
	}
	s.diagrams[current.Slug] = current
	return current, nil
}

func (s *Service) DeleteDiagram(slug string) error {
	if s == nil {
		return fmt.Errorf("service unavailable")
	}
	s.ensureStore()
	s.mu.Lock()
	defer s.mu.Unlock()

	current, ok := s.diagrams[slug]
	if !ok {
	}
	if !ok {
		return fmt.Errorf("diagram %s not found", slug)
	}
	if err := s.removeDiagramFolder(current); err != nil {
		return err
	}
	delete(s.diagrams, current.Slug)
	return nil
}

func diagramFolderName(item Diagram) string {
	if strings.TrimSpace(item.Title) != "" {
		return sanitizeFileName(slugifyTitle(item.Title))
	}
	return sanitizeFileName(item.Slug)
}

func (s *Service) persistDiagramFolder(item Diagram, previousSlug string) error {
	publicRoot := resolvePublicRoot()
	diagramRoot := filepath.Join(publicRoot, "diagram")
	if err := os.MkdirAll(diagramRoot, 0o755); err != nil {
		return fmt.Errorf("create diagram root: %w", err)
	}

	folderName := diagramFolderName(item)
	dirPath := filepath.Join(diagramRoot, folderName)
	if previousSlug != "" && previousSlug != item.Slug {
		oldDir := filepath.Join(diagramRoot, diagramFolderName(Diagram{Slug: previousSlug}))
		if _, err := os.Stat(oldDir); err == nil {
			if _, err := os.Stat(dirPath); os.IsNotExist(err) {
				if err := os.Rename(oldDir, dirPath); err != nil {
					return fmt.Errorf("rename diagram folder: %w", err)
				}
			}
		}
	}
	if err := os.MkdirAll(dirPath, 0o755); err != nil {
		return fmt.Errorf("create diagram folder: %w", err)
	}

	payload, err := json.MarshalIndent(map[string]any{
		"title":       item.Title,
		"slug":        item.Slug,
		"createdAt":   item.CreatedAt.Format(time.RFC3339),
		"updatedAt":   item.UpdatedAt.Format(time.RFC3339),
		"description": item.Prompt,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal diagram metadata: %w", err)
	}
	return os.WriteFile(filepath.Join(dirPath, "diagram.json"), payload, 0o644)
}

func (s *Service) removeDiagramFolder(item Diagram) error {
	publicRoot := resolvePublicRoot()
	dir := filepath.Join(publicRoot, "diagram", diagramFolderName(item))
	if diagramFolderName(item) == "" || diagramFolderName(item) == "diagram" && item.Slug == "" {
		return nil
	}
	return os.RemoveAll(dir)
}

// Close shuts down the child process and associated resources.
func (s *Service) Close() error {
	if s == nil || s.mcpclient == nil {
		return nil
	}
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
