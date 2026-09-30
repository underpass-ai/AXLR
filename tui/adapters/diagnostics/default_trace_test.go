package diagnostics

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/underpass-ai/AXLR/tui/application"
)

func TestOpenDefaultPrivateUniqueTraces(t *testing.T) {
	for _, source := range []string{"XDG_STATE_HOME", "HOME"} {
		t.Run(source, func(t *testing.T) {
			root := t.TempDir()
			if err := os.Chmod(root, 0o700); err != nil {
				t.Fatal(err)
			}
			lookup := func(key string) string {
				if key == source {
					return root
				}
				return ""
			}
			logger, first, err := OpenDefault(lookup)
			if err != nil {
				t.Fatal(err)
			}
			if err := logger.Record(application.DiagnosticEvent{Stage: application.DiagnosticStartup}); err != nil {
				t.Fatal(err)
			}
			if err := logger.Close(); err != nil {
				t.Fatal(err)
			}
			secondLogger, second, err := OpenDefault(lookup)
			if err != nil {
				t.Fatal(err)
			}
			defer secondLogger.Close()
			if first == second {
				t.Fatal("launches reused the same trace")
			}
			for path, mode := range map[string]os.FileMode{first: 0o600, filepath.Dir(first): 0o700, filepath.Dir(filepath.Dir(first)): 0o700} {
				info, err := os.Stat(path)
				if err != nil || info.Mode().Perm() != mode {
					t.Fatalf("permissions %s: %v %v", path, info, err)
				}
			}
		})
	}
}

func TestOpenDefaultRejectsUnsafeDirectoriesAndMissingEnvironment(t *testing.T) {
	for _, kind := range []string{"nil", "missing", "relative", "base-symlink", "nested-symlink", "file", "public-root", "public-logs"} {
		t.Run(kind, func(t *testing.T) {
			base := t.TempDir()
			if err := os.Chmod(base, 0o700); err != nil {
				t.Fatal(err)
			}
			var lookup func(string) string
			if kind != "nil" {
				lookup = func(key string) string {
					if key == "XDG_STATE_HOME" {
						return base
					}
					return ""
				}
			}
			switch kind {
			case "missing":
				base = ""
			case "relative":
				base = "relative"
			case "base-symlink":
				link := filepath.Join(t.TempDir(), "state")
				if err := os.Symlink(base, link); err != nil {
					t.Fatal(err)
				}
				base = link
			case "nested-symlink":
				if err := os.Symlink(t.TempDir(), filepath.Join(base, "axlr")); err != nil {
					t.Fatal(err)
				}
			case "file":
				if err := os.WriteFile(filepath.Join(base, "axlr"), nil, 0o600); err != nil {
					t.Fatal(err)
				}
			case "public-root":
				if err := os.Chmod(base, 0o777); err != nil {
					t.Fatal(err)
				}
			case "public-logs":
				if err := os.MkdirAll(filepath.Join(base, "axlr", "logs"), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(filepath.Join(base, "axlr", "logs"), 0o777); err != nil {
					t.Fatal(err)
				}
			}
			if logger, _, err := OpenDefault(lookup); err == nil {
				logger.Close()
				t.Fatal("unsafe diagnostic directory accepted")
			}
		})
	}
}
