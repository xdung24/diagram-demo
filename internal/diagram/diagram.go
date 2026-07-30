package diagram

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/xdung24/diagram-demo/internal/ai"
	"github.com/xdung24/diagram-demo/internal/helper"
	"github.com/xdung24/diagram-demo/internal/mcp"
)

// Service holds the MCP client, log stream, and LLM integration for the demo server.
type Diagram struct {
	Title       string    `json:"title"`
	Slug        string    `json:"slug"`
	Description string    `json:"description"`
	Code        string    `json:"code"`
	SvgPath     string    `json:"svgPath"`
	MmdPath     string    `json:"mmdPath"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
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

	// Load list of diagrams from the public/diagram folder if it exists
	log.Printf("loading diagrams from public/diagram folder")
	publicRoot := helper.ResolvePublicRoot()
	diagramRoot := filepath.Join(publicRoot, "diagram")
	found := 0
	now := time.Now().UTC()
	if _, err := os.Stat(diagramRoot); err == nil {
		entries, err := os.ReadDir(diagramRoot)
		if err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					diagramPath := filepath.Join(diagramRoot, entry.Name(), "diagram.json")
					if _, err := os.Stat(diagramPath); err == nil {
						data, err := os.ReadFile(diagramPath)
						if err == nil {
							var item Diagram
							if err := json.Unmarshal(data, &item); err == nil {
								svc.diagrams[item.Slug] = item
								found++
							}
						}
					}
				}
			}
		}
	}
	elasped := time.Since(now)
	log.Printf("loaded %d diagrams from public/diagram folder (%dms)", found, elasped.Milliseconds())
	return svc, nil
}

// ListTools returns the MCP tools exposed by the server.
func (s *Service) ListTools(ctx context.Context) ([]mcp.Tool, error) {
	if s == nil {
		return nil, fmt.Errorf("service unavailable")
	}
	return s.mcpclient.ListTools(ctx)
}

// Generate asks Mermaid MCP to generate the diagram.
func (s *Service) Generate(ctx context.Context, description string, toolName string, params map[string]any) (map[string]any, error) {
	if s == nil {
		return nil, fmt.Errorf("service unavailable")
	}
	res, err := s.llmclient.GenerateDiagramCode(ctx, description)
	if err != nil {
		return nil, fmt.Errorf("generating failed: %s", helper.SanitizeError(err))
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
	if input.Title == "" {
		return Diagram{}, fmt.Errorf("diagram title is required")
	}
	s.ensureStore()
	s.mu.Lock()
	input.CreatedAt = time.Now().UTC()
	input.UpdatedAt = input.CreatedAt
	input.Title = strings.TrimSpace(input.Title)
	input.Description = strings.TrimSpace(input.Description)
	input.Code = strings.TrimSpace(input.Code)
	input.Slug = slugifyTitle(input.Title)
	if err := s.persistDiagramFolder(input); err != nil {
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

	current.Description = strings.TrimSpace(input.Description)
	current.Code = strings.TrimSpace(input.Code)
	current.MmdPath = input.MmdPath
	current.SvgPath = input.SvgPath
	current.UpdatedAt = time.Now().UTC()

	if err := s.persistDiagramFolder(current); err != nil {
		return Diagram{}, err
	}
	//
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
		return nil, fmt.Errorf("rendering failed: %s", helper.SanitizeError(err))
	}
	return enrichRenderResult(res), nil
}

func (s *Service) renderMermaidToSVG(ctx context.Context, code string, params map[string]any) (map[string]any, error) {
	publicRoot := helper.ResolvePublicRoot()
	outDir := filepath.Join(publicRoot, "diagram")
	if dir, ok := params["outputDir"].(string); ok && dir != "" {
		if filepath.IsAbs(dir) {
			outDir = filepath.Clean(dir)
		} else {
			outDir = filepath.Clean(filepath.Join(".", dir))
		}
	}
	name := fmt.Sprintf("diagram-%d", time.Now().UnixNano())
	if n, ok := params["name"].(string); ok && n != "" {
		name = sanitizeFileName(n)
	}
	digramDir := filepath.Join(outDir, name)
	if err := os.MkdirAll(digramDir, 0o755); err != nil {
		return nil, fmt.Errorf("create diagram output dir: %w", err)
	}

	svgPath := filepath.Join(digramDir, "diagram.svg")
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

	// Store svgPath and mmdPath to diagram.json for later retrieval
	diagram, ok := s.diagrams[name]
	if ok {
		diagram.SvgPath = svgURL
		diagram.MmdPath = mmdURL
		s.diagrams[name] = diagram
		s.persistDiagramFolder(diagram)
	}

	return map[string]any{
		"svgPath": svgURL,
		"mmdPath": mmdURL,
	}, nil
}

func diagramFolderName(item Diagram) string {
	if strings.TrimSpace(item.Title) != "" {
		return sanitizeFileName(slugifyTitle(item.Title))
	}
	return sanitizeFileName(item.Slug)
}

func (s *Service) persistDiagramFolder(item Diagram) error {
	publicRoot := helper.ResolvePublicRoot()
	diagramRoot := filepath.Join(publicRoot, "diagram")
	if err := os.MkdirAll(diagramRoot, 0o755); err != nil {
		return fmt.Errorf("create diagram root: %w", err)
	}

	folderName := diagramFolderName(item)
	dirPath := filepath.Join(diagramRoot, folderName)
	if err := os.MkdirAll(dirPath, 0o755); err != nil {
		return fmt.Errorf("create diagram folder: %w", err)
	}

	payload, err := json.MarshalIndent(map[string]any{
		"title":       item.Title,
		"slug":        item.Slug,
		"createdAt":   item.CreatedAt.Format(time.RFC3339),
		"updatedAt":   item.UpdatedAt.Format(time.RFC3339),
		"svgPath":     item.SvgPath,
		"mmdPath":     item.MmdPath,
		"description": item.Description,
		"code":        item.Code,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal diagram metadata: %w", err)
	}
	return os.WriteFile(filepath.Join(dirPath, "diagram.json"), payload, 0o644)
}

func (s *Service) removeDiagramFolder(item Diagram) error {
	publicRoot := helper.ResolvePublicRoot()
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
