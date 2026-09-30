package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/underpass-ai/AXLR/tui/adapters/terminal"
)

func TestRunTraceRecordsStartupPersistenceAndShutdownWithoutSecrets(t *testing.T) {
	env := cliEnv(t)
	path := filepath.Join(t.TempDir(), "trace.jsonl")
	var output bytes.Buffer
	code := run(context.Background(), []string{"--root", t.TempDir(), "--model", "test/model", "--trace-file", path}, func(key string) string { return env[key] }, func(tea.Model) error { return nil }, &output)
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
	if _, err := os.Stat(filepath.Join(env["XDG_STATE_HOME"], "axlr", "logs")); !os.IsNotExist(err) {
		t.Fatalf("explicit flag created default logs: %v", err)
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
	path := filepath.Join(t.TempDir(), "relative-trace.jsonl")
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(cwd, path)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	code := run(context.Background(), []string{"--root", t.TempDir(), "--trace-file", relative}, func(key string) string { return env[key] }, func(tea.Model) error { return nil }, &output)
	if code != 0 {
		t.Fatalf("exit=%d output=%s", code, &output)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "diagnostics: "+path) || !strings.Contains(output.String(), "payloads: "+path+".payloads-") {
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
		code := run(context.Background(), []string{"--root", t.TempDir(), "--model", "test/model", "--trace-file", path}, func(key string) string { return env[key] }, func(m tea.Model) error {
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
		if err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
			t.Fatalf("directory=%v err=%v", info, err)
		}
		files, err := os.ReadDir(dir)
		if err != nil || len(files) != 2 {
			t.Fatalf("captures=%v err=%v", files, err)
		}
		if !strings.Contains(output.String(), "payloads: "+dir) {
			t.Fatal("payload path missing")
		}
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
	if err != nil || len(files) != 1 || !strings.HasSuffix(files[0].Name(), ".jsonl") || strings.Contains(output.String(), "payloads:") {
		t.Fatalf("files=%v err=%v output=%s", files, err, &output)
	}
}
