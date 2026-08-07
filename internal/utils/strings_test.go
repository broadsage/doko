package utils

import (
	"testing"
)

func TestSubstitute(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		vars     map[string]string
		expected string
	}{
		{
			name:     "simple substitution",
			input:    "hello ${NAME}",
			vars:     map[string]string{"${NAME}": "world"},
			expected: "hello world",
		},
		{
			name:     "multiple variables",
			input:    "${GREETING} ${NAME}!",
			vars:     map[string]string{"${GREETING}": "Hello", "${NAME}": "Doko"},
			expected: "Hello Doko!",
		},
		{
			name:     "no match",
			input:    "hello ${NAME}",
			vars:     map[string]string{"${OTHER}": "world"},
			expected: "hello ${NAME}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Substitute(tt.input, tt.vars)
			if result != tt.expected {
				t.Errorf("Substitute() = %q, expected %q", result, tt.expected)
			}
		})
	}
}

func TestSubstituteRecursive(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		vars          map[string]string
		maxIterations int
		expected      string
	}{
		{
			name:  "nested variables",
			input: "hello ${WHO}",
			vars: map[string]string{
				"${WHO}":  "${NAME}",
				"${NAME}": "world",
			},
			maxIterations: 5,
			expected:      "hello world",
		},
		{
			name:  "max iterations hit",
			input: "hello ${WHO}",
			vars: map[string]string{
				"${WHO}":  "${NAME}",
				"${NAME}": "world",
			},
			maxIterations: 1,
			expected:      "hello world",
		},
		{
			name:  "circular dependencies",
			input: "hello ${A}",
			vars: map[string]string{
				"${A}": "${B}",
				"${B}": "${A}",
			},
			maxIterations: 5,
			expected:      "hello ${B}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			copiedVars := make(map[string]string)
			for k, v := range tt.vars {
				copiedVars[k] = v
			}
			result := SubstituteRecursive(tt.input, copiedVars, tt.maxIterations)
			if result != tt.expected {
				t.Errorf("SubstituteRecursive() = %q, expected %q", result, tt.expected)
			}
		})
	}
}
