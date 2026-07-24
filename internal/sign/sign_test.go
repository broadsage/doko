package sign

import "testing"

func TestNewSigner(t *testing.T) {
	signer := NewSigner("test-key.key")
	if signer.KeyPath != "test-key.key" {
		t.Errorf("expected KeyPath 'test-key.key', got %q", signer.KeyPath)
	}
}
