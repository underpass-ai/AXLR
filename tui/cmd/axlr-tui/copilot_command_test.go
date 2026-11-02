package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCopilotDeviceLoginUsesOfficialCLIWithoutPassingProviderSecrets(t *testing.T) {
	dir := t.TempDir()
	cli := filepath.Join(dir, "copilot")
	// The child records argument boundaries and whether a model key leaked.
	script := "#!/bin/sh\nprintf 'arg=%s\\n' \"$@\"\nif [ -n \"${OPENROUTER_API_KEY:-}\" ] || [ -n \"${GH_TOKEN:-}\" ]; then echo leaked; fi\n"
	if err := os.WriteFile(cli, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPENROUTER_API_KEY", "dummy-secret")
	t.Setenv("GH_TOKEN", "dummy-token")
	var stdout, stderr bytes.Buffer
	code := runCopilotCommand(context.Background(), []string{"login", "--cli", cli, "--host", "https://example.ghe.com"}, os.Getenv, strings.NewReader(""), &stdout, &stderr)
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("exit %d stderr %s", code, stderr.String())
	}
	if want := "arg=login\narg=--device-code\narg=--host\narg=https://example.ghe.com\n"; stdout.String() != want {
		t.Fatalf("got %q, want %q", stdout.String(), want)
	}
}

func TestCopilotLoginRejectsUnknownCommandsAndMissingCLI(t *testing.T) {
	for _, args := range [][]string{nil, {"other"}, {"status", "--host", "bad"}, {"login", "unexpected"}} {
		var stdout, stderr bytes.Buffer
		if got := runCopilotCommand(context.Background(), args, os.Getenv, strings.NewReader(""), &stdout, &stderr); got != 2 {
			t.Fatalf("args %q: got %d", args, got)
		}
	}
	var stdout, stderr bytes.Buffer
	if got := runCopilotCommand(context.Background(), []string{"login", "--cli", filepath.Join(t.TempDir(), "missing")}, os.Getenv, strings.NewReader(""), &stdout, &stderr); got != 1 || !strings.Contains(stderr.String(), "not found") {
		t.Fatalf("got %d, stderr %q", got, stderr.String())
	}
}
