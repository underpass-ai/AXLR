//go:build !windows

package main

import "testing"

// Without an absolute folder in PATH, commands run with the usual system
// folders; otherwise with the inherited ones alone, and nothing beyond PATH
// and HOME.
func TestLocalRuntimeEnvironmentDefaultsThePath(t *testing.T) {
	for raw, want := range map[string]string{"": "PATH=/usr/local/bin:/usr/bin:/bin", "relative:bin": "PATH=/usr/local/bin:/usr/bin:/bin", "/opt/bin:/usr/bin": "PATH=/opt/bin:/usr/bin"} {
		env := localRuntimeEnvironment(func(name string) string {
			if name == "PATH" {
				return raw
			}
			return ""
		})
		if len(env) != 1 || env[0] != want {
			t.Fatalf("PATH %q: %v", raw, env)
		}
	}
}
