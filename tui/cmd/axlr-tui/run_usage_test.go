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
	"github.com/charmbracelet/x/ansi"
	"github.com/underpass-ai/AXLR/tui/adapters/terminal"
)

// max_session_usd reaches the turn and the footer: the cost OpenRouter
// reports lands in the session's ledger beside its snapshot and in the
// footer, and once it reaches the limit the next request is not sent.
func TestRunKeepsTheSessionCostAndItsBudget(t *testing.T) {
	env := cliEnv(t)
	writeSettings(t, env, `{"max_session_usd":0.001}`)
	previous := http.DefaultTransport
	defer func() { http.DefaultTransport = previous }()
	requests := 0
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		_, _ = io.Copy(io.Discard, r.Body)
		chunk := `{"provider":"Relace","choices":[{"index":0,"delta":{"content":"done"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12,"cost":0.002}}`
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("data: " + chunk + "\n\ndata: [DONE]\n\n")), Header: make(http.Header)}, nil
	})
	var out bytes.Buffer
	var id string
	code := run(context.Background(), []string{"--root", t.TempDir(), "--model", "test/model"}, func(k string) string { return env[k] }, func(m tea.Model) error {
		sized, _ := m.(terminal.AppModel).Update(tea.WindowSizeMsg{Width: 140, Height: 30})
		a := sized.(terminal.AppModel)
		send := func(prompt string) {
			a.Composer.Input.SetValue(prompt)
			next, cmd := a.Update(terminal.ControlIntent("send"))
			a = drain(t, next, cmd)
		}
		send("hello")
		if view := ansi.Strip(a.View().Content); !strings.Contains(view, "$0.0020 turn · $0.0020 session") {
			t.Fatalf("view:\n%s", view)
		}
		send("again")
		id = string(a.Header.State.ID)
		if !strings.Contains(a.Status.Error, "This session reached its $0.0010 budget (max_session_usd)") || requests != 1 {
			t.Fatalf("requests %d, error %q", requests, a.Status.Error)
		}
		return nil
	}, &out)
	if code != 0 {
		t.Fatalf("code=%d output=%s", code, &out)
	}
	if data, err := os.ReadFile(filepath.Join(env["XDG_STATE_HOME"], "axlr", "sessions", id+".usage.json")); err != nil || !strings.Contains(string(data), `"Relace"`) {
		t.Fatalf("ledger %s %v", data, err)
	}
}
