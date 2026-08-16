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
	"github.com/broadsage/doko/internal/signature"
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
		case "lint":
			if err := runLint(os.Args[2:]); err != nil {
				fmt.Fprintf(os.Stderr, "Lint Error: %v\n", err)
				os.Exit(1)
			}
			os.Exit(0)
		case "sign":
			if err := runSign(os.Args[2:]); err != nil {
				fmt.Fprintf(os.Stderr, "Sign Error: %v\n", err)
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

func runLint(args []string) error {
	filePath := "doko.yaml"
	if len(args) > 0 {
		filePath = args[0]
	}
	filePath = filepath.Clean(filePath)
	spec, err := config.ParseFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to parse config file: %w", err)
	}

	result, err := config.Lint(appcontext.Context(), spec)
	if err != nil {
		return fmt.Errorf("linting failed: %w", err)
	}

	for _, w := range result.Warnings {
		fmt.Fprintf(os.Stderr, "WARNING: %s\n", w)
	}
	for _, e := range result.Errors {
		fmt.Fprintf(os.Stderr, "ERROR: %s\n", e)
	}

	if len(result.Errors) > 0 {
		return fmt.Errorf("security policies violated (%d errors, %d warnings)", len(result.Errors), len(result.Warnings))
	}

	if len(result.Warnings) > 0 {
		fmt.Printf("Linting passed with %d warning(s).\n", len(result.Warnings))
	} else {
		fmt.Println("Success: Config complies with all security policies.")
	}
	return nil
}

func runSign(args []string) error {
	var imageRef string
	var keyPath string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--key", "-k":
			if i+1 < len(args) {
				keyPath = args[i+1]
				i++
			}
		default:
			imageRef = args[i]
		}
	}

	if imageRef == "" {
		return fmt.Errorf("missing target image reference to sign. Usage: doko sign <image-ref> [--key <key-path>]")
	}

	return signature.SignImage(imageRef, keyPath)
}
