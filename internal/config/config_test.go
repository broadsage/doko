package config

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
)

func TestParse_ValidConfig(t *testing.T) {
	yamlInput := `
name: test-app

contents:
  packages:
    - curl
    - nginx
  paths:
    - type: directory
      path: /var/www/html
      uid: 65532
      gid: 65532
      mode: "0755"

entrypoint: ["nginx", "-g", "daemon off;"]

runtime:
  user: nonroot
  env:
    NODE_ENV: production
  ports:
    - 80
`
	spec, err := Parse(strings.NewReader(yamlInput))
	if err != nil {
		t.Fatalf("unexpected error parsing yaml: %v", err)
	}

	if spec.Name != "test-app" {
		t.Errorf("expected name 'test-app', got %q", spec.Name)
	}
	if spec.Arch != "" {
		t.Errorf("expected default arch '', got %q", spec.Arch)
	}
	if len(spec.Contents.Packages) != 2 {
		t.Errorf("expected 2 packages, got %d", len(spec.Contents.Packages))
	}
	if spec.Runtime.Env["NODE_ENV"] != "production" {
		t.Errorf("expected env NODE_ENV=production, got %q", spec.Runtime.Env["NODE_ENV"])
	}
}

func TestParse_MissingRequiredFields(t *testing.T) {
	yamlInput := `
contents:
  packages:
    - curl
`
	_, err := Parse(strings.NewReader(yamlInput))
	if err == nil {
		t.Fatal("expected validation error for missing name, got nil")
	}
	if !strings.Contains(err.Error(), "name") {
		t.Errorf("expected 'name' in validation error, got: %v", err)
	}
}

func TestParse_WithVariables(t *testing.T) {
	yamlInput := `
name: test-variables
vars:
  NGINX_VERSION: "1.24.0"
  APP_ENV: "production"
contents:
  packages:
    - nginx=${NGINX_VERSION}
runtime:
  env:
    ENV: ${APP_ENV}
`
	spec, err := Parse(strings.NewReader(yamlInput))
	if err != nil {
		t.Fatalf("unexpected error parsing yaml: %v", err)
	}

	if len(spec.Contents.Packages) != 1 || spec.Contents.Packages[0] != "nginx=1.24.0" {
		t.Errorf("expected package 'nginx=1.24.0', got %v", spec.Contents.Packages)
	}
	if spec.Runtime.Env["ENV"] != "production" {
		t.Errorf("expected ENV to be 'production', got %q", spec.Runtime.Env["ENV"])
	}
}

func TestParse_PrivilegedConfig(t *testing.T) {
	yamlInput := `
name: test-privileged
builds:
  - name: test-builder
    privileged: true
    contents:
      packages:
        - make
`
	spec, err := Parse(strings.NewReader(yamlInput))
	if err != nil {
		t.Fatalf("unexpected error parsing yaml: %v", err)
	}

	if len(spec.Builds) != 1 || !spec.Builds[0].Privileged {
		t.Errorf("expected Builds[0].Privileged to be true")
	}
}

func TestParse_SchemaValidationFailure(t *testing.T) {
	// yamlInput with a typo field ("non-existent-field") and wrong type for entrypoint (must be array, here it is string)
	yamlInput := `
name: bad-config
provider: apk
non-existent-field: hello
entrypoint: "nginx -g daemon off;"
`
	_, err := Parse(strings.NewReader(yamlInput))
	if err == nil {
		t.Fatal("expected schema validation error, got nil")
	}
	if !strings.Contains(err.Error(), "schema validation failed") {
		t.Errorf("expected schema validation failure message, got: %v", err)
	}
}

func TestSchemaIsUpToDate(t *testing.T) {
	// 1. Generate schema dynamically from current config.Spec struct via reflection
	generatedSchema, err := jsonschema.For[Spec](nil)
	if err != nil {
		t.Fatalf("failed to generate schema: %v", err)
	}
	generatedSchema.Schema = "http://json-schema.org/draft-07/schema#"

	generatedBytes, err := json.MarshalIndent(generatedSchema, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal generated schema: %v", err)
	}

	// 2. Compare against embedded schemaData
	cleanGenerated := strings.TrimSpace(string(generatedBytes))
	cleanEmbedded := strings.TrimSpace(string(schemaData))

	if cleanGenerated != cleanEmbedded {
		t.Errorf("schema.json is out of date! Please run: go generate ./...")
	}
}
