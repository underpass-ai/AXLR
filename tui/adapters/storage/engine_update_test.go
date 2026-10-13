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

func TestRegisterEngineAddsAPrivateManifestAndEntry(t *testing.T) {
	dir := t.TempDir()
	store := MCPConfigStore{Path: filepath.Join(dir, "axlr", "mcp.json")}
	launcher := filepath.Join(dir, "made-1.0.0", "scripts", "run-embedded-mcp.sh")
	registration := application.EngineRegistration{Engine: "made", Launcher: launcher, Name: "MADE", Purpose: "ceremony", Approval: "manual", EnvFrom: map[string]string{"HOME": "HOME", "PATH": "PATH"}}
	if err := store.RegisterEngine(context.Background(), registration); err != nil {
		t.Fatal(err)
	}
	config, err := readMCPConfig(store.Path)
	if err != nil || config.Version != 1 || len(config.Plugins) != 1 {
		t.Fatalf("%+v %v", config, err)
	}
	entry := config.Plugins[0]
	if entry.Name != "MADE" || entry.Purpose != "ceremony" || entry.Approval != "manual" || !reflect.DeepEqual(entry.EnvFrom, registration.EnvFrom) {
		t.Fatalf("%+v", entry)
	}
	if filepath.Dir(entry.Manifest) != filepath.Join(dir, "axlr", "engines") {
		t.Fatalf("manifest %s", entry.Manifest)
	}
	manifest, err := plugins.LoadManifest(entry.Manifest)
	if err != nil || manifest.ID != "made" || manifest.Command != launcher || !manifest.AllowAll {
		t.Fatalf("%+v %v", manifest, err)
	}
	if err := store.RegisterEngine(context.Background(), registration); err == nil {
		t.Fatal("a second MADE registration was accepted")
	}
	targets, err := store.EngineTargets(context.Background())
	if err != nil || len(targets) != 1 || targets[0].Command != launcher {
		t.Fatalf("%+v %v", targets, err)
	}
}

func TestRegisterEngineRefusesUnsafeRegistrations(t *testing.T) {
	store := MCPConfigStore{Path: filepath.Join(t.TempDir(), "mcp.json")}
	base := application.EngineRegistration{Engine: "kmp", Launcher: "/opt/kmp/scripts/run-embedded-mcp.sh", Purpose: "memory", Approval: "auto"}
	for name, change := range map[string]func(*application.EngineRegistration){
		"engine":   func(r *application.EngineRegistration) { r.Engine = "other" },
		"relative": func(r *application.EngineRegistration) { r.Launcher = "run.sh" },
		"key":      func(r *application.EngineRegistration) { r.EnvFrom = map[string]string{"KEY": "OPENROUTER_API_KEY"} },
		"overlap": func(r *application.EngineRegistration) {
			r.Env, r.EnvFrom = map[string]string{"HOME": "/x"}, map[string]string{"HOME": "HOME"}
		},
		"purpose":  func(r *application.EngineRegistration) { r.Purpose = "everything" },
		"approval": func(r *application.EngineRegistration) { r.Approval = "always" },
	} {
		registration := base
		change(&registration)
		if err := store.RegisterEngine(context.Background(), registration); err == nil {
			t.Fatalf("%s accepted", name)
		}
	}
	if _, err := os.Stat(store.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("a refused registration wrote mcp.json")
	}
}
