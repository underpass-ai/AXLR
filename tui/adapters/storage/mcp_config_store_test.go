package storage

import (
	"context"
	"encoding/json"
	"errors"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/plugins"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/underpass-ai/AXLR/tui/domain"
)

func configuredPlugin(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	manifest := filepath.Join(dir, "kmp.json")
	if err := os.WriteFile(manifest, []byte(`{"manifest_version":1,"id":"kmp","command":"/bin/echo","args":[],"allow_tools":["*"]}`), 0600); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(map[string]any{"version": 1, "plugins": []any{map[string]any{"manifest": manifest, "name": "KMP", "description": "Graph memory", "purpose": "memory", "env": map[string]string{"TOKEN": "preserve-value"}, "env_from": map[string]string{"HOME": "HOME"}}}})
	path := filepath.Join(dir, "mcp.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}
func TestLoadMCPConfigurationProfilesAndDefaults(t *testing.T) {
	path := configuredPlugin(t)
	config, err := LoadMCPConfiguration(path, func(string) string { return "selected" })
	if err != nil || len(config.Profiles) != 1 || len(config.Registrations) != 1 {
		t.Fatalf("%+v %v", config, err)
	}
	p := config.Profiles[0]
	if p.ID != "kmp" || p.Name != "KMP" || p.Description != "Graph memory" || p.Purpose != domain.PluginPurposeMemory || p.Approval != domain.ApprovalManual {
		t.Fatalf("%+v", p)
	}
	dtoConfig, err := readMCPConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	dtoConfig.Plugins[0].Name = ""
	dtoConfig.Plugins[0].Description = ""
	dtoConfig.Plugins[0].Purpose = ""
	data, _ := json.Marshal(dtoConfig)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	config, err = LoadMCPConfiguration(path, func(string) string { return "" })
	if err != nil || config.Profiles[0].Name != "kmp" || config.Profiles[0].Purpose != domain.PluginPurposeTools {
		t.Fatalf("%+v %v", config, err)
	}
	if _, err := LoadMCPConfiguration(path, nil); err == nil {
		t.Fatal("nil resolver accepted")
	}
}
func TestMCPConfigStorePersistsPolicyAndPreservesEnvironment(t *testing.T) {
	path := configuredPlugin(t)
	store := MCPConfigStore{Path: path}
	if err := store.SaveApproval(context.Background(), "kmp", domain.ApprovalAuto); err != nil {
		t.Fatal(err)
	}
	config, err := readMCPConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	p := config.Plugins[0]
	if p.Approval != "auto" || p.Env["TOKEN"] != "preserve-value" || p.EnvFrom["HOME"] != "HOME" || p.Name != "KMP" || p.Purpose != "memory" {
		t.Fatalf("config fields lost")
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("mode: %v %v", info, err)
	}
	loaded, err := LoadMCPConfiguration(path, func(string) string { return "" })
	if err != nil || loaded.Profiles[0].Approval != domain.ApprovalAuto {
		t.Fatalf("policy not durable: %v", err)
	}
	if err := store.SaveApproval(context.Background(), "kmp", domain.ApprovalManual); err != nil {
		t.Fatal(err)
	}
	leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".axlr-mcp-*"))
	if len(leftovers) != 0 {
		t.Fatal("temporary files remain")
	}
}

func TestMCPInstallPreservesDefaultsAndLoadsHTTP(t *testing.T) {
	path := configuredPlugin(t)
	store := MCPConfigStore{Path: path}
	if err := store.AddURL(context.Background(), "search", "https://example.com/mcp"); err != nil {
		t.Fatal(err)
	}
	config, err := readMCPConfig(path)
	if err != nil || len(config.Plugins) != 2 || config.Plugins[0].Name != "KMP" || config.Plugins[1].Approval != "manual" {
		t.Fatalf("config: %+v %v", config, err)
	}
	manifest, err := plugins.LoadManifest(config.Plugins[1].Manifest)
	if err != nil || manifest.URL != "https://example.com/mcp" {
		t.Fatalf("HTTP manifest: %+v %v", manifest, err)
	}
	loaded, err := LoadMCPConfiguration(path, func(string) string { return "" })
	if err != nil || len(loaded.Registrations) != 2 || loaded.Registrations[1].Manifest.URL != "https://example.com/mcp" {
		t.Fatalf("reload: %+v %v", loaded, err)
	}
	manager, err := plugins.NewManager(loaded.Registrations)
	if err != nil {
		t.Fatalf("HTTP registration failed: %v", err)
	}
	manager.Close()
	if err := store.AddURL(context.Background(), "search", "https://example.com/mcp"); err == nil {
		t.Fatal("duplicate MCP installed")
	}
}

