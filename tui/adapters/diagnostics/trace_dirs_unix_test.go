//go:build !windows

package diagnostics

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// On 10 October 2026 an XDG_STATE_HOME under a group-writable ancestor
// stopped the console with "diagnostic directory is writable by another
// account", which named neither the directory, its mode nor the fix.
func TestUnsafeDiagnosticDirectoryErrorNamesPathModeAndFix(t *testing.T) {
	for _, tc := range []struct {
		name, mode, fix string
		prepare         func(t *testing.T, parent string) (base, offending string)
	}{
		{"group-writable ancestor", "0775", "chmod go-w", func(t *testing.T, parent string) (string, string) {
			if err := os.Chmod(parent, 0o775); err != nil {
				t.Fatal(err)
			}
			base := filepath.Join(parent, "state")
			if err := os.Mkdir(base, 0o700); err != nil {
				t.Fatal(err)
			}
			return base, parent
		}},
		{"public state base", "0777", "chmod go-w", func(t *testing.T, parent string) (string, string) {
			base := filepath.Join(parent, "state")
			if err := os.Mkdir(base, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(base, 0o777); err != nil {
				t.Fatal(err)
			}
			return base, base
		}},
		{"public logs", "0777", "chmod 700", func(t *testing.T, parent string) (string, string) {
			logs := filepath.Join(parent, "axlr", "logs")
			if err := os.MkdirAll(logs, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(logs, 0o777); err != nil {
				t.Fatal(err)
			}
			return parent, logs
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parent := t.TempDir()
			if err := os.Chmod(parent, 0o700); err != nil {
				t.Fatal(err)
			}
			base, offending := tc.prepare(t, parent)
			_, err := DefaultDirectory(func(key string) string {
				if key == "XDG_STATE_HOME" {
					return base
				}
				return ""
			})
			if err == nil {
				t.Fatal("unsafe diagnostic directory accepted")
			}
			// macOS reaches its temporary directories through /var, a link
			// to /private/var; the error names the resolved directory.
			if resolved, err := filepath.EvalSymlinks(offending); err == nil {
				offending = resolved
			}
			for _, want := range []string{offending + " ", "mode " + tc.mode, tc.fix + " " + offending} {
				if !strings.Contains(err.Error()+" ", want) {
					t.Fatalf("error %q does not contain %q", err, want)
				}
			}
		})
	}
}
