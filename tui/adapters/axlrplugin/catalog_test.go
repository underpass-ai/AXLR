package axlrplugin

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
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
	if err != nil || !strings.Contains(guidance, "Example skill") || !strings.Contains(guidance, "axlr_skill") || strings.Contains(guidance, packageRoot) {
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
	if runtime.GOOS == "windows" && strings.Contains(content, `"command":"/bin/echo"`) {
		command, err := json.Marshal(os.Args[0])
		if err != nil {
			t.Fatal(err)
		}
		content = strings.ReplaceAll(content, `"command":"/bin/echo"`, `"command":`+string(command))
	}
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
	var manifest struct {
		Command string   `json:"command"`
		Args    []string `json:"args"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil || filepath.Clean(manifest.Command) != filepath.Join(rootDir, "run.sh") || len(manifest.Args) != 1 || filepath.Clean(manifest.Args[0]) != filepath.Join(rootDir, "config.json") {
		t.Fatalf("root token not expanded: %s", data)
	}
}

func TestInstalledSkillReadIsPagedAndWorkspaceIndependent(t *testing.T) {
	source := t.TempDir()
	mustWrite(t, filepath.Join(source, ".codex-plugin", "plugin.json"), `{"name":"sample","version":"1.0","skills":"./skills/"}`)
	mustWrite(t, filepath.Join(source, "skills", "example", "SKILL.md"), "# A😊B")
	catalog := Catalog{Root: t.TempDir()}
	ctx := context.Background()
	if err := catalog.AddSource(ctx, source); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.ReadSkill(ctx, "sample", "example", "SKILL.md", 0, 4); err == nil {
		t.Fatal("staged skill was readable")
	}
	if err := catalog.Install(ctx, "sample"); err != nil {
		t.Fatal(err)
	}
	first, err := catalog.ReadSkill(ctx, "sample", "example", "SKILL.md", 0, 4)
	if err != nil || first.Text != "# A" || first.NextOffsetBytes != 3 || first.TotalBytes != 8 || !first.HasMore {
		t.Fatalf("first page: %+v %v", first, err)
	}
	second, err := catalog.ReadSkill(ctx, "sample", "example", "SKILL.md", 3, 4)
	if err != nil || second.Text != "😊" || second.NextOffsetBytes != 7 || !second.HasMore {
		t.Fatalf("second page: %+v %v", second, err)
	}
	third, err := catalog.ReadSkill(ctx, "sample", "example", "SKILL.md", 7, 4)
	if err != nil || third.Text != "B" || third.HasMore {
		t.Fatalf("third page: %+v %v", third, err)
	}
}

func TestInstalledSkillReadRejectsEscapes(t *testing.T) {
	source := t.TempDir()
	mustWrite(t, filepath.Join(source, ".codex-plugin", "plugin.json"), `{"name":"sample","version":"1.0","skills":"./skills/"}`)
	mustWrite(t, filepath.Join(source, "skills", "example", "SKILL.md"), "safe")
	mustWrite(t, filepath.Join(source, "references", "detail.md"), "reference details")
	catalog := Catalog{Root: t.TempDir()}
	ctx := context.Background()
	if err := catalog.AddSource(ctx, source); err != nil {
		t.Fatal(err)
	}
	if err := catalog.Install(ctx, "sample"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		plugin, skill string
		offset, limit int
	}{{"../sample", "example", 0, 4}, {"sample", "../example", 0, 4}, {"sample", "example", -1, 4}, {"sample", "example", 1, 0}} {
		if _, err := catalog.ReadSkill(ctx, tc.plugin, tc.skill, "SKILL.md", tc.offset, tc.limit); err == nil {
			t.Fatalf("accepted %+v", tc)
		}
	}
	outside := filepath.Join(t.TempDir(), "secret.txt")
	mustWrite(t, outside, "outside")
	installed := filepath.Join(catalog.Root, "plugins", "installed", "sample", "skills", "example", "SKILL.md")
	if err := os.Remove(installed); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, installed); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.ReadSkill(ctx, "sample", "example", "SKILL.md", 0, 4); err == nil {
		t.Fatal("symlink escaped installed package")
	}
	page, err := catalog.ReadSkill(ctx, "sample", "example", "../../references/detail.md", 0, 4096)
	if err != nil || page.Text != "reference details" {
		t.Fatalf("shared skill reference: %+v %v", page, err)
	}
	for _, path := range []string{"../../../secret.txt", "/etc/passwd", "../../.axlr-source.json"} {
		if _, err := catalog.ReadSkill(ctx, "sample", "example", path, 0, 4096); err == nil {
			t.Fatalf("read unsafe plugin path %q", path)
		}
	}
}
