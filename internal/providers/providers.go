package providers

import (
	"context"
	"fmt"
	"strings"
	"time"

	buildkitllb "github.com/moby/buildkit/client/llb"
)

// Package represents a resolved dependency from any package ecosystem.
type Package struct {
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Arch         string   `json:"arch"`
	License      string   `json:"license"`
	Description  string   `json:"description,omitempty"`
	DownloadURL  string   `json:"download_url"`
	Checksum     string   `json:"checksum,omitempty"`
	Dependencies []string `json:"dependencies,omitempty"`
	Size         int64    `json:"size,omitempty"`
}

// Resolver is the interface that all package providers must implement for dependency parsing.
type Resolver interface {
	Name() string
	Resolve(ctx context.Context, packages []string) ([]Package, error)
}

// PackageManager defines the interface for pluggable package managers to construct LLB graphs.
type PackageManager interface {
	Name() string
	ResolveBaseImage(base string) string
	KeyringDest(filename string) string
	CACertDest(filename string) string
	UpdateCACertCommand() []string
	InstallScript(installs, removals []string) string
	CacheMounts() []buildkitllb.RunOption
	RemovePaths() []string
}

// Options sets configuration for constructing resolvers.
type Options struct {
	Arch           string
	Repositories   []string
	LockedPackages map[string]string
	CACerts        [][]byte
	Timeout        time.Duration
	OSVersion      string
}

// LockedPackage defines a single package record in lockfile.
type LockedPackage struct {
	Name    string `yaml:"name" json:"name"`
	Version string `yaml:"version" json:"version"`
}

// Lockfile defines the schema of the doko.lock file.
type Lockfile struct {
	Provider string          `yaml:"provider" json:"provider"`
	Arch     string          `yaml:"arch"     json:"arch"`
	Packages []LockedPackage `yaml:"packages" json:"packages"`
}

var (
	resolverRegistry = map[string]func(opts Options) (Resolver, error){}
	builderRegistry  = map[string]PackageManager{}
)

// RegisterResolver adds a new resolver factory to the global registry.
func RegisterResolver(name string, factory func(opts Options) (Resolver, error)) {
	resolverRegistry[strings.ToLower(name)] = factory
}

// NewResolver constructs a resolver for the given provider name and options.
func NewResolver(provider string, opts Options) (Resolver, error) {
	factory, ok := resolverRegistry[strings.ToLower(provider)]
	if !ok {
		supported := make([]string, 0, len(resolverRegistry))
		for k := range resolverRegistry {
			supported = append(supported, k)
		}
		return nil, fmt.Errorf("unsupported package provider %q (registered resolvers: %v)", provider, supported)
	}
	return factory(opts)
}

// RegisterBuilder registers a new package manager LLB builder.
func RegisterBuilder(name string, provider PackageManager) {
	builderRegistry[strings.ToLower(name)] = provider
}

// GetBuilder retrieves a registered PackageManager builder by name.
func GetBuilder(name string) (PackageManager, error) {
	p, ok := builderRegistry[strings.ToLower(name)]
	if !ok {
		supported := make([]string, 0, len(builderRegistry))
		for k := range builderRegistry {
			supported = append(supported, k)
		}
		return nil, fmt.Errorf("unsupported package manager builder %q (registered builders: %v)", name, supported)
	}
	return p, nil
}
