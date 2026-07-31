package provenance

import (
	"encoding/json"
	"testing"

	"github.com/broadsage/doko/internal/config"
	"github.com/broadsage/doko/internal/providers"
)

func TestGenerate_Provenance(t *testing.T) {
	spec := &config.Spec{
		Name:     "my-image",
		Provider: "apk",
		Base:     "alpine-3.23",
		Arch:     "amd64",
	}

	packages := []providers.Package{
		{
			Name:        "musl",
			Version:     "1.2.4-r2",
			DownloadURL: "https://example.com/musl.apk",
			Checksum:    "sha256:1234567890abcdef",
		},
	}

	att, err := Generate(spec, packages)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if att.BuildType != "https://doko.io/provenance/v1" {
		t.Errorf("unexpected buildType: %q", att.BuildType)
	}
	if att.Subject.Name != "my-image" {
		t.Errorf("unexpected subject name: %q", att.Subject.Name)
	}

	// 2 default materials (provider + base) + 1 package = 3 total
	if len(att.Materials) != 3 {
		t.Fatalf("expected 3 materials, got %d", len(att.Materials))
	}

	if att.Materials[2].URI != "https://example.com/musl.apk" {
		t.Errorf("expected package URI, got %q", att.Materials[2].URI)
	}
	if att.Materials[2].Checksum != "sha256:1234567890abcdef" {
		t.Errorf("expected package checksum, got %q", att.Materials[2].Checksum)
	}
}

func TestMarshal_Provenance(t *testing.T) {
	spec := &config.Spec{Name: "test"}
	att, _ := Generate(spec, nil)

	data, err := Marshal(att)
	if err != nil {
		t.Fatalf("unexpected marshal error: %v", err)
	}

	var parsed Attestation
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("generated JSON is not valid: %v", err)
	}
	if parsed.BuildType != "https://doko.io/provenance/v1" {
		t.Errorf("round-trip buildType mismatch: %q", parsed.BuildType)
	}
}
