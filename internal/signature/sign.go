package signature

import (
	"bytes"
	"context"
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

	signCmd := exec.CommandContext(context.Background(), "cosign", signArgs...)
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
		cmd := exec.CommandContext(context.Background(), "docker", "run", "--rm", "--entrypoint", "cat", imageRef, pathInsideImage)
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

	attestCmd := exec.CommandContext(context.Background(), "cosign", attestArgs...)
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
	baseName, _ = strings.CutPrefix(baseName, "bhi-")
	return baseName
}

// VerifyImage verifies the cryptographic signatures and SBOM attestations of an image.
func VerifyImage(imageRef string, keyPath string) error {
	// Verify cosign CLI exists
	if _, err := exec.LookPath("cosign"); err != nil {
		return fmt.Errorf("cosign CLI binary not found in PATH: %w", err)
	}

	// 1. Verify image manifest signature
	fmt.Printf("[doko] verifying image signature: %s\n", imageRef)
	verifyArgs := []string{"verify"}
	if keyPath != "" {
		verifyArgs = append(verifyArgs, "--key", keyPath)
	}
	verifyArgs = append(verifyArgs, imageRef)

	verifyCmd := exec.CommandContext(context.Background(), "cosign", verifyArgs...)
	verifyCmd.Stdout = os.Stdout
	verifyCmd.Stderr = os.Stderr
	if err := verifyCmd.Run(); err != nil {
		return fmt.Errorf("image signature verification failed: %w", err)
	}
	fmt.Println("[doko] image signature verified successfully")

	// 2. Verify SBOM attestations (tries both cyclonedx and spdxjson)
	fmt.Println("[doko] verifying SBOM attestations...")
	attestations := []string{"cyclonedx", "spdxjson"}
	var verifyErr error

	for _, t := range attestations {
		attArgs := []string{"verify-attestation", "--type", t}
		if keyPath != "" {
			attArgs = append(attArgs, "--key", keyPath)
		}
		attArgs = append(attArgs, imageRef)

		attCmd := exec.CommandContext(context.Background(), "cosign", attArgs...)
		// Run silently for matching check
		if err := attCmd.Run(); err == nil {
			fmt.Printf("[doko] found and verified valid %s SBOM attestation\n", t)
			return nil
		} else {
			verifyErr = err
		}
	}

	return fmt.Errorf("no verified SBOM attestations found: %w", verifyErr)
}

// GenerateKeypair creates a public/private Cosign keypair.
func GenerateKeypair(outPath string) error {
	if _, err := exec.LookPath("cosign"); err != nil {
		return fmt.Errorf("cosign CLI binary not found in PATH: %w", err)
	}

	fmt.Printf("[doko] generating cosign keypair at: %s\n", outPath)
	cmd := exec.CommandContext(context.Background(), "cosign", "generate-key-pair")

	// If outPath is provided, we can run it in that directory or change output name
	if outPath != "" {
		// Cosign generate-key-pair supports writing to custom paths via environment variables or CLI configs,
		// but key generation by default outputs to cosign.key / cosign.pub.
		// We can change the Cwd of the cmd to output keys in the desired directory.
		cmd.Dir = outPath
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Ensure password is set via environment variable for automation
	if os.Getenv("COSIGN_PASSWORD") == "" {
		cmd.Env = append(os.Environ(), "COSIGN_PASSWORD=")
		fmt.Println("[doko] COSIGN_PASSWORD env var not set; generating keypair without encryption password")
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to generate key pair: %w", err)
	}

	return nil
}
