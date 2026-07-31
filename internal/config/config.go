// Package config handles the parsing, validation, and structures of the doko.yaml spec file.
package config

//go:generate go run github.com/broadsage/doko/cmd/gen-schema

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	jsonschema "github.com/google/jsonschema-go/jsonschema"

	"github.com/broadsage/doko/internal/utils"
)

// Spec defines the top-level schema of doko.yaml.
// It is the single source of truth for every field the build system accepts.
type Spec struct {
	Name           string            `yaml:"name"      json:"name"`
	Image          string            `yaml:"image"     json:"image,omitempty"`
	Variant        string            `yaml:"variant"   json:"variant,omitempty"`
	Tags           []string          `yaml:"tags"      json:"tags,omitempty"`
	Platforms      []string          `yaml:"platforms" json:"platforms,omitempty"`
	Dates          map[string]string `yaml:"dates"     json:"dates,omitempty"`
	Vars           map[string]string `yaml:"vars"      json:"vars,omitempty"`
	Provider       string            `yaml:"-"         json:"provider,omitempty"`
	Base           string            `yaml:"-"         json:"base,omitempty"`
	Arch           string            `yaml:"arch"      json:"arch,omitempty"` // amd64, arm64 — defaults to amd64
	Contents       ContentsConfig    `yaml:"contents,omitempty"  json:"contents,omitempty"`
	Builds         []BuildSpec       `yaml:"builds"    json:"builds,omitempty"`
	Runtime        RuntimeConfig     `yaml:"runtime,omitempty"   json:"runtime,omitempty"`
	Accounts       AccountsConfig    `yaml:"accounts"  json:"accounts,omitempty"`
	Environment    map[string]string `yaml:"environment" json:"environment,omitempty"`
	Annotations    map[string]string `yaml:"annotations" json:"annotations,omitempty"`
	OSRelease      OSReleaseConfig   `yaml:"os-release" json:"os-release,omitempty"`
	StopSignal     string            `yaml:"stop-signal" json:"stop-signal,omitempty"`
	Artifacts      []ArtifactConfig  `yaml:"artifacts"   json:"artifacts,omitempty"`
	WorkDir        string            `yaml:"work-dir"    json:"work-dir,omitempty"`
	EntryPoint     []string          `yaml:"entrypoint"  json:"entrypoint,omitempty"`
	Cmd            []string          `yaml:"cmd"         json:"cmd,omitempty"`
	TimeoutSeconds int               `yaml:"timeout-seconds" json:"timeout-seconds,omitempty"`
}

// OSReleaseConfig defines fields for customizing /etc/os-release inside the image.
type OSReleaseConfig struct {
	Name            string `yaml:"name"             json:"name,omitempty"`
	ID              string `yaml:"id"               json:"id,omitempty"`
	VersionID       string `yaml:"version-id"       json:"version-id,omitempty"`
	VersionCodename string `yaml:"version-codename" json:"version-codename,omitempty"`
	PrettyName      string `yaml:"pretty-name"      json:"pretty-name,omitempty"`
	HomeURL         string `yaml:"home-url"         json:"home-url,omitempty"`
	BugReportURL    string `yaml:"bug-report-url"    json:"bug-report-url,omitempty"`
}

// AccountsConfig defines users, groups, and default run user.
type AccountsConfig struct {
	Root   bool    `yaml:"root"   json:"root,omitempty"`
	RunAs  string  `yaml:"run-as" json:"run-as,omitempty"`
	Users  []User  `yaml:"users"  json:"users,omitempty"`
	Groups []Group `yaml:"groups" json:"groups,omitempty"`
}

// User defines a user profile.
type User struct {
	Name string `yaml:"name" json:"name"`
	UID  int    `yaml:"uid"  json:"uid"`
	GID  int    `yaml:"gid"  json:"gid"`
}

// Group defines a user group.
type Group struct {
	Name    string   `yaml:"name"    json:"name"`
	GID     int      `yaml:"gid"     json:"gid"`
	Members []string `yaml:"members" json:"members,omitempty"`
}

