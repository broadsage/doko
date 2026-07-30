// Package llb translates a parsed LayerKit Spec into a BuildKit LLB definition.
package llb

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	buildkitllb "github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/solver/pb"
	ocispecs "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/broadsage/doko/internal/config"
)

// Generator translates a parsed LayerKit spec into a BuildKit LLB state.
type Generator struct {
	Spec *config.Spec
}

// NewGenerator creates a new LLB generator from a parsed spec.
func NewGenerator(spec *config.Spec) *Generator {
	return &Generator{Spec: spec}
}

// Generate builds the full LLB definition representing the target container image.
func (g *Generator) Generate(ctx context.Context) (*buildkitllb.Definition, error) {
	// 1. Select the base image from the provider and base fields.
	baseRef := g.resolveBaseImage()
	arch := g.Spec.Arch
	if arch == "" {
		arch = "amd64"
	}
	platform := ocispecs.Platform{
		OS:           "linux",
		Architecture: arch,
	}

	// Build sub-stages first
	subBuilds := make(map[string]buildkitllb.State)
	for _, b := range g.Spec.Builds {
		stageState, err := g.buildSubStage(platform, baseRef, b)
		if err != nil {
			return nil, fmt.Errorf("failed to build stage %s: %w", b.Name, err)
		}
		subBuilds[b.Name] = stageState
	}

	// 2. Bootstrap base OS layout starting from scratch to drop parent history
	base := g.bootstrapBaseLayout(platform, baseRef)

	// 2.15. Update CA certificates run (if any are configured)
	state := runUpdateCAsFor(base, g.Spec.Provider, g.Spec.Contents)

	// 2.2. Install packages via the appropriate package manager.
	var err error
	state, err = g.installPackages(state)
	if err != nil {
		return nil, fmt.Errorf("package installation failed: %w", err)
	}

	// 3. Add users and groups (individual layer)
	state = g.setupAccounts(state)

	// 4. Create directories (individual layer per directory)
	state = g.setupDirectories(state)

	// 5. Security hardening (individual layer)
	state = g.applyHardening(state)

	// 6. Run pipeline steps if any (already individual layers per step).
	state = g.runPipeline(state)

	// 7. Copy local files (individual layer per file)
	state = g.copyLocalFiles(state)

	// 8. Merge outputs from sub-builds (individual layer per sub-build)
	state = g.mergeSubBuildOutputs(state, subBuilds)

	// 9. Import artifacts from external images (individual layer per artifact)
	state = g.importArtifacts(state)

	// 10. Write metadata files: os-release, sysctl (individual layer)
	state = g.writeMetadataFiles(state)

	// 11. Apply runtime configuration (user, workdir, env, entrypoint).
	state = g.applyRuntime(state)

	// 12. Marshal the final LLB state into a serializable Definition.
	dt, err := state.Marshal(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal LLB state: %w", err)
	}

	return dt, nil
}

// buildSubStage builds a named build stage filesystem state.
// If the sub-build has its own Base/Provider, those override the top-level spec values.
func (g *Generator) buildSubStage(platform ocispecs.Platform, baseRef string, b config.SubBuild) (buildkitllb.State, error) {
	// Resolve per-stage base image and provider if specified
	stageBaseRef := baseRef
	provider := g.Spec.Provider
	if b.Base != "" {
		if b.Provider != "" {
			provider = b.Provider
		} else {
			// Auto-detect provider from sub-build base
			if p := config.DetectProvider(b.Base); p != "" {
				provider = p
			}
		}
		stageBaseRef = resolveBaseImageFor(provider, b.Base)
	} else if b.Provider != "" {
		provider = b.Provider
	}

	// Bootstrap per-stage base layout starting from scratch
	base := g.bootstrapBaseLayout(platform, stageBaseRef)

	// Update CA certificates run (if any are configured)
	state := runUpdateCAsFor(base, provider, b.Contents)

	state, err := g.installPackagesForContentsWithProvider(state, b.Contents, provider)
	if err != nil {
		return buildkitllb.State{}, err
	}
	state = g.setupPathsForContents(state, b.Contents)
	if b.WorkDir != "" {
		state = state.Dir(b.WorkDir)
	}
	state = g.runPipelineForContents(state, b.Contents, b.Privileged)
	return state, nil
}

