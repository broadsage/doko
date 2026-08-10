package builder

import (
	"archive/tar"
	"compress/gzip"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/broadsage/doko/internal/config"
)

func TestAssembleAPKAndSigning(t *testing.T) {
	// 1. Create a temp directory for input data
	dataDir, err := os.MkdirTemp("", "apk-data-test-*")
	if err != nil {
		t.Fatalf("failed to create temp data dir: %v", err)
	}
	defer os.RemoveAll(dataDir)

	// Write a mock file inside dataDir
	testFilePath := filepath.Join(dataDir, "hello.txt")
	if err := os.WriteFile(testFilePath, []byte("Hello, World!"), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// 2. Create target temp path for APK
	outPath := filepath.Join(t.TempDir(), "test.apk")

	spec := &config.BuildSpec{
		Name:        "test-package",
		Version:     "1.0.0",
		Epoch:       1,
		Description: "A test package",
		URL:         "https://example.com",
	}

	// Generate RSA key pair for testing signature
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate RSA key: %v", err)
	}
	privBytes := x509.MarshalPKCS1PrivateKey(privKey)
	pemBlock := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privBytes,
	}
	privPEM := pem.EncodeToMemory(pemBlock)

	// Set env variables to trigger signing
	os.Setenv("DOKO_SIGNING_KEY", string(privPEM))
	os.Setenv("DOKO_KEY_NAME", "test-key.rsa.pub")
	defer func() {
		os.Unsetenv("DOKO_SIGNING_KEY")
		os.Unsetenv("DOKO_KEY_NAME")
	}()

	// 3. Assemble the APK
	err = AssembleAPK(dataDir, outPath, spec, "x86_64")
	if err != nil {
		t.Fatalf("AssembleAPK failed: %v", err)
	}

	// 4. Verify the assembled APK file
	f, err := os.Open(outPath)
	if err != nil {
		t.Fatalf("failed to open generated APK: %v", err)
	}
	defer f.Close()

	// The first tarball in signed APK must be the signature block containing `.SIGN.RSA.test-key.rsa.pub`
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip decode of first segment failed: %v", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	hdr, err := tr.Next()
	if err != nil {
		t.Fatalf("failed to read tar header from first segment: %v", err)
	}

	expectedSigName := ".SIGN.RSA.test-key.rsa.pub"
	if hdr.Name != expectedSigName {
		t.Errorf("expected signature file name %q, got %q", expectedSigName, hdr.Name)
	}

	// Read signature content
	sigBytes, err := io.ReadAll(tr)
	if err != nil {
		t.Fatalf("failed to read signature bytes: %v", err)
	}
	if len(sigBytes) == 0 {
		t.Error("signature is empty")
	}
}
