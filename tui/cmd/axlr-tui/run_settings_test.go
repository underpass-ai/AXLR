package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/underpass-ai/AXLR/tui/adapters/storage"
	"github.com/underpass-ai/AXLR/tui/adapters/terminal"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func TestRunUsesEditableSettingsJSONAndExplicitOverrides(t *testing.T) {
	env := cliEnv(t)
	env["AXLR_LANG"] = ""
	workspace := t.TempDir()
	path := filepath.Join(env["HOME"], ".config", "axlr", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"model":"provider/from-json","language":"es","theme":"paper","icons":"ascii","reduce_motion":true,"approvals":{"autonomous":true,"allowed":[]}}`), 0600); err != nil {
		t.Fatal(err)
	}
	getenv := func(key string) string { return env[key] }
	for _, tc := range []struct {
		name     string
		args     []string
		model    string
		language terminal.Locale
	}{
		{"settings", nil, "provider/from-json", terminal.Spanish},
		{"flags", []string{"--model", "provider/from-flag", "--lang", "en"}, "provider/from-flag", terminal.English},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"--root", workspace}, tc.args...)
			var output bytes.Buffer
			code := run(context.Background(), args, getenv, func(model tea.Model) error {
				app := model.(terminal.AppModel)
				if string(app.Header.State.Model) != tc.model || app.Theme.Locale != tc.language || app.UIPreferences != (domain.UIPreferences{Theme: domain.ThemePaper, Icons: domain.IconsASCII, ReduceMotion: true}) || !app.Status.Autonomous {
					t.Fatalf("settings not applied: model=%q locale=%q UI=%+v autonomy=%v", app.Header.State.Model, app.Theme.Locale, app.UIPreferences, app.Status.Autonomous)
				}
				return nil
			}, &output)
			if code != 0 {
				t.Fatalf("code=%d output=%s", code, &output)
			}
		})
	}
	data, err := os.ReadFile(path)
	if err != nil || !bytes.Contains(data, []byte(`"provider/from-json"`)) {
		t.Fatalf("flags modified settings: %q, %v", data, err)
	}
}

func TestRunRejectsInvalidSettingsJSONBeforeLaunch(t *testing.T) {
	env := cliEnv(t)
	path := filepath.Join(env["HOME"], ".config", "axlr", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"theme":"broken"}`), 0600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	code := run(context.Background(), []string{"--root", t.TempDir()}, func(key string) string { return env[key] }, func(tea.Model) error {
		t.Fatal("invalid settings launched")
		return nil
	}, &output)
	if code == 0 || !strings.Contains(output.String(), "unknown TUI theme") {
		t.Fatalf("code=%d output=%s", code, &output)
	}
}

func TestRunReadsLegacyPreferencesUntilSettingsJSONIsSaved(t *testing.T) {
	env := cliEnv(t)
	legacy := filepath.Join(env["XDG_STATE_HOME"], "axlr")
	modelStore, err := storage.NewModelPreferenceStore(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := modelStore.Save(context.Background(), "provider/legacy"); err != nil {
		t.Fatal(err)
	}
	uiStore, err := storage.NewUIPreferenceStore(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := uiStore.Save(context.Background(), domain.UIPreferences{Theme: domain.ThemeInk, Icons: domain.IconsNerd, ReduceMotion: true}); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	code := run(context.Background(), []string{"--root", t.TempDir()}, func(key string) string { return env[key] }, func(model tea.Model) error {
		app := model.(terminal.AppModel)
		if string(app.Header.State.Model) != "provider/legacy" || app.UIPreferences.Theme != domain.ThemeInk || app.UIPreferences.Icons != domain.IconsNerd || !app.UIPreferences.ReduceMotion {
			t.Fatalf("legacy preferences lost: model=%q UI=%+v", app.Header.State.Model, app.UIPreferences)
		}
		next, _ := app.Update(terminal.ControlIntent("theme"))
		next, _ = next.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		if next.(terminal.AppModel).Status.Error != "" {
			t.Fatalf("theme save failed: %s", next.(terminal.AppModel).Status.Error)
		}
		return nil
	}, &output)
	if code != 0 {
		t.Fatalf("code=%d output=%s", code, &output)
	}
	path := filepath.Join(env["HOME"], ".config", "axlr", "settings.json")
	data, err := os.ReadFile(path)
	if err != nil || !bytes.Contains(data, []byte(`"model": "provider/legacy"`)) || !bytes.Contains(data, []byte(`"theme": "ink"`)) {
		t.Fatalf("legacy preferences not carried into settings: %q, %v", data, err)
	}
}
