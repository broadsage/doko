// Package sbom generates Software Bill of Materials (SBOM) documents
// in SPDX 2.3 JSON format from resolved package metadata.
package sbom

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/broadsage/doko/internal/config"
	"github.com/broadsage/doko/internal/resolver"
)

// Document represents a complete SPDX 2.3 SBOM document.
type Document struct {
	SPDXVersion       string                 `json:"spdxVersion"`
	DataLicense       string                 `json:"dataLicense"`
	SPDXID            string                 `json:"SPDXID"`
	Name              string                 `json:"name"`
	DocumentNamespace string                 `json:"documentNamespace"`
	CreationInfo      CreationInfo           `json:"creationInfo"`
	Packages          []SPDXPackage          `json:"packages"`
	Relationships     []Relationship         `json:"relationships"`
	ExtractedLicenses []ExtractedLicenseInfo `json:"hasExtractedLicensingInfos,omitempty"`
}

// CreationInfo holds metadata about when and how the SBOM was generated.
type CreationInfo struct {
	Created            string   `json:"created"`
	Creators           []string `json:"creators"`
	LicenseListVersion string   `json:"licenseListVersion,omitempty"`
}

// SPDXPackage represents a single package entry in the SBOM.
type SPDXPackage struct {
	SPDXID           string        `json:"SPDXID"`
	Name             string        `json:"name"`
	VersionInfo      string        `json:"versionInfo"`
	DownloadLocation string        `json:"downloadLocation"`
	FilesAnalyzed    bool          `json:"filesAnalyzed"`
	LicenseConcluded string        `json:"licenseConcluded"`
	LicenseDeclared  string        `json:"licenseDeclared"`
	CopyrightText    string        `json:"copyrightText"`
	Description      string        `json:"description,omitempty"`
	Supplier         string        `json:"supplier,omitempty"`
	ExternalRefs     []ExternalRef `json:"externalRefs,omitempty"`
	Checksums        []Checksum    `json:"checksums,omitempty"`
}

// Relationship describes a dependency relationship between SPDX elements.
type Relationship struct {
	Element        string `json:"spdxElementId"`
	RelType        string `json:"relationshipType"`
	RelatedElement string `json:"relatedSpdxElement"`
}

// ExternalRef provides a reference to an external package identifier (e.g., PURL).
type ExternalRef struct {
	Category string `json:"referenceCategory"`
	Type     string `json:"referenceType"`
	Locator  string `json:"referenceLocator"`
}

// Checksum holds a hash value for integrity verification.
type Checksum struct {
	Algorithm string `json:"algorithm"`
	Value     string `json:"checksumValue"`
}

// ExtractedLicenseInfo holds non-standard license identifiers.
type ExtractedLicenseInfo struct {
	LicenseID     string `json:"licenseId"`
	ExtractedText string `json:"extractedText"`
}

// GenerateSPDX creates an SPDX 2.3 JSON SBOM from the build spec and resolved packages.
func GenerateSPDX(spec *config.Spec, packages []resolver.Package) (*Document, error) {
	namespace := fmt.Sprintf(
		"https://doko.io/sbom/%s/%s",
		sanitizeName(spec.Name),
		time.Now().UTC().Format("20060102T150405Z"),
	)

	doc := &Document{
		SPDXVersion:       "SPDX-2.3",
		DataLicense:       "CC0-1.0",
		SPDXID:            "SPDXRef-DOCUMENT",
		Name:              spec.Name,
		DocumentNamespace: namespace,
		CreationInfo: CreationInfo{
			Created:  time.Now().UTC().Format(time.RFC3339),
			Creators: []string{"Tool: layerkit-builder"},
		},
	}

	// Add the root image package.
	rootPkg := SPDXPackage{
		SPDXID:           "SPDXRef-RootPackage",
		Name:             spec.Name,
		VersionInfo:      "latest",
		DownloadLocation: "NOASSERTION",
		FilesAnalyzed:    false,
		LicenseConcluded: "NOASSERTION",
		LicenseDeclared:  "NOASSERTION",
		CopyrightText:    "NOASSERTION",
		Description:      fmt.Sprintf("Container image built by LayerKit from base %s", spec.Base),
	}
	doc.Packages = append(doc.Packages, rootPkg)

	// Describe the document -> root relationship.
	doc.Relationships = append(doc.Relationships, Relationship{
		Element:        "SPDXRef-DOCUMENT",
		RelType:        "DESCRIBES",
		RelatedElement: "SPDXRef-RootPackage",
	})

	// Add each resolved package.
	for i, pkg := range packages {
		spdxID := fmt.Sprintf("SPDXRef-Package-%d", i)

		spdxPkg := SPDXPackage{
			SPDXID:           spdxID,
			Name:             pkg.Name,
			VersionInfo:      pkg.Version,
			DownloadLocation: pkg.DownloadURL,
			FilesAnalyzed:    false,
			LicenseConcluded: normalizeLicense(pkg.License),
			LicenseDeclared:  normalizeLicense(pkg.License),
			CopyrightText:    "NOASSERTION",
			Description:      pkg.Description,
		}

		// Add PURL external reference.
		purl := BuildPURL(spec.Provider, pkg)
		if purl != "" {
			spdxPkg.ExternalRefs = append(spdxPkg.ExternalRefs, ExternalRef{
				Category: "PACKAGE-MANAGER",
				Type:     "purl",
				Locator:  purl,
			})
		}

		// Add checksum if available.
		if pkg.Checksum != "" {
			spdxPkg.Checksums = append(spdxPkg.Checksums, Checksum{
				Algorithm: "SHA256",
				Value:     pkg.Checksum,
			})
		}

		doc.Packages = append(doc.Packages, spdxPkg)

		// Root depends on this package.
		doc.Relationships = append(doc.Relationships, Relationship{
			Element:        "SPDXRef-RootPackage",
			RelType:        "DEPENDS_ON",
			RelatedElement: spdxID,
		})
	}

	return doc, nil
}

// MarshalSPDX serializes the SBOM document to indented JSON.
func MarshalSPDX(doc *Document) ([]byte, error) {
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal SBOM: %w", err)
	}
	return data, nil
}

// BuildPURL constructs a Package URL (PURL) for the given package.
// See: https://github.com/package-url/purl-spec
func BuildPURL(provider string, pkg resolver.Package) string {
	switch provider {
	case "apk":
		return fmt.Sprintf("pkg:apk/alpine/%s@%s?arch=%s", pkg.Name, pkg.Version, pkg.Arch)
	case "apt":
		return fmt.Sprintf("pkg:deb/debian/%s@%s?arch=%s", pkg.Name, pkg.Version, pkg.Arch)
	case "dnf":
		return fmt.Sprintf("pkg:rpm/fedora/%s@%s?arch=%s", pkg.Name, pkg.Version, pkg.Arch)
	default:
		return ""
	}
}

// normalizeLicense maps a license string to a valid SPDX identifier.
func normalizeLicense(license string) string {
	if license == "" {
		return "NOASSERTION"
	}
	return license
}

// sanitizeName replaces characters not safe for URIs.
func sanitizeName(name string) string {
	return strings.NewReplacer(" ", "-", "/", "-").Replace(name)
}
