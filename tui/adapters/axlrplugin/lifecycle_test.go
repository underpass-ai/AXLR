package axlrplugin

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/plugins"
	"github.com/underpass-ai/AXLR/tui/adapters/axlr"
	"github.com/underpass-ai/AXLR/tui/adapters/storage"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// lifecycle wires a catalog to the console's own MCP manager and persistent
// configuration, so a test sees what the console would see.
type lifecycle struct {
	catalog *Catalog
	config  string
	manager *axlr.PluginManager
	source  string
}

func newLifecycle(t *testing.T) lifecycle {
	t.Helper()
	data, home := t.TempDir(), t.TempDir()
	source := t.TempDir()
	config := filepath.Join(home, "mcp.json")
	runtime, err := plugins.NewManager(nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { runtime.Close() })
	store := &storage.MCPConfigStore{Path: config}
	manager := axlr.NewPluginManager(runtime, nil, store.SaveApproval)
	manager.SetInstaller(store.AddManifest)
	manager.SetEnvironmentInstaller(store.AddManifestWithEnvironment, func(string) string { return "" })
	manager.SetRemover(store.RemoveManifestsUnder)
	return lifecycle{catalog: &Catalog{Root: data, MCP: manager}, config: config, manager: manager, source: source}
}

func (l lifecycle) writePackage(t *testing.T, version, args, skill string) {
	t.Helper()
	mustWrite(t, filepath.Join(l.source, ".codex-plugin", "plugin.json"), `{"name":"sample","version":"`+version+`","description":"Example","skills":"./skills/","mcpServers":{"tools":{"command":"/bin/echo","args":["`+args+`"]}}}`)
	mustWrite(t, filepath.Join(l.source, "skills", skill, "SKILL.md"), "---\nname: "+skill+"\ndescription: "+skill+" skill\n---\nUse it.\n")
}

func (l lifecycle) installedDir() string {
	return filepath.Join(l.catalog.Root, "plugins", "installed")
}

func (l lifecycle) profiles() []domain.PluginProfile { return l.manager.Profiles() }

