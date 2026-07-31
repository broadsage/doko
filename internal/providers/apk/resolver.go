// Package apk implements the Alpine APK package resolver for LayerKit.
// It fetches and parses the APKINDEX.tar.gz from Alpine Linux repositories
// to resolve package names into full metadata.
package apk

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/frontend/gateway/client"

	"github.com/broadsage/doko/internal/providers"
)

const (
	providerName          = "apk"
	defaultAlpineVersion  = "v3.23"
	defaultAlpineArch     = "x86_64"
	defaultAlpineRepoBase = "https://dl-cdn.alpinelinux.org/alpine"
)

// defaultRepos returns the standard main repository for Alpine.
func defaultRepos(version, arch string) []string {
	return []string{
		fmt.Sprintf("%s/%s/main/%s", defaultAlpineRepoBase, version, arch),
	}
}

func init() {
	providers.RegisterResolver(providerName, PKGResolver)
}

// apkResolver resolves packages by parsing Alpine APKINDEX files.
type apkResolver struct {
	repos          []string
	arch           string
	client         client.Client
	lockedPackages map[string]string

	indexOnce sync.Once
	index     map[string]*apkEntry
	indexErr  error
}

// apkEntry holds metadata parsed from a single APKINDEX record.
type apkEntry struct {
	Name         string
	Version      string
	Arch         string
	License      string
	Description  string
	Size         int64
	Dependencies []string
	Provides     []string
	Checksum     string
}

// SanitizeVersion extracts the stable major/minor Alpine version (e.g., "3.23" from "alpine-3.23.5-minimal").
func SanitizeVersion(base string) string {
	version := base
	if remainder, cut := strings.CutPrefix(version, "alpine-"); cut {
		version = remainder
	}
	for _, suffix := range []string{"-minimal", "-slim", "-base"} {
		version = strings.TrimSuffix(version, suffix)
	}
	// Truncate Alpine point releases (e.g. 3.23.5 -> 3.23)
	parts := strings.Split(version, ".")
	if len(parts) > 2 {
		version = parts[0] + "." + parts[1]
	}
	return version
}

func PKGResolver(opts providers.Options) (providers.Resolver, error) {
	arch := opts.Arch
	switch arch {
	case "", "amd64":
		arch = defaultAlpineArch
	case "arm64":
		arch = "aarch64"
	}

	version := SanitizeVersion(opts.OSVersion)
	if version == "" {
		version = defaultAlpineVersion
	}
	if !strings.HasPrefix(version, "v") && version != "edge" {
		version = "v" + version
	}

	repos := opts.Repositories
	if len(repos) == 0 {
		repos = defaultRepos(version, arch)
	}

	if opts.Client == nil {
		return nil, fmt.Errorf("BuildKit gateway client is not initialized in resolver options")
	}

	return &apkResolver{
		repos:          repos,
		arch:           arch,
		client:         opts.Client,
		lockedPackages: opts.LockedPackages,
	}, nil
}

func (r *apkResolver) Name() string { return "Alpine APK" }

// Resolve fetches the APKINDEX (if not cached), then looks up every requested
// package and its transitive dependencies.
func (r *apkResolver) Resolve(ctx context.Context, packages []string) ([]providers.Package, error) {
	r.indexOnce.Do(func() {
		r.index = make(map[string]*apkEntry)
		for _, repo := range r.repos {
			if err := r.fetchIndex(ctx, repo); err != nil {
				r.indexErr = fmt.Errorf("failed to fetch APKINDEX from %s: %w", repo, err)
				return
			}
		}
	})
	if r.indexErr != nil {
		return nil, r.indexErr
	}

	seen := map[string]bool{}
	var resolved []providers.Package
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
			return nil, fmt.Errorf("package %q not found in APKINDEX", name)
		}

		resolvedVersion := entry.Version
		if lockedVersion, exists := r.lockedPackages[entry.Name]; exists {
			resolvedVersion = lockedVersion
		}

		resolved = append(resolved, providers.Package{
			Name:         entry.Name,
			Version:      resolvedVersion,
			Arch:         entry.Arch,
			License:      entry.License,
			Description:  entry.Description,
			DownloadURL:  r.downloadURL(entry),
			Checksum:     entry.Checksum,
			Dependencies: entry.Dependencies,
			Size:         entry.Size,
		})

		// Enqueue transitive dependencies.
		for _, dep := range entry.Dependencies {
			depName := cleanDepName(dep)
			if depName != "" && !seen[depName] {
				queue = append(queue, depName)
			}
		}
	}

	return resolved, nil
}

