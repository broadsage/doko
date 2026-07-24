// Package main is the entrypoint for the Doko BuildKit frontend CLI.
// It parses the build options, evaluates security gates, solves the LLB,
// and attaches SBOMs, SLSA provenance, and security profiles to the image.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/broadsage/doko/internal/config"
	layerkitllb "github.com/broadsage/doko/internal/llb"
	"github.com/broadsage/doko/internal/policy"
	"github.com/broadsage/doko/internal/provenance"
	"github.com/broadsage/doko/internal/resolver"
	"github.com/broadsage/doko/internal/sbom"
	"github.com/broadsage/doko/internal/security"
	"github.com/broadsage/doko/internal/vulnerability"

	// Import provider sub-packages for their init() side-effects (self-registration).
	_ "github.com/broadsage/doko/internal/resolver/apk"
	_ "github.com/broadsage/doko/internal/resolver/apt"
	_ "github.com/broadsage/doko/internal/resolver/dnf"

	"github.com/moby/buildkit/frontend/dockerui"
	"github.com/moby/buildkit/frontend/gateway/client"
	"github.com/moby/buildkit/frontend/gateway/grpcclient"
	"github.com/moby/buildkit/util/appcontext"
)

const defaultFilename = "doko.yaml"

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "version" || os.Args[1] == "--version" || os.Args[1] == "-v") {
		showVersion()
		os.Exit(0)
	}

	if err := grpcclient.RunFromEnvironment(appcontext.Context(), buildFunc); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "doko: fatal error: %v\n", err)
		os.Exit(1)
	}
}

// buildFunc is the main entry point called by BuildKit when processing a build request.
func buildFunc(ctx context.Context, c client.Client) (*client.Result, error) {
	// 1. Initialize the dockerui client to read the build definition file.
	bc, err := dockerui.NewClient(c)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize build client: %w", err)
	}

	// 2. Read the doko.yaml from the build context.
	src, err := bc.ReadEntrypoint(ctx, defaultFilename)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", defaultFilename, err)
	}

	// 3. Parse and validate the YAML configuration.
	spec, err := config.Parse(bytes.NewReader(src.Data))
	if err != nil {
		return nil, fmt.Errorf("failed to parse layerkit config: %w", err)
	}

	var vexData []byte
	if spec.Security.Policy.VEX.Path != "" {
		srcVex, err := bc.ReadEntrypoint(ctx, spec.Security.Policy.VEX.Path)
		if err != nil {
			return nil, fmt.Errorf("failed to read VEX exceptions file %s: %w", spec.Security.Policy.VEX.Path, err)
		}
		vexData = srcVex.Data
	}

	var lockData []byte
	if srcLock, err := bc.ReadEntrypoint(ctx, "doko.lock"); err == nil {
		lockData = srcLock.Data
	}

	var lockedPkgs map[string]string
	if len(lockData) > 0 {
		var lf resolver.Lockfile
		if err := yaml.Unmarshal(lockData, &lf); err == nil {
			lockedPkgs = make(map[string]string)
			for _, p := range lf.Packages {
				lockedPkgs[p.Name] = p.Version
			}
		}
	}

	var caCerts [][]byte
	for _, certPath := range spec.Contents.CACertificates {
		if strings.HasPrefix(certPath, "http://") || strings.HasPrefix(certPath, "https://") {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, certPath, nil)
			if err == nil {
				resp, err := http.DefaultClient.Do(req)
				if err == nil && resp.StatusCode == http.StatusOK {
					defer func() { _ = resp.Body.Close() }()
					if data, err := io.ReadAll(resp.Body); err == nil {
						caCerts = append(caCerts, data)
					}
				}
			}
		} else {
			if srcCert, err := bc.ReadEntrypoint(ctx, certPath); err == nil {
				caCerts = append(caCerts, srcCert.Data)
			}
		}
	}

	platforms := spec.Platforms
	if len(platforms) == 0 {
		// Fallback to single platform
		arch := spec.Arch
		if arch == "" {
			switch {
			case len(bc.TargetPlatforms) > 0 && bc.TargetPlatforms[0].Architecture != "":
				arch = bc.TargetPlatforms[0].Architecture
			case len(bc.BuildPlatforms) > 0 && bc.BuildPlatforms[0].Architecture != "":
				arch = bc.BuildPlatforms[0].Architecture
			default:
				arch = "amd64"
			}
		}
		platforms = []string{"linux/" + arch}
	}

	if len(platforms) == 1 {
		parts := strings.Split(platforms[0], "/")
		arch := parts[len(parts)-1]
		spec.Arch = arch
		return buildPlatformResult(ctx, c, spec, vexData, lockedPkgs, caCerts)
	}

	// Multi-platform manifest generation
	finalResult := client.NewResult()
	for _, p := range platforms {
		parts := strings.Split(p, "/")
		arch := parts[len(parts)-1]

		// Copy spec for this platform
		platformSpec := *spec
		platformSpec.Arch = arch

		subRes, err := buildPlatformResult(ctx, c, &platformSpec, vexData, lockedPkgs, caCerts)
		if err != nil {
			return nil, fmt.Errorf("failed to build platform %s: %w", p, err)
		}

		finalResult.AddRef(p, subRes.Ref)
	}
	return finalResult, nil
}