// resolveBaseImage maps the provider + base fields to a container image reference.
func (g *Generator) resolveBaseImage() string {
	return resolveBaseImageFor(g.Spec.Provider, g.Spec.Base)
}

// installPackages runs the package manager commands for the primary contents config.
func (g *Generator) installPackages(base buildkitllb.State) (buildkitllb.State, error) {
	return g.installPackagesForContentsWithProvider(base, g.Spec.Contents, g.Spec.Provider)
}

func (g *Generator) installPackagesForContentsWithProvider(base buildkitllb.State, contents config.ContentsConfig, providerName string) (buildkitllb.State, error) {
	p, err := GetProvider(providerName)
	if err != nil {
		return base, err
	}

	var installs []string
	var removals []string
	for _, pkg := range contents.Packages {
		if remainder, ok := strings.CutPrefix(pkg, "!"); ok {
			removals = append(removals, remainder)
		} else {
			installs = append(installs, pkg)
		}
	}
	sort.Strings(installs)
	sort.Strings(removals)

	if len(installs) == 0 && len(removals) == 0 {
		return base, nil
	}

	script := p.InstallScript(installs, removals)
	if script == "" {
		return base, nil
	}

	var runOpts []buildkitllb.RunOption
	runOpts = append(runOpts, buildkitllb.Args([]string{"/bin/sh", "-c", strings.TrimSpace(script)}))
	runOpts = append(runOpts, buildkitllb.WithCustomName(fmt.Sprintf("provision packages via %s", providerName)))
	runOpts = append(runOpts, p.CacheMounts()...)

	// Mount keyring files dynamically (Read-Only)
	for i, keyURL := range contents.Keyring {
		filename := fmt.Sprintf("key-%d.pub", i)
		if parts := strings.Split(keyURL, "/"); len(parts) > 0 {
			filename = parts[len(parts)-1]
		}
		dest := p.KeyringDest(filename)
		if dest != "" {
			var srcState buildkitllb.State
			var srcPath string
			if strings.HasPrefix(keyURL, "http://") || strings.HasPrefix(keyURL, "https://") {
				srcState = buildkitllb.HTTP(keyURL)
				srcPath = "/" + filename
			} else {
				srcState = buildkitllb.Local("context", buildkitllb.SharedKeyHint(keyURL))
				srcPath = keyURL
			}
			runOpts = append(runOpts, buildkitllb.AddMount(dest, srcState, buildkitllb.SourcePath(srcPath), buildkitllb.Readonly))
		}
	}

	// Mount CA certificates dynamically (Read-Only)
	for i, certPath := range contents.CACertificates {
		filename := fmt.Sprintf("ca-%d.crt", i)
		if parts := strings.Split(certPath, "/"); len(parts) > 0 {
			filename = parts[len(parts)-1]
		}
		dest := p.CACertDest(filename)
		if dest != "" {
			var srcState buildkitllb.State
			var srcPath string
			if strings.HasPrefix(certPath, "http://") || strings.HasPrefix(certPath, "https://") {
				srcState = buildkitllb.HTTP(certPath)
				srcPath = "/" + filename
			} else {
				srcState = buildkitllb.Local("context", buildkitllb.SharedKeyHint(certPath))
				srcPath = certPath
			}
			runOpts = append(runOpts, buildkitllb.AddMount(dest, srcState, buildkitllb.SourcePath(srcPath), buildkitllb.Readonly))
		}
	}

	// 1. Run the package manager inside a helper stage
	helperState := base.Run(runOpts...).Root()

	// 2. Determine a clean custom name for the package layer
	layerName := fmt.Sprintf("build & install %d packages via %s", len(installs), providerName)

	// 3. Copy files natively from helperState back to base
	fileAction := buildkitllb.Copy(helperState, "/", "/")
	return base.File(fileAction, buildkitllb.WithCustomName(layerName)), nil
}

