package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/dto"
	"github.com/underpass-ai/AXLR/runtime"
	"github.com/underpass-ai/AXLR/tui/adapters/storage"
	"github.com/underpass-ai/AXLR/tui/adapters/terminal"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func canonicalWorkspace(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func TestTUIExecutorResolvesCommandsWithoutForwardingCredentials(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("POSIX shell scenario; native process execution is covered by runtime tests")
	}
	env := map[string]string{"PATH": "/usr/bin:/bin", "OPENROUTER_API_KEY": "private-key"}
	executor, err := runtime.New(runtime.Config{Root: t.TempDir(), Env: localRuntimeEnvironment(func(name string) string { return env[name] })})
	if err != nil {
		t.Fatal(err)
	}
	defer executor.Close()
	result := executor.Execute(context.Background(), dto.Request{ProtocolVersion: 1, RequestID: "path-test", Tool: "exec", Arguments: json.RawMessage(`{"program":"sh","args":["-c","printf path-ok; test -z \"$OPENROUTER_API_KEY\""]}`)})
	output, ok := result.Output.(dto.ExecOutput)
	if result.Status != "completed" || result.Error != nil || !ok || output.ExitCode != 0 || output.Stdout != "path-ok" {
		t.Fatalf("basic shell command unavailable or credential forwarded: %+v", result)
	}
}

func TestTUIExecutorUsesHostHome(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("POSIX shell scenario; native process execution is covered by runtime tests")
	}
	home := t.TempDir()
	env := map[string]string{"PATH": "/usr/bin:/bin", "HOME": home}
	executor, err := runtime.New(runtime.Config{Root: t.TempDir(), Env: localRuntimeEnvironment(func(name string) string { return env[name] })})
	if err != nil {
		t.Fatal(err)
	}
	defer executor.Close()
	result := executor.Execute(context.Background(), dto.Request{ProtocolVersion: 1, RequestID: "home-test", Tool: "exec", Arguments: json.RawMessage(`{"program":"sh","args":["-c","printf %s \"$HOME\""]}`)})
	output, ok := result.Output.(dto.ExecOutput)
	if result.Status != "completed" || !ok || output.Stdout != home {
		t.Fatalf("exec did not receive the host HOME: %+v", result)
	}
}

func TestLocalRuntimeEnvironmentSkipsRelativeHome(t *testing.T) {
	env := map[string]string{"PATH": "/usr/bin", "HOME": "relative/home"}
	for _, value := range localRuntimeEnvironment(func(name string) string { return env[name] }) {
		if strings.HasPrefix(value, "HOME=") {
			t.Fatalf("relative HOME forwarded: %q", value)
		}
	}
}

