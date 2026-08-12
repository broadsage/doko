package pipeline

import (
	"errors"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestInputDef_UnmarshalYAML(t *testing.T) {
	// Test short form (string default)
	var id1 InputDef
	yaml1 := []byte(`"default-value"`)
	if err := yaml.Unmarshal(yaml1, &id1); err != nil {
		t.Fatalf("failed to unmarshal short form: %v", err)
	}
	if id1.Default != "default-value" || id1.Required || id1.Description != "" {
		t.Errorf("unexpected short form result: %+v", id1)
	}

	// Test long form
	var id2 InputDef
	yaml2 := []byte(`
description: "A test input"
default: "test-default"
required: true
`)
	if err := yaml.Unmarshal(yaml2, &id2); err != nil {
		t.Fatalf("failed to unmarshal long form: %v", err)
	}
	if id2.Description != "A test input" || id2.Default != "test-default" || !id2.Required {
		t.Errorf("unexpected long form result: %+v", id2)
	}

	// Test invalid form
	var id3 InputDef
	yaml3 := []byte(`[1, 2, 3]`)
	if err := yaml.Unmarshal(yaml3, &id3); err == nil {
		t.Error("expected error unmarshaling list into InputDef, got nil")
	}
}

func TestGetPipeline(t *testing.T) {
	// Reset loaded pipelines and resolver
	loadedPipelinesMu.Lock()
	loadedPipelines = nil
	templateResolverFn = nil
	loadedPipelinesMu.Unlock()

	// Test no resolver registered
	_, err := GetPipeline("test")
	if err == nil {
		t.Error("expected error when no resolver is registered, got nil")
	} else if !strings.Contains(err.Error(), "no pipeline template resolver registered") {
		t.Errorf("unexpected error: %v", err)
	}

	// Register resolver
	mockTemplates := map[string]string{
		"simple": `
name: simple
runs: echo hello
`,
		"invalid-yaml": `
name: invalid
runs: [
`,
		"missing-runs": `
name: missing
`,
	}

	RegisterTemplateResolver(func(name string) ([]byte, error) {
		content, ok := mockTemplates[name]
		if !ok {
			return nil, errors.New("template not found")
		}
		return []byte(content), nil
	})

	// Test successful load
	p, err := GetPipeline("simple")
	if err != nil {
		t.Fatalf("unexpected error loading pipeline: %v", err)
	}
	if p.Name != "simple" || p.Runs != "echo hello" {
		t.Errorf("unexpected pipeline def: %+v", p)
	}

	// Test caching (loadedPipelines)
	p2, err := GetPipeline("simple")
	if err != nil {
		t.Fatalf("unexpected error loading cached pipeline: %v", err)
	}
	if p2 != p {
		t.Error("expected cached pointer to be identical")
	}

	// Test template not found
	_, err = GetPipeline("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent pipeline, got nil")
	}

	// Test invalid YAML
	_, err = GetPipeline("invalid-yaml")
	if err == nil {
		t.Error("expected error for invalid yaml pipeline, got nil")
	}

	// Test missing runs
	_, err = GetPipeline("missing-runs")
	if err == nil {
		t.Error("expected error for pipeline missing runs, got nil")
	}
}
