package llb

import (
	"fmt"
	"strings"

	buildkitllb "github.com/moby/buildkit/client/llb"
)

func init() {
	RegisterProvider("apt", &aptProvider{})
}

// aptProvider is the concrete implementation of PackageManager for APT.
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
