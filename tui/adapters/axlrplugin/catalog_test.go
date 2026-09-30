package axlrplugin

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type mcpStub struct{ paths []string }

func (s *mcpStub) InstallManifestWithEnvironment(_ context.Context, path string, _, _ map[string]string) error {
	s.paths = append(s.paths, path)
	return nil
}
func (*mcpStub) List(context.Context) ([]domain.PluginState, error)                    { return nil, nil }
func (*mcpStub) SetApproval(context.Context, root.PluginID, domain.ApprovalMode) error { return nil }
func (s *mcpStub) InstallManifest(_ context.Context, path string) error {
	s.paths = append(s.paths, path)
	return nil
}
func (*mcpStub) InstallURL(context.Context, root.PluginID, string) error { return nil }

func TestCodexMarketplaceInstallsOnlyIntoAXLR(t *testing.T) {
	source := t.TempDir()
	axlr := t.TempDir()
	packageRoot := filepath.Join(source, "plugins", "sample")
	mustWrite(t, filepath.Join(source, ".agents", "plugins", "marketplace.json"), `{"name":"third-party","plugins":[{"name":"sample","source":{"source":"local","path":"./plugins/sample"},"policy":{"installation":"AVAILABLE"}}]}`)
	mustWrite(t, filepath.Join(packageRoot, ".codex-plugin", "plugin.json"), `{"name":"sample","version":"1.2.3","description":"Example","skills":"./skills/","mcpServers":{"sample":{"command":"/bin/echo","args":["ready"]}}}`)
	mustWrite(t, filepath.Join(packageRoot, "skills", "example", "SKILL.md"), "---\nname: example\ndescription: Example skill\n---\nUse this skill.\n")
	mcp := &mcpStub{}
	catalog := Catalog{Root: axlr, MCP: mcp}
	ctx := context.Background()
	if err := catalog.AddSource(ctx, source); err != nil {
		t.Fatal(err)
	}
	available, err := catalog.List(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(available) != 3 || available[2].ID != "sample" || available[2].Installed {
		t.Fatalf("available: %#v", available)
	}
	if err := catalog.Install(ctx, "sample"); err != nil {
		t.Fatal(err)
	}
	installed, err := catalog.List(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(installed) != 3 || !installed[2].Installed {
		t.Fatalf("installed: %#v", installed)
	}
	if len(mcp.paths) != 1 || !strings.HasPrefix(mcp.paths[0], filepath.Join(axlr, "plugins", "installed")) {
		t.Fatalf("AXLR MCP manifest paths: %v", mcp.paths)
	}
	if _, err := os.Stat(filepath.Join(packageRoot, ".axlr-mcp")); !os.IsNotExist(err) {
		t.Fatalf("Codex source mutated: %v", err)
	}
	guidance, err := catalog.Guidance(ctx)
	if err != nil || !strings.Contains(guidance, "Example skill") || !strings.Contains(guidance, "SKILL.md") {
		t.Fatalf("guidance: %q %v", guidance, err)
	}
}
func TestCodexPluginSourceAndPathSafety(t *testing.T) {
	source := t.TempDir()
	mustWrite(t, filepath.Join(source, ".codex-plugin", "plugin.json"), `{"name":"sample","version":"1.0","skills":"./skills/"}`)
	catalog := Catalog{Root: t.TempDir()}
	ctx := context.Background()
	if err := catalog.AddSource(ctx, source); err != nil {
		t.Fatal(err)
	}
	if err := catalog.AddSource(ctx, source); err == nil {
		t.Fatal("duplicate source accepted")
	}
	if err := catalog.AddSource(ctx, source+"#../escape"); err == nil {
		t.Fatal("traversal accepted")
	}
}
func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestCodexMCPFileAndEnvironment(t *testing.T) {
	rootDir := t.TempDir()
	mustWrite(t, filepath.Join(rootDir, ".codex-plugin", "plugin.json"), `{"name":"example","version":"1","mcpServers":"./.mcp.json"}`)
	mustWrite(t, filepath.Join(rootDir, ".mcp.json"), `{"mcpServers":{"worker":{"command":"${CLAUDE_PLUGIN_ROOT}/run.sh","args":["${CLAUDE_PLUGIN_ROOT}/config.json"],"env_vars":["API_KEY"],"env":{"PLUGIN_DIR":"${CLAUDE_PLUGIN_ROOT}"}}}}`)
	m, err := loadManifest(rootDir)
	if err != nil {
		t.Fatal(err)
	}
	definitions, err := materializeMCP(rootDir, m)
	if err != nil {
		t.Fatal(err)
	}
	if len(definitions) != 1 || definitions[0].EnvFrom["API_KEY"] != "API_KEY" || definitions[0].Env["PLUGIN_DIR"] != rootDir {
		t.Fatalf("MCP definitions: %+v", definitions)
	}
	data, err := os.ReadFile(definitions[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), filepath.Join(rootDir, "run.sh")) {
		t.Fatalf("root token not expanded: %s", data)
	}
}
