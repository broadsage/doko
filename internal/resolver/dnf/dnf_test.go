package dnf

import (
	"strings"
	"testing"
)

func TestParsePrimary(t *testing.T) {
	primaryXML := `
<metadata xmlns="http://linux.duke.edu/metadata/common" xmlns:rpm="http://linux.duke.edu/metadata/rpm" packages="2">
  <package type="rpm">
    <name>nginx</name>
    <arch>x86_64</arch>
    <version epoch="0" ver="1.27.4" rel="1.fc40"/>
    <checksum type="sha256" pkgid="YES">sha256-abc123nginx</checksum>
    <summary>nginx web server</summary>
    <description>nginx reverse proxy</description>
    <location href="Packages/n/nginx-1.27.4-1.fc40.x86_64.rpm"/>
    <format>
      <rpm:license>BSD-2-Clause</rpm:license>
      <rpm:requires>
        <rpm:entry name="libc.so.6()(64bit)"/>
        <rpm:entry name="pcre2"/>
      </rpm:requires>
    </format>
  </package>
  <package type="rpm">
    <name>pcre2</name>
    <arch>x86_64</arch>
    <version epoch="0" ver="10.42" rel="2.fc40"/>
    <checksum type="sha256" pkgid="YES">sha256-def456pcre2</checksum>
    <summary>pcre2 regex library</summary>
    <description>pcre2 engine</description>
    <location href="Packages/p/pcre2-10.42-2.fc40.x86_64.rpm"/>
    <format>
      <rpm:license>BSD-3-Clause</rpm:license>
      <rpm:requires>
        <rpm:entry name="libc.so.6()(64bit)"/>
      </rpm:requires>
    </format>
  </package>
</metadata>
`

	r := &dnfResolver{
		repos: []string{"https://example.com/repo"},
		arch:  "x86_64",
	}
	r.index = make(map[string]*rpmPackage)

	err := r.parsePrimary(strings.NewReader(primaryXML))
	if err != nil {
		t.Fatalf("unexpected error parsing primary XML: %v", err)
	}

	if len(r.index) != 2 {
		t.Fatalf("expected 2 packages, got %d", len(r.index))
	}

	nginx, ok := r.index["nginx"]
	if !ok {
		t.Fatal("expected nginx in index")
	}
	if nginx.Version.Ver != "1.27.4" {
		t.Errorf("expected nginx version 1.27.4, got %q", nginx.Version.Ver)
	}
	if nginx.Format.License != "BSD-2-Clause" {
		t.Errorf("expected license BSD-2-Clause, got %q", nginx.Format.License)
	}
}

func TestCleanRpmDep(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"pcre2", "pcre2"},
		{"libc.so.6()(64bit)", ""},
		{"rpmlib(PayloadFilesHavePrefix)", ""},
		{"zlib.so", ""},
		{" curl ", "curl"},
	}

	for _, tc := range tests {
		result := cleanRpmDep(tc.input)
		if result != tc.expected {
			t.Errorf("cleanRpmDep(%q) = %q, want %q", tc.input, result, tc.expected)
		}
	}
}