// SubBuild represents a distinct build stage.
// Each sub-build can optionally override the top-level base image and provider,
// enabling patterns like compiling from source in a dev base and copying into runtime.
// BuildSpec represents a distinct build stage or custom package compilation pipeline.
type BuildSpec struct {
	Name         string         `yaml:"name"                  json:"name"`
	Version      string         `yaml:"version,omitempty"     json:"version,omitempty"`
	Epoch        int            `yaml:"epoch,omitempty"       json:"epoch,omitempty"`
	Description  string         `yaml:"description,omitempty" json:"description,omitempty"`
	URL          string         `yaml:"url,omitempty"         json:"url,omitempty"`
	License      string         `yaml:"license,omitempty"     json:"license,omitempty"`
	Dependencies []string       `yaml:"dependencies,omitempty" json:"dependencies,omitempty"`
	Base         string         `yaml:"base,omitempty"        json:"base,omitempty"`
	Provider     string         `yaml:"provider,omitempty"    json:"provider,omitempty"`
	WorkDir      string         `yaml:"work-dir,omitempty"    json:"work-dir,omitempty"`
	SourceDir    string         `yaml:"source-dir,omitempty"  json:"source-dir,omitempty"`
	Privileged   bool           `yaml:"privileged,omitempty"  json:"privileged,omitempty"`
	Contents     ContentsConfig `yaml:"contents,omitempty"    json:"contents,omitempty"`
	Outputs      []Output       `yaml:"outputs,omitempty"     json:"outputs,omitempty"`
	Pipeline     []PipelineStep `yaml:"pipeline,omitempty"    json:"pipeline,omitempty"`
}



// PipelineSecret represents a BuildKit secret mount option.
type PipelineSecret struct {
	ID     string `yaml:"id"     json:"id"`
	Target string `yaml:"target" json:"target"`
}

// PipelineStep represents a single build script runner or template step.
type PipelineStep struct {
	Name    string           `yaml:"name,omitempty" json:"name,omitempty"`
	Runs    string           `yaml:"runs,omitempty" json:"runs,omitempty"`
	Uses    string           `yaml:"uses,omitempty" json:"uses,omitempty"`
	With    map[string]any   `yaml:"with,omitempty" json:"with,omitempty"`
	SSH     bool             `yaml:"ssh,omitempty"  json:"ssh,omitempty"`
	Secrets []PipelineSecret `yaml:"secrets,omitempty" json:"secrets,omitempty"`
	Network string           `yaml:"network,omitempty" json:"network,omitempty"`
}

// ContentsConfig lists packages and local files to include.
type ContentsConfig struct {
	Packages       []string       `yaml:"packages"        json:"packages"`
	Repositories   []string       `yaml:"repositories"    json:"repositories,omitempty"`
	Keyring        []string       `yaml:"keyring"         json:"keyring,omitempty"`
	Paths          []PathConfig   `yaml:"paths"           json:"paths,omitempty"`
	Pipeline       []PipelineStep `yaml:"pipeline"        json:"pipeline,omitempty"`
	CACertificates []string       `yaml:"ca-certificates" json:"ca-certificates,omitempty"`
}

// PathConfig defines explicit directories or files to be created with strict permissions.
type PathConfig struct {
	Type   string `yaml:"type,omitempty" json:"type,omitempty"` // e.g. "directory" or "file"
	Path   string `yaml:"path"           json:"path"`
	UID    int    `yaml:"uid,omitempty"  json:"uid,omitempty"`
	GID    int    `yaml:"gid,omitempty"  json:"gid,omitempty"`
	Mode   string `yaml:"mode,omitempty" json:"mode,omitempty"`
	Source string `yaml:"source,omitempty" json:"source,omitempty"`
}

// Output defines a file or directory exported by a sub-build.
type Output struct {
	Source string `yaml:"source" json:"source"`
	Target string `yaml:"target" json:"target"`
	UID    int    `yaml:"uid,omitempty" json:"uid,omitempty"`
	GID    int    `yaml:"gid,omitempty" json:"gid,omitempty"`
}

// ArtifactConfig defines an external OCI image import.
type ArtifactConfig struct {
	Name     string   `yaml:"name" json:"name"`
	Includes []string `yaml:"includes" json:"includes"`
	UID      int      `yaml:"uid,omitempty" json:"uid,omitempty"`
	GID      int      `yaml:"gid,omitempty" json:"gid,omitempty"`
}

// RuntimeConfig dictates the target execution environment.
type RuntimeConfig struct {
	User  string            `yaml:"user"       json:"user,omitempty"`
	Ports []int             `yaml:"ports"       json:"ports,omitempty"`
	Env   map[string]string `yaml:"env"         json:"env,omitempty"`
}

// supportedProviders is the set of provider names LayerKit currently supports.
var supportedProviders = map[string]bool{
	"apk": true,
}



