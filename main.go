package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/xdung24/diagram-demo/internal/dotenv"
	"github.com/xdung24/diagram-demo/internal/mcp"
	"github.com/xdung24/diagram-demo/internal/server"
)

// app version
var version = "dev"

//go:embed public/**
var publicFS embed.FS

func main() {
	// Load .env file automatically into the system environment
	dotenv.Load()

	// Run basic commands
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version", "-v", "--version":
			fmt.Printf("demo-server_%s\n", version)
			return
		}
	}

	var argPort, diagramType string
	flag.StringVar(&argPort, "p", "", "serve HTTP server at this port")
	flag.StringVar(&argPort, "port", "", "serve HTTP server at this port")
	flag.StringVar(&diagramType, "type", "mermaid", "diagram type, one of: mermaid, bpmn, drawio")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "diagram demo server\n")
		fmt.Fprintf(os.Stderr, "Usage:\n")
		fmt.Fprintf(os.Stderr, "  diagram-demo --version\n")
		fmt.Fprintf(os.Stderr, "  diagram-demo [flags]\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	// Server port
	port := "8080"
	if os.Getenv("PORT") != "" {
		port = os.Getenv("PORT")
	}
	if argPort != "" {
		port = argPort
	}

	// App contxt
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// diagram service
	binary, err := mcp.FindDiagramMCPBinary()
	if err != nil {
		log.Printf("failed to locate or download diagram-mcp: %v", err)
	}
	diagramService, err := server.NewDiagramService(ctx, binary, diagramType+"-mcp")
	if err != nil {
		log.Printf("failed to start MCP client: %v", err)
		os.Exit(1)
	}
	defer func() {
		_ = diagramService.Close()
	}()

	// Start the HTTP server
	log.Printf("serving at %s", ":"+port)
	srv := &http.Server{Addr: ":" + port, Handler: server.CreateHttpServer(publicFS, diagramService)}
	go func() {
		<-ctx.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("demo-server had unexpected error: %v", err)
	}
}
