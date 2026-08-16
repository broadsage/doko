package metadata

import (
	"encoding/json"
	"testing"

	"github.com/moby/buildkit/frontend/gateway/client"

	"github.com/broadsage/doko/internal/config"
)

func TestSetImageConfig(t *testing.T) {
	spec := &config.Spec{
		Name:    "test-image",
		Variant: "slim",
		Arch:    "arm64",
		Base:    "alpine-3.23",
		Runtime: config.RuntimeConfig{
			User:  "nonroot",
			Ports: []int{8080},
			Env:   map[string]string{"ENV_VAR": "value"},
		},
		EntryPoint: []string{"/bin/sh"},
		Cmd:        []string{"-c"},
		Dates: map[string]string{
			"release": "2026-08-07",
		},
		Annotations: map[string]string{
			"custom.annotation": "custom-value",
		},
	}

	result := client.NewResult()
	err := setImageConfig(spec, result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify image config JSON was attached
	configJSON, ok := result.Metadata["containerimage.config"]
	if !ok {
		t.Fatalf("missing containerimage.config metadata")
	}

	var imgConfig map[string]any
	if err := json.Unmarshal(configJSON, &imgConfig); err != nil {
		t.Fatalf("failed to unmarshal image config: %v", err)
	}

	if imgConfig["os"] != "linux" {
		t.Errorf("expected os to be linux, got %q", imgConfig["os"])
	}
	if imgConfig["architecture"] != "arm64" {
		t.Errorf("expected architecture to be arm64, got %q", imgConfig["architecture"])
	}

	innerConfig, ok := imgConfig["config"].(map[string]any)
	if !ok {
		t.Fatalf("missing config section in image config")
	}

	if innerConfig["User"] != "nonroot" {
		t.Errorf("expected User nonroot, got %q", innerConfig["User"])
	}

	// Verify annotations were attached
	annotationsJSON, ok := result.Metadata["containerimage.annotations"]
	if !ok {
		t.Fatalf("missing containerimage.annotations metadata")
	}

	var annotations map[string]string
	if err := json.Unmarshal(annotationsJSON, &annotations); err != nil {
		t.Fatalf("failed to unmarshal annotations: %v", err)
	}

	if annotations["com.broadsage.bsi.title"] != "test-image" {
		t.Errorf("expected title annotation, got %q", annotations["com.broadsage.bsi.title"])
	}
	if annotations["custom.annotation"] != "custom-value" {
		t.Errorf("expected custom annotation, got %q", annotations["custom.annotation"])
	}
}

func TestSetImageConfig_SourceDateEpoch(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1718000000") // 2024-06-10T06:13:20Z

	spec := &config.Spec{
		Name: "test-image",
		Dates: map[string]string{
			"release": "2026-08-07",
		},
	}

	result := client.NewResult()
	err := setImageConfig(spec, result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	configJSON := result.Metadata["containerimage.config"]
	var imgConfig map[string]any
	if err := json.Unmarshal(configJSON, &imgConfig); err != nil {
		t.Fatalf("failed to unmarshal image config: %v", err)
	}

	expectedTime := "2024-06-10T06:13:20Z"
	if imgConfig["created"] != expectedTime {
		t.Errorf("expected created to be %q, got %q", expectedTime, imgConfig["created"])
	}
}