// Interpolate performs string substitution on the raw yaml content using the vars map.
func Interpolate(data []byte) ([]byte, error) {
	var temp struct {
		Vars map[string]string `yaml:"vars"`
	}
	// Decode once to extract the vars
	if err := yaml.Unmarshal(data, &temp); err != nil {
		return nil, fmt.Errorf("failed to parse YAML template variables: %w", err)
	}
	if len(temp.Vars) == 0 {
		return data, nil
	}
	content := string(data)
	subs := make(map[string]string)
	for k, v := range temp.Vars {
		subs[fmt.Sprintf("${%s}", k)] = v
	}
	content = utils.Substitute(content, subs)
	return []byte(content), nil
}

//go:embed schema.json
var schemaData []byte

// validateConfig validates the YAML configuration bytes against the embedded JSON Schema.
func validateConfig(data []byte) error {
	var parsedConfig any
	if err := yaml.Unmarshal(data, &parsedConfig); err != nil {
		return fmt.Errorf("failed to parse config YAML: %w", err)
	}
	if parsedConfig == nil {
		return nil
	}
	jsonBytes, err := json.Marshal(parsedConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal config to JSON: %w", err)
	}
	var jsonParsed any
	if err := json.Unmarshal(jsonBytes, &jsonParsed); err != nil {
		return fmt.Errorf("failed to parse config JSON: %w", err)
	}
	var schema jsonschema.Schema
	if err := json.Unmarshal(schemaData, &schema); err != nil {
		return fmt.Errorf("failed to unmarshal JSON Schema: %w", err)
	}
	resolved, err := schema.Resolve(nil)
	if err != nil {
		return fmt.Errorf("failed to resolve JSON Schema: %w", err)
	}
	if err := resolved.Validate(jsonParsed); err != nil {
		return fmt.Errorf("config schema validation failed: %w", err)
	}
	return nil
}

// Parse reads a LayerKit YAML configuration from a reader.
func Parse(r io.Reader) (*Spec, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read config data: %w", err)
	}
	interpolated, err := Interpolate(data)
	if err == nil {
		data = interpolated
	}
	if err := validateConfig(data); err != nil {
		return nil, err
	}
	var spec Spec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("failed to decode layerkit config: %w", err)
	}
	if err := spec.Validate(); err != nil {
		return nil, err
	}
	return &spec, nil
}

// ParseFile reads a LayerKit YAML configuration from a file path.
func ParseFile(path string) (*Spec, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %w", err)
	}
	defer func() { _ = f.Close() }()
	return Parse(f)
}

// ParseOSRelease reads and parses /etc/os-release to detect base OS and provider.
func ParseOSRelease() (string, string, error) {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return "", "", fmt.Errorf("failed to read /etc/os-release: %w", err)
	}
	lines := strings.Split(string(data), "\n")
	var id, version string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
		switch key {
		case "ID":
			id = strings.ToLower(val)
		case "VERSION_ID":
			version = val
		}
	}
	return id, version, nil
}

// Validate checks the Spec for correctness and applies defaults.
func (s *Spec) Validate() error {
	var errs []string

	if s.TimeoutSeconds <= 0 {
		s.TimeoutSeconds = 30
	}

	if s.Name == "" {
		errs = append(errs, "name is required")
	}

	// Auto-detect base and provider at runtime from /etc/os-release if not populated
	if s.Base == "" || s.Provider == "" {
		id, version, err := ParseOSRelease()
		if err != nil {
			// Fallback to alpine-3.20/apk on non-Linux development machines (like MacOS tests)
			s.Base = "alpine-3.20"
			s.Provider = "apk"
		} else {
			s.Provider = DetectProvider(id)
			if s.Provider != "" {
				baseName := id
				if id == "ubuntu" {
					baseName = "debian"
				}
				s.Base = baseName + "-" + version
			} else {
				// Fallback default
				s.Base = "alpine-3.20"
				s.Provider = "apk"
			}
		}
	}

	if !supportedProviders[s.Provider] {
		errs = append(errs, fmt.Sprintf("unsupported provider %q (supported: apk)", s.Provider))
	}



	// Validate sub-build providers
	for i, b := range s.Builds {
		if b.Provider != "" && !supportedProviders[b.Provider] {
			errs = append(errs, fmt.Sprintf("builds[%d].provider %q is not supported", i, b.Provider))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("layerkit config validation failed:\n  - %s", strings.Join(errs, "\n  - "))
	}
	return nil
}

// DetectProvider detects the package provider from an OS name or base image reference.
func DetectProvider(val string) string {
	valLower := strings.ToLower(val)
	switch {
	case strings.Contains(valLower, "alpine") || strings.Contains(valLower, "apk"):
		return "apk"
	default:
		return ""
	}
}
