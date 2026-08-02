package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/xdung24/diagram-demo/internal/diagram"
	"github.com/xdung24/diagram-demo/internal/helper"
)

// NewContext creates a request-scoped context with 45s timeout.
func NewContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 45*time.Second)
}

func CreateHttpServer(publicFS fs.FS, svc *diagram.Service) http.Handler {
	mux := http.NewServeMux()

	// Diagram listing
	mux.HandleFunc("/api/diagrams", func(w http.ResponseWriter, r *http.Request) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				helper.WriteJSON(w, http.StatusOK, map[string]any{"diagrams": svc.ListDiagrams()})
			case http.MethodPost:
				var input diagram.Diagram
				if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
					http.Error(w, "bad request", http.StatusBadRequest)
					return
				}
				item, err := svc.CreateDiagram(input)
				if err != nil {
					helper.WriteJSON(w, http.StatusBadRequest, map[string]any{"error": helper.SanitizeError(err)})
					return
				}
				helper.WriteJSON(w, http.StatusCreated, item)
			default:
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
		})
		handler.ServeHTTP(w, r)
	})

	// Diagram detail
	mux.HandleFunc("/api/diagram/{slug}", func(w http.ResponseWriter, r *http.Request) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			slug := r.PathValue("slug")
			switch r.Method {
			case http.MethodGet:
				item, err := svc.GetDiagram(slug)
				if err != nil {
					helper.WriteJSON(w, http.StatusBadRequest, map[string]any{"error": helper.SanitizeError(err)})
					return
				}
				helper.WriteJSON(w, http.StatusOK, item)
			case http.MethodPost:
				var input diagram.Diagram
				if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
					http.Error(w, "bad request", http.StatusBadRequest)
					return
				}
				item, err := svc.UpdateDiagram(slug, input)
				if err != nil {
					helper.WriteJSON(w, http.StatusBadRequest, map[string]any{"error": helper.SanitizeError(err)})
					return
				}
				helper.WriteJSON(w, http.StatusOK, item)
			case http.MethodDelete:
				if err := svc.DeleteDiagram(slug); err != nil {
					helper.WriteJSON(w, http.StatusBadRequest, map[string]any{"error": helper.SanitizeError(err)})
					return
				}
				helper.WriteJSON(w, http.StatusOK, map[string]any{"deleted": true})
			default:
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
		})
		handler.ServeHTTP(w, r)
	})

	// Generate diagram
	mux.HandleFunc("/api/generate", func(w http.ResponseWriter, r *http.Request) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			var req struct {
				Description string         `json:"description"`
				Tool        string         `json:"tool"`
				Params      map[string]any `json:"params"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			ctx, cancel := NewContext()
			defer cancel()
			res, err := svc.Generate(ctx, req.Description, req.Tool, req.Params)
			if err != nil {
				log.Printf("failed to generate diagram: %v", err)
				helper.WriteJSON(w, http.StatusBadRequest, map[string]any{"error": helper.SanitizeError(err)})
				return
			}
			helper.WriteJSON(w, http.StatusOK, map[string]any{"generatedCode": res["generatedCode"]})
		})
		handler.ServeHTTP(w, r)
	})

	// Render diagram
	mux.HandleFunc("/api/render", func(w http.ResponseWriter, r *http.Request) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			var req struct {
				Tool   string         `json:"tool"`
				Code   string         `json:"code"`
				Params map[string]any `json:"params"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			ctx, cancel := NewContext()
			defer cancel()
			res, err := svc.Render(ctx, req.Tool, req.Code, req.Params)
			if err != nil {
				log.Printf("failed to render diagram: %v", err)
				helper.WriteJSON(w, http.StatusBadRequest, map[string]any{"error": helper.SanitizeError(err)})
				return
			}
			helper.WriteJSON(w, http.StatusOK, map[string]any{"result": res})
		})
		handler.ServeHTTP(w, r)
	})

	// Stream logs
	mux.HandleFunc("/api/logs/stream", func(w http.ResponseWriter, r *http.Request) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")
			flusher, ok := w.(http.Flusher)
			if !ok {
				http.Error(w, "streaming unsupported", http.StatusInternalServerError)
				return
			}
			ch := svc.Logs().Subscribe()
			defer svc.Logs().Unsubscribe(ch)
			fmt.Fprintf(w, "data: connected\n\n")
			flusher.Flush()
			for entry := range ch {
				fmt.Fprintf(w, "data: %s\n\n", helper.SanitizeLog(entry))
				flusher.Flush()
			}
		})
		handler.ServeHTTP(w, r)
	})

	// Health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			helper.WriteJSON(w, http.StatusOK, map[string]any{"status": "ok"})
		})
		handler.ServeHTTP(w, r)
	})

	// Static content
	sub, err := fs.Sub(publicFS, "public")
	if err == nil {
		fileServer := http.FileServer(http.FS(sub))
		// diagram detail page
		mux.HandleFunc("/diagram/{slug}", func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet {
				serveDiagramRoute(w, r)
				return
			}
			http.NotFound(w, r)
		})
		// interactive d3.js viewer for a single diagram
		mux.HandleFunc("/diagram/{slug}/view", func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet {
				serveDiagramViewer(w, r, sub)
				return
			}
			http.NotFound(w, r)
		})
		// All other routes, serve static content from embedded FS or /public/ folder
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {

			// Check if the file exists in /public/
			publicRoot := helper.ResolvePublicRoot()
			filePath := filepath.Join(publicRoot, r.URL.Path)
			if info, err := os.Stat(filePath); err == nil {
				// If the path is a directory, and not the root path, return 403
				if r.URL.Path != "/" && info.IsDir() {
					http.Error(w, "forbidden", http.StatusForbidden)
					return
				}
				// Not a folder, serve the file from the public directory
				http.ServeFile(w, r, filePath)
				return
			} else {
				// If file exists in embedded FS, serve it
				if _, err := sub.Open(strings.TrimPrefix(r.URL.Path, "/")); err == nil {
					fileServer.ServeHTTP(w, r)
					return
				} else {
					// Return 404 if the file/folder does not exist
					http.NotFound(w, r)
					return
				}
			}
		})
	} else {
		log.Printf("failed to access embedded public dir: %v", err)
		mux.Handle("/", http.FileServer(http.Dir(".")))
	}

	return LogWriter(ResponseWriter(mux, publicFS))
}

