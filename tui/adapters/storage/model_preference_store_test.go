package storage

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
)

func TestModelPreferenceStoreRoundTripAndPermissions(t *testing.T) {
	base := filepath.Join(t.TempDir(), "axlr")
	store, err := NewModelPreferenceStore(base)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := store.Load(context.Background()); err != nil || got != "" {
		t.Fatalf("missing preference: %q %v", got, err)
	}
	model := root.ModelID("provider/chosen")
	if err := store.Save(context.Background(), model); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load(context.Background())
	if err != nil || got != model {
		t.Fatalf("loaded %q: %v", got, err)
	}
	path := filepath.Join(base, "model-preference.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "test-secret-key") || strings.Contains(string(data), "api_key") {
		t.Fatal("preference contains credentials")
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("preference permissions: %v %v", info, err)
	}
	info, err = os.Stat(base)
	if err != nil || info.Mode().Perm() != 0700 {
		t.Fatalf("directory permissions: %v %v", info, err)
	}
}

func TestModelPreferenceStoreRejectsCorruptAndInvalidModel(t *testing.T) {
	base := filepath.Join(t.TempDir(), "axlr")
	store, err := NewModelPreferenceStore(base)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(context.Background(), "bad\nmodel"); err == nil {
		t.Fatal("invalid model saved")
	}
	if err := os.WriteFile(filepath.Join(base, "model-preference.json"), []byte(`{"version":1,"model":"bad\nmodel"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(context.Background()); err == nil {
		t.Fatal("corrupt model accepted")
	}
	if err := os.WriteFile(filepath.Join(base, "model-preference.json"), []byte(`{"version":999,"model":"valid"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(context.Background()); err == nil {
		t.Fatal("unsupported version accepted")
	}
}

func TestModelPreferenceStoreRejectsOversizedModelWithoutReplacingDefault(t *testing.T) {
	base := filepath.Join(t.TempDir(), "axlr")
	store, err := NewModelPreferenceStore(base)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(context.Background(), "provider/default"); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(context.Background(), root.ModelID("provider/"+strings.Repeat("x", maxModelPreferenceBytes))); err == nil {
		t.Fatal("oversized preference accepted")
	}
	got, err := store.Load(context.Background())
	if err != nil || got != "provider/default" {
		t.Fatalf("previous default changed: %q %v", got, err)
	}
}

func TestModelPreferenceStoreRejectsSymlinkFileAndDirectory(t *testing.T) {
	base := filepath.Join(t.TempDir(), "axlr")
	other := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.Mkdir(other, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, base); err != nil {
		t.Fatal(err)
	}
	if _, err := NewModelPreferenceStore(base); err == nil {
		t.Fatal("symlink directory accepted")
	}
	base = filepath.Join(t.TempDir(), "axlr")
	store, err := NewModelPreferenceStore(base)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(base, "model-preference.json")
	if err := os.Symlink(filepath.Join(other, "secret"), path); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(context.Background()); err == nil {
		t.Fatal("symlink file loaded")
	}
	if err := store.Save(context.Background(), "valid"); err == nil {
		t.Fatal("symlink file replaced")
	}
}
