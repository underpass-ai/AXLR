package main

import (
	"bytes"
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/plugins"
	"github.com/underpass-ai/AXLR/tui/adapters/engines"
	"github.com/underpass-ai/AXLR/tui/adapters/madesetup"
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

func TestMADERunsUnderTheFullToolProfileAndTheWorkIdentityUnlessItsEntryNamesThem(t *testing.T) {
	dir := t.TempDir()
	launcher, binary := filepath.Join(dir, "run-embedded-mcp.sh"), filepath.Join(dir, "made-mcp")
	registration := func(id root.PluginID, command string, env []string) plugins.Registration {
		r, err := plugins.NewRegistration(plugins.Manifest{ID: id, Command: command, AllowAll: true}, env)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	const config = "/home/test/.config/axlr/mcp.json"
	registrations := []plugins.Registration{
		registration("made", launcher, []string{"HOME=/home/test"}),
		registration("made", launcher, []string{"MADE_MCP_TOOL_PROFILE=core+ceremony_execution", "MADE_AUTH_TRUSTED_HOST_ID=ops-host"}),
		registration("made", binary, nil),
		registration("kmp", launcher, []string{"KMP_MCP_BACKEND=embedded"}),
	}
	pinMADEEnvironment(config, registrations)
	work := madesetup.WorkIdentity(config, nil)
	if !slices.Contains(registrations[0].Env, "MADE_MCP_TOOL_PROFILE=full") || !slices.Contains(registrations[0].Env, "MADE_AUTH_TRUSTED_HOST_ID="+work) {
		t.Fatalf("the embedded entry kept the launcher's profile or its trusted host: %v", registrations[0].Env)
	}
	if !slices.Equal(registrations[1].Env, []string{"MADE_MCP_TOOL_PROFILE=core+ceremony_execution", "MADE_AUTH_TRUSTED_HOST_ID=ops-host"}) {
		t.Fatalf("an operator's profile or host was replaced: %v", registrations[1].Env)
	}
	if !slices.Equal(registrations[2].Env, []string{"MADE_MCP_TOOL_PROFILE=full"}) {
		t.Fatalf("a bare binary got a work identity: %v", registrations[2].Env)
	}
	if !slices.Equal(registrations[3].Env, []string{"KMP_MCP_BACKEND=embedded"}) {
		t.Fatalf("KMP's environment changed: %v", registrations[3].Env)
	}
}