func (l lifecycle) configText(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(l.config)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// manifests returns the manifest path of every configured registration.
func (l lifecycle) manifests(t *testing.T) []string {
	t.Helper()
	var config struct {
		Plugins []struct {
			Manifest string `json:"manifest"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal([]byte(l.configText(t)), &config); err != nil {
		t.Fatal(err)
	}
	paths := make([]string, len(config.Plugins))
	for i, plugin := range config.Plugins {
		paths[i] = plugin.Manifest
	}
	return paths
}

// noHiddenCopies fails when an update or removal left a copy beside the
// installed packages.
func (l lifecycle) noHiddenCopies(t *testing.T) {
	t.Helper()
	entries, err := os.ReadDir(l.installedDir())
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			t.Fatalf("left behind: %s", entry.Name())
		}
	}
}

func TestUpdateReplacesInstalledPackageAndKeepsApproval(t *testing.T) {
	ctx := context.Background()
	l := newLifecycle(t)
	l.writePackage(t, "1.0.0", "one", "first")
	if err := l.catalog.AddSource(ctx, l.source); err != nil {
		t.Fatal(err)
	}
	if err := l.catalog.Install(ctx, "sample"); err != nil {
		t.Fatal(err)
	}
	if err := l.manager.SetApproval(ctx, "sample-tools", domain.ApprovalAuto); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(l.source, "skills", "first")); err != nil {
		t.Fatal(err)
	}
	l.writePackage(t, "2.0.0", "two", "second")
	if err := l.catalog.Update(ctx, "sample"); err != nil {
		t.Fatal(err)
	}
	items, err := l.catalog.List(ctx, true)
	if err != nil || len(items) != 4 || items[3].Version != "2.0.0" || !items[3].Installed || items[3].Source != l.source {
		t.Fatalf("after update: %#v %v", items, err)
	}
	guidance, err := l.catalog.Guidance(ctx)
	if err != nil || !strings.Contains(guidance, "Skills: second (second skill).") || strings.Contains(guidance, "first skill") {
		t.Fatalf("guidance after update: %q %v", guidance, err)
	}
	definition, err := os.ReadFile(filepath.Join(l.installedDir(), "sample", ".axlr-mcp", "tools.json"))
	if err != nil || !strings.Contains(string(definition), `"two"`) {
		t.Fatalf("MCP definition not rewritten: %s %v", definition, err)
	}
	config := l.configText(t)
	if strings.Count(config, `"manifest"`) != 1 || !strings.Contains(config, `"approval": "auto"`) {
		t.Fatalf("registration after update: %s", config)
	}
	if profiles := l.profiles(); len(profiles) != 1 || profiles[0].ID != "sample-tools" || profiles[0].Approval != domain.ApprovalAuto {
		t.Fatalf("running registrations: %#v", profiles)
	}
	l.noHiddenCopies(t)
}

func TestUpdateKeepsTheInstalledCopyWhenTheSourceBreaks(t *testing.T) {
	ctx := context.Background()
	l := newLifecycle(t)
	l.writePackage(t, "1.0.0", "one", "first")
	if err := l.catalog.AddSource(ctx, l.source); err != nil {
		t.Fatal(err)
	}
	if err := l.catalog.Install(ctx, "sample"); err != nil {
		t.Fatal(err)
	}
	if err := l.manager.SetApproval(ctx, "sample-tools", domain.ApprovalAuto); err != nil {
		t.Fatal(err)
	}
	before := l.configText(t)
	// The new version's server declares two transports: it fails only once
	// the new copy is in place, so the update has to put everything back.
	mustWrite(t, filepath.Join(l.source, ".codex-plugin", "plugin.json"), `{"name":"sample","version":"2.0.0","skills":"./skills/","mcpServers":{"tools":{"command":"/bin/echo","url":"https://example.com/mcp"}}}`)
	if err := l.catalog.Update(ctx, "sample"); err == nil || !strings.Contains(err.Error(), "one transport") {
		t.Fatalf("broken update: %v", err)
	}
	items, err := l.catalog.List(ctx, false)
	if err != nil || len(items) != 4 || items[3].Version != "1.0.0" {
		t.Fatalf("after failed update: %#v %v", items, err)
	}
	if after := l.configText(t); after != before {
		t.Fatalf("configuration changed:\n%s\nwant\n%s", after, before)
	}
	if profiles := l.profiles(); len(profiles) != 1 || profiles[0].Approval != domain.ApprovalAuto {
		t.Fatalf("running registrations: %#v", profiles)
	}
	l.noHiddenCopies(t)
	// A source that no longer provides the package is refused before
	// anything moves.
	mustWrite(t, filepath.Join(l.source, ".codex-plugin", "plugin.json"), `{"name":"other","version":"1.0.0"}`)
	if err := l.catalog.Update(ctx, "sample"); err == nil || !strings.Contains(err.Error(), "no longer provides") {
		t.Fatalf("renamed source: %v", err)
	}
	if err := l.catalog.Update(ctx, "kmp"); err == nil {
		t.Fatal("built-in package updated")
	}
	if err := l.catalog.Update(ctx, "missing"); err == nil || !strings.Contains(err.Error(), "not installed") {
		t.Fatalf("missing package: %v", err)
	}
}

func TestUninstallRemovesRegistrationsBeforeThePackage(t *testing.T) {
	ctx := context.Background()
	l := newLifecycle(t)
	// A server the person registered directly is not the package's.
	direct := filepath.Join(t.TempDir(), "notes.json")
	mustWrite(t, direct, `{"manifest_version":1,"id":"notes","command":"/bin/echo","args":[],"allow_tools":["*"]}`)
	if err := l.manager.InstallManifest(ctx, direct); err != nil {
		t.Fatal(err)
	}
	l.writePackage(t, "1.0.0", "one", "first")
	if err := l.catalog.AddSource(ctx, l.source); err != nil {
		t.Fatal(err)
	}
	if err := l.catalog.Install(ctx, "sample"); err != nil {
		t.Fatal(err)
	}
	if err := l.catalog.Uninstall(ctx, "sample"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(l.installedDir(), "sample")); !os.IsNotExist(err) {
		t.Fatalf("package directory: %v", err)
	}
	// Compare decoded paths: JSON escapes the backslashes of a Windows path.
	if manifests := l.manifests(t); len(manifests) != 1 || manifests[0] != direct {
		t.Fatalf("configuration after uninstall: %q", manifests)
	}
	if profiles := l.profiles(); len(profiles) != 1 || profiles[0].ID != root.PluginID("notes") {
		t.Fatalf("running registrations: %#v", profiles)
	}
	guidance, err := l.catalog.Guidance(ctx)
	if err != nil || strings.Contains(guidance, "sample") {
		t.Fatalf("guidance after uninstall: %q %v", guidance, err)
	}
	l.noHiddenCopies(t)
	// The source can be added and installed again.
	if err := l.catalog.AddSource(ctx, l.source); err != nil {
		t.Fatal(err)
	}
	if err := l.catalog.Install(ctx, "sample"); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"kmp", "made", "axlr", "missing", "../x"} {
		if err := l.catalog.Uninstall(ctx, id); err == nil {
			t.Fatalf("uninstalled %q", id)
		}
	}
}

// Without a manager that can remove servers, a package with servers is not
// removed: its registrations would point at a directory that is gone.
func TestUninstallRefusesWhenServersCannotBeRemoved(t *testing.T) {
	ctx := context.Background()
	source, data := t.TempDir(), t.TempDir()
	mustWrite(t, filepath.Join(source, ".codex-plugin", "plugin.json"), `{"name":"sample","version":"1.0.0","mcpServers":{"tools":{"command":"/bin/echo"}}}`)
	catalog := Catalog{Root: data, MCP: &mcpStub{}}
	if err := catalog.AddSource(ctx, source); err != nil {
		t.Fatal(err)
	}
	if err := catalog.Install(ctx, "sample"); err != nil {
		t.Fatal(err)
	}
	if err := catalog.Uninstall(ctx, "sample"); err == nil {
		t.Fatal("uninstalled without removing servers")
	}
	if _, err := os.Stat(filepath.Join(data, "plugins", "installed", "sample")); err != nil {
		t.Fatalf("package removed: %v", err)
	}
}
