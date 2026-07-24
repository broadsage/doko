// Package examples_test validates all example doko.yaml configuration files
// by parsing and validating each one against the doko config schema.
package examples_test

import (
	"path/filepath"
	"testing"

	"github.com/broadsage/doko/internal/config"
)

func TestExampleConfigs(t *testing.T) {
	examples, err := filepath.Glob("*/doko.yaml")
	if err != nil {
		t.Fatalf("failed to glob examples: %v", err)
	}
	if len(examples) == 0 {
		t.Fatal("no example configs found")
	}
	for _, path := range examples {
		t.Run(filepath.Dir(path), func(t *testing.T) {
			if _, err := config.ParseFile(path); err != nil {
				t.Fatalf("parse/validate %s: %v", path, err)
			}
		})
	}
}
