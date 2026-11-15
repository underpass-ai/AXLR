package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMCPConfigTypeDiagnostic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcp-type.json")
	if err := os.WriteFile(path, []byte(`{"version":"oops","plugins":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadMCPConfiguration(path, os.Getenv)
	if err == nil {
		t.Fatal("expected invalid configuration to be rejected")
	}
	for _, want := range []string{"mcp-type.json", "version", "int"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("diagnostic %q does not contain %q", err, want)
		}
	}
}
