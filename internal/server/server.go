package server

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func CreateHttpServer(publicFS fs.FS, svc *Service) http.Handler {
	mux := http.NewServeMux()

	// Diagram listing
	mux.HandleFunc("/api/diagrams", func(w http.ResponseWriter, r *http.Request) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				writeJSON(w, http.StatusOK, map[string]any{"diagrams": svc.ListDiagrams()})
			case http.MethodPost:
				var input Diagram
				if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
					http.Error(w, "bad request", http.StatusBadRequest)
					return
				}
				item, err := svc.CreateDiagram(input)
				if err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]any{"error": sanitizeError(err)})
					return
				}
				writeJSON(w, http.StatusCreated, item)
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
					writeJSON(w, http.StatusBadRequest, map[string]any{"error": sanitizeError(err)})
					return
				}
				writeJSON(w, http.StatusOK, item)
			case http.MethodPost:
				var input Diagram
				if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
					http.Error(w, "bad request", http.StatusBadRequest)
					return
				}
				item, err := svc.UpdateDiagram(slug, input)
				if err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]any{"error": sanitizeError(err)})
					return
				}
				writeJSON(w, http.StatusOK, item)
			case http.MethodDelete:
				if err := svc.DeleteDiagram(slug); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]any{"error": sanitizeError(err)})
					return
				}
				writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
			default:
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
		})
		handler.ServeHTTP(w, r)
	})

	// List tools
	mux.HandleFunc("/api/tools", func(w http.ResponseWriter, r *http.Request) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			ctx, cancel := NewContext()
			defer cancel()
			tools, err := svc.ListTools(ctx)
			if err != nil {
				log.Printf("failed to list tools: %v", err)
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": sanitizeError(err)})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"tools": tools})
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
				Prompt string         `json:"prompt"`
				Tool   string         `json:"tool"`
				Params map[string]any `json:"params"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			ctx, cancel := NewContext()
			defer cancel()
			res, err := svc.Generate(ctx, req.Prompt, req.Tool, req.Params)
			if err != nil {
				log.Printf("failed to generate diagram: %v", err)
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": sanitizeError(err)})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"generatedCode": res["generatedCode"]})
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
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": sanitizeError(err)})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"result": res})
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
				fmt.Fprintf(w, "data: %s\n\n", sanitizeLog(entry))
				flusher.Flush()
			}
		})
		handler.ServeHTTP(w, r)
	})

	// Static content
	sub, err := fs.Sub(publicFS, "public")
	if err == nil {
		fileServer := http.FileServer(http.FS(sub))
		mux.HandleFunc("/diagram/{slug}", func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet {
				serveDiagramRoute(w, r)
				return
			}
			http.NotFound(w, r)
		})
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			// Add cache control headers to .ico, .css, .js, and .png files
			contentType := r.Header.Get("Content-Type")
			if r.Method == http.MethodGet && (contentType == "image/x-icon" || contentType == "text/css" || contentType == "application/javascript" || contentType == "image/png" || contentType == "image/jpeg") {
				w.Header().Set("Cache-Control", "public, max-age=31536000") // Cache for 1 year
			}
			fileServer.ServeHTTP(w, r)
		})
	} else {
		log.Printf("failed to access embedded public dir: %v", err)
		mux.Handle("/", http.FileServer(http.Dir(".")))
	}

	return LoggingMiddleware(statusPageMiddleware(mux, publicFS))
}

func serveDiagramRoute(w http.ResponseWriter, r *http.Request) {
	publicRoot := resolvePublicRoot()
	indexPath := filepath.Join(publicRoot, "diagram", "index.html")

	slug := r.PathValue("slug")
	if slug == "" || slug == "." || strings.Contains(slug, "..") || strings.Contains(slug, "/") {
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
