// Package apt implements the Debian APT package resolver for LayerKit.
// It fetches and parses the Packages.gz index from Debian repositories
// to resolve package names into full metadata.
package apt

import (
	"bufio"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/broadsage/doko/internal/netutil"
	"github.com/broadsage/doko/internal/resolver"
)

const (
	providerName        = "apt"
	defaultDebianMirror = "https://deb.debian.org/debian"
	defaultSuite        = "trixie" // Debian 13
	defaultComponent    = "main"
	defaultDebianArch   = "amd64"
)

// defaultRepos returns the standard Packages.gz URLs for the given configuration.
func defaultRepos(mirror, suite, component, arch string) []string {
	return []string{
		fmt.Sprintf("%s/dists/%s/%s/binary-%s/Packages.gz", mirror, suite, component, arch),
	}
}

func init() {
	resolver.RegisterProvider(providerName, newAPTResolver)
}

// aptResolver resolves packages by parsing Debian Packages.gz index files.
type aptResolver struct {
	repos          []string
	arch           string
	mirror         string
	httpClient     *http.Client
	lockedPackages map[string]string

	indexOnce sync.Once
	index     map[string]*debEntry
	indexErr  error
}

// debEntry holds metadata parsed from a single Packages record.
type debEntry struct {
	Package      string
	Version      string
	Architecture string
	Section      string
	Filename     string
	Size         int64
	SHA256       string
	Description  string
	Depends      []string
	PreDepends   []string
}

func newAPTResolver(opts resolver.Options) (resolver.Resolver, error) {
	arch := opts.Arch
	if arch == "" {
		arch = defaultDebianArch
	}

	repos := opts.Repositories
	mirror := defaultDebianMirror
	if len(repos) == 0 {
		suite := defaultSuite
		if opts.OSVersion != "" {
			switch opts.OSVersion {
			case "12":
				suite = "bookworm"
			case "13":
				suite = "trixie"
			case "11":
				suite = "bullseye"
			default:
				suite = opts.OSVersion
			}
		}
		repos = defaultRepos(mirror, suite, defaultComponent, arch)
	}

	var httpClient *http.Client
	if len(opts.CACerts) > 0 {
		httpClient = netutil.NewHTTPClientWithCAs(opts.CACerts, opts.Timeout)
	} else {
		httpClient = netutil.NewHTTPClient(opts.Timeout)
	}

	return &aptResolver{
		repos:          repos,
		arch:           arch,
		mirror:         mirror,
		httpClient:     httpClient,
		lockedPackages: opts.LockedPackages,
	}, nil
}

func (r *aptResolver) Name() string { return "Debian APT" }

// Resolve fetches the Packages.gz (if not cached), then looks up every requested
// package and its transitive dependencies.
func (r *aptResolver) Resolve(ctx context.Context, packages []string) ([]resolver.Package, error) {
	r.indexOnce.Do(func() {
		r.index = make(map[string]*debEntry)
		for _, repo := range r.repos {
			if err := r.fetchIndex(ctx, repo); err != nil {
				r.indexErr = fmt.Errorf("failed to fetch Packages.gz from %s: %w", repo, err)
				return
			}
		}
	})
	if r.indexErr != nil {
		return nil, r.indexErr
	}

	seen := map[string]bool{}
	var resolved []resolver.Package
	var queue []string
	queue = append(queue, packages...)

	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]

		if seen[name] {
			continue
		}
		seen[name] = true

		entry, ok := r.index[name]
		if !ok {
			return nil, fmt.Errorf("package %q not found in Packages index", name)
		}

		allDeps := make([]string, len(entry.Depends), len(entry.Depends)+len(entry.PreDepends))
		copy(allDeps, entry.Depends)
		allDeps = append(allDeps, entry.PreDepends...)

		resolvedVersion := entry.Version
		if lockedVersion, exists := r.lockedPackages[entry.Package]; exists {
			resolvedVersion = lockedVersion
		}

		resolved = append(resolved, resolver.Package{
			Name:         entry.Package,
			Version:      resolvedVersion,
			Arch:         entry.Architecture,
			License:      "", // Debian Packages index does not include license info
			Description:  entry.Description,
			DownloadURL:  fmt.Sprintf("%s/%s", r.mirror, entry.Filename),
			Checksum:     entry.SHA256,
			Dependencies: allDeps,
			Size:         entry.Size,
		})

		// Enqueue transitive dependencies.
		for _, dep := range allDeps {
			depName := cleanAptDep(dep)
			if depName != "" && !seen[depName] {
				queue = append(queue, depName)
			}
		}
	}

	return resolved, nil
}

// fetchIndex downloads and parses a Packages.gz file from a single URL.
func (r *aptResolver) fetchIndex(ctx context.Context, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create HTTP request for %s: %w", url, err)
	}
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request to %s failed: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}

	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return fmt.Errorf("gzip decompress failed: %w", err)
	}
	defer func() { _ = gz.Close() }()

	return r.parseIndex(gz)
}

// parseIndex reads the decompressed Packages content and populates r.index.
// The Debian Packages format uses RFC 822-like stanzas separated by blank lines.
func (r *aptResolver) parseIndex(reader io.Reader) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var current *debEntry

	commitEntry := func() {
		if current != nil && current.Package != "" {
			if _, exists := r.index[current.Package]; !exists {
				r.index[current.Package] = current
			}
		}
	}

	for scanner.Scan() {
		line := scanner.Text()

		if line == "" {
			commitEntry()
			current = nil
			continue
		}

		// Continuation line (starts with space or tab) — skip for now.
		if line[0] == ' ' || line[0] == '\t' {
			continue
		}

		if current == nil {
			current = &debEntry{}
		}

		key, value, ok := strings.Cut(line, ": ")
		if !ok {
			continue
		}

		switch key {
		case "Package":
			current.Package = value
		case "Version":
			current.Version = value
		case "Architecture":
			current.Architecture = value
		case "Section":
			current.Section = value
		case "Filename":
			current.Filename = value
		case "Size":
			current.Size, _ = strconv.ParseInt(value, 10, 64)
		case "SHA256":
			current.SHA256 = value
		case "Description":
			current.Description = value
		case "Depends":
			current.Depends = parseDebDeps(value)
		case "Pre-Depends":
			current.PreDepends = parseDebDeps(value)
		}
	}
	commitEntry()

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to scan Packages index: %w", err)
	}
	return nil
}

// parseDebDeps splits a Debian dependency line into individual package names.
// Input format: "libc6 (>= 2.35), zlib1g (>= 1:1.2.0) | libdeflate0".
func parseDebDeps(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		// Handle alternatives (e.g. "zlib1g | libdeflate0") — take first.
		if altIdx := strings.Index(part, " | "); altIdx > 0 {
			part = part[:altIdx]
		}
		name := cleanAptDep(part)
		if name != "" {
			out = append(out, name)
		}
	}
	return out
}

// cleanAptDep extracts the package name from a dependency token.
// Strips version constraints like "(>= 2.35)" and arch qualifiers like ":any".
func cleanAptDep(dep string) string {
	dep = strings.TrimSpace(dep)
	// Handle alternatives — take first
	if altIdx := strings.Index(dep, " | "); altIdx > 0 {
		dep = dep[:altIdx]
	}
	// Strip version constraint "(>= X.Y)"
	if parenIdx := strings.Index(dep, " ("); parenIdx > 0 {
		dep = dep[:parenIdx]
	}
	// Strip arch qualifier ":any", ":amd64", etc.
	if colonIdx := strings.Index(dep, ":"); colonIdx > 0 {
		dep = dep[:colonIdx]
	}
	return strings.TrimSpace(dep)
}
