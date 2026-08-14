package metadata

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/anchore/syft/syft"
	"github.com/anchore/syft/syft/format"
	"github.com/anchore/syft/syft/format/cyclonedxjson"
	_ "github.com/glebarez/go-sqlite"

	"github.com/broadsage/doko/internal/providers"
)

// GetCleanImageName parses an OCI image reference and returns a clean brand-prefixed lowercase name (e.g. "bhi-alpine").
func GetCleanImageName(imageRef string) string {
	if imageRef == "" {
		return "bhi-image"
	}
	parts := strings.Split(imageRef, "/")
	baseName := parts[len(parts)-1]
	if idx := strings.Index(baseName, ":"); idx != -1 {
		baseName = baseName[:idx]
	}
	if idx := strings.Index(baseName, "@"); idx != -1 {
		baseName = baseName[:idx]
	}
	return "bhi-" + strings.ToLower(baseName)
}

// GenerateSBOM scans a simulated package directory using Syft to generate a CycloneDX JSON SBOM.
func GenerateSBOM(ctx context.Context, imageName string, resolvedPkgs []providers.Package) ([]byte, error) {
	// Create a temporary directory to simulate the container filesystem
	tempDir, err := os.MkdirTemp("", "doko-sbom-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory for SBOM: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Simulate Alpine package database: /lib/apk/db/installed
	apkDbDir := filepath.Join(tempDir, "lib", "apk", "db")
	if err := os.MkdirAll(apkDbDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create apk db dir: %w", err)
	}

	apkDbPath := filepath.Join(apkDbDir, "installed")
	f, err := os.OpenFile(apkDbPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to create apk installed db: %w", err)
	}

	for _, p := range resolvedPkgs {
		// Construct the APK installed format entry
		entry := fmt.Sprintf("P:%s\nV:%s\nA:%s\nL:%s\nT:%s\nU:%s\nS:%d\n\n",
			p.Name,
			p.Version,
			p.Arch,
			p.License,
			p.Description,
			p.DownloadURL,
			p.Size,
		)
		if _, err := f.WriteString(entry); err != nil {
			f.Close()
			return nil, fmt.Errorf("failed to write apk database entry: %w", err)
		}
	}
	f.Close() // Close before scanning

	// Use Syft Go SDK to scan the simulated directory
	src, err := syft.GetSource(ctx, tempDir, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get source for Syft scan: %w", err)
	}

	sbomObj, err := syft.CreateSBOM(ctx, src, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to generate SBOM: %w", err)
	}

	// Set the name of the source in the description
	sbomObj.Source.Name = imageName

	// Format/encode to CycloneDX JSON
	cfg := cyclonedxjson.DefaultEncoderConfig()
	encoder, err := cyclonedxjson.NewFormatEncoderWithConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create CycloneDX encoder: %w", err)
	}

	encodedBytes, err := format.Encode(*sbomObj, encoder)
	if err != nil {
		return nil, fmt.Errorf("failed to encode SBOM: %w", err)
	}

	return encodedBytes, nil
}
