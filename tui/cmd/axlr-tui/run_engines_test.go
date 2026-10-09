package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/plugins"
	"github.com/underpass-ai/AXLR/tui/adapters/engines"
)

func TestOnlyKMPAndMADECommandsAreShared(t *testing.T) {
	command := filepath.Join(t.TempDir(), "engine")
	for _, tc := range []struct {
		manifest plugins.Manifest
		shared   bool
	}{
		{plugins.Manifest{ID: "kmp", Command: command, AllowAll: true}, true},
		{plugins.Manifest{ID: "made", Command: command, AllowAll: true}, true},
		{plugins.Manifest{ID: "github", Command: command, AllowAll: true}, false},
		{plugins.Manifest{ID: "kmp", URL: "https://kmp.example/mcp", AllowAll: true}, false},
	} {
		registration, err := plugins.NewRegistration(tc.manifest, nil)
		if err != nil {
			t.Fatal(err)
		}
		if sharedEngine(registration) != tc.shared {
			t.Fatalf("%+v shared = %v", tc.manifest, !tc.shared)
		}
	}
}

func TestEngineSocketsLiveInTheRuntimeDirectory(t *testing.T) {
	runtimeDir, stateBase := t.TempDir(), t.TempDir()
	env := map[string]string{"XDG_RUNTIME_DIR": runtimeDir}
	if got := engineSocketDir(func(k string) string { return env[k] }, stateBase); got != filepath.Join(runtimeDir, "axlr", "engines") {
		t.Fatalf("dir = %s", got)
	}
	if got := engineSocketDir(func(string) string { return "relative" }, stateBase); got != filepath.Join(stateBase, "axlr", "engines") {
		t.Fatalf("fallback dir = %s", got)
	}
}

// A shared engine that cannot start leaves the console starting its own,
// and the launch says so.
func TestSharedEnginesThatCannotStartFallBack(t *testing.T) {
	registration, err := plugins.NewRegistration(plugins.Manifest{ID: "kmp", Command: filepath.Join(t.TempDir(), "kmp-mcp"), AllowAll: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := plugins.NewManager([]plugins.Registration{registration})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	var out bytes.Buffer
	shareEngines(context.Background(), manager, []plugins.Registration{registration}, engines.Supervisor{Dir: "relative"}, &out)
	if !strings.Contains(out.String(), "shared engine kmp unavailable (") || !strings.Contains(out.String(), "this console starts its own") || strings.Contains(out.String(), "shared engines:") {
		t.Fatalf("output %q", &out)
	}
}
