package config

import (
	"strings"
	"testing"
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
	if !strings.Contains(err.Error(), "name is required") {
		t.Errorf("expected 'name is required' in error, got: %v", err)
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
