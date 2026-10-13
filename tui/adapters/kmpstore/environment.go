package kmpstore

import (
	"sort"
	"strings"
)

// Value reads one variable from a KEY=VALUE environment.
func Value(env []string, key string) string {
	for _, entry := range env {
		if name, value, ok := strings.Cut(entry, "="); ok && name == key {
			return value
		}
	}
	return ""
}

// WithStore pins the store in a KEY=VALUE environment and selects the
// embedded backend unless one is set; the result is sorted, as mcp.json's
// loader sorts.
func WithStore(env []string, path string) []string {
	out := make([]string, 0, len(env)+2)
	backend := false
	for _, entry := range env {
		name, _, _ := strings.Cut(entry, "=")
		if name == DataDirVariable {
			continue
		}
		if name == BackendVariable {
			backend = true
		}
		out = append(out, entry)
	}
	out = append(out, DataDirVariable+"="+path)
	if !backend {
		out = append(out, BackendVariable+"=embedded")
	}
	sort.Strings(out)
	return out
}
