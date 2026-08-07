package pipeline

import (
	"testing"

	"github.com/broadsage/doko/internal/config"
)

func TestNewSubstitutionMap(t *testing.T) {
	spec := &config.BuildSpec{
		Name:        "test-pkg",
		Version:     "1.0.0",
		Epoch:       1,
		Description: "A test package",
		SourceDir:   "/custom-src",
	}

	sm, err := NewSubstitutionMap(spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := map[string]string{
		SubstitutionPackageName:        "test-pkg",
		SubstitutionPackageVersion:     "1.0.0",
		SubstitutionPackageFullVersion: "1.0.0-r1",
		SubstitutionPackageEpoch:       "1",
		SubstitutionPackageDescription: "A test package",
		SubstitutionPackageSrcdir:      "/workspace/build-src/custom-src",
		SubstitutionTargetsOutdir:      "/workspace/build-out",
		SubstitutionTargetsDestdir:     "/workspace/build-out",
		SubstitutionTargetsContextdir:  "/workspace/build-out",
		SubstitutionContextName:        "test-pkg",
	}

	for k, v := range expected {
		got, ok := sm.Substitutions[k]
		if !ok {
			t.Errorf("missing key: %s", k)
		}
		if got != v {
			t.Errorf("key %s: got %q, expected %q", k, got, v)
		}
	}
}

func TestSubstitutionMap_MutateWith(t *testing.T) {
	spec := &config.BuildSpec{
		Name:    "test-pkg",
		Version: "1.0.0",
	}

	sm, err := NewSubstitutionMap(spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	with := map[string]string{
		"prefix":       "/usr",
		"custom-input": "value-${{inputs.prefix}}",
	}

	mutated, err := sm.MutateWith(with)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mutated["${{inputs.prefix}}"] != "/usr" {
		t.Errorf("expected ${{inputs.prefix}} to be /usr, got %q", mutated["${{inputs.prefix}}"])
	}

	if mutated["${{inputs.custom-input}}"] != "value-/usr" {
		t.Errorf("expected ${{inputs.custom-input}} to be value-/usr, got %q", mutated["${{inputs.custom-input}}"])
	}
}
