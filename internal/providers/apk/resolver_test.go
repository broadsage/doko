package apk

import (
	"strings"
	"testing"
)

func TestParseIndex(t *testing.T) {
	// Simulated APKINDEX content (key:value pairs separated by blank lines)
	indexContent := `C:Q1abc123==
P:curl
V:8.12.1-r0
A:x86_64
T:URL retrieval utility and library
L:MIT
D:ca-certificates libcurl

C:Q1def456==
P:libcurl
V:8.12.1-r0
A:x86_64
T:The multiprotocol file transfer library
L:MIT
D:musl nghttp2-libs

C:Q1ghi789==
P:nginx
V:1.27.4-r0
A:x86_64
T:HTTP and reverse proxy server
L:BSD-2-Clause
D:musl pcre2 zlib

`

	r := &apkResolver{
		repos: []string{"https://dl-cdn.alpinelinux.org/alpine/v3.23/main/x86_64"},
		arch:  "x86_64",
	}
	r.index = make(map[string]*apkEntry)

	err := r.parseIndex(strings.NewReader(indexContent), r.repos[0])
	if err != nil {
		t.Fatalf("unexpected error parsing index: %v", err)
	}

	if len(r.index) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(r.index))
	}

	curl, ok := r.index["curl"]
	if !ok {
		t.Fatal("expected 'curl' in index")
	}
	if curl.Version != "8.12.1-r0" {
		t.Errorf("expected curl version '8.12.1-r0', got %q", curl.Version)
	}
	if curl.License != "MIT" {
		t.Errorf("expected curl license 'MIT', got %q", curl.License)
	}
	if len(curl.Dependencies) != 2 {
		t.Errorf("expected 2 curl dependencies, got %d: %v", len(curl.Dependencies), curl.Dependencies)
	}

	nginx, ok := r.index["nginx"]
	if !ok {
		t.Fatal("expected 'nginx' in index")
	}
	if nginx.License != "BSD-2-Clause" {
		t.Errorf("expected nginx license 'BSD-2-Clause', got %q", nginx.License)
	}
}

func TestCleanDepName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"musl", "musl"},
		{"musl>=1.2.4", "musl"},
		{"so:libz.so.1", ""}, // virtual provider — skip
		{"pc:libssl", ""},    // pkg-config virtual — skip
		{"openssl=3.1.0-r0", "openssl"},
		{"zlib~1.3", "zlib"},
	}

	for _, tc := range tests {
		result := cleanDepName(tc.input)
		if result != tc.expected {
			t.Errorf("cleanDepName(%q) = %q, want %q", tc.input, result, tc.expected)
		}
	}
}

func TestSplitDeps(t *testing.T) {
	result := splitDeps("ca-certificates libcurl musl")
	if len(result) != 3 {
		t.Errorf("expected 3 deps, got %d: %v", len(result), result)
	}
	if result[0] != "ca-certificates" {
		t.Errorf("expected first dep 'ca-certificates', got %q", result[0])
	}
}

func TestDownloadURLMultiRepo(t *testing.T) {
	r := &apkResolver{
		repos: []string{
			"https://dl-cdn.alpinelinux.org/alpine/v3.23/main/x86_64",
			"https://dl-cdn.alpinelinux.org/alpine/v3.23/community/x86_64",
		},
		arch: "x86_64",
	}

	entry1 := &apkEntry{
		Name:    "curl",
		Version: "8.12.1-r0",
		RepoURL: r.repos[0],
	}
	entry2 := &apkEntry{
		Name:    "nginx",
		Version: "1.27.4-r0",
		RepoURL: r.repos[1],
	}

	url1 := r.downloadURL(entry1)
	expected1 := "https://dl-cdn.alpinelinux.org/alpine/v3.23/main/x86_64/curl-8.12.1-r0.apk"
	if url1 != expected1 {
		t.Errorf("expected url %q, got %q", expected1, url1)
	}

	url2 := r.downloadURL(entry2)
	expected2 := "https://dl-cdn.alpinelinux.org/alpine/v3.23/community/x86_64/nginx-1.27.4-r0.apk"
	if url2 != expected2 {
		t.Errorf("expected url %q, got %q", expected2, url2)
	}
}