func cliEnv(t *testing.T) map[string]string {
	t.Helper()
	state, home := t.TempDir(), t.TempDir()
	for _, path := range []string{state, home} {
		if err := os.Chmod(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return map[string]string{"OPENROUTER_API_KEY": "test-key-never-print", "XDG_STATE_HOME": state, "HOME": home}
}
func TestRunRejectsInvalidConfigurationBeforeLaunch(t *testing.T) {
	rootDir := t.TempDir()
	for _, tc := range []struct {
		name  string
		args  []string
		noKey bool
	}{
		{"key", []string{"--root", rootDir, "--model", "test/model"}, true},
		{"bare-key", nil, true},
		{"bare-manifest", []string{"--plugin", filepath.Join(rootDir, "missing.json")}, false},
		{"unknown", []string{"--unknown"}, false}, {"positional", []string{"--root", rootDir, "--model", "test/model", "extra"}, false},
		{"language", []string{"--lang", "fr"}, false},
		{"missing-root", []string{"--root", filepath.Join(rootDir, "absent"), "--model", "test/model"}, false},
		{"manifest", []string{"--root", rootDir, "--model", "test/model", "--plugin", filepath.Join(rootDir, "missing.json")}, false},
		{"env-syntax", []string{"--root", rootDir, "--model", "test/model", "--plugin-env-from", "broken"}, false},
		{"env-unknown", []string{"--root", rootDir, "--model", "test/model", "--plugin-env-from", "unknown:KEY=HOST"}, false},
		{"session", []string{"--root", rootDir, "--model", "test/model", "--session", "../bad"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := cliEnv(t)
			if tc.noKey {
				delete(env, "OPENROUTER_API_KEY")
			}
			var out bytes.Buffer
			called := false
			code := run(context.Background(), tc.args, func(k string) string { return env[k] }, func(tea.Model) error { called = true; return nil }, &out)
			if code == 0 || called || out.Len() == 0 || strings.Contains(out.String(), "test-key-never-print") {
				t.Fatalf("code=%d called=%v output=%s", code, called, &out)
			}
		})
	}
}

func TestRunLanguageFlagOverridesEnvironment(t *testing.T) {
	for _, test := range []struct {
		name string
		env  string
		flag string
		want terminal.Locale
	}{
		{"default", "", "", terminal.English},
		{"environment", "es_ES.UTF-8", "", terminal.Spanish},
		{"flag", "es", "en", terminal.English},
	} {
		t.Run(test.name, func(t *testing.T) {
			env := cliEnv(t)
			env["AXLR_LANG"] = test.env
			args := []string{"--root", t.TempDir(), "--model", "test/model"}
			if test.flag != "" {
				args = append(args, "--lang", test.flag)
			}
			var output bytes.Buffer
			code := run(context.Background(), args, func(k string) string { return env[k] }, func(model tea.Model) error {
				app := model.(terminal.AppModel)
				if app.Theme.Locale != test.want {
					t.Fatalf("locale=%q, want %q", app.Theme.Locale, test.want)
				}
				return nil
			}, &output)
			if code != 0 {
				t.Fatalf("run failed: %s", &output)
			}
		})
	}
}
func TestRunDiscardsAnEmptySessionAndReleasesItsLock(t *testing.T) {
	env := cliEnv(t)
	workspace := t.TempDir()
	var id domain.SessionID
	var out bytes.Buffer
	code := run(context.Background(), []string{"--root", workspace, "--model", "test/model"}, func(k string) string { return env[k] }, func(m tea.Model) error {
		state := m.(terminal.AppModel).Header.State
		id = state.ID
		if state.Workspace != domain.Workspace(canonicalWorkspace(t, workspace)) || state.Model != "test/model" || len(id) != 32 {
			t.Fatalf("%+v", state)
		}
		return nil
	}, &out)
	if code != 0 {
		t.Fatalf("%d %s", code, &out)
	}
	dir := filepath.Join(env["XDG_STATE_HOME"], "axlr", "sessions")
	for _, name := range []string{string(id) + ".json", string(id) + ".lock"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Fatalf("empty session left %s behind: %v", name, err)
		}
	}
	// The lock is released: a new store can save under the same ID.
	store, err := storage.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	reused, err := domain.NewSession(id, domain.Workspace(canonicalWorkspace(t, workspace)), "test/model")
	if err != nil {
		t.Fatal(err)
	}
	if err := reused.BeginTurn("hello", nil); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(context.Background(), reused); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, string(id)+".json"))
	if err != nil || bytes.Contains(data, []byte(env["OPENROUTER_API_KEY"])) {
		t.Fatalf("snapshot error: %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func drain(t *testing.T, m tea.Model, cmd tea.Cmd) terminal.AppModel {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for m.(terminal.AppModel).Busy {
		if cmd == nil {
			t.Fatal("busy without command")
		}
		result := make(chan tea.Msg, 1)
		go func() {
			message := cmd()
			// Dispatch the operation reader in Bubble Tea batches; timer ticks
			// are exercised separately by terminal tests and the PTY harness.
			for {
				batch, ok := message.(tea.BatchMsg)
				if !ok {
					break
				}
				message = batch[0]()
			}
			result <- message
		}()
		select {
		case msg := <-result:
			m, cmd = m.Update(msg)
		case <-deadline:
			t.Fatal("operation stalled")
		}
	}
	return m.(terminal.AppModel)
}
func TestRunResumeRequiresExplicitContinuationAndUsesConfiguredAgent(t *testing.T) {
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
	previous := http.DefaultTransport
	defer func() { http.DefaultTransport = previous }()
	calls := 0
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		var body struct {
			Model    string
			Messages []struct{ Content string }
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Model != "test/model" || len(body.Messages) != 2 || body.Messages[1].Content != "resume me" || r.Header.Get("Authorization") != "Bearer test-key-never-print" {
			t.Fatalf("wrong request %+v", body)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"resumed\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")), Header: make(http.Header)}, nil
	})
	var out bytes.Buffer
	code := run(ctx, []string{"--root", workspace, "--session", string(s.Export().ID)}, func(k string) string { return env[k] }, func(m tea.Model) error {
		if calls != 0 || m.(terminal.AppModel).Header.State.Status != domain.StatusInterrupted {
			t.Fatal("auto resume")
		}
		m, cmd := m.Update(terminal.ControlIntent("continue"))
		final := drain(t, m, cmd)
		if final.Status.Error != "" || final.Header.State.Status != domain.StatusComplete || calls != 1 {
			t.Fatalf("%+v calls=%d", final.Status, calls)
		}
		return nil
	}, &out)
	if code != 0 {
		t.Fatalf("%d %s", code, &out)
	}
}
func TestRunPluginEnvironmentIsExplicit(t *testing.T) {
	env := cliEnv(t)
	env["CHOSEN"] = "selected"
	env["UNSELECTED_SECRET"] = "must-not-leak"
	t.Setenv("UNSELECTED_SECRET", "must-not-leak")
	t.Setenv("OPENROUTER_API_KEY", env["OPENROUTER_API_KEY"])
	workspace := t.TempDir()
	log := filepath.Join(workspace, "environment")
	manifest := filepath.Join(workspace, "plugin.json")
	data, _ := json.Marshal(map[string]any{"manifest_version": 1, "id": "probe", "command": os.Args[0], "args": []string{"-test.run=^TestPluginEnvironmentHelper$"}, "allow_tools": []string{"probe"}})
	os.WriteFile(manifest, data, 0600)
	env["LOG"] = log
	var out bytes.Buffer
	code := run(context.Background(), []string{"--root", workspace, "--model", "test/model", "--plugin", manifest, "--plugin-env-from", "probe:SELECTED=CHOSEN", "--plugin-env-from", "probe:OUTPUT=LOG"}, func(k string) string { return env[k] }, func(m tea.Model) error {
		a := m.(terminal.AppModel)
		a.Composer.Input.SetValue("discover")
		m, cmd := a.Update(terminal.ControlIntent("send"))
		final := drain(t, m, cmd)
		if final.Status.Error == "" {
			t.Fatal("exiting plugin should fail discovery")
		}
		return nil
	}, &out)
	if code != 0 {
		t.Fatalf("%d %s", code, &out)
	}
	got, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(got, []byte("SELECTED=selected")) || bytes.Contains(got, []byte("UNSELECTED_SECRET")) || bytes.Contains(got, []byte("OPENROUTER_API_KEY")) {
		t.Fatalf("unsafe env %s", got)
	}
}

