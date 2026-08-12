package utils

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	gwclient "github.com/moby/buildkit/frontend/gateway/client"
	fstypes "github.com/tonistiigi/fsutil/types"
)

type mockRef struct {
	gwclient.Reference
	dirs  map[string][]*fstypes.Stat
	files map[string][]byte
}

func (m *mockRef) ReadDir(ctx context.Context, req gwclient.ReadDirRequest) ([]*fstypes.Stat, error) {
	entries, ok := m.dirs[req.Path]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return entries, nil
}

func (m *mockRef) ReadFile(ctx context.Context, req gwclient.ReadRequest) ([]byte, error) {
	content, ok := m.files[req.Filename]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return content, nil
}

func TestCopyRefToDir(t *testing.T) {
	tempDir := t.TempDir()

	ref := &mockRef{
		dirs: map[string][]*fstypes.Stat{
			"/src": {
				{
					Path: "file.txt",
					Mode: 0o644,
				},
				{
					Path: "sub",
					Mode: uint32(os.ModeDir) | 0o755,
				},
			},
			"/src/sub": {
				{
					Path: "nested.txt",
					Mode: 0o755,
				},
			},
		},
		files: map[string][]byte{
			"/src/file.txt":       []byte("hello"),
			"/src/sub/nested.txt": []byte("nested hello"),
		},
	}

	ctx := context.Background()
	err := CopyRefToDir(ctx, ref, "/src", tempDir)
	if err != nil {
		t.Fatalf("unexpected error CopyRefToDir: %v", err)
	}

	// Verify file.txt
	data1, err := os.ReadFile(filepath.Join(tempDir, "file.txt"))
	if err != nil {
		t.Fatalf("failed to read copied file: %v", err)
	}
	if string(data1) != "hello" {
		t.Errorf("expected 'hello', got %q", string(data1))
	}

	// Verify sub/nested.txt
	data2, err := os.ReadFile(filepath.Join(tempDir, "sub", "nested.txt"))
	if err != nil {
		t.Fatalf("failed to read copied nested file: %v", err)
	}
	if string(data2) != "nested hello" {
		t.Errorf("expected 'nested hello', got %q", string(data2))
	}

	// Test error scenario
	err = CopyRefToDir(ctx, ref, "/nonexistent", tempDir)
	if err == nil {
		t.Error("expected error copying nonexistent path, got nil")
	}
}
