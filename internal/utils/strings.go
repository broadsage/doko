package utils

import (
	"sort"
	"strings"
)

// Substitute replaces all occurrences of the keys in vars map with their values in s.
func Substitute(s string, vars map[string]string) string {
	out := s
	keys := make([]string, 0, len(vars))
	for k := range vars {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i] > keys[j]
	})
	for _, k := range keys {
		out = strings.ReplaceAll(out, k, vars[k])
	}
	return out
}

// SubstituteRecursive recursively replaces variables until a fixed point is reached or maxIterations is hit.
func SubstituteRecursive(s string, vars map[string]string, maxIterations int) string {
	out := s
	// Gather and sort keys to guarantee deterministic order of substitutions
	keys := make([]string, 0, len(vars))
	for k := range vars {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i] > keys[j]
	})

	for range maxIterations {
		changed := false
		for _, k := range keys {
			v := vars[k]
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