// runPipeline executes any custom pipeline shell commands for the primary spec.
func (g *Generator) runPipeline(state buildkitllb.State) buildkitllb.State {
	return g.runPipelineForContents(state, g.Spec.Contents, false)
}

// runPipelineForContents executes custom pipeline commands for a specific contents config.
func (g *Generator) runPipelineForContents(state buildkitllb.State, contents config.ContentsConfig, privileged bool) buildkitllb.State {
	for _, step := range contents.Pipeline {
		name := step.Name
		if name == "" {
			name = "run custom command"
		}
		opts := []buildkitllb.RunOption{
			buildkitllb.Args([]string{"/bin/sh", "-c", step.Runs}),
			buildkitllb.WithCustomName(fmt.Sprintf("pipeline: %s", name)),
		}
		if privileged {
			opts = append(opts, buildkitllb.With(buildkitllb.Security(buildkitllb.SecurityModeInsecure)))
		}
		if step.SSH {
			opts = append(opts, buildkitllb.AddSSHSocket(buildkitllb.SSHID("default"), buildkitllb.SSHSocketTarget("/run/ssh-agent.sock")))
			opts = append(opts, buildkitllb.AddEnv("SSH_AUTH_SOCK", "/run/ssh-agent.sock"))
		}
		for _, sec := range step.Secrets {
			opts = append(opts, buildkitllb.AddSecret(sec.Target, buildkitllb.SecretID(sec.ID)))
		}

		stepState := state
		if step.Network != "" {
			switch strings.ToLower(step.Network) {
			case "none":
				stepState = stepState.Network(pb.NetMode_NONE)
			case "host":
				stepState = stepState.Network(pb.NetMode_HOST)
			}
		}
		state = stepState.Run(opts...).Root()
	}
	return state
}

// setupPathsForContents runs commands to generate explicit paths for a specific contents config.
func (g *Generator) setupPathsForContents(state buildkitllb.State, contents config.ContentsConfig) buildkitllb.State {
	if len(contents.Paths) == 0 {
		return state
	}

	var commands []string
	for _, p := range contents.Paths {
		if p.Type == "directory" || p.Type == "dir" {
			commands = append(commands, fmt.Sprintf("mkdir -p %s", p.Path))
			commands = append(commands, fmt.Sprintf("chown -R %d:%d %s", p.UID, p.GID, p.Path))
			if p.Mode != "" {
				commands = append(commands, fmt.Sprintf("chmod %s %s", p.Mode, p.Path))
			}
		}
	}

	if len(commands) == 0 {
		return state
	}

	return state.Run(
		buildkitllb.Args([]string{"/bin/sh", "-c", strings.Join(commands, " && ")}),
		buildkitllb.WithCustomName("setup explicit paths"),
	).Root()
}

// applyRuntime sets user, workdir, and environment on the final state.
func (g *Generator) applyRuntime(state buildkitllb.State) buildkitllb.State {
	user := g.Spec.Runtime.User
	if user == "" && g.Spec.Accounts.RunAs != "" {
		user = g.Spec.Accounts.RunAs
	}
	if user != "" {
		state = state.User(user)
	}
	if g.Spec.WorkDir != "" {
		state = state.Dir(g.Spec.WorkDir)
	}
	for key, val := range g.Spec.Runtime.Env {
		state = state.AddEnv(key, val)
	}
	return state
}

// sanitizeBaseTag extracts a usable tag from the base field.
// e.g. "alpine-3.23" -> "3.23".
func sanitizeBaseTag(base string) string {
	if remainder, cut := strings.CutPrefix(base, "alpine-"); cut {
		tag := remainder
		for _, suffix := range []string{"-minimal", "-slim", "-base"} {
			tag = strings.TrimSuffix(tag, suffix)
		}
		return tag
	}
	return base
}

