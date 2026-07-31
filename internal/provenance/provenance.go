// Package provenance implements custom SLSA-style build provenance generation
// for LayerKit, capturing all resolved package URLs and cryptographic checksums.
package provenance

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/broadsage/doko/internal/config"
	"github.com/broadsage/doko/internal/providers"
)

// Attestation defines the SLSA-like build provenance payload structure.
type Attestation struct {
	BuildType string         `json:"buildType"`
	BuiltAt   string         `json:"builtAt"`
	Builder   string         `json:"builder"`
	Subject   Subject        `json:"subject"`
	Materials []Material     `json:"materials"`
	Config    map[string]any `json:"config"`
}

// Subject describes the target OCI image name.
type Subject struct {
	Name string `json:"name"`
}

// Material list holds the precise URL and checksum of every resolved dependency.
type Material struct {
	URI      string `json:"uri"`
	Checksum string `json:"checksum,omitempty"`
}

// Generate creates a LayerKit provenance attestation document.
func Generate(spec *config.Spec, packages []providers.Package) (*Attestation, error) {
	materials := make([]Material, 0, 2+len(packages))
	materials = append(materials,
		Material{URI: fmt.Sprintf("provider:%s", spec.Provider)},
		Material{URI: fmt.Sprintf("base:%s", spec.Base)},
	)

	for _, pkg := range packages {
		materials = append(materials, Material{
			URI:      pkg.DownloadURL,
			Checksum: pkg.Checksum,
		})
	}

	return &Attestation{
		BuildType: "https://doko.io/provenance/v1",
		BuiltAt:   time.Now().UTC().Format(time.RFC3339),
		Builder:   "layerkit-builder/v1",
		Subject: Subject{
			Name: spec.Name,
		},
		Materials: materials,
		Config: map[string]any{
			"policy": spec.Security.Policy,
			"arch":   spec.Arch,
		},
	}, nil
}

// Marshal serializes the attestation to indented JSON.
func Marshal(att *Attestation) ([]byte, error) {
	data, err := json.MarshalIndent(att, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal provenance attestation: %w", err)
	}
	return data, nil
}
