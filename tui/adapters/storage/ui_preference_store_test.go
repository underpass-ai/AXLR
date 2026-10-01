package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/underpass-ai/AXLR/tui/domain"
)

func TestUIPreferenceStoreRoundTripAndPrivateFile(t *testing.T) {
	store, err := NewUIPreferenceStore(filepath.Join(t.TempDir(), "axlr"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	got, err := store.Load(ctx)
	if err != nil || got != domain.DefaultUIPreferences() {
		t.Fatalf("defaults = %+v, %v", got, err)
	}
	want := domain.UIPreferences{Theme: domain.ThemePaper, Icons: domain.IconsASCII, ReduceMotion: true}
	if err := store.Save(ctx, want); err != nil {
		t.Fatal(err)
	}
	got, err = store.Load(ctx)
	if err != nil || got != want {
		t.Fatalf("saved = %+v, %v", got, err)
	}
	info, err := os.Stat(filepath.Join(store.dir, uiPreferenceFilename))
	if err != nil {
		t.Fatal(err)
	}
	if !testMode(info, 0600) {
		t.Fatalf("file mode = %v", info.Mode())
	}
}

func TestUIPreferenceStoreRejectsInvalidDataWithoutReplacingDefaults(t *testing.T) {
	store, err := NewUIPreferenceStore(filepath.Join(t.TempDir(), "axlr"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(store.dir, uiPreferenceFilename)
	if err := os.WriteFile(path, []byte(`{"version":1,"theme":"unknown","icons":"safe"}`), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load(context.Background())
	if err == nil || got != domain.DefaultUIPreferences() {
		t.Fatalf("invalid preference = %+v, %v", got, err)
	}
	if err := store.Save(context.Background(), domain.UIPreferences{Theme: "unknown", Icons: domain.IconsSafe}); err == nil {
		t.Fatal("invalid theme saved")
	}
}
