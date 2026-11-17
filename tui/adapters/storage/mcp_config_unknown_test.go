package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMCPConfigUnknownFieldDiagnostic(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"top level", `{"version":1,"plugins":[],"qa_unknown":"private-value"}`, `unknown field "qa_unknown"`},
		{"nested", `{"version":1,"plugins":[{"unexpected":true}]}`, `unknown field "unexpected"`},
		{"syntax", `{"version":1,`, "invalid JSON"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "mcp.json")
			if err := os.WriteFile(path, []byte(tc.body), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := LoadMCPConfiguration(path, func(string) string { return "" })
			if err == nil || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected path and %q in diagnostic, got %v", tc.want, err)
			}
			if strings.Contains(err.Error(), "private-value") {
				t.Fatalf("diagnostic exposed a value: %v", err)
			}
			if tc.name != "syntax" && (strings.Contains(err.Error(), "invalid JSON") || !strings.Contains(err.Error(), "remove")) {
				t.Fatalf("expected actionable schema diagnostic, got %v", err)
			}
		})
	}
}
