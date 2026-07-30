package resolver

import (
	"context"
	"fmt"
	"time"
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

// Resolver is the interface that all package providers must implement.
// Implementations live in sub-packages (apk/).
type Resolver interface {
	// Name returns the human-readable name of the provider (e.g. "Alpine APK").
	Name() string

	// Resolve takes a list of requested package names and returns their full
	// metadata including version, license, download URL, and transitive dependencies.
	// It must be safe for concurrent use.
	Resolve(ctx context.Context, packages []string) ([]Package, error)
}

// Registry maps provider names to factory functions.
// New providers register themselves via RegisterProvider.
var registry = map[string]func(opts Options) (Resolver, error){}

// Options holds provider-agnostic configuration for resolver construction.
type Options struct {
	Arch           string            // Target architecture (e.g. "x86_64", "amd64")
	Repositories   []string          // Custom repository URLs (overrides defaults)
	CacheDir       string            // Local cache directory for index files
	LockedPackages map[string]string // Pin package name -> exact version from doko.lock
	CACerts        [][]byte          // Custom CA certificates PEM data
	Timeout        time.Duration     // Custom HTTP client timeout
	OSVersion      string            // Target OS Version (e.g., "3.23", "12")
}

// LockedPackage represents a pinned package version from a lockfile.
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

// RegisterProvider adds a new resolver factory to the global registry.
// This should be called from provider init() functions.
func RegisterProvider(name string, factory func(opts Options) (Resolver, error)) {
	registry[name] = factory
}

// NewResolver constructs a resolver for the given provider name and options.
func NewResolver(provider string, opts Options) (Resolver, error) {
	factory, ok := registry[provider]
	if !ok {
		supported := make([]string, 0, len(registry))
		for k := range registry {
			supported = append(supported, k)
		}
		return nil, fmt.Errorf("unsupported package provider %q (registered: %v)", provider, supported)
	}
	return factory(opts)
}
