package signature

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// SignImage invokes the cosign CLI to sign the specified remote image and attach any SBOM attestations.
func SignImage(imageRef string, keyPath string) error {
	// 1. Verify cosign CLI exists
	if _, err := exec.LookPath("cosign"); err != nil {
		return fmt.Errorf("cosign CLI binary not found in PATH. Please install cosign: https://github.com/sigstore/cosign")
	}

	// 2. Sign the image manifest
	fmt.Printf("[doko] signing image: %s\n", imageRef)
	signArgs := []string{"sign"}
	if keyPath != "" {
		signArgs = append(signArgs, "--key", keyPath)
	} else {
		fmt.Println("[doko] Warning: No private key specified. Attempting keyless signing...")
	}
	// Add signature validation skip prompts/confirmations for CI/non-interactive environments
	signArgs = append(signArgs, "--yes", imageRef)

	signCmd := exec.Command("cosign", signArgs...)
	signCmd.Stdout = os.Stdout
	signCmd.Stderr = os.Stderr
	if err := signCmd.Run(); err != nil {
		return fmt.Errorf("cosign signature failed: %w", err)
	}
	fmt.Println("[doko] image signed successfully")

	// 3. Attempt to extract and attach the SBOM as an attestation
	if err := attachSBOMAttestation(imageRef, keyPath); err != nil {
		fmt.Printf("[doko] Warning: could not attach SBOM attestation: %v\n", err)
	}

	return nil
}

func attachSBOMAttestation(imageRef string, keyPath string) error {
	baseName := getCleanBaseName(imageRef)
	
	// We check for both cdx and spdx formats
	formats := []struct {
		suffix string
		typeId string
	}{
		{"sbom.cdx.json", "cyclonedx"},
		{"sbom.spdx.json", "spdxjson"},
	}

	var foundSuffix string
	var foundTypeId string
	var sbomContent []byte

	// Check if docker CLI is available to extract the SBOM
	if _, err := exec.LookPath("docker"); err != nil {
		return fmt.Errorf("docker CLI binary not found, skipping SBOM attestation extraction")
	}

	for _, f := range formats {
		pathInsideImage := fmt.Sprintf("/opt/docker/sbom/bhi-%s/%s", baseName, f.suffix)
		cmd := exec.Command("docker", "run", "--rm", "--entrypoint", "cat", imageRef, pathInsideImage)
		var out bytes.Buffer
		cmd.Stdout = &out
		
		if err := cmd.Run(); err == nil && out.Len() > 0 {
			sbomContent = out.Bytes()
			foundSuffix = f.suffix
			foundTypeId = f.typeId
			break
		}
	}

	if len(sbomContent) == 0 {
		return fmt.Errorf("no generated SBOM file found inside image at /opt/docker/sbom/bhi-%s/", baseName)
	}

	// Write temp file to pass as predicate
	tempDir, err := os.MkdirTemp("", "doko-attest-*")
	if err != nil {
		return fmt.Errorf("create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	tempFile := filepath.Join(tempDir, foundSuffix)
	if err := os.WriteFile(tempFile, sbomContent, 0600); err != nil {
		return fmt.Errorf("write temp SBOM: %w", err)
	}

	fmt.Printf("[doko] attaching %s SBOM attestation for: %s\n", foundTypeId, imageRef)
	attestArgs := []string{"attest", "--type", foundTypeId, "--predicate", tempFile}
	if keyPath != "" {
		attestArgs = append(attestArgs, "--key", keyPath)
	}
	attestArgs = append(attestArgs, "--yes", imageRef)

	attestCmd := exec.Command("cosign", attestArgs...)
	attestCmd.Stdout = os.Stdout
	attestCmd.Stderr = os.Stderr
	if err := attestCmd.Run(); err != nil {
		return fmt.Errorf("cosign attestation failed: %w", err)
	}

	fmt.Println("[doko] SBOM attestation attached successfully")
	return nil
}

func getCleanBaseName(imageRef string) string {
	parts := strings.Split(imageRef, "/")
	baseName := parts[len(parts)-1]
	if idx := strings.Index(baseName, ":"); idx != -1 {
		baseName = baseName[:idx]
	}
	if idx := strings.Index(baseName, "@"); idx != -1 {
		baseName = baseName[:idx]
	}
	baseName = strings.ToLower(baseName)
	if strings.HasPrefix(baseName, "bhi-") {
		baseName = strings.TrimPrefix(baseName, "bhi-")
	}
	return baseName
}