// buildPlatformResult compiles and solves the image filesystem and metadata for a given target architecture.
func buildPlatformResult(ctx context.Context, c client.Client, spec *config.Spec, vexData []byte, lockedPkgs map[string]string, caCerts [][]byte) (*client.Result, error) {
	// 1. Construct a resolver for the specified package provider.
	res, err := resolver.NewResolver(spec.Provider, resolver.Options{
		Arch:           spec.Arch,
		Repositories:   spec.Contents.Repositories,
		LockedPackages: lockedPkgs,
		CACerts:        caCerts,
	})
	if err != nil {
		return nil, err
	}

	// 2. Resolve packages using real index lookups.
	// Filter out negative package entries (e.g. "!telnet") — they are removals handled
	// by the LLB generator and have no entry in any package index.
	var resolvePkgs []string
	for _, pkg := range spec.Contents.Packages {
		if !strings.HasPrefix(pkg, "!") {
			resolvePkgs = append(resolvePkgs, pkg)
		}
	}
	resolvedPkgs, err := res.Resolve(ctx, resolvePkgs)
	if err != nil {
		return nil, fmt.Errorf("[doko] package resolution via %s failed: %w", res.Name(), err)
	}
	fmt.Fprintf(os.Stderr, "[doko] resolved %d packages via %s\n", len(resolvedPkgs), res.Name())

	// 3. Evaluate security policies including real OSV.dev CVE scanning (compile-time gate).
	gate := policy.NewGate(spec.Security.Policy.FailOnCVE, spec.Security.Policy.AllowedLicenses)
	if len(vexData) > 0 {
		var matcher *vulnerability.VEXMatcher
		var err error
		switch strings.ToLower(spec.Security.Policy.VEX.Format) {
		case "cyclonedx-vex":
			matcher, err = vulnerability.ParseCycloneDXVEX(vexData)
		default: // default to openvex
			matcher, err = vulnerability.ParseOpenVEX(vexData)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to parse VEX exception list: %w", err)
		}
		gate = gate.WithVEXMatcher(matcher)
	}

	// OSV.dev ecosystem names: use the standard ecosystem identifier only.
	// The package version already scopes the query to the right release.
	var ecosystem string
	switch spec.Provider {
	case "apk":
		ecosystem = "Alpine"
	case "apt":
		ecosystem = "Debian"
	case "dnf":
		ecosystem = "Fedora"
	}
	if err := gate.Evaluate(ctx, resolvedPkgs, ecosystem); err != nil {
		return nil, fmt.Errorf("[doko] build blocked by security policy: %w", err)
	}
	fmt.Fprintf(os.Stderr, "[doko] security policy evaluation passed\n")

	// 4. Generate LLB definition.
	fmt.Fprintf(os.Stderr, "[doko] step 4: generating LLB definition\n")
	gen := layerkitllb.NewGenerator(spec)
	def, err := gen.Generate(ctx)
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(os.Stderr, "[doko] step 4: LLB definition generated successfully\n")

	// 5. Solve the LLB definition through BuildKit.
	fmt.Fprintf(os.Stderr, "[doko] step 5: calling c.Solve\n")
	result, err := c.Solve(ctx, client.SolveRequest{
		Definition: def.ToPB(),
	})
	if err != nil {
		return nil, fmt.Errorf("solve failed: %w", err)
	}
	fmt.Fprintf(os.Stderr, "[doko] step 5: c.Solve succeeded\n")

	// 6. Generate and attach SBOMs.
	fmt.Fprintf(os.Stderr, "[doko] step 6: attaching SBOMs\n")
	if err := attachSBOMs(spec, resolvedPkgs, result); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "[doko] warning: failed to attach SBOMs: %v\n", err)
	}
	fmt.Fprintf(os.Stderr, "[doko] step 6 finished\n")

	// 7. Generate and attach SLSA-style build provenance.
	fmt.Fprintf(os.Stderr, "[doko] step 7: attaching provenance\n")
	if err := attachProvenance(spec, resolvedPkgs, result); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "[doko] warning: failed to attach provenance: %v\n", err)
	} else {
		_, _ = fmt.Fprintf(os.Stderr, "[doko] info: successfully generated and attached SLSA build provenance (doko.provenance-slsa)\n")
	}
	fmt.Fprintf(os.Stderr, "[doko] step 7 finished\n")

	// 8. Generate and attach security profiles as image annotations.
	fmt.Fprintf(os.Stderr, "[doko] step 8: attaching security profiles\n")
	if err := attachSecurityProfiles(spec, result); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "[doko] warning: failed to attach security profiles: %v\n", err)
	} else if len(spec.Security.Profiles) > 0 {
		_, _ = fmt.Fprintf(os.Stderr, "[doko] info: successfully generated and attached security profiles as annotations\n")
	}
	fmt.Fprintf(os.Stderr, "[doko] step 8 finished\n")

	// 9. Set image metadata (entrypoint, user, workdir, ports).
	fmt.Fprintf(os.Stderr, "[doko] step 9: setting image config\n")
	if err := setImageConfig(spec, result); err != nil {
		return nil, err
	}
	fmt.Fprintf(os.Stderr, "[doko] step 9 finished\n")
	fmt.Fprintf(os.Stderr, "[doko] buildPlatformResult successfully returning result\n")

	return result, nil
}

