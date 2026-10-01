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

func TestRunUsesLastSelectedModelUnlessOverridden(t *testing.T) {
	env := cliEnv(t)
	workspace := t.TempDir()
	getenv := func(key string) string { return env[key] }
	preferences, err := storage.NewModelPreferenceStore(filepath.Join(env["XDG_STATE_HOME"], "axlr"))
	if err != nil {
		t.Fatal(err)
	}
	if err := preferences.Save(context.Background(), "provider/selected"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(env["XDG_STATE_HOME"], "axlr", "model-preference.json"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte(env["OPENROUTER_API_KEY"])) || !bytes.Equal(data, []byte(`{"version":1,"model":"provider/selected"}`)) {
		t.Fatalf("unexpected preference contents: %q", data)
	}
	var sessionID string
	for _, tc := range []struct {
		name  string
		args  []string
		model string
	}{
		{"saved-default", nil, "provider/selected"},
		{"explicit-flag", []string{"--model", "provider/explicit"}, "provider/explicit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"--root", workspace}, tc.args...)
			var output bytes.Buffer
			code := run(context.Background(), args, getenv, func(m tea.Model) error {
				state := m.(terminal.AppModel).Header.State
				if string(state.Model) != tc.model {
					t.Fatalf("model=%q, want %q", state.Model, tc.model)
				}
				if tc.name == "explicit-flag" {
					sessionID = string(state.ID)
				}
				return nil
			}, &output)
			if code != 0 {
				t.Fatalf("code=%d: %s", code, &output)
			}
		})
	}
	// Only sessions with a message survive the console closing.
	sessions, err := storage.New(filepath.Join(env["XDG_STATE_HOME"], "axlr", "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	saved, err := domain.NewSession(domain.SessionID(sessionID), domain.Workspace(canonicalWorkspace(t, workspace)), "provider/explicit")
	if err != nil {
		t.Fatal(err)
	}
	if err := saved.BeginTurn("kept", nil); err != nil {
		t.Fatal(err)
	}
	if err := sessions.Save(context.Background(), saved); err != nil {
		t.Fatal(err)
	}
	sessions.Close()
	var output bytes.Buffer
	code := run(context.Background(), []string{"--root", workspace, "--session", sessionID}, getenv, func(m tea.Model) error {
		if got := m.(terminal.AppModel).Header.State.Model; got != "provider/explicit" {
			t.Fatalf("resumed model=%q", got)
		}
		return nil
	}, &output)
	if code != 0 {
		t.Fatalf("resume code=%d: %s", code, &output)
	}
}

func TestRunCorruptModelPreferenceFallsBackWithoutLeakingSecrets(t *testing.T) {
	env := cliEnv(t)
	workspace := t.TempDir()
	base := filepath.Join(env["XDG_STATE_HOME"], "axlr")
	if err := os.MkdirAll(base, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "model-preference.json"), []byte(env["OPENROUTER_API_KEY"]), 0600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	code := run(context.Background(), []string{"--root", workspace}, func(k string) string { return env[k] }, func(m tea.Model) error {
		state := m.(terminal.AppModel).Header.State
		if state.ID != "" || state.Model != "" {
			t.Fatalf("corrupt preference created session: %+v", state)
		}
		return nil
	}, &output)
	if code != 0 || strings.Contains(output.String(), env["OPENROUTER_API_KEY"]) {
		t.Fatalf("code=%d output=%s", code, &output)
	}
	output.Reset()
	code = run(context.Background(), []string{"--root", workspace, "--model", "provider/explicit"}, func(k string) string { return env[k] }, func(m tea.Model) error {
		if got := m.(terminal.AppModel).Header.State.Model; got != "provider/explicit" {
			t.Fatalf("explicit model=%q", got)
		}
		return nil
	}, &output)
	if code != 0 || strings.Contains(output.String(), "ignoring invalid saved model preference") {
		t.Fatalf("explicit flag inspected preference: code=%d output=%s", code, &output)
	}
}
