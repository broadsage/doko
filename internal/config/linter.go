// Package config handles the parsing, validation, and structures of the doko.yaml spec file.
package config

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/open-policy-agent/opa/v1/rego"
)

//go:embed policy/security.rego
var securityPolicy string

// LintResult contains lists of lint errors and warnings.
type LintResult struct {
	Errors   []string
	Warnings []string
}

// Lint evaluates the parsed Spec configuration against the embedded security policies using OPA.
func Lint(ctx context.Context, spec *Spec) (*LintResult, error) {
	// Marshal spec to JSON, then unmarshal to map[string]any to prepare input for OPA
	specBytes, err := json.Marshal(spec)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize spec for linting: %w", err)
	}

	var input map[string]any
	if err := json.Unmarshal(specBytes, &input); err != nil {
		return nil, fmt.Errorf("failed to prepare linter input: %w", err)
	}

	// Prepare OPA rego query
	r := rego.New(
		rego.Query("data.doko.security"),
		rego.Module("security.rego", securityPolicy),
	)

	query, err := r.PrepareForEval(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to compile security policy: %w", err)
	}

	results, err := query.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		return nil, fmt.Errorf("policy evaluation error: %w", err)
	}

	res := &LintResult{
		Errors:   []string{},
		Warnings: []string{},
	}

	if len(results) == 0 {
		return res, nil
	}

	if len(results[0].Expressions) > 0 {
		val, ok := results[0].Expressions[0].Value.(map[string]any)
		if ok {
			if errs, exists := val["deny_errors"]; exists {
				if list, ok := errs.([]any); ok {
					for _, e := range list {
						if m, ok := e.(map[string]any); ok {
							id, _ := m["id"].(string)
							msg, _ := m["msg"].(string)
							if id != "" && msg != "" && !slices.Contains(spec.IgnoreRules, id) {
								res.Errors = append(res.Errors, fmt.Sprintf("[%s] %s", id, msg))
							}
						}
					}
				}
			}
			if warns, exists := val["deny_warnings"]; exists {
				if list, ok := warns.([]any); ok {
					for _, w := range list {
						if m, ok := w.(map[string]any); ok {
							id, _ := m["id"].(string)
							msg, _ := m["msg"].(string)
							if id != "" && msg != "" && !slices.Contains(spec.IgnoreRules, id) {
								res.Warnings = append(res.Warnings, fmt.Sprintf("[%s] %s", id, msg))
							}
						}
					}
				}
			}
		}
	}

	return res, nil
}
