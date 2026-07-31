package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/broadsage/doko/internal/config"
	"github.com/google/jsonschema-go/jsonschema"
)

func main() {
	// 1. Generate schema from config.Spec struct via reflection
	schema, err := jsonschema.For[config.Spec](nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to generate schema: %v\n", err)
		os.Exit(1)
	}

	// 2. Add standard JSON Schema draft meta-schemas
	schema.Schema = "http://json-schema.org/draft-07/schema#"

	// 3. Marshal with indentation
	data, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to marshal schema: %v\n", err)
		os.Exit(1)
	}

	// 4. Determine output filepath (handling execution from workspace root vs internal/config)
	outPath := "schema.json"
	if _, err := os.Stat("config.go"); err != nil {
		outPath = filepath.Join("internal", "config", "schema.json")
	}
	if err := os.WriteFile(outPath, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write schema.json at %s: %v\n", outPath, err)
		os.Exit(1)
	}

	fmt.Printf("Successfully generated and wrote JSON Schema to %s\n", outPath)
}
