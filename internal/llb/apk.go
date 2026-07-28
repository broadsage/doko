package llb

import (
	"fmt"
	"strings"

	buildkitllb "github.com/moby/buildkit/client/llb"
)

func init() {
	RegisterProvider("apk", &apkProvider{})
}

// apkProvider is the concrete implementation of PackageManager for APK.
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
