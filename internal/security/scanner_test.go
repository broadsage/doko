package security

import (
	"testing"

	"github.com/broadsage/doko/internal/config"
)

func TestEvaluateVulnerabilities_ThresholdPassing(t *testing.T) {
	// Create mock matches
	var output GrypeOutput
	m1 := Match{}
	m1.Vulnerability.ID = "CVE-2024-1234"
	m1.Vulnerability.Severity = "high"
	m1.Artifact.Name = "openssl"
	m1.Artifact.Version = "3.0.1"

	m2 := Match{}
	m2.Vulnerability.ID = "CVE-2024-5678"
	m2.Vulnerability.Severity = "medium"
	m2.Artifact.Name = "zlib"
	m2.Artifact.Version = "1.2.11"

	output.Matches = []Match{m1, m2}

	// Case 1: threshold set to critical (should pass, since we only have high and medium)
	cfg := config.SecurityConfig{
		FailOn: "critical",
	}
	err := EvaluateVulnerabilities(output, cfg)
	if err != nil {
		t.Errorf("expected clean pass on critical threshold, got error: %v", err)
	}

	// Case 2: threshold set to high (should fail, since openssl has a high severity CVE)
	cfgHigh := config.SecurityConfig{
		FailOn: "high",
	}
	errHigh := EvaluateVulnerabilities(output, cfgHigh)
	if errHigh == nil {
		t.Error("expected build failure on high severity threshold, got nil")
	}

	// Case 3: threshold set to high, but CVE is ignored
	cfgIgnore := config.SecurityConfig{
		FailOn:     "high",
		IgnoreCVEs: []string{"CVE-2024-1234"},
	}
	errIgnore := EvaluateVulnerabilities(output, cfgIgnore)
	if errIgnore != nil {
		t.Errorf("expected clean pass when CVE is ignored, got error: %v", errIgnore)
	}
}
