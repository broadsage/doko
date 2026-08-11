// Package main is the entrypoint for the Doko BuildKit frontend CLI.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/moby/buildkit/frontend/gateway/grpcclient"
	"github.com/moby/buildkit/util/appcontext"

	"github.com/broadsage/doko/internal/builder"
	"github.com/broadsage/doko/internal/config"
)

func main() {
	if len(os.Args) > 1 {
		cmd := os.Args[1]
		switch cmd {
		case "version", "--version", "-v":
			showVersion()
			os.Exit(0)
		case "init":
			if err := runInit(os.Args[2:]); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			os.Exit(0)
		case "validate":
			if err := runValidate(os.Args[2:]); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			os.Exit(0)
		}
	}

	if err := grpcclient.RunFromEnvironment(appcontext.Context(), builder.Build); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "doko: fatal error: %v\n", err)
		os.Exit(1)
	}
}

func runInit(args []string) error {
	filePath := "doko.yaml"
	force := false
	for _, arg := range args {
		if arg == "-f" || arg == "--force" {
			force = true
		} else {
			filePath = arg
		}
	}
	filePath = filepath.Clean(filePath)

	if _, err := os.Stat(filePath); err == nil && !force {
		return fmt.Errorf("file %s already exists. Use -f or --force to overwrite", filePath)
	}

	template := `# syntax=ghcr.io/broadsage/doko:latest

name: secure-web-app

accounts:
  root: false
  run-as: nonroot
  users:
    - name: nonroot
      uid: 65532
      gid: 65532
  groups:
    - name: nonroot
      gid: 65532
      members:
        - nonroot

contents:
  packages:
    - alpine-baselayout
    - ca-certificates-bundle
  paths:
    - type: directory
      path: /app
      uid: 65532
      gid: 65532
      mode: "0755"

work-dir: /app
entrypoint: ["/bin/sh"]
`
	err := os.WriteFile(filePath, []byte(template), 0o600)
	if err != nil {
		return fmt.Errorf("failed to write %s: %w", filePath, err)
	}
	fmt.Printf("Success: initialized %s successfully!\n", filePath)
	return nil
}

func runValidate(args []string) error {
	filePath := "doko.yaml"
	if len(args) > 0 {
		filePath = args[0]
	}
	filePath = filepath.Clean(filePath)
	_, err := config.ParseFile(filePath)
	if err != nil {
		return fmt.Errorf("validation failed for %s: %w", filePath, err)
	}
	fmt.Printf("Success: %s is valid!\n", filePath)
	return nil
}
