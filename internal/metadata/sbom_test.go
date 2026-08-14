package metadata

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/broadsage/doko/internal/providers"
)

func TestGenerateSBOM(t *testing.T) {
	resolved := []providers.Package{
		{
			Name:        "curl",
			Version:     "8.5.0-r0",
			Arch:        "x86_64",
			License:     "MIT",
			Description: "URL transfer utility",
			DownloadURL: "https://dl-cdn.alpinelinux.org/alpine/v3.19/main/x86_64/curl-8.5.0-r0.apk",
			Size:        12345,
		},
	}

	payload, err := GenerateSBOM(context.Background(), "dhi-curl-image", resolved)
	if err != nil {
		t.Fatalf("failed to generate SBOM: %v", err)
	}

	// Verify it is valid JSON
	var result map[string]any
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("generated SBOM is not valid JSON: %v", err)
	}

	// Verify CycloneDX spec fields exist
	bomFormat, ok := result["bomFormat"].(string)
	if !ok || bomFormat != "CycloneDX" {
		t.Errorf("expected bomFormat to be 'CycloneDX', got %v", result["bomFormat"])
	}

	// Verify package is present in the output
	payloadStr := string(payload)
	if !strings.Contains(payloadStr, "curl") {
		t.Error("expected SBOM to contain 'curl' package name, but it was missing")
	}
	if !strings.Contains(payloadStr, "8.5.0-r0") {
		t.Error("expected SBOM to contain package version '8.5.0-r0', but it was missing")
	}
}
