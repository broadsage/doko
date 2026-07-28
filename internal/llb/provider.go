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

var providers = map[string]PackageManager{
	"apk": &apkProvider{},
	"apt": &aptProvider{},
	"dnf": &dnfProvider{},
}

// GetProvider retrieves a registered PackageManager by name.
func GetProvider(name string) (PackageManager, error) {
	p, ok := providers[strings.ToLower(name)]
	if !ok {
		return nil, fmt.Errorf("unsupported package provider: %s", name)
	}
	return p, nil
}

// Concrete APK Provider
type apkProvider struct{}

func (p *apkProvider) Name() string { return "apk" }
func (p *apkProvider) ResolveBaseImage(base string) string {
	return fmt.Sprintf("alpine:%s", sanitizeBaseTag(base))
}
func (p *apkProvider) KeyringDest(filename string) string {
	return "/etc/apk/keys/" + filename
}
func (p *apkProvider) CACertDest(filename string) string {
	return "/usr/local/share/ca-certificates/" + filename
}
func (p *apkProvider) UpdateCACertCommand() []string {
	return []string{"update-ca-certificates"}
}
func (p *apkProvider) InstallScript(installs, removals []string) string {
	var script string
	if len(installs) > 0 {
		script += "apk add --no-cache " + strings.Join(installs, " ") + "\n"
	}
	if len(removals) > 0 {
		script += "apk del --no-cache " + strings.Join(removals, " ") + " || true\n"
	}
	return script
}
func (p *apkProvider) CacheMounts() []buildkitllb.RunOption {
	return []buildkitllb.RunOption{
		buildkitllb.AddMount("/var/cache/apk", buildkitllb.Scratch(), buildkitllb.AsPersistentCacheDir("doko-apk-cache", buildkitllb.CacheMountShared)),
	}
}
func (p *apkProvider) RemovePaths() []string {
	return []string{"/sbin/apk", "/lib/apk", "/var/cache/apk", "/etc/apk"}
}

// Concrete APT Provider
type aptProvider struct{}

func (p *aptProvider) Name() string { return "apt" }
func (p *aptProvider) ResolveBaseImage(base string) string {
	return fmt.Sprintf("debian:%s-slim", sanitizeBaseTag(base))
}
func (p *aptProvider) KeyringDest(filename string) string {
	return "/etc/apt/trusted.gpg.d/" + filename
}
func (p *aptProvider) CACertDest(filename string) string {
	return "/usr/local/share/ca-certificates/" + filename
}
func (p *aptProvider) UpdateCACertCommand() []string {
	return []string{"update-ca-certificates"}
}
func (p *aptProvider) InstallScript(installs, removals []string) string {
	var script string
	if len(installs) > 0 {
		script += "apt-get update && apt-get install -y --no-install-recommends " + strings.Join(installs, " ") + " && rm -rf /var/lib/apt/lists/*\n"
	}
	if len(removals) > 0 {
		script += "apt-get purge -y " + strings.Join(removals, " ") + " && apt-get autoremove -y\n"
	}
	return script
}
func (p *aptProvider) CacheMounts() []buildkitllb.RunOption {
	return []buildkitllb.RunOption{
		buildkitllb.AddMount("/var/cache/apt", buildkitllb.Scratch(), buildkitllb.AsPersistentCacheDir("doko-apt-cache", buildkitllb.CacheMountShared)),
		buildkitllb.AddMount("/var/lib/apt/lists", buildkitllb.Scratch(), buildkitllb.AsPersistentCacheDir("doko-apt-lists", buildkitllb.CacheMountShared)),
	}
}
func (p *aptProvider) RemovePaths() []string {
	return []string{"/usr/bin/apt*", "/usr/bin/dpkg*", "/var/lib/apt", "/var/lib/dpkg", "/etc/apt"}
}

// Concrete DNF Provider
type dnfProvider struct{}

func (p *dnfProvider) Name() string { return "dnf" }
func (p *dnfProvider) ResolveBaseImage(base string) string {
	return fmt.Sprintf("fedora:%s", sanitizeBaseTag(base))
}
func (p *dnfProvider) KeyringDest(filename string) string {
	return "/etc/pki/rpm-gpg/" + filename
}
func (p *dnfProvider) CACertDest(filename string) string {
	return "/etc/pki/ca-trust/source/anchors/" + filename
}
func (p *dnfProvider) UpdateCACertCommand() []string {
	return []string{"update-ca-trust"}
}
func (p *dnfProvider) InstallScript(installs, removals []string) string {
	var script string
	if len(installs) > 0 {
		script += "dnf install -y --setopt=install_weak_deps=False " + strings.Join(installs, " ") + " && dnf clean all\n"
	}
	if len(removals) > 0 {
		script += "dnf remove -y " + strings.Join(removals, " ") + " && dnf clean all\n"
	}
	return script
}
func (p *dnfProvider) CacheMounts() []buildkitllb.RunOption {
	return []buildkitllb.RunOption{
		buildkitllb.AddMount("/var/cache/dnf", buildkitllb.Scratch(), buildkitllb.AsPersistentCacheDir("doko-dnf-cache", buildkitllb.CacheMountShared)),
	}
}
func (p *dnfProvider) RemovePaths() []string {
	return []string{"/usr/bin/dnf*", "/usr/bin/rpm*", "/var/lib/dnf", "/var/lib/rpm", "/etc/dnf"}
}
