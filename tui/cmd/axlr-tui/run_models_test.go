package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/underpass-ai/AXLR/tui/adapters/terminal"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// settings.json's models reach the request OpenRouter receives for that
// model.
func TestRunSendsConfiguredModelOptionsToOpenRouter(t *testing.T) {
	env := cliEnv(t)
	writeSettings(t, env, `{"models":{"test/model":{"provider":{"sort":"throughput","ignore":["openinference"]},"max_tokens":4096}}}`)
	previous := http.DefaultTransport
	defer func() { http.DefaultTransport = previous }()
	var sent []byte
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		sent, _ = io.ReadAll(r.Body)
		chunk := `{"choices":[{"index":0,"delta":{"content":"done"},"finish_reason":"stop"}]}`
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("data: " + chunk + "\n\ndata: [DONE]\n\n")), Header: make(http.Header)}, nil
	})
	var out bytes.Buffer
	code := run(context.Background(), []string{"--root", t.TempDir(), "--model", "test/model"}, func(k string) string { return env[k] }, func(m tea.Model) error {
		a := m.(terminal.AppModel)
		a.Composer.Input.SetValue("hello")
		m, cmd := a.Update(terminal.ControlIntent("send"))
		if a = drain(t, m, cmd); a.Header.State.Status != domain.StatusComplete {
			t.Fatalf("status = %+v", a.Status)
		}
		return nil
	}, &out)
	if code != 0 {
		t.Fatalf("code=%d output=%s", code, &out)
	}
	for _, want := range []string{`"provider":{"sort":"throughput","ignore":["openinference"]}`, `"max_tokens":4096`} {
		if !bytes.Contains(sent, []byte(want)) {
			t.Fatalf("request lacks %s: %s", want, sent)
		}
	}
}

func TestRunRefusesInvalidModelOptionsBeforeLaunch(t *testing.T) {
	env := cliEnv(t)
	writeSettings(t, env, `{"models":{"test/model":{"provider":{"sort":"cheapest"}}}}`)
	var out bytes.Buffer
	code := run(context.Background(), []string{"--root", t.TempDir(), "--model", "test/model"}, func(k string) string { return env[k] }, func(tea.Model) error {
		t.Fatal("invalid settings launched")
		return nil
	}, &out)
	if code == 0 || !strings.Contains(out.String(), "settings models test/model provider.sort must be") {
		t.Fatalf("code=%d output=%s", code, &out)
	}
}
