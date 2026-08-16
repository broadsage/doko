package signature

import (
	"testing"
)

func TestGetCleanBaseName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"ghcr.io/broadsage/doko/alpine", "alpine"},
		{"ghcr.io/broadsage/doko/postgres:latest", "postgres"},
		{"my-registry.org/custom-app:1.2.3@sha256:12345", "custom-app"},
		{"local-image", "local-image"},
	}

	for _, tc := range tests {
		got := getCleanBaseName(tc.input)
		if got != tc.expected {
			t.Errorf("getCleanBaseName(%q) = %q; expected %q", tc.input, got, tc.expected)
		}
	}
}

func TestSignImage_FailsOnInvalidImage(t *testing.T) {
	// Assert that it fails cleanly when attempting to sign an invalid image reference
	err := SignImage("non-existent-image:latest", "")
	if err == nil {
		t.Error("expected error when signing non-existent image, got nil")
	}
}
