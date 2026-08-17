package signature

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") == "1" {
		runHelperProcess()
		os.Exit(0)
	}
}

func runHelperProcess() {
	args := os.Args
	for len(args) > 0 && args[0] != "--" {
		args = args[1:]
	}
	if len(args) == 0 {
		os.Exit(1)
	}
	cmd := args[1]
	subCmd := ""
	if len(args) > 2 {
		subCmd = args[2]
	}

	switch cmd {
	case "docker":
		for _, arg := range args {
			if strings.HasSuffix(arg, "sbom.cdx.json") {
				fmt.Print(`{"bomFormat": "CycloneDX"}`)
				os.Exit(0)
			}
			if strings.HasSuffix(arg, "sbom.spdx.json") {
				fmt.Print(`{"spdxVersion": "SPDX-2.3"}`)
				os.Exit(0)
			}
		}
		os.Exit(1)
	case "cosign":
		switch subCmd {
		case "sign", "attest", "verify-attestation", "generate-key-pair":
			os.Exit(0)
		case "verify":
			for _, arg := range args {
				if arg == "invalid-image" {
					os.Exit(1)
				}
			}
			os.Exit(0)
		default:
			os.Exit(0)
		}
	default:
		os.Exit(0)
	}
}

func fakeExecCommand(ctx context.Context, name string, arg ...string) *exec.Cmd {
	args := make([]string, 0, 3+len(arg))
	args = append(args, "-test.run=TestHelperProcess", "--", name)
	args = append(args, arg...)
	cmd := exec.CommandContext(ctx, os.Args[0], args...)
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
	return cmd
}

func mockLookPath(file string) (string, error) {
	return "/usr/bin/" + file, nil
}

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

func TestVerifyImage_FailsOnInvalidImage(t *testing.T) {
	// Assert that it fails cleanly when attempting to verify an invalid image reference
	err := VerifyImage("non-existent-image:latest", "")
	if err == nil {
		t.Error("expected error when verifying non-existent image, got nil")
	}
}

func TestSignatureFlows_Success(t *testing.T) {
	// Swap functions with mock hooks
	oldExec := ExecCommand
	oldLookPath := LookPath
	ExecCommand = fakeExecCommand
	LookPath = mockLookPath
	defer func() {
		ExecCommand = oldExec
		LookPath = oldLookPath
	}()

	t.Run("SignImage Success", func(t *testing.T) {
		err := SignImage("test-image:latest", "dummy.key")
		if err != nil {
			t.Errorf("SignImage failed: %v", err)
		}
	})

	t.Run("VerifyImage Success", func(t *testing.T) {
		err := VerifyImage("test-image:latest", "dummy.key")
		if err != nil {
			t.Errorf("VerifyImage failed: %v", err)
		}
	})

	t.Run("VerifyImage Failure", func(t *testing.T) {
		err := VerifyImage("invalid-image", "dummy.key")
		if err == nil {
			t.Error("expected VerifyImage to fail for invalid-image, got nil")
		}
	})

	t.Run("GenerateKeypair Success", func(t *testing.T) {
		err := GenerateKeypair(t.TempDir())
		if err != nil {
			t.Errorf("GenerateKeypair failed: %v", err)
		}
	})

	t.Run("SignImage LookPath Fail", func(t *testing.T) {
		LookPath = func(file string) (string, error) {
			return "", fmt.Errorf("not found")
		}
		defer func() { LookPath = mockLookPath }()

		err := SignImage("test-image:latest", "dummy.key")
		if err == nil {
			t.Error("expected error when cosign is missing, got nil")
		}
	})

	t.Run("SignImage Exec Fail", func(t *testing.T) {
		ExecCommand = func(ctx context.Context, name string, arg ...string) *exec.Cmd {
			return exec.CommandContext(ctx, "false")
		}
		defer func() { ExecCommand = fakeExecCommand }()

		err := SignImage("test-image:latest", "dummy.key")
		if err == nil {
			t.Error("expected error when cosign sign command fails, got nil")
		}
	})

	t.Run("attachSBOMAttestation docker missing", func(t *testing.T) {
		LookPath = func(file string) (string, error) {
			if file == "docker" {
				return "", fmt.Errorf("docker not found")
			}
			return "/usr/bin/" + file, nil
		}
		defer func() { LookPath = mockLookPath }()

		err := SignImage("test-image:latest", "dummy.key")
		if err != nil {
			t.Errorf("SignImage should succeed even if docker is missing, got: %v", err)
		}
	})

	t.Run("attachSBOMAttestation temp dir failure", func(t *testing.T) {
		ExecCommand = func(ctx context.Context, name string, arg ...string) *exec.Cmd {
			if name == "docker" {
				return exec.CommandContext(ctx, "false")
			}
			return exec.CommandContext(ctx, "true")
		}
		defer func() { ExecCommand = fakeExecCommand }()

		err := SignImage("test-image:latest", "dummy.key")
		if err != nil {
			t.Errorf("SignImage should succeed even if attachSBOM fails, got: %v", err)
		}
	})

	t.Run("VerifyImage LookPath Fail", func(t *testing.T) {
		LookPath = func(file string) (string, error) {
			return "", fmt.Errorf("not found")
		}
		defer func() { LookPath = mockLookPath }()

		err := VerifyImage("test-image:latest", "dummy.key")
		if err == nil {
			t.Error("expected error when cosign is missing during verification, got nil")
		}
	})

	t.Run("VerifyImage cosign command fails", func(t *testing.T) {
		ExecCommand = func(ctx context.Context, name string, arg ...string) *exec.Cmd {
			return exec.CommandContext(ctx, "false")
		}
		defer func() { ExecCommand = fakeExecCommand }()

		err := VerifyImage("test-image:latest", "dummy.key")
		if err == nil {
			t.Error("expected verification to fail when cosign command fails, got nil")
		}
	})

	t.Run("GenerateKeypair LookPath Fail", func(t *testing.T) {
		LookPath = func(file string) (string, error) {
			return "", fmt.Errorf("not found")
		}
		defer func() { LookPath = mockLookPath }()

		err := GenerateKeypair(t.TempDir())
		if err == nil {
			t.Error("expected error when cosign is missing during keygen, got nil")
		}
	})

	t.Run("GenerateKeypair Exec Fail", func(t *testing.T) {
		ExecCommand = func(ctx context.Context, name string, arg ...string) *exec.Cmd {
			return exec.CommandContext(ctx, "false")
		}
		defer func() { ExecCommand = fakeExecCommand }()

		err := GenerateKeypair(t.TempDir())
		if err == nil {
			t.Error("expected error when cosign keygen fails, got nil")
		}
	})
}
