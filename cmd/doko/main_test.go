package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
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

func TestRunMain(t *testing.T) {
	t.Log("Helper main runner subprocess")
	if os.Getenv("GO_RUN_MAIN") == "1" {
		args := []string{os.Args[0]}
		for i := 1; i < len(os.Args); i++ {
			if os.Args[i] == "--" {
				args = append(args, os.Args[i+1:]...)
				break
			}
		}
		os.Args = args
		main()
		return
	}
}

func TestCLIMain_Commands(t *testing.T) {
	runMainSubprocess := func(args ...string) (string, int) {
		cmdArgs := append([]string{"-test.run=TestRunMain", "--"}, args...)
		cmd := exec.CommandContext(context.Background(), os.Args[0], cmdArgs...)
		cmd.Env = append(os.Environ(), "GO_RUN_MAIN=1")
		output, err := cmd.CombinedOutput()
		exitCode := 0
		if err != nil {
			if exitError, ok := errors.AsType[*exec.ExitError](err); ok {
				exitCode = exitError.ExitCode()
			} else {
				exitCode = -1
			}
		}
		return string(output), exitCode
	}

	t.Run("version", func(t *testing.T) {
		out, code := runMainSubprocess("version")
		if code != 0 || !strings.Contains(out, "Doko - BuildKit") {
			t.Errorf("expected version success, got code %d, output: %s", code, out)
		}
	})

	t.Run("help", func(t *testing.T) {
		out, code := runMainSubprocess("help")
		if code != 0 || !strings.Contains(out, "Available Commands") {
			t.Errorf("expected help success, got code %d, output: %s", code, out)
		}
	})

	t.Run("invalid-cmd", func(t *testing.T) {
		out, code := runMainSubprocess("invalid-cmd")
		if code != 1 || !strings.Contains(out, "unknown command") {
			t.Errorf("expected invalid command to fail, got code %d, output: %s", code, out)
		}
	})

	t.Run("init-error", func(t *testing.T) {
		out, code := runMainSubprocess("init", "/non-existent-dir/doko.yaml")
		if code != 1 || !strings.Contains(out, "Error:") {
			t.Errorf("expected init failure, got code %d, output: %s", code, out)
		}
	})

	t.Run("validate-error", func(t *testing.T) {
		out, code := runMainSubprocess("validate", "non-existent.yaml")
		if code != 1 || !strings.Contains(out, "Error:") {
			t.Errorf("expected validate failure, got code %d, output: %s", code, out)
		}
	})

	t.Run("lint-error", func(t *testing.T) {
		out, code := runMainSubprocess("lint", "non-existent.yaml")
		if code != 1 || !strings.Contains(out, "Error:") {
			t.Errorf("expected lint failure, got code %d, output: %s", code, out)
		}
	})
}
