package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/moby/buildkit/frontend/gateway/client"

	"github.com/broadsage/doko/internal/config"
)

// AttachAll generates OCI image configurations and attaches them to the Solve result.
func AttachAll(ctx context.Context, spec *config.Spec, result *client.Result) error {
	// Set image metadata and history
	if err := setImageConfig(spec, result); err != nil {
		return fmt.Errorf("failed to configure image metadata: %w", err)
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
