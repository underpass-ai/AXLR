package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/underpass-ai/AXLR/tui/adapters/terminal"
)

func TestRunTraceRecordsStartupPersistenceAndShutdownWithoutSecrets(t *testing.T) {
	env := cliEnv(t)
	path := filepath.Join(t.TempDir(), "trace.jsonl")
	var output bytes.Buffer
	code := run(context.Background(), []string{"--root", t.TempDir(), "--model", "test/model", "--trace-file", path, "--trace-payloads"}, func(key string) string { return env[key] }, func(tea.Model) error { return nil }, &output)
	if code != 0 {
		t.Fatalf("exit=%d output=%s", code, &output)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, stage := range []string{"startup", "session_save", "shutdown"} {
		if !bytes.Contains(data, []byte(`"stage":"`+stage+`"`)) {
			t.Fatalf("missing %s: %s", stage, data)
		}
	}
	if bytes.Contains(data, []byte(env["OPENROUTER_API_KEY"])) || strings.Contains(string(data), "test/model") {
		t.Fatalf("trace leaked configuration: %s", data)
	}
}

func TestRunTraceRecordsStartupFailureWithoutSecret(t *testing.T) {
	env := cliEnv(t)
	delete(env, "OPENROUTER_API_KEY")
	path := filepath.Join(t.TempDir(), "trace.jsonl")
	var output bytes.Buffer
	code := run(context.Background(), []string{"--root", t.TempDir(), "--trace-file", path}, func(key string) string { return env[key] }, func(tea.Model) error { t.Fatal("unexpected launch"); return nil }, &output)
	if code == 0 {
		t.Fatal("missing key accepted")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, stage := range []string{"startup", "operation_failed", "shutdown"} {
		if !bytes.Contains(data, []byte(`"stage":"`+stage+`"`)) {
			t.Fatalf("missing %s: %s", stage, data)
		}
	}
}

func TestRunDefaultTraceRecordsEveryLaunchAndPrintsItsLocation(t *testing.T) {
	env := cliEnv(t)
	var output bytes.Buffer
	for i := 0; i < 2; i++ {
		code := run(context.Background(), []string{"--root", t.TempDir(), "--model", "test/model"}, func(key string) string { return env[key] }, func(tea.Model) error { return nil }, &output)
		if code != 0 {
			t.Fatalf("exit=%d output=%s", code, &output)
		}
	}
	files, err := filepath.Glob(filepath.Join(env["XDG_STATE_HOME"], "axlr", "logs", "trace-*.jsonl"))
	if err != nil || len(files) != 2 {
		t.Fatalf("files=%v err=%v", files, err)
	}
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil || !bytes.Contains(data, []byte(`"stage":"startup"`)) || !bytes.Contains(data, []byte(`"stage":"shutdown"`)) {
			t.Fatalf("trace=%s err=%v", data, err)
		}
		if !strings.Contains(output.String(), path) || bytes.Contains(data, []byte(env["OPENROUTER_API_KEY"])) {
			t.Fatal("missing trace path or leaked key")
		}
	}
}

func TestRunHelpAndInvalidFlagsDoNotCreateDefaultTrace(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"--unknown"}, {"extra"}} {
		env := cliEnv(t)
		var output bytes.Buffer
		run(context.Background(), args, func(key string) string { return env[key] }, func(tea.Model) error { t.Fatal("unexpected launch"); return nil }, &output)
		if _, err := os.Stat(filepath.Join(env["XDG_STATE_HOME"], "axlr", "logs")); !os.IsNotExist(err) {
			t.Fatalf("help or invalid flags created logs: %v", err)
		}
	}
}

func TestRunExplicitTraceDoesNotCreateDefaultTrace(t *testing.T) {
	env := cliEnv(t)
	path := filepath.Join(t.TempDir(), "explicit.jsonl")
	var output bytes.Buffer
	if code := run(context.Background(), []string{"--root", t.TempDir(), "--trace-file", path}, func(key string) string { return env[key] }, func(tea.Model) error { return nil }, &output); code != 0 {
		t.Fatalf("code=%d output=%s", code, &output)
	}
	// The app log stays in the default directory; no trace joins it.
	if traces, err := filepath.Glob(filepath.Join(env["XDG_STATE_HOME"], "axlr", "logs", "*.jsonl")); err != nil || len(traces) != 0 {
		t.Fatalf("explicit flag created a default trace: %v %v", traces, err)
	}
	if !strings.Contains(output.String(), path) {
		t.Fatal("explicit trace location was not printed")
	}
}

