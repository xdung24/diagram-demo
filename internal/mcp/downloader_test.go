package mcp

import "testing"

func TestBinaryNameForPlatform(t *testing.T) {
	if got := binaryNameForPlatform("windows"); got != "diagram-mcp.exe" {
		t.Fatalf("expected windows binary name, got %s", got)
	}
	if got := binaryNameForPlatform("linux"); got != "diagram-mcp" {
		t.Fatalf("expected linux binary name, got %s", got)
	}
}

func TestExtractVersion(t *testing.T) {
	if got := extractVersion("diagram-mcp version v1.2.3\n"); got != "1.2.3" {
		t.Fatalf("expected parsed version, got %s", got)
	}
	if got := extractVersion("diagram-mcp 2.0.0"); got != "2.0.0" {
		t.Fatalf("expected parsed version, got %s", got)
	}
	if got := extractVersion("unknown output"); got != "" {
		t.Fatalf("expected empty version for unknown output, got %s", got)
	}
}

func TestCompareVersions(t *testing.T) {
	if compareVersions("1.2.3", "1.2.4") >= 0 {
		t.Fatalf("expected 1.2.3 to be older than 1.2.4")
	}
	if compareVersions("2.0.0", "1.9.9") <= 0 {
		t.Fatalf("expected 2.0.0 to be newer than 1.9.9")
	}
	if compareVersions("1.2.3", "1.2.3") != 0 {
		t.Fatalf("expected equal versions to compare equal")
	}
}

func TestSelectAssetForPlatform(t *testing.T) {
	assets := []releaseAsset{
		{Name: "diagram-mcp-linux-amd64.tar.gz", BrowserDownloadURL: "https://example.com/linux"},
		{Name: "diagram-mcp_Windows_x86_64.zip", BrowserDownloadURL: "https://example.com/windows"},
		{Name: "diagram-mcp-darwin-arm64", BrowserDownloadURL: "https://example.com/darwin"},
	}

	got, ok := selectAssetForPlatform(assets, "linux", "amd64")
	if !ok {
		t.Fatalf("expected asset for linux/amd64")
	}
	if got.Name != assets[0].Name {
		t.Fatalf("expected linux asset, got %s", got.Name)
	}

	got, ok = selectAssetForPlatform(assets, "windows", "amd64")
	if !ok {
		t.Fatalf("expected asset for windows/amd64")
	}
	if got.Name != assets[1].Name {
		t.Fatalf("expected windows asset, got %s", got.Name)
	}

	got, ok = selectAssetForPlatform(assets, "darwin", "arm64")
	if !ok {
		t.Fatalf("expected asset for darwin/arm64")
	}
	if got.Name != assets[2].Name {
		t.Fatalf("expected darwin asset, got %s", got.Name)
	}
}
