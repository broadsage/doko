package utils

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	gwclient "github.com/moby/buildkit/frontend/gateway/client"
)

// CopyRefToDir recursively copies files and directories from a solved BuildKit reference path to a destination directory on the local disk.
func CopyRefToDir(ctx context.Context, ref gwclient.Reference, refPath, destDir string) error {
	entries, err := ref.ReadDir(ctx, gwclient.ReadDirRequest{Path: refPath})
	if err != nil {
		return fmt.Errorf("read dir %s: %w", refPath, err)
	}
	for _, e := range entries {
		name := e.Path
		if name == "" || name == "." || name == ".." {
			continue
		}
		srcPath := refPath + "/" + name
		dstPath := filepath.Join(destDir, filepath.FromSlash(name))
		if e.Mode&uint32(os.ModeDir) != 0 {
			if err := os.MkdirAll(dstPath, 0o755); err != nil {
				return fmt.Errorf("mkdir %s: %w", dstPath, err)
			}
			if err := CopyRefToDir(ctx, ref, srcPath, dstPath); err != nil {
				return err
			}
			continue
		}
		data, err := ref.ReadFile(ctx, gwclient.ReadRequest{Filename: srcPath})
		if err != nil {
			return fmt.Errorf("read file %s: %w", srcPath, err)
		}
		if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
			return fmt.Errorf("mkdir parent %s: %w", filepath.Dir(dstPath), err)
		}
		mode := e.Mode & uint32(os.ModePerm)
		if mode == 0 {
			mode = 0o644
		}
		if err := os.WriteFile(dstPath, data, os.FileMode(mode)); err != nil {
			return fmt.Errorf("write file %s: %w", dstPath, err)
		}
	}
	return nil
}