func TestRunDefaultTraceFailureRedactsKeyAndDoesNotFallBackToHostEnvironment(t *testing.T) {
	env := cliEnv(t)
	var output bytes.Buffer
	code := run(context.Background(), []string{"--root", t.TempDir()}, func(key string) string { return env[key] }, func(tea.Model) error { return errors.New(env["OPENROUTER_API_KEY"]) }, &output)
	if code == 0 || strings.Contains(output.String(), env["OPENROUTER_API_KEY"]) || !strings.Contains(output.String(), "[redacted]") {
		t.Fatalf("exit=%d output=%s", code, &output)
	}
	files, err := filepath.Glob(filepath.Join(env["XDG_STATE_HOME"], "axlr", "logs", "trace-*.jsonl"))
	if err != nil || len(files) != 1 {
		t.Fatalf("files=%v err=%v", files, err)
	}
	data, err := os.ReadFile(files[0])
	if err != nil || !bytes.Contains(data, []byte(`"stage":"operation_failed"`)) || bytes.Contains(data, []byte(env["OPENROUTER_API_KEY"])) {
		t.Fatalf("trace=%s err=%v", data, err)
	}
	output.Reset()
	delete(env, "HOME")
	delete(env, "XDG_STATE_HOME")
	code = run(context.Background(), nil, func(key string) string { return env[key] }, func(tea.Model) error { t.Fatal("missing fake environment launched"); return nil }, &output)
	if code == 0 || !strings.Contains(output.String(), "required for diagnostics") {
		t.Fatalf("exit=%d output=%s", code, &output)
	}
}

func TestRunAcceptsRelativeExplicitTraceWithPayloadCapture(t *testing.T) {
	env := cliEnv(t)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(cwd, ".axlr-relative-trace-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	path := filepath.Join(dir, "relative-trace.jsonl")
	relative, err := filepath.Rel(cwd, path)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	code := run(context.Background(), []string{"--root", t.TempDir(), "--trace-file", relative, "--trace-payloads"}, func(key string) string { return env[key] }, func(tea.Model) error { return nil }, &output)
	if code != 0 {
		t.Fatalf("exit=%d output=%s", code, &output)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "diagnostics: "+path) || !strings.Contains(output.String(), "payload capture is on: request and response bodies, prompts and tool results included, are stored in "+path+".payloads-") {
		t.Fatalf("normalized locations missing: %s", &output)
	}
}

func TestRunReusedTraceGetsUniquePayloadDirectoryPerLaunch(t *testing.T) {
	env := cliEnv(t)
	path := filepath.Join(t.TempDir(), "reused-trace.jsonl")
	previous := http.DefaultTransport
	defer func() { http.DefaultTransport = previous }()
	http.DefaultTransport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"done\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")), Header: make(http.Header)}, nil
	})
	var output bytes.Buffer
	for i := 0; i < 2; i++ {
		code := run(context.Background(), []string{"--root", t.TempDir(), "--model", "test/model", "--trace-file", path, "--trace-payloads"}, func(key string) string { return env[key] }, func(m tea.Model) error {
			a := m.(terminal.AppModel)
			a.Composer.Input.SetValue("reply")
			m, cmd := a.Update(terminal.ControlIntent("send"))
			final := drain(t, m, cmd)
			if final.Status.Error != "" {
				t.Fatal(final.Status.Error)
			}
			return nil
		}, &output)
		if code != 0 {
			t.Fatalf("exit=%d output=%s", code, &output)
		}
	}
	dirs, err := filepath.Glob(path + ".payloads-*")
	if err != nil || len(dirs) != 2 {
		t.Fatalf("dirs=%v err=%v", dirs, err)
	}
	for _, dir := range dirs {
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() || (runtime.GOOS != "windows" && info.Mode().Perm() != 0o700) {
			t.Fatalf("directory=%v err=%v", info, err)
		}
		files, err := os.ReadDir(dir)
		if err != nil || len(files) != 2 {
			t.Fatalf("captures=%v err=%v", files, err)
		}
		for _, file := range files {
			if info, err := file.Info(); err != nil || (runtime.GOOS != "windows" && info.Mode().Perm() != 0o600) {
				t.Fatalf("capture %s mode=%v err=%v", file.Name(), info, err)
			}
		}
		if !strings.Contains(output.String(), "are stored in "+dir) {
			t.Fatal("payload path missing")
		}
	}
	if info, err := os.Stat(path); err != nil || (runtime.GOOS != "windows" && info.Mode().Perm() != 0o600) {
		t.Fatalf("trace mode=%v err=%v", info, err)
	}
	data, err := os.ReadFile(path)
	if err != nil || bytes.Count(data, []byte(`"stage":"startup"`)) != 2 || bytes.Count(data, []byte(`"stage":"payload_saved"`)) != 4 || bytes.Contains(data, []byte(`"stage":"payload_failed"`)) {
		t.Fatalf("trace=%s err=%v", data, err)
	}
}

