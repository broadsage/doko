// Package config handles the parsing, validation, and structures of the doko.yaml spec file.
package config

import (
	"fmt"
	"io"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
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
	Security       SecurityConfig    `yaml:"security"  json:"security"`
	Contents       ContentsConfig    `yaml:"contents"  json:"contents"`
	Builds         []SubBuild        `yaml:"builds"    json:"builds,omitempty"`
	Runtime        RuntimeConfig     `yaml:"runtime"   json:"runtime"`
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
type SubBuild struct {
	Name       string         `yaml:"name"       json:"name"`
	Base       string         `yaml:"base"       json:"base,omitempty"`
	Provider   string         `yaml:"provider"   json:"provider,omitempty"`
	WorkDir    string         `yaml:"work-dir"   json:"work-dir,omitempty"`
	Privileged bool           `yaml:"privileged" json:"privileged,omitempty"`
	Contents   ContentsConfig `yaml:"contents"   json:"contents"`
	Outputs    []Output       `yaml:"outputs"    json:"outputs,omitempty"`
}

// HardeningConfig defines declarative OS hardening settings.
type HardeningConfig struct {
	RemovePackageManager bool              `yaml:"remove-package-manager" json:"remove-package-manager,omitempty"`
	LockShellAccounts    bool              `yaml:"lock-shell-accounts"    json:"lock-shell-accounts,omitempty"`
	Sysctl               map[string]string `yaml:"sysctl"                 json:"sysctl,omitempty"`
	ReadOnlyRootFS       bool              `yaml:"read-only-rootfs"       json:"read-only-rootfs,omitempty"`
}

// SBOMConfig defines which SBOM metadata formats should be generated.
type SBOMConfig struct {
	Formats []string `yaml:"formats" json:"formats,omitempty"`
}

// SecurityConfig defines policies and run-time sandbox profile requests.
type SecurityConfig struct {
	Policy     PolicyConfig    `yaml:"policy"     json:"policy"`
	Profiles   []string        `yaml:"profiles"   json:"profiles,omitempty"`
	Privileged bool            `yaml:"privileged" json:"privileged,omitempty"`
	Hardening  HardeningConfig `yaml:"hardening"  json:"hardening,omitempty"`
	SBOM       SBOMConfig      `yaml:"sbom"       json:"sbom,omitempty"`
}

// VEXConfig holds vulnerability exception list configuration.
type VEXConfig struct {
	Format string `yaml:"format" json:"format,omitempty"`
	Path   string `yaml:"path"   json:"path,omitempty"`
}

// PolicyConfig holds the rules evaluated at build-time.
type PolicyConfig struct {
	FailOnCVE       string    `yaml:"fail-on-cve"       json:"fail-on-cve,omitempty"`
	AllowedLicenses []string  `yaml:"allowed-licenses"  json:"allowed-licenses,omitempty"`
	CustomRego      string    `yaml:"custom-rego"       json:"custom-rego,omitempty"`
	VEX             VEXConfig `yaml:"vex"               json:"vex,omitempty"`
}

// PipelineStep represents a single build script runner.
type PipelineStep struct {
	Name string `yaml:"name,omitempty" json:"name,omitempty"`
	Runs string `yaml:"runs"           json:"runs"`
	SSH  bool   `yaml:"ssh,omitempty"  json:"ssh,omitempty"`
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
	"apt": true,
	"dnf": true,
}

// supportedProfiles is the set of security profile types LayerKit can generate.
var supportedProfiles = map[string]bool{
	"seccomp":  true,
	"landlock": true,
}

// supportedCVELevels is the ordered set of allowed fail-on-cve values.
var supportedCVELevels = map[string]bool{
	"critical": true,
	"high":     true,
	"medium":   true,
	"low":      true,
	"none":     true,
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
	for k, v := range temp.Vars {
		placeholder := fmt.Sprintf("${%s}", k)
		content = strings.ReplaceAll(content, placeholder, v)
	}
	return []byte(content), nil
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
		errs = append(errs, fmt.Sprintf("unsupported provider %q (supported: apk, apt, dnf)", s.Provider))
	}

	// Validate security profiles
	for _, p := range s.Security.Profiles {
		if !supportedProfiles[p] {
			errs = append(errs, fmt.Sprintf("unsupported security profile %q (supported: seccomp, landlock)", p))
		}
	}

	// Validate fail-on-cve level
	if s.Security.Policy.FailOnCVE != "" {
		level := strings.ToLower(s.Security.Policy.FailOnCVE)
		if !supportedCVELevels[level] {
			errs = append(errs, fmt.Sprintf("unsupported fail-on-cve level %q (supported: critical, high, medium, low, none)", s.Security.Policy.FailOnCVE))
		}
		s.Security.Policy.FailOnCVE = level
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
	case strings.Contains(valLower, "debian") || strings.Contains(valLower, "ubuntu") || strings.Contains(valLower, "apt"):
		return "apt"
	case strings.Contains(valLower, "fedora") || strings.Contains(valLower, "centos") || strings.Contains(valLower, "rhel") || strings.Contains(valLower, "dnf"):
		return "dnf"
	default:
		return ""
	}
}
