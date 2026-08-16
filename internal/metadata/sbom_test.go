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

	// 1. Test CycloneDX
	payload, suffix, err := GenerateSBOM(context.Background(), "bhi-curl-image", resolved, "cyclonedx")
	if err != nil {
		t.Fatalf("failed to generate CycloneDX SBOM: %v", err)
	}
	if suffix != "sbom.cdx.json" {
		t.Errorf("expected suffix 'sbom.cdx.json', got %q", suffix)
	}

	var cdxResult map[string]any
	if err := json.Unmarshal(payload, &cdxResult); err != nil {
		t.Fatalf("generated CycloneDX SBOM is not valid JSON: %v", err)
	}
	bomFormat, ok := cdxResult["bomFormat"].(string)
	if !ok || bomFormat != "CycloneDX" {
		t.Errorf("expected bomFormat to be 'CycloneDX', got %v", cdxResult["bomFormat"])
	}

	// 2. Test SPDX
	payloadSpdx, suffixSpdx, err := GenerateSBOM(context.Background(), "bhi-curl-image", resolved, "spdx")
	if err != nil {
		t.Fatalf("failed to generate SPDX SBOM: %v", err)
	}
	if suffixSpdx != "sbom.spdx.json" {
		t.Errorf("expected suffix 'sbom.spdx.json', got %q", suffixSpdx)
	}

	var spdxResult map[string]any
	if err := json.Unmarshal(payloadSpdx, &spdxResult); err != nil {
		t.Fatalf("generated SPDX SBOM is not valid JSON: %v", err)
	}
	spdxVersion, ok := spdxResult["spdxVersion"].(string)
	if !ok || !strings.HasPrefix(spdxVersion, "SPDX-") {
		t.Errorf("expected spdxVersion prefix 'SPDX-', got %v", spdxResult["spdxVersion"])
	}
}

func TestGetCleanImageName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"ghcr.io/broadsage/doko/alpine", "bhi-alpine"},
		{"ghcr.io/broadsage/doko/postgres:latest", "bhi-postgres"},
		{"my-registry.org/custom-app:1.2.3@sha256:12345", "bhi-custom-app"},
		{"", "bhi-image"},
	}

	for _, tc := range tests {
		got := GetCleanImageName(tc.input)
		if got != tc.expected {
			t.Errorf("GetCleanImageName(%q) = %q; expected %q", tc.input, got, tc.expected)
		}
	}
}
