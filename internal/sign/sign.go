// Package sign wraps the Cosign CLI to sign OCI images and attach SBOM/provenance attestations.
package sign

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

// Signer wraps cosign execution.
type Signer struct {
	KeyPath string // Optional path to private key; keyless/OIDC is used if empty
}

// NewSigner creates a new Cosign wrapper signer.
func NewSigner(keyPath string) *Signer {
	return &Signer{KeyPath: keyPath}
}

// Sign signs the specified OCI image reference.
func (s *Signer) Sign(ctx context.Context, imageRef string) (string, error) {
	args := []string{"sign"}
	if s.KeyPath != "" {
		args = append(args, "--key", s.KeyPath)
	} else {
		// Keyless sign requires OIDC confirmation or setting environment variable
		args = append(args, "--yes")
	}
	args = append(args, imageRef)

	cmd := exec.CommandContext(ctx, "cosign", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("cosign sign failed: %w (stderr: %s)", err, stderr.String())
	}
	return stdout.String(), nil
}

// AttachSBOM attaches an SPDX or CycloneDX SBOM to the OCI image.
func (s *Signer) AttachSBOM(ctx context.Context, imageRef, sbomPath string) error {
	cmd := exec.CommandContext(ctx, "cosign", "attach", "sbom", "--sbom", sbomPath, imageRef)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("cosign attach sbom failed: %w (stderr: %s)", err, stderr.String())
	}
	return nil
}

// AttachProvenance attaches a SLSA-style provenance predicate to the OCI image.
func (s *Signer) AttachProvenance(ctx context.Context, imageRef, provenancePath string) error {
	args := []string{"attest"}
	if s.KeyPath != "" {
		args = append(args, "--key", s.KeyPath)
	} else {
		args = append(args, "--yes")
	}
	args = append(args, "--type", "slsaprovenance", "--predicate", provenancePath, imageRef)

	cmd := exec.CommandContext(ctx, "cosign", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("cosign attest failed: %w (stderr: %s)", err, stderr.String())
	}
	return nil
}
