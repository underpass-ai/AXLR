package axlr

import (
	"context"
	"encoding/json"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/plugins"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
	"os"
	"regexp"
	"testing"
)

func TestToolCatalogLocalSchemasAndFrozenLookup(t *testing.T) {
	m, err := plugins.NewManager(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	c := ToolCatalog{Plugins: m}
	snapshot, err := c.Snapshot(context.Background())
	if err != nil || len(snapshot) != len(localToolDefinitions())+len(application.HostTools()) {
		t.Fatalf("%+v %v", snapshot, err)
	}
	for i, op := range []string{"read", "write", "edit", "exec"} {
		tool := snapshot[i]
		if tool.Definition.Name != root.ToolName("local_"+op) || tool.Identity.LocalOperation != op {
			t.Fatalf("%+v", tool)
		}
		var schema struct {
			Type                 string
			Properties           map[string]json.RawMessage
			Required             []string
			AdditionalProperties bool
		}
		if err := json.Unmarshal(tool.Definition.Parameters.Bytes(), &schema); err != nil || schema.Type != "object" || len(schema.Required) == 0 || len(schema.Properties) == 0 || schema.AdditionalProperties {
			t.Fatalf("bad schema %s", tool.Definition.Parameters.Bytes())
		}
		id, err := ResolveTool(snapshot, tool.Definition.Name)
		if err != nil || id != tool.Identity {
			t.Fatalf("%+v %v", id, err)
		}
	}
	for _, name := range []root.ToolName{"local.read", "local_remove", "mcp_fake"} {
		if _, err := ResolveTool(snapshot, name); err == nil {
			t.Fatalf("resolved %s", name)
		}
	}
	snapshot[1].Definition.Name = snapshot[0].Definition.Name
	if _, err := ResolveTool(snapshot, "local_read"); err == nil {
		t.Fatal("ambiguous alias accepted")
	}
	snapshot[0].Definition.Name = "bad/name"
	if _, err := ResolveTool(snapshot, "bad/name"); err == nil {
		t.Fatal("invalid alias accepted")
	}
}

func TestToolCatalogFilteredPluginsAndSameName(t *testing.T) {
	m := testManager(t, "")
	snapshot, err := (ToolCatalog{Plugins: m}).Snapshot(context.Background())
	pluginSnapshot := pluginTools(snapshot)
	if err != nil || len(pluginSnapshot) != 2 {
		t.Fatalf("%+v %v", snapshot, err)
	}
	seen := map[root.ToolName]bool{}
	for _, tool := range pluginSnapshot {
		if !regexp.MustCompile("^mcp_[0-9a-f]{48}$").MatchString(string(tool.Definition.Name)) || seen[tool.Definition.Name] || tool.Identity.Plugin.ToolName != "echo" {
			t.Fatalf("%+v", tool)
		}
		seen[tool.Definition.Name] = true
	}
	if pluginSnapshot[0].Identity.Plugin.PluginID == pluginSnapshot[1].Identity.Plugin.PluginID {
		t.Fatal("plugin identity lost")
	}
}

func TestToolCatalogKeepsUniqueNativeNamesUsableFromGuides(t *testing.T) {
	registration, err := plugins.NewRegistration(plugins.Manifest{ID: "guide-plugin", Command: os.Args[0], Args: []string{"-test.run=^TestMCPHelper$"}, AllowTools: []root.PluginToolName{"echo"}}, []string{"AXLR_TUI_HELPER=1", "IDENTITY=guide-plugin"})
	if err != nil {
		t.Fatal(err)
	}
	manager, err := plugins.NewManager([]plugins.Registration{registration})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	snapshot, err := (ToolCatalog{Plugins: manager}).Snapshot(context.Background())
	pluginSnapshot := pluginTools(snapshot)
	if err != nil || len(pluginSnapshot) != 1 || pluginSnapshot[0].Definition.Name != "echo" {
		t.Fatalf("unique native name lost: %+v %v", snapshot, err)
	}
	id, err := ResolveTool(snapshot, "echo")
	if err != nil || id.Plugin.PluginID != "guide-plugin" || id.Plugin.ToolName != "echo" {
		t.Fatalf("native name lost exact identity: %+v %v", id, err)
	}
}

func pluginTools(snapshot []domain.AvailableTool) []domain.AvailableTool {
	var result []domain.AvailableTool
	for _, tool := range snapshot {
		if tool.Identity.Kind == domain.ToolKindPlugin {
			result = append(result, tool)
		}
	}
	return result
}

func TestToolCatalogReservesHostAndLocalNamesWithoutHidingPlugins(t *testing.T) {
	names := []root.PluginToolName{"axlr_tools", "axlr_history", "axlr_call_tool", "local_read"}
	registration, err := plugins.NewRegistration(plugins.Manifest{ID: "collision", Command: os.Args[0], Args: []string{"-test.run=^TestMCPHelper$"}, AllowTools: names}, []string{"AXLR_TUI_HELPER=1", "IDENTITY=collision", "AXLR_TUI_TOOL_NAMES=axlr_tools,axlr_history,axlr_call_tool,local_read"})
	if err != nil {
		t.Fatal(err)
	}
	manager, err := plugins.NewManager([]plugins.Registration{registration})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	snapshot, err := (ToolCatalog{Plugins: manager}).Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	pluginSnapshot := pluginTools(snapshot)
	if len(pluginSnapshot) != len(names) {
		t.Fatal("colliding plugin tools disappeared")
	}
	for _, tool := range pluginSnapshot {
		if !regexp.MustCompile(`^mcp_[0-9a-f]{48}$`).MatchString(string(tool.Definition.Name)) {
			t.Fatalf("plugin stole reserved alias: %+v", tool)
		}
		id, err := ResolveTool(snapshot, tool.Definition.Name)
		if err != nil || id != tool.Identity {
			t.Fatal("alias lost exact plugin authority")
		}
	}
	for _, host := range application.HostTools() {
		id, err := ResolveTool(snapshot, host.Definition.Name)
		if err != nil || id.Kind != domain.ToolKindHost || id != host.Identity {
			t.Fatal("reserved host authority changed")
		}
	}
}
