package policy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/broadsage/doko/internal/resolver"
	"github.com/broadsage/doko/internal/vulnerability"
)

func TestGate_Evaluate_Compliant(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results": [{}, {}]}`))
	}))
	defer srv.Close()

	g := NewGate("high", []string{"MIT", "Apache-2.0"})
	g.WithScanner(vulnerability.NewScannerWithEndpoint(srv.URL, srv.Client()))

	pkgs := []resolver.Package{
		{Name: "nginx", Version: "1.27.4-r0", License: "MIT"},
		{Name: "curl", Version: "8.12.1-r0", License: "Apache-2.0"},
	}

	if err := g.Evaluate(context.Background(), pkgs, "Alpine:v3.23"); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestGate_Evaluate_LicenseViolation(t *testing.T) {
	g := NewGate("high", []string{"MIT", "Apache-2.0"})
	pkgs := []resolver.Package{
		{Name: "gpl-app", Version: "1.0", License: "GPL-3.0"},
	}
	if err := g.Evaluate(context.Background(), pkgs, "Alpine:v3.23"); err == nil {
		t.Error("expected error due to GPL-3.0 license, but got none")
	}
}

func TestGate_Evaluate_CVEViolation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"results": [
				{
					"vulns": [
						{
							"id": "CVE-2026-9999",
							"summary": "Critical vulnerability",
							"severity": [{"type": "CVSS_V3", "score": "9.8"}]
						}
					]
				}
			]
		}`))
	}))
	defer srv.Close()

	g := NewGate("high", []string{"MIT"})
	g.WithScanner(vulnerability.NewScannerWithEndpoint(srv.URL, srv.Client()))

	pkgs := []resolver.Package{
		{Name: "dirty-package", Version: "1.0", License: "MIT"},
	}
	if err := g.Evaluate(context.Background(), pkgs, "Alpine:v3.23"); err == nil {
		t.Error("expected error due to active CVE, but got none")
	}
}

func TestGate_Evaluate_NoLicenseEnforcement(t *testing.T) {
	g := NewGate("high", nil)
	pkgs := []resolver.Package{
		{Name: "anything", Version: "1.0", License: "WTFPL"},
	}
	if err := g.Evaluate(context.Background(), pkgs, "Alpine:v3.23"); err != nil {
		t.Errorf("expected no error when no license list set, got: %v", err)
	}
}

func TestGate_Evaluate_EmptyLicenseInPackage(t *testing.T) {
	g := NewGate("high", []string{"MIT"})
	pkgs := []resolver.Package{
		{Name: "libc6", Version: "2.41", License: ""},
	}
	// Empty license in package should pass — Debian packages often lack license in index
	if err := g.Evaluate(context.Background(), pkgs, "Alpine:v3.23"); err != nil {
		t.Errorf("expected no error for empty license in package, got: %v", err)
	}
}
