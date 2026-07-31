package apk

import (
	"context"
	"fmt"
	"strings"

	buildkitllb "github.com/moby/buildkit/client/llb"

	"github.com/broadsage/doko/internal/config"
	"github.com/broadsage/doko/internal/providers"
	apkbuilder "github.com/broadsage/doko/internal/providers/apk/builder"
)

func init() {
	providers.RegisterBuilder("apk", &apkProvider{})
}

// apkProvider is the concrete implementation of PackageManager for APK.
type apkProvider struct{}

func (p *apkProvider) Name() string { return "apk" }
func (p *apkProvider) ResolveBaseImage(base string) string {
	return fmt.Sprintf("alpine:%s", SanitizeVersion(base))
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

func (p *apkProvider) BuildPackage(ctx context.Context, spec *config.BuildSpec, sourceState buildkitllb.State, resolver buildkitllb.ImageMetaResolver, opts ...buildkitllb.ConstraintsOpt) (buildkitllb.State, error) {
	base := spec.Base
	if base == "" {
		base = "alpine-3.23"
	}
	workerBaseImage := p.ResolveBaseImage(base)
	state, err := apkbuilder.BuildAPK(ctx, spec, sourceState, workerBaseImage, resolver, opts...)
	if err != nil {
		return state, fmt.Errorf("build APK: %w", err)
	}
	return state, nil
}

func (p *apkProvider) AssemblePackage(dataDir, outPath string, spec *config.BuildSpec, arch string) (string, error) {
	if err := apkbuilder.AssembleAPK(dataDir, outPath, spec, arch); err != nil {
		return "", fmt.Errorf("assemble APK: %w", err)
	}
	epoch := "0"
	if spec.Epoch > 0 {
		epoch = fmt.Sprintf("%d", spec.Epoch)
	}
	apkName := fmt.Sprintf("%s-%s-r%s.apk", strings.ToLower(spec.Name), spec.Version, epoch)
	return apkName, nil
}