// attachSBOMs generates and embeds the SBOMs (SPDX, CycloneDX) in the build result meta.
func attachSBOMs(spec *config.Spec, packages []resolver.Package, result *client.Result) error {
	formats := spec.Security.SBOM.Formats
	if len(formats) == 0 {
		formats = []string{"spdx"}
	}

	for _, format := range formats {
		switch strings.ToLower(format) {
		case "spdx":
			doc, err := sbom.GenerateSPDX(spec, packages)
			if err != nil {
				return fmt.Errorf("failed to generate SPDX SBOM: %w", err)
			}
			data, err := sbom.MarshalSPDX(doc)
			if err != nil {
				return fmt.Errorf("failed to marshal SPDX SBOM: %w", err)
			}
			result.AddMeta("doko.sbom-spdx", data)
			_, _ = fmt.Fprintf(os.Stderr, "[doko] info: successfully generated and attached SPDX SBOM (doko.sbom-spdx)\n")

		case "cyclonedx":
			doc, err := sbom.GenerateCycloneDX(spec, packages)
			if err != nil {
				return fmt.Errorf("failed to generate CycloneDX SBOM: %w", err)
			}
			data, err := sbom.MarshalCycloneDX(doc)
			if err != nil {
				return fmt.Errorf("failed to marshal CycloneDX SBOM: %w", err)
			}
			result.AddMeta("doko.sbom-cyclonedx", data)
			_, _ = fmt.Fprintf(os.Stderr, "[doko] info: successfully generated and attached CycloneDX SBOM (doko.sbom-cyclonedx)\n")
		}
	}
	return nil
}

// attachProvenance generates and embeds SLSA-style build provenance.
func attachProvenance(spec *config.Spec, packages []resolver.Package, result *client.Result) error {
	att, err := provenance.Generate(spec, packages)
	if err != nil {
		return err
	}
	data, err := provenance.Marshal(att)
	if err != nil {
		return err
	}
	result.AddMeta("doko.provenance-slsa", data)
	return nil
}

