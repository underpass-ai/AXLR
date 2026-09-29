package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
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
