package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/adapters/storage"
	"github.com/underpass-ai/AXLR/tui/adapters/terminal"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func exitConsole(m tea.Model) error {
	app := m.(terminal.AppModel)
	app.Composer.Input.SetValue("/exit")
	_, cmd := app.Update(terminal.ControlIntent("send"))
	if cmd == nil {
		return nil
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		panic("/exit did not quit")
	}
	return nil
}

// On 10 October 2026 /exit left only the log, diagnostics and settings
// paths: resuming needed a 32-hex session id the console never printed.
func TestExitPrintsHowToResumeASessionWithMessages(t *testing.T) {
	env := cliEnv(t)
	workspace := t.TempDir()
	ctx := context.Background()
	store, _ := storage.New(filepath.Join(env["XDG_STATE_HOME"], "axlr", "sessions"))
	s, _ := domain.NewSession("0123456789abcdef0123456789abcdef", domain.Workspace(canonicalWorkspace(t, workspace)), "test/model")
	if err := s.BeginTurn(root.Text("resume me"), nil); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(ctx, s); err != nil {
		t.Fatal(err)
	}
	store.Close()
	var out bytes.Buffer
	if code := run(ctx, []string{"--root", workspace, "--session", string(s.Export().ID)}, func(k string) string { return env[k] }, exitConsole, &out); code != 0 {
		t.Fatalf("%d %s", code, &out)
	}
	if want := "axlr-tui: resume with: axlr-tui --session 0123456789abcdef0123456789abcdef"; !strings.Contains(out.String(), want) {
		t.Fatalf("no resume hint:\n%s", &out)
	}
}

func TestExitOfAnEmptySessionPrintsNoResumeHint(t *testing.T) {
	env := cliEnv(t)
	var out bytes.Buffer
	if code := run(context.Background(), []string{"--root", t.TempDir(), "--model", "test/model"}, func(k string) string { return env[k] }, exitConsole, &out); code != 0 {
		t.Fatalf("%d %s", code, &out)
	}
	if strings.Contains(out.String(), "resume with") {
		t.Fatalf("an empty session printed a resume hint:\n%s", &out)
	}
}
