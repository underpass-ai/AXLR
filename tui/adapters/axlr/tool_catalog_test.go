package axlr

import (
	"context"
	"encoding/json"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/plugins"
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
	if err != nil || len(snapshot) != 4 {
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
	if err != nil || len(snapshot) != 6 {
		t.Fatalf("%+v %v", snapshot, err)
	}
	seen := map[root.ToolName]bool{}
	for _, tool := range snapshot[4:] {
		if !regexp.MustCompile("^mcp_[0-9a-f]{48}$").MatchString(string(tool.Definition.Name)) || seen[tool.Definition.Name] || tool.Identity.Plugin.ToolName != "echo" {
			t.Fatalf("%+v", tool)
		}
		seen[tool.Definition.Name] = true
	}
	if snapshot[4].Identity.Plugin.PluginID == snapshot[5].Identity.Plugin.PluginID {
		t.Fatal("plugin identity lost")
	}
}