// resolveBaseImageFor maps a provider + base combo to a container image reference.
// Used for sub-builds that override the top-level base.
func resolveBaseImageFor(providerName, base string) string {
	p, err := GetProvider(providerName)
	if err != nil {
		return base
	}
	return p.ResolveBaseImage(base)
}

// bootstrapBaseLayout packages the base image rootfs into an archive at build time
// and extracts it onto a clean scratch state. This ensures that the base filesystem
// gets a completely unique layer hash, preventing Docker Desktop from showing a parent-child
// relationship to standard upstream images.
func (g *Generator) bootstrapBaseLayout(platform ocispecs.Platform, baseRef string) buildkitllb.State {
	baseImageState := buildkitllb.Image(baseRef, buildkitllb.Platform(platform), buildkitllb.WithMetaResolver(nil))

	// 1. Pack the base image filesystem into a tarball
	packRun := baseImageState.Run(
		buildkitllb.Args([]string{
			"/bin/sh", "-c",
			"tar --exclude=proc --exclude=sys --exclude=dev --exclude=tmp --exclude=run --exclude=mnt --exclude=media -cf /tmp/rootfs.tar -C / .",
		}),
		buildkitllb.WithCustomName("pack root layout archive"),
	)

	// 2. Copy the tarball onto a temporary scratch layer
	tarFile := buildkitllb.Scratch().File(
		buildkitllb.Copy(packRun.Root(), "/tmp/rootfs.tar", "/rootfs.tar"),
		buildkitllb.WithCustomName("export root layout archive"),
	)

	// 3. Extract the tarball onto a clean scratch state to generate a unique layer hash
	cleanScratch := buildkitllb.Scratch()
	unpackRun := baseImageState.Run(
		buildkitllb.Args([]string{
			"tar", "-xf", "/archive/rootfs.tar", "-C", "/rootfs",
		}),
		buildkitllb.AddMount("/archive", tarFile, buildkitllb.Readonly),
		buildkitllb.AddMount("/rootfs", cleanScratch),
		buildkitllb.WithCustomName("extract root layout archive"),
	)

	return buildkitllb.Scratch().File(
		buildkitllb.Copy(unpackRun.GetMount("/rootfs"), "/", "/"),
		buildkitllb.WithCustomName("add root layout"),
	)
}

// setupAccounts creates user accounts and groups as an individual layer.
func (g *Generator) setupAccounts(state buildkitllb.State) buildkitllb.State {
	var commands []string

	if len(g.Spec.Accounts.Users) > 0 || len(g.Spec.Accounts.Groups) > 0 || g.Spec.Accounts.RunAs != "" {
		if !g.Spec.Accounts.Root {
			commands = append(commands, "sed -i '/^root:/d' /etc/passwd /etc/group /etc/shadow 2>/dev/null || true")
		} else {
			commands = append(commands, "if ! grep -q '^root:' /etc/passwd; then echo 'root:x:0:0:root:/root:/bin/sh' >> /etc/passwd; fi")
			commands = append(commands, "if ! grep -q '^root:' /etc/group; then echo 'root:x:0:root' >> /etc/group; fi")
			commands = append(commands, "mkdir -p /root && chown 0:0 /root 2>/dev/null || true")
		}
		for _, group := range g.Spec.Accounts.Groups {
			commands = append(commands, fmt.Sprintf("echo %s:x:%d:%s >> /etc/group", group.Name, group.GID, strings.Join(group.Members, ",")))
		}
		for _, user := range g.Spec.Accounts.Users {
			commands = append(commands, fmt.Sprintf("echo %s:x:%d:%d:%s:/home/%s:/sbin/nologin >> /etc/passwd", user.Name, user.UID, user.GID, user.Name, user.Name))
			commands = append(commands, fmt.Sprintf("mkdir -p /home/%s && chown -R %d:%d /home/%s", user.Name, user.UID, user.GID, user.Name))
		}
	}

	if len(commands) == 0 {
		return state
	}

	// 1. Run the accounts creation script inside a helper stage
	helperState := state.Run(
		buildkitllb.Args([]string{"/bin/sh", "-c", strings.Join(commands, " && ")}),
		buildkitllb.WithCustomName("provision user and group accounts"),
	).Root()

	// 2. Copy back files natively to the main stage to create a clean, named layer
	fileAction := buildkitllb.Copy(helperState, "/etc/passwd", "/etc/passwd")
	fileAction = fileAction.Copy(helperState, "/etc/group", "/etc/group")
	fileAction = fileAction.Copy(helperState, "/etc/shadow", "/etc/shadow")
	fileAction = fileAction.Copy(helperState, "/root", "/root", &buildkitllb.CopyInfo{CreateDestPath: true})

	for _, user := range g.Spec.Accounts.Users {
		homeDir := fmt.Sprintf("/home/%s", user.Name)
		fileAction = fileAction.Copy(helperState, homeDir, homeDir, &buildkitllb.CopyInfo{CreateDestPath: true})
	}

	return state.File(fileAction, buildkitllb.WithCustomName("add users and groups"))
}

