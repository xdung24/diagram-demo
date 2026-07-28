package mcp

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

const (
	defaultReleaseRepo = "https://api.github.com/repos/xdung24/diagram-mcp/releases/latest"
)

type releaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type releaseResponse struct {
	TagName string         `json:"tag_name"`
	Assets  []releaseAsset `json:"assets"`
}

// DownloadDiagramMCPServer fetches the latest release of diagram-mcp, selects
// the asset that matches the current OS and architecture, downloads it, and
// returns the local path to the extracted binary.
func DownloadDiagramMCPServer() (string, error) {
	release, err := fetchLatestRelease(defaultReleaseRepo)
	if err != nil {
		log.Printf("error fetching latest release: %v", err)
		return "", err
	}

	asset, ok := selectAssetForPlatform(release.Assets, runtime.GOOS, runtime.GOARCH)
	if !ok {
		log.Printf("no release asset found for %s/%s", runtime.GOOS, runtime.GOARCH)
		return "", fmt.Errorf("no release asset found for %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	appDir, err := executableDir()
	if err != nil {
		log.Printf("error resolving app directory: %v", err)
		return "", err
	}

	outputPath := filepath.Join(appDir, binaryNameForPlatform(runtime.GOOS))
	if _, err := os.Stat(outputPath); err == nil {
		currentVersion, versionErr := versionForBinary(outputPath)
		if versionErr == nil {
			latestVersion := strings.TrimPrefix(release.TagName, "v")
			if compareVersions(currentVersion, latestVersion) >= 0 {
				return outputPath, nil
			}
		} else {
			log.Printf("could not determine current binary version: %v", versionErr)
		}
	} else if !os.IsNotExist(err) {
		log.Printf("error checking binary path: %v", err)
		return "", fmt.Errorf("check binary path: %w", err)
	}

	if err := downloadFile(asset.BrowserDownloadURL, outputPath); err != nil {
		log.Printf("error downloading file: %v", err)
		return "", err
	}

	if err := os.Chmod(outputPath, 0o755); err != nil {
		log.Printf("error setting binary permissions: %v", err)
		return "", fmt.Errorf("chmod binary: %w", err)
	}

	return outputPath, nil
}

func fetchLatestRelease(url string) (releaseResponse, error) {
	var release releaseResponse

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return release, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "diagram-demo")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return release, fmt.Errorf("request release metadata: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return release, fmt.Errorf("github release lookup failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return release, fmt.Errorf("decode release metadata: %w", err)
	}
	return release, nil
}

func selectAssetForPlatform(assets []releaseAsset, goos, goarch string) (releaseAsset, bool) {
	var candidates []releaseAsset
	for _, asset := range assets {
		name := strings.ToLower(asset.Name)
		if strings.Contains(name, "windows") && goos != "windows" {
			continue
		}
		if strings.Contains(name, "linux") && goos != "linux" {
			continue
		}
		if strings.Contains(name, "darwin") && goos != "darwin" {
			continue
		}
		if strings.Contains(name, "amd64") && goarch != "amd64" {
			continue
		}
		if strings.Contains(name, "arm64") && goarch != "arm64" {
			continue
		}
		if strings.Contains(name, "x86_64") && goarch != "amd64" {
			continue
		}
		if strings.Contains(name, "aarch64") && goarch != "arm64" {
			continue
		}
		candidates = append(candidates, asset)
	}
	if len(candidates) == 0 {
		return releaseAsset{}, false
	}
	return candidates[0], true
}

func executableDir() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("get executable path: %w", err)
	}
	return filepath.Dir(executable), nil
}

func binaryNameForPlatform(goos string) string {
	if goos == "windows" {
		return "diagram-mcp.exe"
	}
	return "diagram-mcp"
}

func versionForBinary(binaryPath string) (string, error) {
	cmd := exec.Command(binaryPath, "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("run version command: %w", err)
	}
	return extractVersion(string(output)), nil
}

func extractVersion(output string) string {
	output = strings.TrimSpace(output)
	for _, candidate := range strings.Fields(output) {
		if strings.HasPrefix(candidate, "v") && len(candidate) > 1 {
			candidate = strings.TrimPrefix(candidate, "v")
		}
		if strings.Count(candidate, ".") >= 1 {
			parts := strings.Split(candidate, ".")
			if len(parts) >= 2 {
				for _, part := range parts {
					if _, err := strconv.Atoi(part); err != nil {
						return ""
					}
				}
				return strings.Join(parts, ".")
			}
		}
	}
	return ""
}

func compareVersions(left, right string) int {
	leftParts := strings.Split(left, ".")
	rightParts := strings.Split(right, ".")
	maxLen := len(leftParts)
	if len(rightParts) > maxLen {
		maxLen = len(rightParts)
	}
	for i := 0; i < maxLen; i++ {
		var leftValue, rightValue int
		var leftErr, rightErr error
		if i < len(leftParts) {
			leftValue, leftErr = strconv.Atoi(leftParts[i])
		}
		if i < len(rightParts) {
			rightValue, rightErr = strconv.Atoi(rightParts[i])
		}
		if leftErr != nil || rightErr != nil {
			return 0
		}
		if leftValue < rightValue {
			return -1
		}
		if leftValue > rightValue {
			return 1
		}
	}
	return 0
}

func downloadFile(url, outputPath string) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("download asset: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download asset failed: %s", resp.Status)
	}

	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create archive file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("save archive file: %w", err)
	}
	return nil
}
