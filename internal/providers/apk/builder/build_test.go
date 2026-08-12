package builder

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/moby/buildkit/client/llb"

	"github.com/broadsage/doko/internal/config"
	"github.com/broadsage/doko/internal/pipeline"
)

func TestCollectPipelinePackages(t *testing.T) {
	// Set up pipeline templates
	pipeline.RegisterTemplateResolver(func(name string) ([]byte, error) {
		if name == "curl-pipeline" {
			return []byte(`
name: curl-pipeline
needs:
  packages:
    - curl
    - ca-certificates
runs: echo curl
`), nil
		}
		if name == "git-pipeline" {
			return []byte(`
name: git-pipeline
needs:
  packages:
    - git
runs: echo git
`), nil
		}
		return nil, errors.New("pipeline not found")
	})

	spec := &config.BuildSpec{
		Pipeline: []config.PipelineStep{
			{Uses: "curl-pipeline"},
			{Uses: "git-pipeline"},
			{Uses: "curl-pipeline"}, // duplicate should be filtered
		},
	}

	pkgs, err := collectPipelinePackages(spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{"ca-certificates", "curl", "git"}
	if len(pkgs) != len(expected) {
		t.Fatalf("expected package list %v, got %v", expected, pkgs)
	}
	for i, v := range expected {
		if pkgs[i] != v {
			t.Errorf("at index %d: expected %q, got %q", i, v, pkgs[i])
		}
	}
}

func TestBuildInstallCommand(t *testing.T) {
	spec := &config.BuildSpec{
		Contents: config.ContentsConfig{
			Repositories: []string{"https://dl-cdn.alpinelinux.org/alpine/v3.20/main"},
			Packages:     []string{"bash", "jq"},
		},
		Pipeline: []config.PipelineStep{
			{Uses: "curl-pipeline"},
		},
	}

	cmd, err := buildInstallCommand(spec)
	if err != nil {
		t.Fatalf("unexpected error building install command: %v", err)
	}

	if !strings.Contains(cmd, "https://dl-cdn.alpinelinux.org/alpine/v3.20/main") {
		t.Errorf("expected repo addition, got: %q", cmd)
	}
	if !strings.Contains(cmd, "apk add --no-cache bash jq ca-certificates curl") {
		t.Errorf("expected packages to be added, got: %q", cmd)
	}
}

func TestResolvePipelineSteps(t *testing.T) {
	pipeline.RegisterTemplateResolver(func(name string) ([]byte, error) {
		if name == "args-pipeline" {
			return []byte(`
name: args-pipeline
inputs:
  dest:
    default: /usr/bin
    required: true
  opt:
    default: default-val
    required: false
runs: cp file ${{inputs.dest}}/${{inputs.opt}}
`), nil
		}
		return nil, errors.New("pipeline not found")
	})

	spec := &config.BuildSpec{
		Name:    "my-app",
		Version: "1.0.0",
		Pipeline: []config.PipelineStep{
			{
				Name: "Custom Run Step",
				Runs: "echo ${{package.name}} version ${{package.version}}",
			},
			{
				Uses: "args-pipeline",
				With: map[string]any{
					"dest": "/usr/bin",
					"opt":  "my-val",
				},
			},
		},
	}

	steps, err := ResolvePipelineSteps(spec)
	if err != nil {
		t.Fatalf("unexpected error resolving steps: %v", err)
	}

	if len(steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(steps))
	}

	if steps[0].Name != "Custom Run Step" || steps[0].Script != "echo my-app version 1.0.0" {
		t.Errorf("unexpected custom step: %+v", steps[0])
	}

	if steps[1].Name != "pipeline step 2 (args-pipeline)" || steps[1].Script != "cp file /usr/bin/my-val" {
		t.Errorf("unexpected pipeline step: %+v", steps[1])
	}

	// Test validation error: both runs and uses
	invalidSpec1 := &config.BuildSpec{
		Pipeline: []config.PipelineStep{
			{Runs: "echo", Uses: "args-pipeline"},
		},
	}
	_, err = ResolvePipelineSteps(invalidSpec1)
	if err == nil {
		t.Error("expected error for both runs and uses, got nil")
	}

	// Test validation error: neither runs nor uses
	invalidSpec2 := &config.BuildSpec{
		Pipeline: []config.PipelineStep{
			{},
		},
	}
	_, err = ResolvePipelineSteps(invalidSpec2)
	if err == nil {
		t.Error("expected error for neither runs nor uses, got nil")
	}

	// Test validation error: missing required inputs
	invalidSpec3 := &config.BuildSpec{
		Pipeline: []config.PipelineStep{
			{
				Uses: "args-pipeline",
				With: map[string]any{
					"dest": "", // required input must not be empty
				},
			},
		},
	}
	_, err = ResolvePipelineSteps(invalidSpec3)
	if err == nil {
		t.Error("expected error for empty required input, got nil")
	}
}

func TestBuildAPK(t *testing.T) {
	spec := &config.BuildSpec{
		Name:        "test-pkg",
		Version:     "1.0",
		Description: "testing",
		URL:         "https://example.com",
		License:     "MIT",
		Pipeline: []config.PipelineStep{
			{
				Runs: "echo building",
				SSH:  true,
				Secrets: []config.PipelineSecret{
					{ID: "mysecret", Target: "/run/secrets/secret"},
				},
				Network: "none",
			},
		},
	}

	state, err := BuildAPK(context.Background(), spec, llb.Scratch(), "alpine:3.20", nil)
	if err != nil {
		t.Fatalf("BuildAPK failed: %v", err)
	}

	_, err = state.Marshal(context.Background())
	if err != nil {
		t.Fatalf("failed to marshal BuildAPK state: %v", err)
	}

	// Test incomplete spec validations
	incompleteSpecs := []*config.BuildSpec{
		{Version: "1.0", Description: "d", URL: "u", License: "l"},
		{Name: "n", Description: "d", URL: "u", License: "l"},
		{Name: "n", Version: "v", URL: "u", License: "l"},
		{Name: "n", Version: "v", Description: "d", License: "l"},
		{Name: "n", Version: "v", Description: "d", URL: "u"},
	}

	for i, is := range incompleteSpecs {
		_, err = BuildAPK(context.Background(), is, llb.Scratch(), "alpine:3.20", nil)
		if err == nil {
			t.Errorf("expected error for incomplete spec at index %d, got nil", i)
		}
	}
}