// attachSecurityProfiles generates and embeds Seccomp and Landlock profiles.
func attachSecurityProfiles(spec *config.Spec, result *client.Result) error {
	for _, profileType := range spec.Security.Profiles {
		switch profileType {
		case "seccomp":
			profile, err := security.GenerateSeccompProfile(spec.Contents.Packages, spec.Runtime.Ports)
			if err != nil {
				return err
			}
			data, err := security.MarshalSeccomp(profile)
			if err != nil {
				return err
			}
			result.AddMeta("doko.seccomp-profile", data)

		case "landlock":
			var paths []string
			for _, p := range spec.Contents.Paths {
				paths = append(paths, p.Path)
			}
			llPolicy, err := security.GenerateLandlockPolicy(paths, true)
			if err != nil {
				return err
			}
			data, err := security.MarshalLandlock(llPolicy)
			if err != nil {
				return err
			}
			result.AddMeta("doko.landlock-policy", data)
		}
	}
	return nil
}

// setImageConfig writes the OCI image configuration metadata.
func setImageConfig(spec *config.Spec, result *client.Result) error {
	// Resolve the OCI USER: prefer runtime.user, fall back to accounts.run-as.
	ociUser := spec.Runtime.User
	if ociUser == "" {
		ociUser = spec.Accounts.RunAs
	}
	configMap := map[string]any{
		"Entrypoint": spec.EntryPoint,
		"Cmd":        spec.Cmd,
		"User":       ociUser,
		"WorkingDir": spec.WorkDir,
	}

	// Merge top-level environment and legacy runtime.env into OCI config environment list.
	envMap := make(map[string]string, len(spec.Environment)+len(spec.Runtime.Env))
	maps.Copy(envMap, spec.Environment)
	maps.Copy(envMap, spec.Runtime.Env)

	envList := make([]string, 0, len(envMap))
	for k, v := range envMap {
		envList = append(envList, fmt.Sprintf("%s=%s", k, v))
	}
	if len(envList) > 0 {
		configMap["Env"] = envList
	}

	if len(spec.Runtime.Ports) > 0 {
		exposedPorts := make(map[string]struct{})
		for _, port := range spec.Runtime.Ports {
			exposedPorts[fmt.Sprintf("%d/tcp", port)] = struct{}{}
		}
		configMap["ExposedPorts"] = exposedPorts
	}

	if spec.StopSignal != "" {
		configMap["StopSignal"] = spec.StopSignal
	}

	// Build OCI manifest annotations
	annotations := make(map[string]string, len(spec.Annotations))
	maps.Copy(annotations, spec.Annotations)

	// Auto-inject standard OCI metadata annotations
	if _, ok := annotations["org.opencontainers.image.title"]; !ok && spec.Name != "" {
		annotations["org.opencontainers.image.title"] = spec.Name
	}
	if _, ok := annotations["org.opencontainers.image.variant"]; !ok && spec.Variant != "" {
		annotations["org.opencontainers.image.variant"] = spec.Variant
	}
	if release, ok := spec.Dates["release"]; ok {
		if _, exists := annotations["org.opencontainers.image.created"]; !exists {
			annotations["org.opencontainers.image.created"] = release
		}
	}

	// Auto-inject custom branded com.broadsage.bsi.* metadata annotations
	if spec.Name != "" {
		annotations["com.broadsage.bsi.title"] = spec.Name
	}
	if spec.Variant != "" {
		annotations["com.broadsage.bsi.variant"] = spec.Variant
	}
	if spec.Base != "" {
		annotations["com.broadsage.bsi.distro"] = spec.Base
	}
	if spec.Provider != "" {
		annotations["com.broadsage.bsi.package-manager"] = spec.Provider
	}

	// Extract version from vars if present (e.g. NGINX_VERSION, PYTHON_VERSION)
	version := ""
	for k, v := range spec.Vars {
		if strings.Contains(strings.ToLower(k), "version") {
			version = v
			break
		}
	}
	if version != "" {
		annotations["com.broadsage.bsi.version"] = version
		annotations["org.opencontainers.image.version"] = version
	}

	// Dates
	if release, ok := spec.Dates["release"]; ok {
		annotations["com.broadsage.bsi.created"] = release
		annotations["com.broadsage.bsi.date.release"] = release
	}
	if eol, ok := spec.Dates["end-of-life"]; ok {
		annotations["com.broadsage.bsi.date.end-of-life"] = eol
	}

	// Compliance and Security
	if spec.Security.Policy.FailOnCVE != "" {
		annotations["com.broadsage.bsi.compliance"] = fmt.Sprintf("cis (fail-on-cve: %s)", spec.Security.Policy.FailOnCVE)
	} else {
		annotations["com.broadsage.bsi.compliance"] = "cis"
	}
	if spec.Security.Privileged {
		annotations["com.broadsage.bsi.privileged"] = "true"
	}

	arch := spec.Arch
	if arch == "" {
		arch = "amd64"
	}

	var imgConfig map[string]any
	if existingJSON, ok := result.Metadata["containerimage.config"]; ok {
		fmt.Fprintf(os.Stderr, "[doko] debug: existing containerimage.config: %s\n", string(existingJSON))
		if err := json.Unmarshal(existingJSON, &imgConfig); err != nil {
			imgConfig = make(map[string]any)
		}
	} else {
		fmt.Fprintf(os.Stderr, "[doko] debug: no existing containerimage.config in solve result\n")
		imgConfig = make(map[string]any)
	}

	imgConfig["os"] = "linux"
	imgConfig["architecture"] = arch

	// Populate top-level created timestamp (RFC3339) and author fields for OCI spec compliance
	if release, ok := spec.Dates["release"]; ok {
		if t, err := time.Parse("2006-01-02", release); err == nil {
			imgConfig["created"] = t.Format(time.RFC3339)
		} else {
			imgConfig["created"] = release
		}
	}
	if author, ok := spec.Annotations["org.opencontainers.image.authors"]; ok {
		imgConfig["author"] = author
	}

	innerConfig, ok := imgConfig["config"].(map[string]any)
	if !ok {
		innerConfig = make(map[string]any)
	}
	maps.Copy(innerConfig, configMap)
	imgConfig["config"] = innerConfig

	// Build the clean OCI history array explicitly in the exact order of execution
	var history []any

	// 1. Add base layout
	history = append(history, map[string]any{
		"created_by": "add root layout",
		"comment":    "buildkit.exporter.image.v0",
	})

	// 1.5. Add os-release customization
	if spec.OSRelease.Name != "" || spec.OSRelease.ID != "" {
		history = append(history, map[string]any{
			"created_by": "add metadata",
			"comment":    "buildkit.exporter.image.v0",
		})
	}

	// 2. Add keyring copies
	for i, keyURL := range spec.Contents.Keyring {
		filename := fmt.Sprintf("key-%d.pub", i)
		if parts := strings.Split(keyURL, "/"); len(parts) > 0 {
			filename = parts[len(parts)-1]
		}
		history = append(history, map[string]any{
			"created_by": fmt.Sprintf("copy keyring %s", filename),
			"comment":    "buildkit.exporter.image.v0",
		})
	}

	// 3. Add package installs
	if len(spec.Contents.Packages) > 0 {
		history = append(history, map[string]any{
			"created_by": fmt.Sprintf("install packages: %s", strings.Join(spec.Contents.Packages, ", ")),
			"comment":    "buildkit.exporter.image.v0",
		})
	}

	// 4. Add account creation
	if len(spec.Accounts.Users) > 0 || len(spec.Accounts.Groups) > 0 {
		history = append(history, map[string]any{
			"created_by": "setup user and group accounts",
			"comment":    "buildkit.exporter.image.v0",
		})
	}

	// 5. Add pipeline steps
	for _, step := range spec.Contents.Pipeline {
		name := step.Name
		if name == "" {
			name = "run custom command"
		}
		history = append(history, map[string]any{
			"created_by": fmt.Sprintf("pipeline: %s", name),
			"comment":    "buildkit.exporter.image.v0",
		})
	}

	// 6. Add file copies and outputs
	for _, p := range spec.Contents.Paths {
		history = append(history, map[string]any{
			"created_by": fmt.Sprintf("setup path %s", p.Path),
			"comment":    "buildkit.exporter.image.v0",
		})
	}
	for _, b := range spec.Builds {
		for _, o := range b.Outputs {
			history = append(history, map[string]any{
				"created_by": fmt.Sprintf("copy %s -> %s", o.Source, o.Target),
				"comment":    "buildkit.exporter.image.v0",
			})
		}
	}

	imgConfig["history"] = history

	configJSON, err := json.Marshal(imgConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal image config: %w", err)
	}
	result.AddMeta("containerimage.config", configJSON)

	// Attach annotations to the OCI manifest descriptor (standard best practice)
	if len(annotations) > 0 {
		annotationsJSON, err := json.Marshal(annotations)
		if err != nil {
			return fmt.Errorf("failed to marshal annotations: %w", err)
		}
		result.AddMeta("containerimage.annotations", annotationsJSON)
	}

	return nil
}
