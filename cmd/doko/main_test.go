package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunInitAndValidate(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "doko-cli-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	filePath := filepath.Join(tempDir, "doko.yaml")

	// 1. Test init successfully
	err = runInit([]string{filePath})
	if err != nil {
		t.Fatalf("runInit failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Fatal("expected doko.yaml to be created, but it does not exist")
	}

	// 2. Test init failure on duplicate
	err = runInit([]string{filePath})
	if err == nil {
		t.Fatal("expected runInit to fail on duplicate file, but it succeeded")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("expected duplicate file error, got %v", err)
	}

	// 3. Test init force overwrite
	err = runInit([]string{"-f", filePath})
	if err != nil {
		t.Fatalf("runInit with force overwrite failed: %v", err)
	}

	// 4. Test validate successfully
	err = runValidate([]string{filePath})
	if err != nil {
		t.Fatalf("runValidate failed on valid config: %v", err)
	}

	// 5. Test validate failure on invalid config
	invalidFilePath := filepath.Join(tempDir, "invalid.yaml")
	invalidContent := `
name: bad-config
accounts:
  root: "should_be_a_boolean"
`
	if err := os.WriteFile(invalidFilePath, []byte(invalidContent), 0o644); err != nil {
		t.Fatalf("failed to write invalid config: %v", err)
	}

	err = runValidate([]string{invalidFilePath})
	if err == nil {
		t.Fatal("expected runValidate to fail on invalid config, but it succeeded")
	}
	if !strings.Contains(err.Error(), "validation failed") {
		t.Errorf("expected validation failure error, got %v", err)
	}
}

func TestShowVersion(t *testing.T) {
	// Redirect stdout to capture showVersion output
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	showVersion()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	output := buf.String()

	if !strings.Contains(output, "Doko - BuildKit Image Orchestrator") {
		t.Errorf("unexpected showVersion output: %q", output)
	}
}
