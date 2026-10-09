package storage

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/underpass-ai/AXLR/tui/application"
)

func forgedTool(name string) application.ForgedTool {
	return application.ForgedTool{Name: name, Description: "d", InputSchema: json.RawMessage(`{"type":"object"}`), Program: "sh", Args: []string{".axlr/tools/" + name + "/run.sh"}}
}

func TestForgedToolStoreRegistersReplacesAndVerifies(t *testing.T) {
	workspace := t.TempDir()
	store := &ForgedToolStore{Dir: t.TempDir(), Now: func() time.Time { return time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC) }}
	ctx := context.Background()
	stored, replaced, err := store.Forge(ctx, workspace, forgedTool("echo_it"), []application.ForgedFile{{Path: "run.sh", Content: "cat"}, {Path: "lib/old.sh", Content: "old"}})
	if err != nil || replaced || stored.ForgedAt != "2026-10-09T12:00:00Z" || len(stored.Files) != 2 {
		t.Fatalf("forge = %+v, %v, %v", stored, replaced, err)
	}
	if err := store.Verify(ctx, workspace, stored); err != nil {
		t.Fatal(err)
	}
	// A new version replaces the directory: the old file is gone.
	again, replaced, err := store.Forge(ctx, workspace, forgedTool("echo_it"), []application.ForgedFile{{Path: "run.sh", Content: "cat -n"}})
	if err != nil || !replaced {
		t.Fatalf("replace = %v, %v", replaced, err)
	}
	if _, err := os.Stat(filepath.Join(workspace, ".axlr", "tools", "echo_it", "lib", "old.sh")); !os.IsNotExist(err) {
		t.Fatalf("old file kept: %v", err)
	}
	if store.Verify(ctx, workspace, stored) == nil {
		t.Fatal("the previous version still verifies")
	}
	listed, err := store.List(ctx, workspace)
	if err != nil || len(listed) != 1 || listed[0].Files["run.sh"] != again.Files["run.sh"] {
		t.Fatalf("list = %+v, %v", listed, err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".axlr", "tools", "echo_it", "run.sh"), []byte("curl evil"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := store.Verify(ctx, workspace, again); err == nil || !strings.Contains(err.Error(), "changed since it was forged") {
		t.Fatalf("verify = %v", err)
	}
	if err := os.Remove(filepath.Join(workspace, ".axlr", "tools", "echo_it", "run.sh")); err != nil {
		t.Fatal(err)
	}
	if err := store.Verify(ctx, workspace, again); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("verify = %v", err)
	}
}

// Only Forge registers a tool: a directory planted beside the tools, or a
// registry that arrives with a checkout, is not one.
func TestForgedToolStoreDoesNotScanDirectories(t *testing.T) {
	workspace := t.TempDir()
	dir := filepath.Join(workspace, ".axlr", "tools", "planted")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "run.sh"), []byte("echo planted"), 0o644); err != nil {
		t.Fatal(err)
	}
	checkout := `{"version":1,"workspace":"` + workspace + `","tools":[{"name":"planted","description":"d","input_schema":{"type":"object"},"program":"sh","args":[".axlr/tools/planted/run.sh"],"files":{"run.sh":"x"}}]}`
	if err := os.WriteFile(filepath.Join(workspace, ".axlr", "tools", "registry.json"), []byte(checkout), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := NewForgedToolStore(filepath.Join(t.TempDir(), "forged"))
	if err != nil {
		t.Fatal(err)
	}
	listed, err := store.List(context.Background(), workspace)
	if err != nil || len(listed) != 0 {
		t.Fatalf("list = %+v, %v", listed, err)
	}
}

func TestForgedToolStoreKeepsAPrivateRegistryPerWorkspace(t *testing.T) {
	store, err := NewForgedToolStore(filepath.Join(t.TempDir(), "forged"))
	if err != nil {
		t.Fatal(err)
	}
	first, second := t.TempDir(), t.TempDir()
	if _, _, err := store.Forge(context.Background(), first, forgedTool("only_here"), []application.ForgedFile{{Path: "run.sh", Content: "x"}}); err != nil {
		t.Fatal(err)
	}
	if listed, _ := store.List(context.Background(), second); len(listed) != 0 {
		t.Fatalf("another workspace sees %+v", listed)
	}
	info, err := os.Stat(store.registry(first))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("registry %v, %v", info, err)
	}
	// A damaged registry, or one of another workspace, is refused, not read.
	for _, text := range []string{`{"version":2,"workspace":"` + first + `","tools":[]}`, `{"version":1,"workspace":"` + second + `","tools":[]}`} {
		if err := os.WriteFile(store.registry(first), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := store.List(context.Background(), first); err == nil {
			t.Fatalf("read %s", text)
		}
	}
	if _, err := store.List(context.Background(), "relative"); err == nil {
		t.Fatal("a relative workspace was opened")
	}
	if _, err := NewForgedToolStore("relative"); err == nil {
		t.Fatal("a relative state directory was accepted")
	}
}

func TestForgedToolStoreDoesNotWriteThroughASymlinkOutOfTheWorkspace(t *testing.T) {
	outside, linked := t.TempDir(), t.TempDir()
	if err := os.Symlink(outside, filepath.Join(linked, ".axlr")); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	store := &ForgedToolStore{Dir: t.TempDir()}
	if _, _, err := store.Forge(context.Background(), linked, forgedTool("escape"), []application.ForgedFile{{Path: "run.sh", Content: "x"}}); err == nil {
		t.Fatal("forged through a symlink out of the workspace")
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Fatalf("wrote outside the workspace: %v", entries)
	}
}

func TestForgedToolsSetting(t *testing.T) {
	if !DefaultUserSettings().ForgedToolsEnabled() {
		t.Fatal("forged tools are off by default")
	}
	var parsed UserSettings
	if err := json.Unmarshal([]byte(`{"forged_tools":false}`), &parsed); err != nil {
		t.Fatal(err)
	}
	if err := parsed.Validate(); err != nil || parsed.ForgedToolsEnabled() || len(parsed.Extra) != 0 {
		t.Fatalf("parsed = %+v, %v", parsed, err)
	}
	encoded, err := json.Marshal(parsed)
	if err != nil || !strings.Contains(string(encoded), `"forged_tools":false`) {
		t.Fatalf("encoded %s: %v", encoded, err)
	}
}
