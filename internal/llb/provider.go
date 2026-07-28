package llb

import (
	"fmt"
	"strings"

	buildkitllb "github.com/moby/buildkit/client/llb"
)

// PackageManager defines the interface for pluggable package managers.
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

var providers = map[string]PackageManager{}

// RegisterProvider registers a new package manager provider.
func RegisterProvider(name string, provider PackageManager) {
	providers[strings.ToLower(name)] = provider
}

// GetProvider retrieves a registered PackageManager by name.
func GetProvider(name string) (PackageManager, error) {
	p, ok := providers[strings.ToLower(name)]
	if !ok {
		return nil, fmt.Errorf("unsupported package provider: %s", name)
	}
	return p, nil
}