// fetchIndex downloads and parses an APKINDEX.tar.gz from a single repo URL.
func (r *apkResolver) fetchIndex(ctx context.Context, repoURL string) error {
	indexURL := repoURL + "/APKINDEX.tar.gz"

	st := llb.HTTP(indexURL, llb.Filename("APKINDEX.tar.gz"))
	def, err := st.Marshal(ctx)
	if err != nil {
		return fmt.Errorf("failed to marshal APKINDEX HTTP state: %w", err)
	}

	res, err := r.client.Solve(ctx, client.SolveRequest{
		Definition: def.ToPB(),
	})
	if err != nil {
		return fmt.Errorf("failed to solve/download APKINDEX from %s: %w", indexURL, err)
	}

	ref, err := res.SingleRef()
	if err != nil {
		return fmt.Errorf("failed to get result reference for APKINDEX: %w", err)
	}

	data, err := ref.ReadFile(ctx, client.ReadRequest{
		Filename: "APKINDEX.tar.gz",
	})
	if err != nil {
		return fmt.Errorf("failed to read APKINDEX archive: %w", err)
	}

	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("gzip decompress failed: %w", err)
	}
	defer func() { _ = gz.Close() }()

	return r.parseIndex(gz)
}

// parseIndex reads the decompressed APKINDEX content and populates r.index.
// The format is a series of key:value records separated by blank lines.
func (r *apkResolver) parseIndex(reader io.Reader) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var current *apkEntry

	commitEntry := func() {
		if current != nil && current.Name != "" {
			// Only keep the first occurrence (highest priority repo).
			if _, exists := r.index[current.Name]; !exists {
				r.index[current.Name] = current
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

		if current == nil {
			current = &apkEntry{Arch: r.arch}
		}

		if len(line) < 2 || line[1] != ':' {
			continue
		}

		key := line[0]
		value := line[2:]

		switch key {
		case 'P':
			current.Name = value
		case 'V':
			current.Version = value
		case 'A':
			current.Arch = value
		case 'L':
			current.License = value
		case 'T':
			current.Description = value
		case 'D':
			current.Dependencies = splitDeps(value)
		case 'p':
			current.Provides = splitDeps(value)
		case 'C':
			current.Checksum = value
		}
	}
	// Commit the last record if file does not end with a blank line.
	commitEntry()

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to scan APKINDEX: %w", err)
	}
	return nil
}

// downloadURL constructs the package download URL from its metadata.
func (r *apkResolver) downloadURL(entry *apkEntry) string {
	if len(r.repos) == 0 {
		return ""
	}
	return fmt.Sprintf("%s/%s-%s.apk", r.repos[0], entry.Name, entry.Version)
}

// splitDeps splits the APK dependency string (space-separated) into individual names.
func splitDeps(s string) []string {
	parts := strings.Fields(s)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// cleanDepName extracts the package name from a dependency string.
// APK dependencies can have version constraints like "musl>=1.2",
// virtual providers like "so:libz.so.1", or path requirements like "/bin/sh".
// We strip or skip these to get the base package name.
func cleanDepName(dep string) string {
	// Skip path dependencies (e.g. /bin/sh)
	if len(dep) > 0 && dep[0] == '/' {
		return ""
	}
	// Skip virtual providers (so:, pc:, cmd:)
	if strings.ContainsRune(dep, ':') {
		return ""
	}
	// Strip version operators
	for _, op := range []string{">=", "<=", ">", "<", "=", "~"} {
		if idx := strings.Index(dep, op); idx > 0 {
			return dep[:idx]
		}
	}
	return dep
}