func TestPluginEnvironmentHelper(t *testing.T) {
	path := os.Getenv("OUTPUT")
	if path == "" {
		return
	}
	if err := os.WriteFile(path, []byte(strings.Join(os.Environ(), "\n")), 0600); err != nil {
		os.Exit(2)
	}
	os.Exit(0)
}
func TestRunCancellationAndLaunchFailure(t *testing.T) {
	for _, cancelled := range []bool{true, false} {
		t.Run(map[bool]string{true: "cancel", false: "failure"}[cancelled], func(t *testing.T) {
			env := cliEnv(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if cancelled {
				cancel()
			}
			var out bytes.Buffer
			code := run(ctx, []string{"--root", t.TempDir(), "--model", "test/model"}, func(k string) string { return env[k] }, func(tea.Model) error { return errors.New("launch failed") }, &out)
			if cancelled && code != 0 || !cancelled && code == 0 {
				t.Fatalf("%d %s", code, &out)
			}
		})
	}
}

func TestRunLaunchFailureWaitsForCancelledStreamAndPersistsInterruption(t *testing.T) {
	env := cliEnv(t)
	workspace := t.TempDir()
	started := make(chan struct{})
	stopped := make(chan struct{})
	var id domain.SessionID
	previous := http.DefaultTransport
	defer func() { http.DefaultTransport = previous }()
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		close(started)
		<-r.Context().Done()
		close(stopped)
		return nil, r.Context().Err()
	})
	var out bytes.Buffer
	code := run(context.Background(), []string{"--root", workspace, "--model", "test/model"}, func(k string) string { return env[k] }, func(m tea.Model) error {
		a := m.(terminal.AppModel)
		id = a.Header.State.ID
		a.Composer.Input.SetValue("keep my prompt")
		_, cmd := a.Update(terminal.ControlIntent("send"))
		go cmd()
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("stream did not start")
		}
		return errors.New("terminal failed")
	}, &out)
	if code == 0 {
		t.Fatal("launch error lost")
	}
	select {
	case <-stopped:
	default:
		t.Fatal("stream still running")
	}
	store, err := storage.New(filepath.Join(env["XDG_STATE_HOME"], "axlr", "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	s, err := store.Load(context.Background(), id)
	if err != nil || s.Status() != domain.StatusInterrupted || len(s.Messages()) != 1 {
		t.Fatalf("%+v %v", s.Export(), err)
	}
}

func TestRunStartsApprovesLocalReadAndContinues(t *testing.T) {
	env := cliEnv(t)
	workspace := t.TempDir()
	os.WriteFile(filepath.Join(workspace, "note"), []byte("local content"), 0600)
	previous := http.DefaultTransport
	defer func() { http.DefaultTransport = previous }()
	calls := 0
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		data, _ := io.ReadAll(r.Body)
		var chunk string
		if calls == 1 {
			if !bytes.Contains(data, []byte("local_read")) {
				t.Error("local tools not advertised")
			}
			chunk = `{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"read-1","type":"function","function":{"name":"local_read","arguments":"{\"path\":\"note\"}"}}]},"finish_reason":"tool_calls"}]}`
		} else {
			if !bytes.Contains(data, []byte("local content")) || !bytes.Contains(data, []byte("read-1")) {
				t.Errorf("missing correlated result: %s", data)
			}
			chunk = `{"choices":[{"index":0,"delta":{"content":"done"},"finish_reason":"stop"}]}`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("data: " + chunk + "\n\ndata: [DONE]\n\n")), Header: make(http.Header)}, nil
	})
	var out bytes.Buffer
	code := run(context.Background(), []string{"--root", workspace, "--model", "test/model"}, func(k string) string { return env[k] }, func(m tea.Model) error {
		a := m.(terminal.AppModel)
		a.Composer.Input.SetValue("read note")
		m, cmd := a.Update(terminal.ControlIntent("send"))
		a = drain(t, m, cmd)
		if a.Header.State.Status != domain.StatusApproval || calls != 1 {
			t.Fatalf("approval missing: %+v", a.Status)
		}
		m, cmd = a.Update(terminal.ControlIntent("approve"))
		a = drain(t, m, cmd)
		if a.Status.Error != "" || a.Header.State.Status != domain.StatusComplete || calls != 2 {
			t.Fatalf("%+v calls=%d", a.Status, calls)
		}
		return nil
	}, &out)
	if code != 0 {
		t.Fatalf("%d %s", code, &out)
	}
}

func TestRunRejectsResumeMismatchAndAllowsHelp(t *testing.T) {
	env := cliEnv(t)
	workspace := t.TempDir()
	var id domain.SessionID
	getenv := func(k string) string { return env[k] }
	var out bytes.Buffer
	if code := run(context.Background(), []string{"--root", workspace, "--model", "one"}, getenv, func(m tea.Model) error { id = m.(terminal.AppModel).Header.State.ID; return nil }, &out); code != 0 {
		t.Fatal(out.String())
	}
	if code := run(context.Background(), []string{"--root", workspace, "--model", "two", "--session", string(id)}, getenv, func(tea.Model) error { t.Fatal("mismatched session launched"); return nil }, &out); code == 0 {
		t.Fatal("model mismatch accepted")
	}
	if code := run(context.Background(), []string{"--help"}, getenv, func(tea.Model) error { t.Fatal("help launched"); return nil }, &out); code != 0 || !strings.Contains(out.String(), "plugin-env-from") {
		t.Fatalf("%d %s", code, &out)
	}
}
