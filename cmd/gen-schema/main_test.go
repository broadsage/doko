package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGenSchema(t *testing.T) {
	// Create a temp directory for schema output to avoid overwriting the real schema.json
	tempDir := t.TempDir()

	// Temporarily change working directory to tempDir
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get wd: %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("failed to change wd: %v", err)
	}
	defer func() {
		_ = os.Chdir(oldWD)
	}()

	// Write a dummy config.go file in current dir so the tool thinks it is in the config directory,
	// writing directly to schema.json instead of internal/config/schema.json
	if err := os.WriteFile("config.go", []byte("package config"), 0o644); err != nil {
		t.Fatalf("failed to write dummy config.go: %v", err)
	}

	// Run main()
	main()

	// Verify schema.json was generated
	if _, err := os.Stat("schema.json"); err != nil {
		t.Errorf("expected schema.json to be created: %v", err)
	}

	// Now try without config.go - it should write to internal/config/schema.json
	_ = os.Remove("config.go")
	if err := os.MkdirAll(filepath.Join("internal", "config"), 0o755); err != nil {
		t.Fatalf("failed to create dummy directory: %v", err)
	}

	main()

	if _, err := os.Stat(filepath.Join("internal", "config", "schema.json")); err != nil {
		t.Errorf("expected schema.json to be created in nested path: %v", err)
	}
}
