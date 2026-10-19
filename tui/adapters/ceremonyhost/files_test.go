package ceremonyhost

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/runtime"
	"github.com/underpass-ai/AXLR/tui/adapters/axlr"
	"github.com/underpass-ai/AXLR/tui/application"
)

func TestFilesKeepTheWorkspaceBoundaryAndRoundTripLargeDrafts(t *testing.T) {
	root := t.TempDir()
	executor, err := runtime.New(runtime.Config{Root: root, Env: []string{"PATH=" + os.Getenv("PATH")}})
	if err != nil {
		t.Fatal(err)
	}
	// Windows cannot remove the temporary root while the executor holds it.
	t.Cleanup(func() { _ = executor.Close() })
	files := Files{Tools: axlr.ToolRunner{Executor: executor}}
	ctx := context.Background()
	if err := files.MakeDir(ctx, "docs/incidents"); err != nil {
		t.Fatal(err)
	}
	draft := []byte("# Postmortem\n" + strings.Repeat("línea con evidencia y detalle. ", 2500))
	for range 2 { // create, then replace
		if err := files.Write(ctx, "docs/incidents/x.draft.md", draft); err != nil {
			t.Fatal(err)
		}
	}
	got, found, err := files.Read(ctx, "docs/incidents/x.draft.md", 1<<20)
	if err != nil || !found || string(got) != string(draft) {
		t.Fatalf("read back %d of %d bytes, found=%v err=%v", len(got), len(draft), found, err)
	}
	if _, found, err := files.Read(ctx, "docs/incidents/missing.md", 1); err != nil || found {
		t.Fatalf("missing file: found=%v err=%v", found, err)
	}
	if _, _, err := files.Read(ctx, "../outside.md", 1); err == nil {
		t.Fatal("read outside the workspace")
	}
	if _, err := os.Stat(filepath.Join(root, "docs/incidents/x.draft.md")); err != nil {
		t.Fatal(err)
	}
}

// The runtime refuses a binary file with invalid_text; on 20 October 2026
// that code was dropped and a plan task blocked on an untracked .pyc. The
// console now tells it apart, keeping the runtime's message.
func TestFilesReportABinaryFileAsNotText(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "calc.pyc"), []byte("\xcb\x0d\x0d\x0a\x00\x00\x00\x00\xe3"), 0o644); err != nil {
		t.Fatal(err)
	}
	executor, err := runtime.New(runtime.Config{Root: root, Env: []string{"PATH=" + os.Getenv("PATH")}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = executor.Close() })
	_, found, err := Files{Tools: axlr.ToolRunner{Executor: executor}}.Read(context.Background(), "calc.pyc", 1<<20)
	if found || !errors.Is(err, application.ErrNotText) || !strings.Contains(err.Error(), "calc.pyc is not UTF-8 text") {
		t.Fatalf("found=%v err=%v, want ErrNotText with the runtime's message", found, err)
	}
}
