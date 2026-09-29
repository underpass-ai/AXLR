package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/underpass-ai/AXLR/tui/adapters/terminal"
)

func TestRunModelSelectionBareAndCatalog(t *testing.T) {
	for _, mode := range []string{"quit", "select", "failure", "direct"} {
		t.Run(mode, func(t *testing.T) {
			env := cliEnv(t)
			workspace := t.TempDir()
			t.Chdir(workspace)
			previous := http.DefaultTransport
			defer func() { http.DefaultTransport = previous }()
			calls := 0
			http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.Method != "GET" || r.URL.Path != "/api/v1/models" || r.Header.Get("Authorization") != "Bearer "+env["OPENROUTER_API_KEY"] {
					t.Errorf("unexpected provider request: %s %s", r.Method, r.URL.Path)
				}
				status := 200
				body := `{"data":[{"id":"provider/chosen","name":"Chosen","architecture":{"output_modalities":["text"]},"supported_parameters":["tools"]}]}`
				if mode == "failure" {
					status = 503
					body = env["OPENROUTER_API_KEY"]
				}
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
			})
			var out bytes.Buffer
			var args []string
			if mode == "direct" {
				args = []string{"--model", "provider/direct"}
			}
			code := run(context.Background(), args, func(k string) string { return env[k] }, func(m tea.Model) error {
				resized, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
				a := resized.(terminal.AppModel)
				if calls != 0 || string(a.Header.State.Workspace) != workspace {
					t.Fatalf("startup: %+v calls=%d", a.Header.State, calls)
				}
				if mode == "direct" {
					if a.Header.State.Model != "provider/direct" {
						t.Fatal("direct model lost")
					}
					return nil
				}
				if a.Header.State.ID != "" || a.Header.State.Model != "" {
					t.Fatal("bare launch created session")
				}
				a.Composer.Input.SetValue("draft")
				n, cmd := a.Update(terminal.ControlIntent("send"))
				a = n.(terminal.AppModel)
				if cmd != nil || calls != 0 || a.Composer.Input.Value() != "draft" {
					t.Fatal("unconfigured prompt sent")
				}
				if mode == "quit" {
					return nil
				}
				a.Composer.Input.SetValue("/model")
				n, cmd = a.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
				a = drain(t, n, cmd)
				if calls != 1 {
					t.Fatalf("catalog calls=%d", calls)
				}
				if mode == "failure" {
					if !strings.Contains(a.View().Content, "503") || strings.Contains(a.View().Content, env["OPENROUTER_API_KEY"]) || a.Header.State.ID != "" {
						t.Fatal("unsafe failure state")
					}
					return nil
				}
				n, cmd = a.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
				a = drain(t, n, cmd)
				if a.Header.State.Model != "provider/chosen" || len(a.Header.State.ID) != 32 || calls != 1 || len(a.Header.State.Messages) != 0 {
					t.Fatalf("selection: %+v calls=%d", a.Header.State, calls)
				}
				data, err := os.ReadFile(filepath.Join(env["XDG_STATE_HOME"], "axlr", "sessions", string(a.Header.State.ID)+".json"))
				if err != nil || bytes.Contains(data, []byte(env["OPENROUTER_API_KEY"])) {
					t.Fatalf("snapshot: %v", err)
				}
				return nil
			}, &out)
			if code != 0 {
				t.Fatalf("code=%d %s", code, &out)
			}
			if mode == "quit" || mode == "failure" {
				entries, err := os.ReadDir(filepath.Join(env["XDG_STATE_HOME"], "axlr", "sessions"))
				if err != nil || len(entries) != 0 {
					t.Fatalf("bare startup left files: %v %v", entries, err)
				}
			}
		})
	}
}

func TestRunModelSelectionResumeWorkspaceMismatch(t *testing.T) {
	env := cliEnv(t)
	var id string
	var out bytes.Buffer
	getenv := func(k string) string { return env[k] }
	if run(context.Background(), []string{"--root", t.TempDir(), "--model", "chosen"}, getenv, func(m tea.Model) error { id = string(m.(terminal.AppModel).Header.State.ID); return nil }, &out) != 0 {
		t.Fatal(out.String())
	}
	if run(context.Background(), []string{"--root", t.TempDir(), "--session", id}, getenv, func(tea.Model) error { t.Fatal("mismatch launched"); return nil }, &out) == 0 {
		t.Fatal("workspace mismatch accepted")
	}
}
