// Package dnf implements the RHEL/Fedora DNF package resolver for LayerKit.
// It parses the repomd.xml and primary.xml.gz metadata to resolve RPM dependencies.
package dnf

import (
	"compress/gzip"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/broadsage/doko/internal/netutil"
	"github.com/broadsage/doko/internal/resolver"
)

const (
	providerName      = "dnf"
	defaultFedoraRepo = "https://dl.fedoraproject.org/pub/fedora/linux/releases/40/Everything/x86_64/os"
	defaultFedoraArch = "x86_64"
)

func init() {
	resolver.RegisterProvider(providerName, newDNFResolver)
}

// repomdxml represents the repomd.xml metadata catalog file.
type repomdxml struct {
	XMLName xml.Name     `xml:"repomd"`
	Data    []repomdData `xml:"data"`
}

type repomdData struct {
	Type     string         `xml:"type,attr"`
	Location repomdLocation `xml:"location"`
}

type repomdLocation struct {
	Href string `xml:"href,attr"`
}

// primaryxml matches the primary.xml metadata file containing package listings.
type primaryxml struct {
	XMLName  xml.Name     `xml:"metadata"`
	Packages []rpmPackage `xml:"package"`
}

type rpmPackage struct {
	Type        string      `xml:"type,attr"`
	Name        string      `xml:"name"`
	Arch        string      `xml:"arch"`
	Version     rpmVersion  `xml:"version"`
	Checksum    rpmChecksum `xml:"checksum"`
	Summary     string      `xml:"summary"`
	Description string      `xml:"description"`
	Location    rpmLocation `xml:"location"`
	Format      rpmFormat   `xml:"format"`
}

type rpmVersion struct {
	Ver string `xml:"ver,attr"`
	Rel string `xml:"rel,attr"`
}

type rpmChecksum struct {
	Value string `xml:",chardata"`
}

type rpmLocation struct {
	Href string `xml:"href,attr"`
}

type rpmFormat struct {
	License  string      `xml:"license"`
	Requires rpmRequires `xml:"requires"`
}

type rpmRequires struct {
	Entries []rpmEntry `xml:"entry"`
}

type rpmEntry struct {
	Name string `xml:"name,attr"`
}

type dnfResolver struct {
	repos          []string
	arch           string
	httpClient     *http.Client
	lockedPackages map[string]string

	indexOnce sync.Once
	index     map[string]*rpmPackage
	indexErr  error
}

func newDNFResolver(opts resolver.Options) (resolver.Resolver, error) {
	arch := opts.Arch
	if arch == "" || arch == "amd64" {
		arch = defaultFedoraArch
	}

	repos := opts.Repositories
	if len(repos) == 0 {
		repos = []string{defaultFedoraRepo}
	}

	var httpClient *http.Client
	if len(opts.CACerts) > 0 {
		httpClient = netutil.NewHTTPClientWithCAs(opts.CACerts)
	} else {
		httpClient = netutil.NewHTTPClient()
	}

	return &dnfResolver{
		repos:          repos,
		arch:           arch,
		httpClient:     httpClient,
		lockedPackages: opts.LockedPackages,
	}, nil
}

func (r *dnfResolver) Name() string { return "RHEL/Fedora DNF" }

func (r *dnfResolver) Resolve(ctx context.Context, packages []string) ([]resolver.Package, error) {
	r.indexOnce.Do(func() {
		r.index = make(map[string]*rpmPackage)
		for _, repo := range r.repos {
			if err := r.fetchIndex(ctx, repo); err != nil {
				r.indexErr = fmt.Errorf("failed to fetch DNF index from %s: %w", repo, err)
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
			return nil, fmt.Errorf("package %q not found in DNF repomd index", name)
		}

		var deps []string
		for _, req := range entry.Format.Requires.Entries {
			cleanDep := cleanRpmDep(req.Name)
			if cleanDep != "" {
				deps = append(deps, cleanDep)
			}
		}

		resolvedVersion := fmt.Sprintf("%s-%s", entry.Version.Ver, entry.Version.Rel)
		if lockedVersion, exists := r.lockedPackages[entry.Name]; exists {
			resolvedVersion = lockedVersion
		}

		resolved = append(resolved, resolver.Package{
			Name:         entry.Name,
			Version:      resolvedVersion,
			Arch:         entry.Arch,
			License:      entry.Format.License,
			Description:  entry.Description,
			DownloadURL:  fmt.Sprintf("%s/%s", r.repos[0], entry.Location.Href),
			Checksum:     entry.Checksum.Value,
			Dependencies: deps,
		})

		for _, dep := range deps {
			if !seen[dep] {
				queue = append(queue, dep)
			}
		}
	}

	return resolved, nil
}

func (r *dnfResolver) fetchIndex(ctx context.Context, repoURL string) error {
	// 1. Download repomd.xml
	repomdURL := repoURL + "/repodata/repomd.xml"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, repomdURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create HTTP request for %s: %w", repomdURL, err)
	}
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request to %s failed: %w", repomdURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d from %s", resp.StatusCode, repomdURL)
	}

	var repomd repomdxml
	if err := xml.NewDecoder(resp.Body).Decode(&repomd); err != nil {
		return fmt.Errorf("failed to decode repomd.xml: %w", err)
	}

	// 2. Find primary.xml.gz location
	var primaryPath string
	for _, d := range repomd.Data {
		if d.Type == "primary" {
			primaryPath = d.Location.Href
			break
		}
	}
	if primaryPath == "" {
		return fmt.Errorf("primary metadata not found in repomd.xml")
	}

	// 3. Download and parse primary.xml.gz
	primaryURL := repoURL + "/" + primaryPath
	req, err = http.NewRequestWithContext(ctx, http.MethodGet, primaryURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create HTTP request for %s: %w", primaryURL, err)
	}
	resp, err = r.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request to %s failed: %w", primaryURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d from %s", resp.StatusCode, primaryURL)
	}

	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to decompress primary.xml.gz: %w", err)
	}
	defer func() { _ = gz.Close() }()

	return r.parsePrimary(gz)
}

func (r *dnfResolver) parsePrimary(reader io.Reader) error {
	var primary primaryxml
	if err := xml.NewDecoder(reader).Decode(&primary); err != nil {
		return fmt.Errorf("failed to decode primary.xml: %w", err)
	}

	for i := range primary.Packages {
		pkg := &primary.Packages[i]
		if _, exists := r.index[pkg.Name]; !exists {
			r.index[pkg.Name] = pkg
		}
	}
	return nil
}

func cleanRpmDep(dep string) string {
	dep = strings.TrimSpace(dep)
	// Skip helper system/virtual capabilities, library names (e.g. libc.so.6)
	if strings.Contains(dep, "(") || strings.Contains(dep, ".so") || strings.HasPrefix(dep, "rpmlib") {
		return ""
	}
	return dep
}
