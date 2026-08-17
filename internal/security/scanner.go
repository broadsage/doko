package security

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/broadsage/doko/internal/config"
)

// Match represents a single vulnerability match in Grype's JSON output.
type Match struct {
	Vulnerability struct {
		ID       string `json:"id"`
		Severity string `json:"severity"`
	} `json:"vulnerability"`
	Artifact struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"artifact"`
}

// GrypeOutput represents the top-level structure of Grype's JSON output.
type GrypeOutput struct {
	Matches []Match `json:"matches"`
}

// ScanSBOM takes the SBOM JSON payload, executes the Grype scanner,
// and returns errors if any vulnerability violates the threshold configuration.
func ScanSBOM(sbomBytes []byte, cfg config.SecurityConfig) error {
	// 1. Verify Grype is installed
	grypePath, err := exec.LookPath("grype")
	if err != nil {
		return fmt.Errorf("grype CLI binary not found in PATH. Please install Grype: https://github.com/anchore/grype#installation")
	}

	fmt.Println("[doko] running vulnerability scan using Grype...")

	// 2. Prepare command to read from stdin and output json
	cmd := exec.Command(grypePath, "sbom:-", "-o", "json")
	cmd.Stdin = bytes.NewReader(sbomBytes)

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to execute grype: %w (details: %s)", err, stderrBuf.String())
	}

	// 3. Unmarshal the results
	var output GrypeOutput
	if err := json.Unmarshal(stdoutBuf.Bytes(), &output); err != nil {
		return fmt.Errorf("failed to parse grype json output: %w", err)
	}

	// 4. Evaluate vulnerabilities against thresholds
	return EvaluateVulnerabilities(output, cfg)
}

// EvaluateVulnerabilities filters and verifies vulnerability scanner matches against the config specs.
func EvaluateVulnerabilities(output GrypeOutput, cfg config.SecurityConfig) error {
	failOn := strings.ToLower(cfg.FailOn)
	if failOn == "" || failOn == "never" {
		fmt.Println("[doko] vulnerability scans completed (fail-on threshold set to 'never' or not specified)")
		return nil
	}

	ignoreMap := make(map[string]bool)
	for _, cve := range cfg.IgnoreCVEs {
		ignoreMap[strings.ToUpper(cve)] = true
	}

	failSeverityMap := map[string]int{
		"low":      1,
		"medium":   2,
		"high":     3,
		"critical": 4,
	}

	failLevel, ok := failSeverityMap[failOn]
	if !ok {
		// Default fallback
		failLevel = 4 // critical
	}

	var thresholdViolations []string
	var loggedVulnerabilities int

	for _, match := range output.Matches {
		vulnID := strings.ToUpper(match.Vulnerability.ID)
		severity := strings.ToLower(match.Vulnerability.Severity)

		if ignoreMap[vulnID] {
			continue
		}

		level, exists := failSeverityMap[severity]
		if !exists {
			continue
		}

		loggedVulnerabilities++

		if level >= failLevel {
			msg := fmt.Sprintf("[%s] severity CVE %s found in package %s (%s)", strings.ToUpper(severity), vulnID, match.Artifact.Name, match.Artifact.Version)
			thresholdViolations = append(thresholdViolations, msg)
		}
	}

	fmt.Printf("[doko] scanned image packages: found %d total active vulnerabilities\n", loggedVulnerabilities)

	if len(thresholdViolations) > 0 {
		fmt.Fprintln(os.Stderr, "\n[doko] SECURITY THRESHOLD VIOLATION DETECTED:")
		for _, msg := range thresholdViolations {
			fmt.Fprintln(os.Stderr, "  -", msg)
		}
		return fmt.Errorf("vulnerability scan failed: found %d vulnerability violations exceeding '%s' threshold", len(thresholdViolations), failOn)
	}

	fmt.Println("[doko] vulnerability scan passed successfully")
	return nil
}