// setupDirectories creates each directory path as an individual layer using native BuildKit Mkdir.
func (g *Generator) setupDirectories(state buildkitllb.State) buildkitllb.State {
	for _, p := range g.Spec.Contents.Paths {
		if p.Type == "directory" || p.Type == "dir" {
			var mode os.FileMode = 0755
			if p.Mode != "" {
				var parsed uint32
				if _, err := fmt.Sscanf(p.Mode, "%o", &parsed); err == nil && parsed != 0 {
					mode = os.FileMode(parsed)
				}
			}
			opts := []buildkitllb.MkdirOption{
				buildkitllb.WithParents(true),
			}
			if p.UID != 0 || p.GID != 0 {
				opts = append(opts, buildkitllb.WithUser(fmt.Sprintf("%d:%d", p.UID, p.GID)))
			}
			state = state.File(
				buildkitllb.Mkdir(p.Path, mode, opts...),
				buildkitllb.WithCustomName(fmt.Sprintf("mkdir %s", p.Path)),
			)
		}
	}
	return state
}

// applyHardening removes the package manager (using native BuildKit Rm) and locks shell accounts as individual layers.
func (g *Generator) applyHardening(state buildkitllb.State) buildkitllb.State {
	hcfg := g.Spec.Security.Hardening

	// 1. Remove Package Manager natively
	if hcfg.RemovePackageManager {
		p, err := GetProvider(g.Spec.Provider)
		if err == nil {
			paths := p.RemovePaths()
			if len(paths) > 0 {
				var fileAction *buildkitllb.FileAction
				for _, path := range paths {
					if fileAction == nil {
						fileAction = buildkitllb.Rm(path, buildkitllb.WithAllowNotFound(true), buildkitllb.WithAllowWildcard(true))
					} else {
						fileAction = fileAction.Rm(path, buildkitllb.WithAllowNotFound(true), buildkitllb.WithAllowWildcard(true))
					}
				}
				if fileAction != nil {
					state = state.File(fileAction, buildkitllb.WithCustomName("remove package manager files"))
				}
			}
		}
	}

	// 2. Lock shell accounts (requires sed shell execution)
	if hcfg.LockShellAccounts {
		state = state.Run(
			buildkitllb.Args([]string{"/bin/sh", "-c", "sed -i -E '/^root:/! s|:(/bin/[a-z]*sh)$|:/sbin/nologin|g' /etc/passwd || true"}),
			buildkitllb.WithCustomName("lock shell accounts"),
		).Root()
	}

	return state
}

