//go:build !windows

package storage

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A lock file another account can read names itself, its mode and the fix.
func TestMCPConfigLockRefusalNamesPathModeAndFix(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcp.json")
	if err := os.WriteFile(path+".lock", nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path+".lock", 0o664); err != nil {
		t.Fatal(err)
	}
	_, err := acquireMCPConfigLock(context.Background(), path)
	if err == nil {
		t.Fatal("a group-writable lock was accepted")
	}
	for _, want := range []string{path + ".lock", "0664", "chmod 600 " + path + ".lock"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not name %q", err, want)
		}
	}
}