// diagramViewerPath is the slug-agnostic d3.js renderer; it loads ./data.json
// (or ./diagram.json) relative to the requested /diagram/{slug}/view URL.
const diagramViewerPath = "diagram/view.html"

func serveDiagramViewer(w http.ResponseWriter, r *http.Request, embedded fs.FS) {
	slug := r.PathValue("slug")
	if !validSlug(slug) {
		http.NotFound(w, r)
		return
	}

	publicRoot := helper.ResolvePublicRoot()
	if info, err := os.Stat(filepath.Join(publicRoot, "diagram", slug)); err != nil || !info.IsDir() {
		http.NotFound(w, r)
		return
	}

	viewerPath := filepath.Join(publicRoot, filepath.FromSlash(diagramViewerPath))
	if _, err := os.Stat(viewerPath); err == nil {
		http.ServeFile(w, r, viewerPath)
		return
	}

	data, err := fs.ReadFile(embedded, diagramViewerPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}

func validSlug(slug string) bool {
	if slug == "" || slug == "." || strings.Contains(slug, "..") || strings.Contains(slug, "/") || strings.Contains(slug, `\`) {
		return false
	}
	return true
}

func serveDiagramRoute(w http.ResponseWriter, r *http.Request) {
	publicRoot := helper.ResolvePublicRoot()
	indexPath := filepath.Join(publicRoot, "diagram", "index.html")

	slug := r.PathValue("slug")
	if !validSlug(slug) {
		http.ServeFile(w, r, indexPath)
		return
	}

	assetPath := filepath.Join(publicRoot, "diagram", slug)
	info, err := os.Stat(assetPath)
	if err == nil && !info.IsDir() {
		http.ServeFile(w, r, assetPath)
		return
	}

	http.ServeFile(w, r, indexPath)
}
