package storage

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func TestSessionLabelsSetLoadAndRemove(t *testing.T) {
	path := filepath.Join(t.TempDir(), "axlr", "session-labels.json")
	store, err := NewSessionLabelStore(path)
	must(t, err)
	ctx := context.Background()
	id := domain.SessionID("0123456789abcdef0123456789abcdef")
	must(t, store.Set(ctx, id, domain.SessionLabel{Title: "Revisión de ceremonias", Archived: true}))
	labels, err := store.Load(ctx)
	must(t, err)
	if got := labels[id]; got.Title != "Revisión de ceremonias" || !got.Archived {
		t.Fatalf("label = %+v", got)
	}
	if info, err := os.Stat(path); err != nil || runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatalf("labels file not private: %v", err)
	}
	must(t, store.Set(ctx, id, domain.SessionLabel{}))
	labels, err = store.Load(ctx)
	must(t, err)
	if _, ok := labels[id]; ok {
		t.Fatal("empty label was kept")
	}
	if err := store.Set(ctx, id, domain.SessionLabel{Title: root.Text("x" + strings.Repeat("y", domain.MaxSessionTitleRunes))}); err == nil {
		t.Fatal("over-long title accepted")
	}
	if err := store.Set(ctx, "not-an-id", domain.SessionLabel{Title: "x"}); err == nil {
		t.Fatal("invalid session id accepted")
	}
}