// copyLocalFiles copies each local file path as an individual layer.
func (g *Generator) copyLocalFiles(state buildkitllb.State) buildkitllb.State {
	for _, p := range g.Spec.Contents.Paths {
		if (p.Type == "file" || p.Type == "") && p.Source != "" {
			srcState := buildkitllb.Local("context", buildkitllb.SharedKeyHint(p.Source))
			var cOpt *buildkitllb.ChownOpt
			var mOpt *buildkitllb.ChmodOpt
			if p.UID != 0 || p.GID != 0 {
				cOpt = &buildkitllb.ChownOpt{
					User:  &buildkitllb.UserOpt{UID: p.UID},
					Group: &buildkitllb.UserOpt{UID: p.GID},
				}
			}
			if p.Mode != "" {
				var mode uint32
				_, _ = fmt.Sscanf(p.Mode, "%o", &mode)
				if mode != 0 {
					mOpt = &buildkitllb.ChmodOpt{Mode: os.FileMode(mode)}
				}
			}

			copyInfo := &buildkitllb.CopyInfo{
				CreateDestPath: true,
				ChownOpt:       cOpt,
				Mode:           mOpt,
			}

			state = state.File(
				buildkitllb.Copy(srcState, p.Source, p.Path, copyInfo),
				buildkitllb.WithCustomName(fmt.Sprintf("copy %s %s", p.Source, p.Path)),
			)
		}
	}
	return state
}

// mergeSubBuildOutputs copies outputs from each sub-build as an individual layer per sub-build.
func (g *Generator) mergeSubBuildOutputs(state buildkitllb.State, subBuilds map[string]buildkitllb.State) buildkitllb.State {
	for _, b := range g.Spec.Builds {
		stageState, ok := subBuilds[b.Name]
		if !ok || len(b.Outputs) == 0 {
			continue
		}

		var fileAction *buildkitllb.FileAction
		for _, out := range b.Outputs {
			var cOpt *buildkitllb.ChownOpt
			if out.UID != 0 || out.GID != 0 {
				cOpt = &buildkitllb.ChownOpt{
					User:  &buildkitllb.UserOpt{UID: out.UID},
					Group: &buildkitllb.UserOpt{UID: out.GID},
				}
			}
			copyInfo := &buildkitllb.CopyInfo{
				CreateDestPath: true,
				ChownOpt:       cOpt,
			}
			if fileAction == nil {
				fileAction = buildkitllb.Copy(stageState, out.Source, out.Target, copyInfo)
			} else {
				fileAction = fileAction.Copy(stageState, out.Source, out.Target, copyInfo)
			}
		}
		if fileAction != nil {
			state = state.File(fileAction, buildkitllb.WithCustomName(fmt.Sprintf("build %s", b.Name)))
		}
	}
	return state
}

// importArtifacts imports files from external OCI images as individual layers per artifact.
func (g *Generator) importArtifacts(state buildkitllb.State) buildkitllb.State {
	for _, artifact := range g.Spec.Artifacts {
		srcState := buildkitllb.Image(artifact.Name)
		var fileAction *buildkitllb.FileAction
		for _, inc := range artifact.Includes {
			var cOpt *buildkitllb.ChownOpt
			if artifact.UID != 0 || artifact.GID != 0 {
				cOpt = &buildkitllb.ChownOpt{
					User:  &buildkitllb.UserOpt{UID: artifact.UID},
					Group: &buildkitllb.UserOpt{UID: artifact.GID},
				}
			}
			copyInfo := &buildkitllb.CopyInfo{
				CreateDestPath: true,
				ChownOpt:       cOpt,
			}
			if fileAction == nil {
				fileAction = buildkitllb.Copy(srcState, inc, inc, copyInfo)
			} else {
				fileAction = fileAction.Copy(srcState, inc, inc, copyInfo)
			}
		}
		if fileAction != nil {
			state = state.File(fileAction, buildkitllb.WithCustomName(fmt.Sprintf("import artifact from %s", artifact.Name)))
		}
	}
	return state
}

