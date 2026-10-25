package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// A workspace file is hashed where it is; one outside the workspace, through
// a link out of it, not regular or past the bound is refused.
func TestDeliverablesHashAWorkspaceFile(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "out"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "out", "report.md"), []byte("# Report\n"), 0644); err != nil {
		t.Fatal(err)
	}
	file, err := Deliverables{}.DeliverFile(context.Background(), workspace, "out/report.md")
	sum := sha256.Sum256([]byte("# Report\n"))
	if err != nil || file.Size != 9 || file.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("%+v %v", file, err)
	}
	for path, contains := range map[string]string{"missing.md": "cannot be delivered", "out": "not a regular file", "../x": "relative to the workspace"} {
		if _, err := (Deliverables{}).DeliverFile(context.Background(), workspace, path); err == nil || !strings.Contains(err.Error(), contains) {
			t.Fatalf("%s: %v", path, err)
		}
	}
	big := filepath.Join(workspace, "big.bin")
	handle, err := os.Create(big)
	if err != nil {
		t.Fatal(err)
	}
	if err := handle.Truncate(domain.MaxDeliverableFileBytes + 1); err != nil {
		t.Fatal(err)
	}
	handle.Close()
	if _, err := (Deliverables{}).DeliverFile(context.Background(), workspace, "big.bin"); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("%v", err)
	}
	outside := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(outside, []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(workspace, "link")); err != nil {
		t.Skipf("no symlinks here: %v", err)
	}
	if _, err := (Deliverables{}).DeliverFile(context.Background(), workspace, "link"); err == nil || !strings.Contains(err.Error(), "leaves the workspace") {
		t.Fatalf("%v", err)
	}
}

// Inline content is written under the deliverables folder, which ignores
// itself in git; a second delivery of the name replaces the first, and a
// link planted on the way is refused.
func TestDeliverablesWriteInlineContent(t *testing.T) {
	workspace := t.TempDir()
	target := domain.DeliverablesDirectory + "/0123456789abcdef0123456789abcdef/plan.md"
	file, err := Deliverables{}.WriteDeliverable(context.Background(), workspace, target, []byte("first"))
	if err != nil || file.Size != 5 {
		t.Fatalf("%+v %v", file, err)
	}
	file, err = Deliverables{}.WriteDeliverable(context.Background(), workspace, target, []byte("second"))
	sum := sha256.Sum256([]byte("second"))
	if err != nil || file.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("%+v %v", file, err)
	}
	if data, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(target))); err != nil || string(data) != "second" {
		t.Fatalf("%q %v", data, err)
	}
	if data, err := os.ReadFile(filepath.Join(workspace, ".axlr", "deliverables", ".gitignore")); err != nil || string(data) != "*\n" {
		t.Fatalf("%q %v", data, err)
	}
	if _, err := (Deliverables{}).WriteDeliverable(context.Background(), workspace, "notes.md", []byte("x")); err == nil {
		t.Fatal("inline content written outside the deliverables folder")
	}
	planted := t.TempDir()
	elsewhere := t.TempDir()
	if err := os.Symlink(elsewhere, filepath.Join(planted, ".axlr")); err != nil {
		t.Skipf("no symlinks here: %v", err)
	}
	if _, err := (Deliverables{}).WriteDeliverable(context.Background(), planted, target, []byte("x")); err == nil || !strings.Contains(err.Error(), "not a folder of the workspace") {
		t.Fatalf("%v", err)
	}
	if entries, _ := os.ReadDir(elsewhere); len(entries) != 0 {
		t.Fatalf("wrote through the link: %v", entries)
	}
}
