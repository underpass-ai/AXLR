package storage

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// An export is owner-only, creates its directory and never replaces a file.
func TestExportFilesCreateOnlyNewPrivateFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".axlr", "exports", "session.md")
	if err := (ExportFiles{}).Create(context.Background(), path, []byte("# one")); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatalf("export file %v %v", info, err)
	}
	if err := (ExportFiles{}).Create(context.Background(), path, []byte("# two")); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("overwrote: %v", err)
	}
	if data, _ := os.ReadFile(path); string(data) != "# one" {
		t.Fatalf("content %q", data)
	}
	if err := (ExportFiles{}).Create(context.Background(), "relative.md", nil); err == nil {
		t.Fatal("a relative path was accepted")
	}
}