// writeMetadataFiles writes os-release and sysctl config as an individual layer.
func (g *Generator) writeMetadataFiles(state buildkitllb.State) buildkitllb.State {
	var fileAction *buildkitllb.FileAction

	// 1. Write /etc/os-release
	cfg := g.Spec.OSRelease
	if cfg.Name != "" || cfg.ID != "" {
		var sb strings.Builder
		if cfg.Name != "" {
			_, _ = fmt.Fprintf(&sb, "NAME=%q\n", cfg.Name)
		}
		if cfg.ID != "" {
			_, _ = fmt.Fprintf(&sb, "ID=%q\n", cfg.ID)
		}
		if cfg.VersionID != "" {
			_, _ = fmt.Fprintf(&sb, "VERSION_ID=%q\n", cfg.VersionID)
		}
		if cfg.VersionCodename != "" {
			_, _ = fmt.Fprintf(&sb, "VERSION_CODENAME=%q\n", cfg.VersionCodename)
		}
		if cfg.PrettyName != "" {
			_, _ = fmt.Fprintf(&sb, "PRETTY_NAME=%q\n", cfg.PrettyName)
		}
		if cfg.HomeURL != "" {
			_, _ = fmt.Fprintf(&sb, "HOME_URL=%q\n", cfg.HomeURL)
		}
		if cfg.BugReportURL != "" {
			_, _ = fmt.Fprintf(&sb, "BUG_REPORT_URL=%q\n", cfg.BugReportURL)
		}

		fileAction = buildkitllb.Mkfile("/etc/os-release", 0o644, []byte(sb.String()))
	}

	// 2. Write sysctl config
	sysctl := g.Spec.Security.Hardening.Sysctl
	if len(sysctl) > 0 {
		var sb strings.Builder
		for k, v := range sysctl {
			fmt.Fprintf(&sb, "%s = %s\n", k, v)
		}
		if fileAction == nil {
			fileAction = buildkitllb.Mkfile("/etc/sysctl.d/99-doko.conf", 0o644, []byte(sb.String()))
		} else {
			fileAction = fileAction.Mkfile("/etc/sysctl.d/99-doko.conf", 0o644, []byte(sb.String()))
		}
	}

	if fileAction != nil {
		state = state.File(fileAction, buildkitllb.WithCustomName("add metadata"))
	}
	return state
}

// runUpdateCAsFor runs update-ca-certificates/update-ca-trust for a given provider with dynamic CA certs mounts.
func runUpdateCAsFor(state buildkitllb.State, providerName string, contents config.ContentsConfig) buildkitllb.State {
	p, err := GetProvider(providerName)
	if err != nil {
		return state
	}
	updateCmd := p.UpdateCACertCommand()
	if len(updateCmd) > 0 && len(contents.CACertificates) > 0 {
		var runOpts []buildkitllb.RunOption
		runOpts = append(runOpts, buildkitllb.Args(updateCmd))
		runOpts = append(runOpts, buildkitllb.WithCustomName("update-ca-certificates"))

		for i, certPath := range contents.CACertificates {
			filename := fmt.Sprintf("ca-%d.crt", i)
			if parts := strings.Split(certPath, "/"); len(parts) > 0 {
				filename = parts[len(parts)-1]
			}
			dest := p.CACertDest(filename)
			if dest != "" {
				var srcState buildkitllb.State
				var srcPath string
				if strings.HasPrefix(certPath, "http://") || strings.HasPrefix(certPath, "https://") {
					srcState = buildkitllb.HTTP(certPath)
					srcPath = "/" + filename
				} else {
					srcState = buildkitllb.Local("context", buildkitllb.SharedKeyHint(certPath))
					srcPath = certPath
				}
				runOpts = append(runOpts, buildkitllb.AddMount(dest, srcState, buildkitllb.SourcePath(srcPath), buildkitllb.Readonly))
			}
		}
		state = state.Run(runOpts...).Root()
	}
	return state
}
