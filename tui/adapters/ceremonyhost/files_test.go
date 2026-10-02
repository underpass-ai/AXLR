package ceremonyhost

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/runtime"
	"github.com/underpass-ai/AXLR/tui/adapters/axlr"
)

func TestFilesKeepTheWorkspaceBoundaryAndRoundTripLargeDrafts(t *testing.T) {
	root := t.TempDir()
	executor, err := runtime.New(runtime.Config{Root: root, Env: []string{"PATH=" + os.Getenv("PATH")}})
	if err != nil {
		t.Fatal(err)
	}
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
