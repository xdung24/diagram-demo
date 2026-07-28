package server

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSanitizeFileName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "replaces spaces with hyphens", in: "My Diagram_1.2", want: "My-Diagram_1.2"},
		{name: "removes unsupported characters", in: "Sales/Report: Q4?", want: "SalesReport-Q4"},
		{name: "trims whitespace", in: "  Final Diagram  ", want: "Final-Diagram"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sanitizeFileName(tt.in); got != tt.want {
				t.Fatalf("sanitizeFileName(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestNormalizeToolName(t *testing.T) {
	if got := normalizeToolName("generate_mermaid_diagram"); got != "generate_diagram" {
		t.Fatalf("normalizeToolName() = %q, want %q", got, "generate_diagram")
	}
	if got := normalizeToolName("render_diagram"); got != "render_diagram" {
		t.Fatalf("normalizeToolName() = %q, want %q", got, "render_diagram")
	}
}

func TestIsGenerateTool(t *testing.T) {
	if !isGenerateTool("generate_diagram") {
		t.Fatal("expected generate_diagram to be treated as a generate tool")
	}
	if !isGenerateTool("generate_flowchart_diagram") {
		t.Fatal("expected generate_flowchart_diagram to be treated as a generate tool")
	}
	if isGenerateTool("render_diagram") {
		t.Fatal("did not expect render_diagram to be treated as a generate tool")
	}
}

func TestGetOutputPath(t *testing.T) {
	args := map[string]any{"outputPath": "out/diagram.svg"}
	if got := getOutputPath(args); got != "out/diagram.svg" {
		t.Fatalf("getOutputPath() = %q, want %q", got, "out/diagram.svg")
	}

	if got := getOutputPath(map[string]any{"name": "demo"}); got != "" {
		t.Fatalf("getOutputPath() = %q, want empty string", got)
	}
}

func TestMergeArgs(t *testing.T) {
	args := mergeArgs(map[string]any{"theme": "dark"}, "graph TD")

	if args["theme"] != "dark" {
		t.Fatalf("expected merged theme to be preserved")
	}
	for _, key := range []string{"content", "code", "diagramCode", "input", "prompt"} {
		if got, ok := args[key].(string); !ok || got != "graph TD" {
			t.Fatalf("expected %s to contain diagram code, got %#v", key, args[key])
		}
	}
}

func TestEnrichRenderResult(t *testing.T) {
	tempDir := t.TempDir()
	svgPath := filepath.Join(tempDir, "sample.svg")
	if err := os.WriteFile(svgPath, []byte("<svg>ok</svg>"), 0o644); err != nil {
		t.Fatalf("write svg fixture: %v", err)
	}

	res := map[string]any{
		"content": map[string]any{"svgPath": svgPath},
	}

	enriched := enrichRenderResult(res)
	if got, ok := enriched["svg"].(string); !ok || got != "<svg>ok</svg>" {
		t.Fatalf("expected svg content to be enriched, got %#v", enriched["svg"])
	}
	if got, ok := enriched["content"].(string); !ok || got != "<svg>ok</svg>" {
		t.Fatalf("expected content payload to be populated, got %#v", enriched["content"])
	}
}

func TestDiagramFolderNameUsesIDThenSlug(t *testing.T) {
	item := Diagram{Title: "My slug", Slug: "my-slug"}
	if got := diagramFolderName(item); got != "my-slug" {
		t.Fatalf("expected folder name, got %q", got)
	}

	item.Slug = ""
	if got := diagramFolderName(item); got != "my-slug" {
		t.Fatalf("expected slug fallback folder name, got %q", got)
	}
}

func TestPersistDiagramFolderUsesIDBasedDirectory(t *testing.T) {
	tempDir := t.TempDir()
	publicDir := filepath.Join(tempDir, "public")
	if err := os.MkdirAll(filepath.Join(publicDir, "diagram"), 0o755); err != nil {
		t.Fatalf("create public diagram root: %v", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("chdir to temp dir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	svc := &Service{}
	item := Diagram{Title: "My Diagram", Slug: "my-slug"}
	if err := svc.persistDiagramFolder(item, ""); err != nil {
		t.Fatalf("persist diagram folder: %v", err)
	}

	folderPath := filepath.Join(publicDir, "diagram", "my-slug")
	if _, err := os.Stat(folderPath); err != nil {
		t.Fatalf("expected folder %s to exist: %v", folderPath, err)
	}
	if _, err := os.Stat(filepath.Join(folderPath, "diagram.json")); err != nil {
		t.Fatalf("expected diagram metadata file to exist: %v", err)
	}
}
