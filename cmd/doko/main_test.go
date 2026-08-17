package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/broadsage/doko/internal/signature"
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

func TestShowHelp(t *testing.T) {
	t.Log("Testing showHelp")
	old := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	showHelp("help")
	showHelp("invalid")

	w.Close()
	os.Stdout = old
}

func TestRunLint(t *testing.T) {
	t.Log("Testing runLint with non-existent file")
	err := runLint([]string{"non-existent.yaml"})
	if err == nil {
		t.Error("expected runLint to fail for non-existent file, got nil")
	}
}

func TestCLICommandsWithMocks(t *testing.T) {
	oldExec := signature.ExecCommand
	oldLookPath := signature.LookPath
	defer func() {
		signature.ExecCommand = oldExec
		signature.LookPath = oldLookPath
	}()

	signature.LookPath = func(file string) (string, error) {
		return "/usr/bin/" + file, nil
	}
	signature.ExecCommand = func(ctx context.Context, name string, arg ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "echo")
	}

	t.Run("runKeygen Success", func(t *testing.T) {
		err := runKeygen([]string{"--out", t.TempDir()})
		if err != nil {
			t.Errorf("runKeygen failed: %v", err)
		}
	})

	t.Run("runKeygen Invalid Arg", func(t *testing.T) {
		err := runKeygen([]string{"--invalid-flag"})
		if err == nil {
			t.Error("expected runKeygen to fail on invalid arg, got nil")
		}
	})

	t.Run("runSign Success", func(t *testing.T) {
		err := runSign([]string{"test-image:latest", "--key", "dummy.key"})
		if err != nil {
			t.Errorf("runSign failed: %v", err)
		}
	})

	t.Run("runSign Missing Arg", func(t *testing.T) {
		err := runSign([]string{})
		if err == nil {
			t.Error("expected runSign to fail without arguments, got nil")
		}
	})

	t.Run("runVerify Success", func(t *testing.T) {
		err := runVerify([]string{"test-image:latest", "--key", "dummy.key"})
		if err != nil {
			t.Errorf("runVerify failed: %v", err)
		}
	})

	t.Run("runVerify Missing Arg", func(t *testing.T) {
		err := runVerify([]string{})
		if err == nil {
			t.Error("expected runVerify to fail without arguments, got nil")
		}
	})
}
