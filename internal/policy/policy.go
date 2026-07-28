// Package policy implements compile-time security policy enforcement, evaluating package vulnerability severity and licenses.
package policy

import (
	"context"
	"fmt"
	"strings"

	"github.com/broadsage/doko/internal/config"
	"github.com/broadsage/doko/internal/resolver"
	"github.com/broadsage/doko/internal/sbom"
	"github.com/broadsage/doko/internal/vulnerability"
)

// Gate checks packages against user-defined security policies.
type Gate struct {
	FailOnCVE       string
	AllowedLicenses map[string]bool
	scanner         *vulnerability.Scanner
	vexMatcher      *vulnerability.VEXMatcher
}

// NewGate creates a Gate with the specified policies.
func NewGate(failOnCVE string, allowedLicenses []string) *Gate {
	allowedMap := make(map[string]bool)
	for _, l := range allowedLicenses {
		allowedMap[strings.ToUpper(l)] = true
	}
	return &Gate{
		FailOnCVE:       strings.ToLower(failOnCVE),
		AllowedLicenses: allowedMap,
		scanner:         vulnerability.NewScanner(),
	}
}

// WithScanner overrides the default vulnerability scanner (useful for testing).
func (g *Gate) WithScanner(s *vulnerability.Scanner) *Gate {
	g.scanner = s
	return g
}

// WithVEXMatcher sets the VEX vulnerability exception list matcher.
func (g *Gate) WithVEXMatcher(m *vulnerability.VEXMatcher) *Gate {
	g.vexMatcher = m
	return g
}

// Evaluate checks packages for vulnerabilities and license compliance.
// Returns an error describing the first violation found.
func (g *Gate) Evaluate(ctx context.Context, packages []resolver.Package, ecosystem string) error {
	// 1. License Check
	for _, pkg := range packages {
		if err := g.checkLicense(pkg); err != nil {
			return err
		}
	}

	// 2. CVE Check via OSV.dev
	threshold := vulnerability.ParseSeverity(g.FailOnCVE)
	if threshold == vulnerability.SeverityNone {
		return nil
	}

	scanResults, err := g.scanner.Scan(ctx, packages, ecosystem)
	if err != nil {
		return fmt.Errorf("vulnerability scan failed: %w", err)
	}

	// Filter excused vulnerabilities if VEXMatcher is present.
	if g.vexMatcher != nil {
		var filteredResults []vulnerability.ScanResult
		for _, res := range scanResults {
			var nonExcused []vulnerability.Vulnerability
			purl := sbom.BuildPURL(g.providerName(ecosystem), res.Package)
			for _, vuln := range res.Vulnerabilities {
				if !g.vexMatcher.IsExcused(vuln.ID, purl) {
					nonExcused = append(nonExcused, vuln)
				}
			}
			if len(nonExcused) > 0 {
				res.Vulnerabilities = nonExcused
				filteredResults = append(filteredResults, res)
			}
		}
		scanResults = filteredResults
	}

	return vulnerability.Evaluate(scanResults, threshold) //nolint:wrapcheck // internal package error is self-describing
}

// checkLicense verifies the package license is in the allowed list.
func (g *Gate) checkLicense(pkg resolver.Package) error {
	if len(g.AllowedLicenses) == 0 || pkg.License == "" {
		return nil
	}
	normalizedLicense := strings.ToUpper(pkg.License)
	if !g.AllowedLicenses[normalizedLicense] {
		return fmt.Errorf("policy violation: package %s (%s) has forbidden license %q", pkg.Name, pkg.Version, pkg.License)
	}
	return nil
}

// providerName maps an ecosystem name back to its provider string.
func (g *Gate) providerName(ecosystem string) string {
	parts := strings.Split(ecosystem, ":")
	if len(parts) == 0 {
		return "apk"
	}
	if prov := config.DetectProvider(parts[0]); prov != "" {
		return prov
	}
	return "apk"
}
