package providers

import (
	"context"
	"strings"
	"testing"

	buildkitllb "github.com/moby/buildkit/client/llb"

	"github.com/broadsage/doko/internal/config"
)

type mockResolver struct{}

func (m *mockResolver) Name() string { return "mock" }
func (m *mockResolver) Resolve(ctx context.Context, packages []string) ([]Package, error) {
	return []Package{{Name: "mock-pkg"}}, nil
}

type mockBuilder struct{}

func (m *mockBuilder) Name() string                        { return "mock-builder" }
func (m *mockBuilder) ResolveBaseImage(base string) string { return base }
func (m *mockBuilder) KeyringDest(filename string) string  { return "/keys/" + filename }
func (m *mockBuilder) CACertDest(filename string) string   { return "/certs/" + filename }
func (m *mockBuilder) UpdateCACertCommand() []string       { return []string{"update-certs"} }
func (m *mockBuilder) InstallScript(installs, removals []string) string {
	return "install " + strings.Join(installs, ",")
}
func (m *mockBuilder) CacheMounts() []buildkitllb.RunOption { return nil }
func (m *mockBuilder) RemovePaths() []string                { return nil }
func (m *mockBuilder) BuildPackage(ctx context.Context, spec *config.BuildSpec, sourceState buildkitllb.State, resolver buildkitllb.ImageMetaResolver, opts ...buildkitllb.ConstraintsOpt) (buildkitllb.State, error) {
	return buildkitllb.Scratch(), nil
}
func (m *mockBuilder) AssemblePackage(dataDir, outPath string, spec *config.BuildSpec, arch string) (string, error) {
	return "mock.pkg", nil
}

func TestResolverRegistry(t *testing.T) {
	RegisterResolver("mock", func(opts Options) (Resolver, error) {
		return &mockResolver{}, nil
	})

	r, err := NewResolver("mock", Options{})
	if err != nil {
		t.Fatalf("failed to create registered resolver: %v", err)
	}
	if r.Name() != "mock" {
		t.Errorf("expected resolver name 'mock', got %q", r.Name())
	}

	_, err = NewResolver("nonexistent", Options{})
	if err == nil {
		t.Error("expected error creating unregistered resolver, got nil")
	} else if !strings.Contains(err.Error(), "unsupported package provider") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestBuilderRegistry(t *testing.T) {
	b := &mockBuilder{}
	RegisterBuilder("mock-builder", b)

	retrieved, err := GetBuilder("mock-builder")
	if err != nil {
		t.Fatalf("failed to retrieve registered builder: %v", err)
	}
	if retrieved.Name() != "mock-builder" {
		t.Errorf("expected builder name 'mock-builder', got %q", retrieved.Name())
	}

	_, err = GetBuilder("nonexistent")
	if err == nil {
		t.Error("expected error retrieving unregistered builder, got nil")
	} else if !strings.Contains(err.Error(), "unsupported package manager builder") {
		t.Errorf("unexpected error message: %v", err)
	}
}
