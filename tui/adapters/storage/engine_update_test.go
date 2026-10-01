package storage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/underpass-ai/AXLR/plugins"
	"github.com/underpass-ai/AXLR/tui/application"
)

func TestEngineActivationPreservesPolicyEnvironmentAndOldManifest(t *testing.T) {
	store := MCPConfigStore{Path: configuredPlugin(t)}
	if err := store.AddURL(context.Background(), "docs", "https://example.com/mcp"); err != nil {
		t.Fatal(err)
	}
	before, err := readMCPConfig(store.Path)
	if err != nil {
		t.Fatal(err)
	}
	targets, err := store.EngineTargets(context.Background())
	if err != nil || len(targets) != 1 {
		t.Fatalf("%+v %v", targets, err)
	}
	original, err := os.ReadFile(targets[0].Manifest)
	if err != nil {
		t.Fatal(err)
	}
	launcher := filepath.Join(t.TempDir(), "run-embedded-mcp.sh")
	if err := store.ActivateEngine(context.Background(), targets[0], launcher); err != nil {
		t.Fatal(err)
	}
	after, err := readMCPConfig(store.Path)
	if err != nil {
		t.Fatal(err)
	}
	updated := after.Plugins[0]
	if updated.Manifest == before.Plugins[0].Manifest {
		t.Fatal("manifest reference was not switched")
	}
	updated.Manifest = before.Plugins[0].Manifest
	if !reflect.DeepEqual(updated, before.Plugins[0]) || !reflect.DeepEqual(after.Plugins[1], before.Plugins[1]) {
		t.Fatal("environment, policy or other server changed")
	}
	manifest, err := plugins.LoadManifest(after.Plugins[0].Manifest)
	if err != nil || manifest.Command != launcher || !manifest.AllowAll {
		t.Fatalf("%+v %v", manifest, err)
	}
	old, err := os.ReadFile(targets[0].Manifest)
	if err != nil || string(old) != string(original) {
		t.Fatal("old manifest was modified")
	}
	if err := store.ActivateEngine(context.Background(), targets[0], launcher); err == nil {
		t.Fatal("stale target accepted")
	}
	info, err := os.Stat(after.Plugins[0].Manifest)
	if err != nil || runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatal("new manifest is not private")
	}
}

func TestEngineUpdateSkipsRemoteAndCustomArguments(t *testing.T) {
	for _, body := range []string{
		`{"manifest_version":1,"id":"kmp","url":"https://example.com/mcp","allow_tools":["*"]}`,
		`{"manifest_version":1,"id":"kmp","command":"/bin/echo","args":["custom"],"allow_tools":["*"]}`,
	} {
		store := MCPConfigStore{Path: configuredPlugin(t)}
		c, _ := readMCPConfig(store.Path)
		if err := os.WriteFile(c.Plugins[0].Manifest, portableManifest([]byte(body)), 0600); err != nil {
			t.Fatal(err)
		}
		targets, err := store.EngineTargets(context.Background())
		if err != nil || len(targets) != 1 || targets[0].Command != "" {
			t.Fatalf("%+v %v", targets, err)
		}
	}
}

func TestEngineUpdateRefusesCanceledAndInvalidConfiguration(t *testing.T) {
	store := MCPConfigStore{Path: configuredPlugin(t)}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.EngineTargets(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("%v", err)
	}
	targets, _ := store.EngineTargets(context.Background())
	launcher := filepath.Join(t.TempDir(), "launcher")
	if err := store.ActivateEngine(ctx, targets[0], launcher); !errors.Is(err, context.Canceled) {
		t.Fatalf("%v", err)
	}
	if err := store.ActivateEngine(context.Background(), application.EngineUpdateTarget{Engine: "other"}, launcher); err == nil {
		t.Fatal("unknown engine accepted")
	}
	if err := store.ActivateEngine(context.Background(), targets[0], "relative"); err == nil {
		t.Fatal("relative launcher accepted")
	}
	if err := os.WriteFile(store.Path, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EngineTargets(context.Background()); err == nil {
		t.Fatal("invalid config accepted")
	}
}
