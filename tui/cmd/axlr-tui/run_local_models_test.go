package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/adapters/localmodels"
	"github.com/underpass-ai/AXLR/tui/adapters/storage"
	"github.com/underpass-ai/AXLR/tui/adapters/terminal"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func writeSettings(t *testing.T, env map[string]string, data string) {
	t.Helper()
	path := filepath.Join(env["HOME"], ".config", "axlr", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestRunStartsWithLocalModelsAndNoOpenRouterKey(t *testing.T) {
	env := cliEnv(t)
	delete(env, "OPENROUTER_API_KEY")
	writeSettings(t, env, `{"model":"local/qwen","local_models":[{"id":"local/qwen","url":"http://127.0.0.1:8080/v1","context_tokens":65536}]}`)
	var output bytes.Buffer
	called := false
	code := run(context.Background(), []string{"--root", t.TempDir()}, func(k string) string { return env[k] }, func(model tea.Model) error {
		called = true
		if app := model.(terminal.AppModel); app.Header.State.Model != "local/qwen" {
			t.Fatalf("model = %q", app.Header.State.Model)
		}
		return nil
	}, &output)
	if code != 0 || !called {
		t.Fatalf("code=%d called=%v output=%s", code, called, &output)
	}
}

func TestRunRefusesLocalModelSetupErrors(t *testing.T) {
	for name, tc := range map[string]struct {
		settings string
		args     []string
		want     string
	}{
		"remote server without key": {
			settings: `{"local_models":[{"id":"local/far","url":"https://models.example.test/v1","api_key_env":"FAR_KEY","context_tokens":65536}]}`,
			want:     "FAR_KEY",
		},
		"remote server over http": {
			settings: `{"local_models":[{"id":"local/far","url":"http://10.1.2.3:8000/v1","context_tokens":65536}]}`,
			want:     "https",
		},
		"local key passed to a plugin": {
			settings: `{"local_models":[{"id":"local/qwen","url":"http://127.0.0.1:8080/v1","api_key_env":"QWEN_KEY","context_tokens":65536}]}`,
			args:     []string{"--plugin-env-from", "tool:TOKEN=QWEN_KEY"},
			want:     "QWEN_KEY cannot be passed to plugins",
		},
	} {
		t.Run(name, func(t *testing.T) {
			env := cliEnv(t)
			delete(env, "OPENROUTER_API_KEY")
			env["QWEN_KEY"] = "qwen-secret-never-print"
			writeSettings(t, env, tc.settings)
			var output bytes.Buffer
			called := false
			code := run(context.Background(), append([]string{"--root", t.TempDir()}, tc.args...), func(k string) string { return env[k] }, func(tea.Model) error { called = true; return nil }, &output)
			if code == 0 || called || !strings.Contains(output.String(), tc.want) || strings.Contains(output.String(), "qwen-secret-never-print") {
				t.Fatalf("code=%d called=%v output=%s", code, called, &output)
			}
		})
	}
}

func TestLocalModelsReachTheModelPickerWithoutOpenRouter(t *testing.T) {
	no := false
	setup := newLocalModelSetup(storage.UserSettings{ContextTokens: 32768, LocalModels: []storage.LocalModel{
		{ID: "local/qwen", Name: "Qwen3.8-27B", URL: "http://127.0.0.1:8080/v1", ContextTokens: 262144},
		{ID: "local/plain", URL: "http://127.0.0.1:8083/v1", ContextTokens: 8192, Tools: &no},
	}}, func(string) string { return "" })
	models, err := application.ListModelsUseCase{Catalog: localmodels.Catalog{Local: setup.available}}.Execute(context.Background())
	if err != nil || len(models) != 1 || models[0].ID != "local/qwen" || models[0].Name != "Qwen3.8-27B" || models[0].Context != 32768 {
		t.Fatalf("picker models = %+v, %v", models, err)
	}
}

func TestRunJevNeedsItsKeyOnlyWhenEnabled(t *testing.T) {
	env := cliEnv(t)
	writeSettings(t, env, `{"jev":{"tool":false,"final_check":false}}`)
	var output bytes.Buffer
	if code := run(context.Background(), []string{"--root", t.TempDir(), "--model", "test/model"}, func(k string) string { return env[k] }, func(tea.Model) error { return nil }, &output); code != 0 || strings.Contains(output.String(), "Jev enabled") {
		t.Fatalf("Jev off: code=%d output=%s", code, &output)
	}
	writeSettings(t, env, `{"jev":{"final_check":true}}`)
	output.Reset()
	if code := run(context.Background(), []string{"--root", t.TempDir(), "--model", "test/model"}, func(k string) string { return env[k] }, func(tea.Model) error { return nil }, &output); code == 0 || !strings.Contains(output.String(), "TYPESAFE_API_KEY") {
		t.Fatalf("Jev on without key: code=%d output=%s", code, &output)
	}
	env["TYPESAFE_API_KEY"] = "typesafe-never-print"
	output.Reset()
	if code := run(context.Background(), []string{"--root", t.TempDir(), "--model", "test/model"}, func(k string) string { return env[k] }, func(tea.Model) error { return nil }, &output); code != 0 || !strings.Contains(output.String(), "Jev enabled") || strings.Contains(output.String(), "typesafe-never-print") {
		t.Fatalf("Jev on: code=%d output=%s", code, &output)
	}
}

func TestCompactProfileFollowsTheSettingAndTheWindow(t *testing.T) {
	windows := localmodels.Windows{Local: map[root.ModelID]domain.ContextWindow{"local/small": 32768, "local/large": 262144}}
	auto := compactProfile("auto", windows)
	if !auto("local/small") || auto("local/large") || auto("openai/gpt-4o") {
		t.Fatal("auto profile does not follow the window")
	}
	if compactProfile("standard", windows) != nil || !compactProfile("compact", windows)("openai/gpt-4o") {
		t.Fatal("explicit profiles ignored")
	}
	capped := compactProfile("auto", localmodels.Windows{Local: windows.Local, Cap: 65536})
	if !capped("local/large") {
		t.Fatal("the global cap does not make a large model compact")
	}
}
