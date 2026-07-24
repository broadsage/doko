// Package sbom generates Software Bill of Materials (SBOM) documents.
package sbom

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/broadsage/doko/internal/config"
	"github.com/broadsage/doko/internal/resolver"
)

// CycloneDXDocument represents a CycloneDX 1.6 JSON SBOM.
type CycloneDXDocument struct {
	BOMFormat    string               `json:"bomFormat"`
	SpecVersion  string               `json:"specVersion"`
	SerialNumber string               `json:"serialNumber,omitempty"`
	Version      int                  `json:"version"`
	Metadata     CycloneDXMetadata    `json:"metadata"`
	Components   []CycloneDXComponent `json:"components"`
}

type CycloneDXMetadata struct {
	Timestamp string              `json:"timestamp"`
	Tools     *CycloneDXTools     `json:"tools,omitempty"`
	Component *CycloneDXComponent `json:"component,omitempty"`
}

type CycloneDXTools struct {
	Components []CycloneDXToolComponent `json:"components,omitempty"`
}

type CycloneDXToolComponent struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Version string `json:"version"`
}

type CycloneDXComponent struct {
	Type        string                `json:"type"`
	Name        string                `json:"name"`
	Version     string                `json:"version,omitempty"`
	PURL        string                `json:"purl,omitempty"`
	Description string                `json:"description,omitempty"`
	Licenses    []CycloneDXLicenseRef `json:"licenses,omitempty"`
	Hashes      []CycloneDXHash       `json:"hashes,omitempty"`
}

type CycloneDXLicenseRef struct {
	License CycloneDXLicense `json:"license"`
}

type CycloneDXLicense struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

type CycloneDXHash struct {
	Alg   string `json:"alg"`
	Value string `json:"content"`
}

// GenerateCycloneDX creates a CycloneDX 1.6 JSON SBOM document.
func GenerateCycloneDX(spec *config.Spec, packages []resolver.Package) (*CycloneDXDocument, error) {
	doc := &CycloneDXDocument{
		BOMFormat:   "CycloneDX",
		SpecVersion: "1.6",
		Version:     1,
		Metadata: CycloneDXMetadata{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Tools: &CycloneDXTools{
				Components: []CycloneDXToolComponent{
					{
						Type:    "application",
						Name:    "doko",
						Version: "1.0.0",
					},
				},
			},
			Component: &CycloneDXComponent{
				Type:        "container",
				Name:        spec.Name,
				Version:     "latest",
				Description: fmt.Sprintf("Container image built by Doko from base %s", spec.Base),
			},
		},
		Components: []CycloneDXComponent{},
	}

	for _, pkg := range packages {
		comp := CycloneDXComponent{
			Type:        "library",
			Name:        pkg.Name,
			Version:     pkg.Version,
			PURL:        BuildPURL(spec.Provider, pkg),
			Description: pkg.Description,
		}

		if pkg.License != "" {
			comp.Licenses = []CycloneDXLicenseRef{
				{
					License: CycloneDXLicense{
						ID: pkg.License,
					},
				},
			}
		}

		if pkg.Checksum != "" {
			comp.Hashes = []CycloneDXHash{
				{
					Alg:   "SHA-256",
					Value: pkg.Checksum,
				},
			}
		}

		doc.Components = append(doc.Components, comp)
	}

	return doc, nil
}

// MarshalCycloneDX serializes the CycloneDX SBOM to indented JSON.
func MarshalCycloneDX(doc *CycloneDXDocument) ([]byte, error) {
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal CycloneDX SBOM: %w", err)
	}
	return data, nil
}
