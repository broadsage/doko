package sbom

import (
	"testing"

	"github.com/broadsage/doko/internal/config"
	"github.com/broadsage/doko/internal/resolver"
)

func TestGenerateCycloneDX_BasicSBOM(t *testing.T) {
	spec := &config.Spec{
		Name:     "test-app",
		Provider: "apk",
		Base:     "alpine-3.23",
	}

	packages := []resolver.Package{
		{Name: "curl", Version: "8.12.1-r0", Arch: "x86_64", License: "MIT", DownloadURL: "https://example.com/curl.apk", Checksum: "abc123"},
	}

	doc, err := GenerateCycloneDX(spec, packages)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if doc.BOMFormat != "CycloneDX" {
		t.Errorf("expected CycloneDX, got %q", doc.BOMFormat)
	}
	if doc.SpecVersion != "1.6" {
		t.Errorf("expected 1.6, got %q", doc.SpecVersion)
	}

	if len(doc.Components) != 1 {
		t.Fatalf("expected 1 component, got %d", len(doc.Components))
	}

	c := doc.Components[0]
	if c.Name != "curl" {
		t.Errorf("expected 'curl', got %q", c.Name)
	}
	if len(c.Licenses) != 1 || c.Licenses[0].License.ID != "MIT" {
		t.Errorf("expected license 'MIT', got %v", c.Licenses)
	}
	if c.PURL != "pkg:apk/alpine/curl@8.12.1-r0?arch=x86_64" {
		t.Errorf("unexpected PURL: %q", c.PURL)
	}
	if len(c.Hashes) != 1 || c.Hashes[0].Value != "abc123" {
		t.Errorf("expected checksum 'abc123', got %v", c.Hashes)
	}
}