func TestRunCanDisablePayloadsAndKeepDefaultDiagnostics(t *testing.T) {
	env := cliEnv(t)
	var output bytes.Buffer
	code := run(context.Background(), []string{"--root", t.TempDir(), "--trace-payloads=false"}, func(key string) string { return env[key] }, func(tea.Model) error { return nil }, &output)
	if code != 0 {
		t.Fatalf("exit=%d output=%s", code, &output)
	}
	files, err := os.ReadDir(filepath.Join(env["XDG_STATE_HOME"], "axlr", "logs"))
	if err != nil || len(files) != 2 || files[0].Name() != "axlr.log" || !strings.HasSuffix(files[1].Name(), ".jsonl") || strings.Contains(output.String(), "payload") {
		t.Fatalf("files=%v err=%v output=%s", files, err, &output)
	}
}

// Every launch leaves a trace and a payload directory that holds whole
// transcripts. Those of earlier launches are deleted at startup once older
// than trace_retention_days (default 30); 0 keeps them. A trace AXLR did not
// name, such as one chosen with --trace-file, is never deleted.
func TestRunPrunesOldDefaultTracesAndPayloads(t *testing.T) {
	for _, tc := range []struct {
		name     string
		settings string
		pruned   bool
	}{
		{"default retention", "", true},
		{"pruning disabled", `{"trace_retention_days":0}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := cliEnv(t)
			logs := filepath.Join(env["XDG_STATE_HOME"], "axlr", "logs")
			if err := os.MkdirAll(logs, 0o700); err != nil {
				t.Fatal(err)
			}
			old := time.Now().Add(-31 * 24 * time.Hour)
			oldTrace := filepath.Join(logs, "trace-1-2.jsonl")
			oldPayloads := filepath.Join(logs, "trace-1-2.jsonl.payloads-3")
			oldCapture := filepath.Join(oldPayloads, "000001-request.json")
			explicit := filepath.Join(logs, "mine.jsonl")
			recent := filepath.Join(logs, "trace-4-5.jsonl")
			if err := os.Mkdir(oldPayloads, 0o700); err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{oldTrace, oldCapture, explicit, recent} {
				if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			for _, path := range []string{oldTrace, oldCapture, oldPayloads, explicit} {
				if err := os.Chtimes(path, old, old); err != nil {
					t.Fatal(err)
				}
			}
			if tc.settings != "" {
				settings := filepath.Join(env["HOME"], ".config", "axlr", "settings.json")
				if err := os.MkdirAll(filepath.Dir(settings), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(settings, []byte(tc.settings), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			var output bytes.Buffer
			code := run(context.Background(), []string{"--root", t.TempDir(), "--model", "test/model"}, func(key string) string { return env[key] }, func(tea.Model) error { return nil }, &output)
			if code != 0 {
				t.Fatalf("exit=%d output=%s", code, &output)
			}
			for _, path := range []string{oldTrace, oldPayloads} {
				if _, err := os.Lstat(path); os.IsNotExist(err) != tc.pruned {
					t.Errorf("%s: pruned=%v, want %v", filepath.Base(path), os.IsNotExist(err), tc.pruned)
				}
			}
			for _, path := range []string{explicit, recent} {
				if _, err := os.Lstat(path); err != nil {
					t.Errorf("%s was deleted: %v", filepath.Base(path), err)
				}
			}
			// This launch's own trace and payload directory are kept.
			want := 2
			if !tc.pruned {
				want = 3
			}
			if traces, _ := filepath.Glob(filepath.Join(logs, "trace-*.jsonl")); len(traces) != want {
				t.Errorf("traces=%v", traces)
			}
			// Payload capture is off by default, so this launch adds none.
			if payloads, _ := filepath.Glob(filepath.Join(logs, "trace-*.jsonl.payloads-*")); len(payloads) != want-2 {
				t.Errorf("payload directories=%v", payloads)
			}
			if strings.Contains(output.String(), "trace-1-2") {
				t.Errorf("pruning wrote to the console: %s", &output)
			}
		})
	}
}

// Payloads hold whole requests; beside a trace inside the workspace, the
// model's own searches would read them and send them back, larger each time.
func TestRunKeepsPayloadsOutsideTheWorkspace(t *testing.T) {
	env := cliEnv(t)
	root := t.TempDir()
	path := filepath.Join(root, "trace.jsonl")
	var output bytes.Buffer
	code := run(context.Background(), []string{"--root", root, "--trace-file", path, "--trace-payloads"}, func(key string) string { return env[key] }, func(tea.Model) error { return nil }, &output)
	if code != 0 {
		t.Fatalf("exit=%d output=%s", code, &output)
	}
	if beside, _ := filepath.Glob(path + ".payloads-*"); len(beside) != 0 {
		t.Fatalf("payloads inside the workspace: %v", beside)
	}
	logs := filepath.Join(env["XDG_STATE_HOME"], "axlr", "logs")
	moved, err := filepath.Glob(filepath.Join(logs, "trace.jsonl.payloads-*"))
	if err != nil || len(moved) != 1 || !strings.Contains(output.String(), "are stored in "+moved[0]) || !strings.Contains(output.String(), "payloads go to "+logs) {
		t.Fatalf("moved=%v err=%v output=%s", moved, err, &output)
	}
}

// Payloads hold whole transcripts (295 MB in the default logs directory on
// 9 October 2026, 74 MB from one run), so a launch captures them only when
// asked: trace_payloads in settings.json, or --trace-payloads, which decides
// for one launch either way. Timing traces are always written.
func TestRunCapturesPayloadsOnlyWhenAsked(t *testing.T) {
	for _, tc := range []struct {
		name, settings string
		args           []string
		capture        bool
	}{
		{"default", "", nil, false},
		{"setting on", `{"trace_payloads":true}`, nil, true},
		{"flag off over setting on", `{"trace_payloads":true}`, []string{"--trace-payloads=false"}, false},
		{"flag on", "", []string{"--trace-payloads"}, true},
		{"flag on over setting off", `{"trace_payloads":false}`, []string{"--trace-payloads=true"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := cliEnv(t)
			if tc.settings != "" {
				settings := filepath.Join(env["HOME"], ".config", "axlr", "settings.json")
				if err := os.MkdirAll(filepath.Dir(settings), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(settings, []byte(tc.settings), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			var output bytes.Buffer
			code := run(context.Background(), append([]string{"--root", t.TempDir()}, tc.args...), func(key string) string { return env[key] }, func(tea.Model) error { return nil }, &output)
			if code != 0 {
				t.Fatalf("exit=%d output=%s", code, &output)
			}
			logs := filepath.Join(env["XDG_STATE_HOME"], "axlr", "logs")
			traces, _ := filepath.Glob(filepath.Join(logs, "trace-*.jsonl"))
			payloads, _ := filepath.Glob(filepath.Join(logs, "trace-*.jsonl.payloads-*"))
			if len(traces) != 1 || (len(payloads) == 1) != tc.capture || len(payloads) > 1 {
				t.Fatalf("traces=%v payloads=%v", traces, payloads)
			}
			if announced := strings.Count(output.String(), "payload capture is on"); announced != len(payloads) {
				t.Fatalf("announced %d times for %d directories: %s", announced, len(payloads), &output)
			}
			if tc.capture && !strings.Contains(output.String(), "are stored in "+payloads[0]) {
				t.Fatalf("output=%s", &output)
			}
		})
	}
}
