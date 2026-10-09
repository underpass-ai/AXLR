package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/plugins"
	"github.com/underpass-ai/AXLR/tui/adapters/diagnostics"
	"github.com/underpass-ai/AXLR/tui/application"
)

func appLogText(t *testing.T, env map[string]string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(env["XDG_STATE_HOME"], "axlr", "logs", diagnostics.AppLogName))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// A launch leaves its notes, its start and its stop in the app log, which
// `axlr-tui logs` prints; a failed launch logs its error once.
func TestRunKeepsTheConsoleNotesInTheAppLog(t *testing.T) {
	env := cliEnv(t)
	var output bytes.Buffer
	workspace := t.TempDir()
	if code := run(context.Background(), []string{"--root", workspace}, func(key string) string { return env[key] }, func(tea.Model) error { return nil }, &output); code != 0 {
		t.Fatalf("code=%d output=%s", code, &output)
	}
	text := appLogText(t, env)
	for _, want := range []string{"INFO  [", "axlr-tui: app log: ", "axlr-tui: diagnostics: ", "console started: build ", "workspace ", "console stopped"} {
		if !strings.Contains(text, want) {
			t.Fatalf("log lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, env["OPENROUTER_API_KEY"]) {
		t.Fatal("the model key reached the app log")
	}
	if code := run(context.Background(), []string{"--root", filepath.Join(workspace, "missing")}, func(key string) string { return env[key] }, func(tea.Model) error { return nil }, &output); code != 1 {
		t.Fatalf("missing root: code=%d", code)
	}
	if text := appLogText(t, env); strings.Count(text, "ERROR [") != 1 || !strings.Contains(text, "missing") {
		t.Fatalf("failure not logged once:\n%s", text)
	}
	var printed, errs bytes.Buffer
	if code := runLogs([]string{"--level", "error"}, func(key string) string { return env[key] }, &printed, &errs); code != 0 || strings.Count(printed.String(), "\n") != 1 || !strings.Contains(printed.String(), "ERROR [") {
		t.Fatalf("logs: code=%d out=%q err=%q", code, &printed, &errs)
	}
	printed.Reset()
	if code := runLogs([]string{"--path"}, func(key string) string { return env[key] }, &printed, &errs); code != 0 || strings.TrimSpace(printed.String()) != filepath.Join(env["XDG_STATE_HOME"], "axlr", "logs", diagnostics.AppLogName) {
		t.Fatalf("path: code=%d out=%q", code, &printed)
	}
	for _, bad := range [][]string{{"--level", "debug"}, {"--lines", "0"}, {"extra"}} {
		if code := runLogs(bad, func(key string) string { return env[key] }, &printed, &errs); code != 2 {
			t.Fatalf("%v: code=%d", bad, code)
		}
	}
}

func TestConsoleNotesAndPluginEventsReachTheAppLogByPluginID(t *testing.T) {
	log, err := diagnostics.OpenAppLog(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	var terminal bytes.Buffer
	notes := teeConsoleNotes(&terminal, log)
	_, _ = notes.Write([]byte("axlr-tui: settings: /x/settings.json\n"))
	_, _ = notes.Write([]byte("axlr-tui: exec sandbox unavailable (no bwrap): local commands run unsandboxed\n"))
	observer := pluginLog{log: log}
	observer.PluginConnection("kmp", plugins.ConnectFailed, false, errors.New("McpError: Connection closed"))
	observer.PluginConnection("kmp", plugins.Connected, true, nil)
	observer.PluginConnection("made", plugins.ConnectionLost, false, errors.New("EOF"))
	_, _ = observer.PluginStderr("kmp").Write([]byte("panic: store locked\n"))
	if !strings.Contains(terminal.String(), "settings.json") {
		t.Fatal("notes no longer reach the terminal")
	}
	page, err := (appLogReader{log: log}).TailLog(context.Background(), application.AppLogQuery{Lines: 10, Level: "WARN"})
	if err != nil || page.Console != os.Getpid() || len(page.Entries) != 3 {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	for i, want := range []string{"local commands run unsandboxed", "plugin kmp: connection failed, the next call tries again: McpError: Connection closed", "plugin made: connection lost"} {
		if !strings.Contains(page.Entries[i], want) {
			t.Fatalf("entry %d = %q, want %q", i, page.Entries[i], want)
		}
	}
	page, err = (appLogReader{log: log}).TailLog(context.Background(), application.AppLogQuery{Lines: 10, Level: "INFO", Contains: "KMP"})
	if err != nil || len(page.Entries) != 3 || !strings.Contains(page.Entries[1], "plugin kmp: connected to the shared engine") || !strings.Contains(page.Entries[2], "plugin kmp stderr: panic: store locked") {
		t.Fatalf("kmp entries = %+v %v", page.Entries, err)
	}
	if logReader(nil) != nil || failureLogger(nil) != nil {
		t.Fatal("a console without a log offers no axlr_logs")
	}
}

func TestPluginSecretsAreTheValuesOfSecretLookingVariables(t *testing.T) {
	registration, err := plugins.NewRegistration(plugins.Manifest{ID: root.PluginID("kmp"), Command: "/bin/true", AllowTools: []root.PluginToolName{"x"}}, []string{"KMP_API_KEY=abcdefgh-123", "KMP_STORE=/data/store", "GITHUB_TOKEN=ghp_secretvalue"})
	if err != nil {
		t.Fatal(err)
	}
	got := pluginSecrets([]plugins.Registration{registration})
	if len(got) != 2 || got[0] != "abcdefgh-123" || got[1] != "ghp_secretvalue" {
		t.Fatalf("secrets = %v", got)
	}
}
