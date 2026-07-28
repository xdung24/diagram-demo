package mcp

import (
	"os"
	"path/filepath"
	"runtime"
)

func FindDiagramMCPBinary() (string, error) {
	candidates := []string{
		"diagram-mcp.exe",
		"./diagram-mcp.exe",
		"diagram-mcp",
		"./diagram-mcp",
	}

	for _, candidate := range candidates {
		if abs, err := filepath.Abs(candidate); err == nil {
			if info, err := os.Stat(abs); err == nil && !info.IsDir() {
				return abs, nil
			}
		}
	}

	binary := filepath.Join(".", "diagram-mcp.exe")
	if runtime.GOOS != "windows" {
		binary = filepath.Join(".", "diagram-mcp")
	}
	if _, err := os.Stat(binary); err == nil {
		if abs, err := filepath.Abs(binary); err == nil {
			return abs, nil
		}
	}

	if downloadedBinary, downloadErr := DownloadDiagramMCPServer(); downloadErr == nil {
		return downloadedBinary, nil
	} else {
		return binary, downloadErr
	}
}
