package apt

import (
	"strings"
	"testing"
)

func TestParseIndex(t *testing.T) {
	// Simulated Debian Packages file content
	packagesContent := `Package: curl
Version: 8.12.1-3
Architecture: amd64
Section: web
Filename: pool/main/c/curl/curl_8.12.1-3_amd64.deb
Size: 254832
SHA256: abc123def456
Description: command line tool for transferring data with URL syntax
Depends: libc6 (>= 2.35), libcurl4t64 (= 8.12.1-3), zlib1g (>= 1:1.2.0)

Package: nginx
Version: 1.27.4-1
Architecture: amd64
Section: httpd
Filename: pool/main/n/nginx/nginx_1.27.4-1_amd64.deb
Size: 598400
SHA256: ghi789jkl012
Description: small, powerful, scalable web/proxy server
Depends: libc6 (>= 2.35), libpcre2-8-0 (>= 10.22), zlib1g (>= 1:1.1.4)
Pre-Depends: init-system-helpers (>= 1.54~)

Package: libc6
Version: 2.41-6
Architecture: amd64
Filename: pool/main/g/glibc/libc6_2.41-6_amd64.deb
Size: 3012544
SHA256: mno345pqr678
Description: GNU C Library: Shared libraries

`
	r := &aptResolver{
		repos:  []string{"https://deb.debian.org/debian/dists/trixie/main/binary-amd64/Packages.gz"},
		arch:   "amd64",
		mirror: "https://deb.debian.org/debian",
	}
	r.index = make(map[string]*debEntry)

	err := r.parseIndex(strings.NewReader(packagesContent))
	if err != nil {
		t.Fatalf("unexpected error parsing Packages: %v", err)
	}

	if len(r.index) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(r.index))
	}

	curl, ok := r.index["curl"]
	if !ok {
		t.Fatal("expected 'curl' in index")
	}
	if curl.Version != "8.12.1-3" {
		t.Errorf("expected curl version '8.12.1-3', got %q", curl.Version)
	}
	if curl.Section != "web" {
		t.Errorf("expected curl section 'web', got %q", curl.Section)
	}
	if len(curl.Depends) != 3 {
		t.Errorf("expected 3 curl dependencies, got %d: %v", len(curl.Depends), curl.Depends)
	}

	nginx, ok := r.index["nginx"]
	if !ok {
		t.Fatal("expected 'nginx' in index")
	}
	if len(nginx.PreDepends) != 1 {
		t.Errorf("expected 1 nginx pre-depend, got %d: %v", len(nginx.PreDepends), nginx.PreDepends)
	}
	if nginx.SHA256 != "ghi789jkl012" {
		t.Errorf("expected nginx sha256 'ghi789jkl012', got %q", nginx.SHA256)
	}
}

func TestParseDebDeps(t *testing.T) {
	result := parseDebDeps("libc6 (>= 2.35), libcurl4t64 (= 8.12.1-3), zlib1g (>= 1:1.2.0)")
	if len(result) != 3 {
		t.Fatalf("expected 3 deps, got %d: %v", len(result), result)
	}
	if result[0] != "libc6" {
		t.Errorf("expected first dep 'libc6', got %q", result[0])
	}
	if result[1] != "libcurl4t64" {
		t.Errorf("expected second dep 'libcurl4t64', got %q", result[1])
	}
}

func TestParseDebDeps_Alternatives(t *testing.T) {
	result := parseDebDeps("zlib1g (>= 1.2) | libdeflate0, libc6")
	if len(result) != 2 {
		t.Fatalf("expected 2 deps (first alternative taken), got %d: %v", len(result), result)
	}
	if result[0] != "zlib1g" {
		t.Errorf("expected first dep 'zlib1g', got %q", result[0])
	}
}

func TestCleanAptDep(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"libc6", "libc6"},
		{"libc6 (>= 2.35)", "libc6"},
		{"libc6:any", "libc6"},
		{"zlib1g (>= 1:1.2.0) | libdeflate0", "zlib1g"},
		{" curl ", "curl"},
	}

	for _, tc := range tests {
		result := cleanAptDep(tc.input)
		if result != tc.expected {
			t.Errorf("cleanAptDep(%q) = %q, want %q", tc.input, result, tc.expected)
		}
	}
}
