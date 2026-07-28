package llb

import (
	"fmt"
	"strings"

	buildkitllb "github.com/moby/buildkit/client/llb"
)

func init() {
	RegisterProvider("dnf", &dnfProvider{})
}

// dnfProvider is the concrete implementation of PackageManager for DNF.
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
