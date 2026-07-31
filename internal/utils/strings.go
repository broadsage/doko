package utils

import (
	"strings"
)

// Substitute replaces all occurrences of the keys in vars map with their values in s.
func Substitute(s string, vars map[string]string) string {
	out := s
	for k, v := range vars {
		out = strings.ReplaceAll(out, k, v)
	}
	return out
}

// SubstituteRecursive recursively replaces variables until a fixed point is reached or maxIterations is hit.
func SubstituteRecursive(s string, vars map[string]string, maxIterations int) string {
	out := s
	for range maxIterations {
		changed := false
		for k, v := range vars {
			replaced := Substitute(v, vars)
			if replaced != v {
				changed = true
				vars[k] = replaced
			}
		}
		replacedOut := Substitute(out, vars)
		if replacedOut != out {
			out = replacedOut
			changed = true
		}
		if !changed {
			break
		}
	}
	return out
}
