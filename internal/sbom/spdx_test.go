package sbom

import (
	"encoding/json"
	"testing"

	"github.com/broadsage/doko/internal/config"
	"github.com/broadsage/doko/internal/resolver"
)

func TestGenerateSPDX_BasicSBOM(t *testing.T) {
	spec := &config.Spec{
		Name:     "test-app",
		Provider: "apk",
		Base:     "alpine-3.23",
	}

	packages := []resolver.Package{
		{Name: "curl", Version: "8.12.1-r0", Arch: "x86_64", License: "MIT", DownloadURL: "https://example.com/curl.apk", Checksum: "abc123"},
		{Name: "nginx", Version: "1.27.4-r0", Arch: "x86_64", License: "BSD-2-Clause", DownloadURL: "https://example.com/nginx.apk"},
	}

	doc, err := GenerateSPDX(spec, packages)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if doc.SPDXVersion != "SPDX-2.3" {
		t.Errorf("expected SPDX-2.3, got %q", doc.SPDXVersion)
	}
	if doc.Name != "test-app" {
		t.Errorf("expected name 'test-app', got %q", doc.Name)
	}

	// 1 root + 2 resolved packages = 3 total
	if len(doc.Packages) != 3 {
		t.Fatalf("expected 3 packages, got %d", len(doc.Packages))
	}

	// Verify root package.
	if doc.Packages[0].SPDXID != "SPDXRef-RootPackage" {
		t.Errorf("expected root SPDXID 'SPDXRef-RootPackage', got %q", doc.Packages[0].SPDXID)
	}

	// Verify curl package.
	curlPkg := doc.Packages[1]
	if curlPkg.Name != "curl" {
		t.Errorf("expected 'curl', got %q", curlPkg.Name)
	}
	if curlPkg.LicenseConcluded != "MIT" {
		t.Errorf("expected license 'MIT', got %q", curlPkg.LicenseConcluded)
	}
	if len(curlPkg.ExternalRefs) != 1 {
		t.Fatalf("expected 1 external ref (PURL), got %d", len(curlPkg.ExternalRefs))
	}
	if curlPkg.ExternalRefs[0].Locator != "pkg:apk/alpine/curl@8.12.1-r0?arch=x86_64" {
		t.Errorf("unexpected PURL: %q", curlPkg.ExternalRefs[0].Locator)
	}
	if len(curlPkg.Checksums) != 1 || curlPkg.Checksums[0].Value != "abc123" {
		t.Errorf("expected checksum 'abc123', got %v", curlPkg.Checksums)
	}

	// Verify relationships: 1 DESCRIBES + 2 DEPENDS_ON = 3
	if len(doc.Relationships) != 3 {
		t.Errorf("expected 3 relationships, got %d", len(doc.Relationships))
	}
}

func TestMarshalSPDX_ValidJSON(t *testing.T) {
	spec := &config.Spec{Name: "json-test", Provider: "apk", Base: "alpine-3.23"}
	doc, _ := GenerateSPDX(spec, nil)

	data, err := MarshalSPDX(doc)
	if err != nil {
		t.Fatalf("unexpected marshal error: %v", err)
	}

	var parsed Document
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("generated JSON is not valid: %v", err)
	}
	if parsed.SPDXVersion != "SPDX-2.3" {
		t.Errorf("round-trip SPDX version mismatch: %q", parsed.SPDXVersion)
	}
}

func TestBuildPURL(t *testing.T) {
	tests := []struct {
		provider string
		pkg      resolver.Package
		expected string
	}{
		{"apk", resolver.Package{Name: "curl", Version: "8.12.1-r0", Arch: "x86_64"}, "pkg:apk/alpine/curl@8.12.1-r0?arch=x86_64"},
		{"unknown", resolver.Package{Name: "test"}, ""},
	}

	for _, tc := range tests {
		result := BuildPURL(tc.provider, tc.pkg)
		if result != tc.expected {
			t.Errorf("BuildPURL(%q, %q) = %q, want %q", tc.provider, tc.pkg.Name, result, tc.expected)
		}
	}
}
