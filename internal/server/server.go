package server

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
)

func CreateHttpServer(publicFS fs.FS, svc *Service) http.Handler {
	mux := http.NewServeMux()

	sub, err := fs.Sub(publicFS, "public")
	if err == nil {
		mux.Handle("/diagram/", http.StripPrefix("/diagram/", http.FileServer(http.Dir("./public/diagram"))))
		fileServer := http.FileServer(http.FS(sub))
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
			writeJSON(w, http.StatusOK, map[string]any{"result": res, "generatedCode": res["generatedCode"]})
		})
		handler.ServeHTTP(w, r)
	})

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

	return LoggingMiddleware(statusPageMiddleware(mux, publicFS))
}