func TestInstallLocalMCPIntoNewConfiguration(t *testing.T) {
	root := t.TempDir()
	manifestPath := filepath.Join(root, "third-party.json")
	if err := os.WriteFile(manifestPath, []byte(`{"manifest_version":1,"id":"thirdparty","command":"/bin/echo","args":[],"allow_tools":["*"]}`), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "config", "mcp.json")
	store := MCPConfigStore{Path: path}
	if err := store.AddManifest(context.Background(), manifestPath); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadMCPConfiguration(path, func(string) string { return "" })
	if err != nil || len(loaded.Profiles) != 1 || loaded.Profiles[0].ID != "thirdparty" || loaded.Profiles[0].Approval != domain.ApprovalManual {
		t.Fatalf("installed config: %+v %v", loaded, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("unsafe config mode: %v %v", info, err)
	}
}
func TestMCPConfigStoreRejectsInvalidAndCancelledChange(t *testing.T) {
	path := configuredPlugin(t)
	store := MCPConfigStore{Path: path}
	original, _ := os.ReadFile(path)
	if store.SaveApproval(context.Background(), "unknown", domain.ApprovalAuto) == nil || store.SaveApproval(context.Background(), "bad/id", domain.ApprovalAuto) == nil || store.SaveApproval(context.Background(), "kmp", "bad") == nil {
		t.Fatal("invalid change accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if store.SaveApproval(ctx, "kmp", domain.ApprovalAuto) == nil {
		t.Fatal("cancelled change accepted")
	}
	unchanged, _ := os.ReadFile(path)
	if string(unchanged) != string(original) {
		t.Fatal("failed change modified config")
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if store.SaveApproval(context.Background(), "kmp", domain.ApprovalAuto) == nil {
		t.Fatal("unsafe file accepted")
	}
}
func TestMCPConfigurationRejectsInvalidProfilesAndDuplicateIDs(t *testing.T) {
	for _, field := range []string{"purpose", "approval", "name", "duplicate"} {
		t.Run(field, func(t *testing.T) {
			path := configuredPlugin(t)
			config, err := readMCPConfig(path)
			if err != nil {
				t.Fatal(err)
			}
			switch field {
			case "purpose":
				config.Plugins[0].Purpose = "invalid"
			case "approval":
				config.Plugins[0].Approval = "invalid"
			case "name":
				config.Plugins[0].Name = "bad\x00name"
			case "duplicate":
				config.Plugins = append(config.Plugins, config.Plugins[0])
			}
			data, _ := json.Marshal(config)
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadMCPConfiguration(path, func(string) string { return "" }); err == nil {
				t.Fatal("invalid profile accepted")
			}
		})
	}
}

func TestMCPConfigStoreRejectsAmbiguousOrMissingManifest(t *testing.T) {
	for _, failure := range []string{"duplicate", "manifest"} {
		t.Run(failure, func(t *testing.T) {
			path := configuredPlugin(t)
			store := MCPConfigStore{Path: path}
			config, err := readMCPConfig(path)
			if err != nil {
				t.Fatal(err)
			}
			if failure == "duplicate" {
				config.Plugins = append(config.Plugins, config.Plugins[0])
			} else {
				if err := os.Remove(config.Plugins[0].Manifest); err != nil {
					t.Fatal(err)
				}
			}
			data, _ := json.Marshal(config)
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			if store.SaveApproval(context.Background(), "kmp", domain.ApprovalAuto) == nil {
				t.Fatal("invalid manifest configuration modified")
			}
			after, _ := os.ReadFile(path)
			if string(after) != string(data) {
				t.Fatal("failed save changed file")
			}
		})
	}
}

func TestMCPConfigStoreSynchronizesDirectoryAfterReplacement(t *testing.T) {
	path := configuredPlugin(t)
	wantErr := errors.New("directory durability unconfirmed")
	called := false
	store := MCPConfigStore{Path: path, syncDirectory: func(directory *os.File) error {
		called = true
		if directory.Name() != filepath.Dir(path) {
			t.Fatal("synchronizing wrong directory")
		}
		info, err := directory.Stat()
		if err != nil || !info.IsDir() {
			t.Fatal("directory descriptor invalid")
		}
		config, err := readMCPConfig(path)
		if err != nil || config.Plugins[0].Approval != "auto" {
			t.Fatal("sync happened before atomic replacement")
		}
		return wantErr
	}}
	if err := store.SaveApproval(context.Background(), "kmp", domain.ApprovalAuto); !errors.Is(err, wantErr) || !called {
		t.Fatalf("directory sync failure lost: %v", err)
	}
}

func TestMCPConfigStoreConcurrentInstancesPreserveBothPolicies(t *testing.T) {
	path := configuredPlugin(t)
	config, err := readMCPConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	otherManifest := filepath.Join(filepath.Dir(path), "made.json")
	if err := os.WriteFile(otherManifest, []byte(`{"manifest_version":1,"id":"made","command":"/bin/echo","args":[],"allow_tools":["*"]}`), 0600); err != nil {
		t.Fatal(err)
	}
	other := config.Plugins[0]
	other.Manifest = otherManifest
	other.Name = "MADE"
	other.Purpose = "ceremony"
	config.Plugins = append(config.Plugins, other)
	data, _ := json.Marshal(config)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	first, second := MCPConfigStore{Path: path}, MCPConfigStore{Path: path}
	for trial := 0; trial < 20; trial++ {
		for _, id := range []root.PluginID{"kmp", "made"} {
			if err := first.SaveApproval(context.Background(), id, domain.ApprovalManual); err != nil {
				t.Fatal(err)
			}
		}
		start := make(chan struct{})
		done := make(chan error, 2)
		go func() { <-start; done <- first.SaveApproval(context.Background(), "kmp", domain.ApprovalAuto) }()
		go func() { <-start; done <- second.SaveApproval(context.Background(), "made", domain.ApprovalAuto) }()
		close(start)
		for range 2 {
			if err := <-done; err != nil {
				t.Fatal(err)
			}
		}
		after, err := readMCPConfig(path)
		if err != nil {
			t.Fatal(err)
		}
		if after.Plugins[0].Approval != "auto" || after.Plugins[1].Approval != "auto" {
			t.Fatalf("concurrent policy lost on trial %d", trial)
		}
	}
	info, err := os.Stat(path + ".lock")
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("lock is not private")
	}
}

func TestMCPConfigStoreLockWaitCanBeCancelled(t *testing.T) {
	path := configuredPlugin(t)
	release, err := acquireMCPConfigLock(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	store := MCPConfigStore{Path: path}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	if err := store.SaveApproval(ctx, "kmp", domain.ApprovalAuto); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("lock cancellation: %v", err)
	}
	if time.Since(started) > time.Second {
		t.Fatal("lock cancellation was unresponsive")
	}
	config, err := readMCPConfig(path)
	if err != nil || config.Plugins[0].Approval != "" {
		t.Fatal("cancelled lock waiter changed configuration")
	}
}

func TestMCPConfigLockRejectsSymlinkAndPublicFile(t *testing.T) {
	for _, kind := range []string{"symlink", "public"} {
		t.Run(kind, func(t *testing.T) {
			path := configuredPlugin(t)
			if kind == "symlink" {
				if err := os.Symlink(path, path+".lock"); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.WriteFile(path+".lock", nil, 0644); err != nil {
					t.Fatal(err)
				}
			}
			store := MCPConfigStore{Path: path}
			if store.SaveApproval(context.Background(), "kmp", domain.ApprovalAuto) == nil {
				t.Fatal("unsafe lock path accepted")
			}
		})
	}
}
