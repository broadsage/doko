package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"strings"
	"time"

	"github.com/moby/buildkit/frontend/gateway/client"

	"github.com/broadsage/doko/internal/config"
	"github.com/broadsage/doko/internal/provenance"
	"github.com/broadsage/doko/internal/resolver"
	"github.com/broadsage/doko/internal/sbom"
	"github.com/broadsage/doko/internal/security"
)

// AttachAll generates SBOMs, SLSA provenance, sandbox profiles, and OCI image configurations,
// attaching all of them to the Solve result.
func AttachAll(ctx context.Context, spec *config.Spec, resolvedPkgs []resolver.Package, result *client.Result) error {
	// 1. Generate and attach SBOMs
	if err := attachSBOMs(spec, resolvedPkgs, result); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "[doko] warning: failed to attach SBOMs: %v\n", err)
	}

	// 2. Generate and attach SLSA-style build provenance
	if err := attachProvenance(spec, resolvedPkgs, result); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "[doko] warning: failed to attach provenance: %v\n", err)
	}

	// 3. Generate and attach security profiles
	if err := attachSecurityProfiles(spec, result); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "[doko] warning: failed to attach security profiles: %v\n", err)
	}

	// 4. Set image metadata and history
	if err := setImageConfig(spec, result); err != nil {
		return fmt.Errorf("failed to configure image metadata: %w", err)
	}

	return nil
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
		return fmt.Errorf("failed to generate provenance: %w", err)
	}
	data, err := provenance.Marshal(att)
	if err != nil {
		return fmt.Errorf("failed to marshal provenance: %w", err)
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
				return fmt.Errorf("failed to generate seccomp profile: %w", err)
			}
			data, err := security.MarshalSeccomp(profile)
			if err != nil {
				return fmt.Errorf("failed to marshal seccomp profile: %w", err)
			}
			result.AddMeta("doko.seccomp-profile", data)

		case "landlock":
			var paths []string
			for _, p := range spec.Contents.Paths {
				paths = append(paths, p.Path)
			}
			llPolicy, err := security.GenerateLandlockPolicy(paths, true)
			if err != nil {
				return fmt.Errorf("failed to generate landlock policy: %w", err)
			}
			data, err := security.MarshalLandlock(llPolicy)
			if err != nil {
				return fmt.Errorf("failed to marshal landlock policy: %w", err)
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
		if err := json.Unmarshal(existingJSON, &imgConfig); err != nil {
			imgConfig = make(map[string]any)
		}
	} else {
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

	// NOTE: History is intentionally NOT set here. BuildKit's exporter auto-generates
	// history entries from the WithCustomName() labels in the LLB graph. Each LLB
	// state transition becomes a physical layer, and BuildKit creates a corresponding
	// history entry with the correct size. Manually overriding the history array
	// would create orphaned entries that show 0 B in docker history.

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
