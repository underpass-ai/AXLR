package storage

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
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

func TestConcurrentInitialTitleCannotOverwriteUserLabel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "labels.json")
	a, err := NewSessionLabelStore(path)
	must(t, err)
	b, err := NewSessionLabelStore(path)
	must(t, err)
	id := domain.SessionID("0123456789abcdef0123456789abcdef")
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if err := a.Set(context.Background(), id, domain.SessionLabel{Title: "Manual", Archived: true, About: "project:AXLR"}); err != nil {
			t.Error(err)
		}
	}()
	go func() {
		defer wg.Done()
		if _, err := b.Initialize(context.Background(), id, domain.SessionLabel{Title: "Automatic", About: "project:other"}); err != nil {
			t.Error(err)
		}
	}()
	wg.Wait()
	got, err := a.Load(context.Background())
	must(t, err)
	if got[id] != (domain.SessionLabel{Title: "Manual", Archived: true, About: "project:AXLR"}) {
		t.Fatalf("lost manual label: %+v", got[id])
	}
}

func TestStalePickerLabelPreservesNewMemoryScope(t *testing.T) {
	store, err := NewSessionLabelStore(filepath.Join(t.TempDir(), "labels.json"))
	must(t, err)
	id := domain.SessionID("0123456789abcdef0123456789abcdef")
	_, err = store.Initialize(context.Background(), id, domain.SessionLabel{Title: "Automatic", About: "project:AXLR"})
	must(t, err)
	must(t, store.Set(context.Background(), id, domain.SessionLabel{Title: "Manual", Archived: true}))
	got, err := store.Load(context.Background())
	must(t, err)
	if got[id] != (domain.SessionLabel{Title: "Manual", Archived: true, About: "project:AXLR"}) {
		t.Fatalf("picker erased memory scope: %+v", got[id])
	}
	must(t, store.Set(context.Background(), id, domain.SessionLabel{}))
	got, err = store.Load(context.Background())
	must(t, err)
	if got[id] != (domain.SessionLabel{About: "project:AXLR"}) {
		t.Fatalf("clearing title erased memory scope: %+v", got[id])
	}
}

// Forget removes a label whole, about included, which Set cannot do.
func TestForgetRemovesALabelWithItsAbout(t *testing.T) {
	store, err := NewSessionLabelStore(filepath.Join(t.TempDir(), "axlr", "session-labels.json"))
	must(t, err)
	ctx := context.Background()
	id := domain.SessionID("0123456789abcdef0123456789abcdef")
	if _, err := store.Initialize(ctx, id, domain.SessionLabel{About: "project:AXLR"}); err != nil {
		t.Fatal(err)
	}
	must(t, store.Set(ctx, id, domain.SessionLabel{}))
	labels, err := store.Load(ctx)
	must(t, err)
	if labels[id].About != "project:AXLR" {
		t.Fatalf("Set must keep the about: %+v", labels[id])
	}
	must(t, store.Forget(ctx, id))
	must(t, store.Forget(ctx, id))
	labels, err = store.Load(ctx)
	must(t, err)
	if _, ok := labels[id]; ok {
		t.Fatalf("label kept: %+v", labels[id])
	}
	if err := store.Forget(ctx, "not-an-id"); err == nil {
		t.Fatal("invalid session id accepted")
	}
}

// On 14 October 2026 axlr_session returned the console's derived title
// unchanged and the model said the console kept its own. A derived title
// yields to the model's or the person's, after a restart too; theirs never
// yields to a derived one; a file written before the mark still loads.
func TestADerivedTitleYieldsToAGivenOne(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "labels.json")
	id := domain.SessionID("0123456789abcdef0123456789abcdef")
	must(t, os.WriteFile(path, []byte(`{"version":1,"labels":{"`+string(id)+`":{"about":"project:AXLR"}}}`), 0o600))
	store, err := NewSessionLabelStore(path)
	must(t, err)
	if _, err := store.Load(ctx); err != nil {
		t.Fatalf("an older file does not load: %v", err)
	}
	if _, err := store.Initialize(ctx, id, domain.SessionLabel{Title: "uno · dos", TitleDerived: true}); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewSessionLabelStore(path)
	must(t, err)
	got, err := reopened.Initialize(ctx, id, domain.SessionLabel{Title: "calc: bugfix con errores y tests"})
	must(t, err)
	if got != (domain.SessionLabel{Title: "calc: bugfix con errores y tests", About: "project:AXLR"}) {
		t.Fatalf("the model's title did not replace the derived one: %+v", got)
	}
	if got, _ := reopened.Initialize(ctx, id, domain.SessionLabel{Title: "otro · derivado", TitleDerived: true}); got.Title != "calc: bugfix con errores y tests" || got.TitleDerived {
		t.Fatalf("a derived title replaced the model's: %+v", got)
	}
	if got, _ := reopened.Initialize(ctx, id, domain.SessionLabel{Title: "segundo del modelo"}); got.Title != "calc: bugfix con errores y tests" {
		t.Fatalf("a second title replaced the model's: %+v", got)
	}
	labels, err := reopened.Load(ctx)
	must(t, err)
	if labels[id].Title != "calc: bugfix con errores y tests" || labels[id].TitleDerived {
		t.Fatalf("stored %+v", labels[id])
	}
}

func TestSessionLabelsIgnoreFieldsOfANewerVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "axlr", "session-labels.json")
	must(t, os.MkdirAll(filepath.Dir(path), 0700))
	id := "0123456789abcdef0123456789abcdef"
	must(t, os.WriteFile(path, []byte(`{"version":1,"labels":{"`+id+`":{"title":"Plan","future_field":true}},"later":1}`), 0600))
	store, err := NewSessionLabelStore(path)
	must(t, err)
	labels, err := store.Load(context.Background())
	must(t, err)
	if got := labels[domain.SessionID(id)]; got.Title != "Plan" {
		t.Fatalf("label = %+v", got)
	}
}
