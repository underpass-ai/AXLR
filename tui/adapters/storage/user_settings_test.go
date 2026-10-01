package storage

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/underpass-ai/AXLR/tui/domain"
)

func TestUserSettingsCanBeEditedAndSelectorsPreserveOtherSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "axlr", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"model":"provider/hand-edited","language":"es","theme":"paper","icons":"ascii","reduce_motion":true,"approvals":{"autonomous":false,"future_policy":"keep"},"future":{"new_setting":42}}`), 0600); err != nil {
		t.Fatal(err)
	}
	store, err := NewUserSettingsStore(path, DefaultUserSettings())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	loaded, err := store.Load(ctx)
	if err != nil || loaded.Model != "provider/hand-edited" || loaded.Language != "es" || loaded.UIPreferences() != (domain.UIPreferences{Theme: domain.ThemePaper, Icons: domain.IconsASCII, ReduceMotion: true}) {
		t.Fatalf("loaded = %+v, %v", loaded, err)
	}
	if err := store.ModelPreference().Save(ctx, "provider/picked"); err != nil {
		t.Fatal(err)
	}
	if err := store.UIPreference().Save(ctx, domain.UIPreferences{Theme: domain.ThemeInk, Icons: domain.IconsSafe}); err != nil {
		t.Fatal(err)
	}
	loaded, err = store.Load(ctx)
	if err != nil || loaded.Model != "provider/picked" || loaded.Language != "es" || loaded.Theme != "ink" || loaded.Icons != "safe" || loaded.ReduceMotion {
		t.Fatalf("saved = %+v, %v", loaded, err)
	}
	info, err := os.Stat(path)
	if err != nil || runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatalf("settings mode = %v, %v", info, err)
	}
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), `"new_setting": 42`) || !strings.Contains(string(data), `"future_policy": "keep"`) {
		t.Fatalf("future setting lost: %q, %v", data, err)
	}
}

func TestUserSettingsPartialJSONUsesDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"theme":"aurora"}`), 0600); err != nil {
		t.Fatal(err)
	}
	store, _ := NewUserSettingsStore(path, DefaultUserSettings())
	got, err := store.Load(context.Background())
	if err != nil || got.Theme != "aurora" || got.Language != "en" || got.Icons != "safe" || got.Model != "" {
		t.Fatalf("partial settings = %+v, %v", got, err)
	}
}

func TestUserSettingsRejectInvalidFileWithoutOverwritingIt(t *testing.T) {
	for _, content := range []string{
		`{"theme":"unknown"}`,
		`{"language":"fr"}`,
		`{"model":"   "}`,
		`{"reduce_motion":"yes"}`,
		`{"approvals":{"allowed":[{"Kind":"host","LocalOperation":"tools"}]}}`,
		`null`,
		`{"theme":"ink"} {"theme":"paper"}`,
	} {
		t.Run(content, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "settings.json")
			if err := os.WriteFile(path, []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
			store, _ := NewUserSettingsStore(path, DefaultUserSettings())
			if _, err := store.Load(context.Background()); err == nil {
				t.Fatal("invalid settings accepted")
			}
			if err := store.ModelPreference().Save(context.Background(), "provider/new"); err == nil {
				t.Fatal("invalid settings overwritten")
			}
			data, err := os.ReadFile(path)
			if err != nil || string(data) != content {
				t.Fatalf("settings changed: %q, %v", data, err)
			}
		})
	}
}

func TestUserSettingsRejectSymlinkAndWorldWritableFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	target := filepath.Join(dir, "target.json")
	if err := os.WriteFile(target, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	store, _ := NewUserSettingsStore(path, DefaultUserSettings())
	if _, err := store.Load(context.Background()); err == nil {
		t.Fatal("symlink accepted")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		return // permission bits are synthetic there; ACLs govern access
	}
	if err := os.WriteFile(path, []byte(`{}`), 0666); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0666); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(context.Background()); err == nil || !strings.Contains(err.Error(), "non-writable-by-others") {
		t.Fatalf("world-writable settings accepted: %v", err)
	}
}

func TestUserSettingsConcurrentSelectorsKeepBothChanges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	first, _ := NewUserSettingsStore(path, DefaultUserSettings())
	second, _ := NewUserSettingsStore(path, DefaultUserSettings())
	var wg sync.WaitGroup
	errors := make(chan error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		errors <- first.ModelPreference().Save(context.Background(), "provider/new")
	}()
	go func() {
		defer wg.Done()
		errors <- second.UIPreference().Save(context.Background(), domain.UIPreferences{Theme: domain.ThemePaper, Icons: domain.IconsSafe})
	}()
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	got, err := first.Load(context.Background())
	if err != nil || got.Model != "provider/new" || got.Theme != "paper" {
		t.Fatalf("concurrent saves lost a setting: %+v, %v", got, err)
	}
}
