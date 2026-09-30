package application

import (
	"bytes"
	"encoding/json"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
	"reflect"
	"testing"
)

func hostJSON(t *testing.T, text string) root.JSONValue {
	t.Helper()
	value, err := root.NewJSONValue([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func hostPlugin(t *testing.T, alias, plugin, native string) domain.AvailableTool {
	t.Helper()
	id, err := domain.NewPluginToolIdentity(root.PluginRef{PluginID: root.PluginID(plugin), ToolName: root.PluginToolName(native)})
	if err != nil {
		t.Fatal(err)
	}
	return domain.AvailableTool{Identity: id, Definition: root.ToolDefinition{Name: root.ToolName(alias), Description: "Memory wake guide", Parameters: hostJSON(t, `{"type":"object","properties":{"about":{"type":"string"}},"required":["about"]}`)}}
}

func TestHostWrapperResolvesExactPluginWithoutChangingCallOrSnapshot(t *testing.T) {
	snapshot := append(turnTools(), HostTools()...)
	kmp := hostPlugin(t, "kmp_wake", "kmp", "kmp_wake")
	snapshot = append(snapshot, kmp)
	before := append([]domain.AvailableTool(nil), snapshot...)
	call := root.ToolCall{ID: "same-call-id", Name: HostCallToolName, Arguments: hostJSON(t, `{"name":"kmp_wake","arguments":{"about":"project:AXLR","n":9007199254740993}}`)}
	tool, args, known, err := ResolveToolCall(snapshot, call)
	if err != nil || !known || tool.Identity != kmp.Identity || !bytes.Equal(args.Bytes(), []byte(`{"about":"project:AXLR","n":9007199254740993}`)) {
		t.Fatalf("resolution: %+v %s %v %v", tool, args.Bytes(), known, err)
	}
	if call.ID != "same-call-id" || !reflect.DeepEqual(before, snapshot) {
		t.Fatal("resolution changed persisted authority or call ID")
	}
	local, args, known, err := ResolveToolCall(snapshot, root.ToolCall{Name: "read", Arguments: hostJSON(t, `{"path":"x"}`)})
	if err != nil || !known || local.Identity.Kind != domain.ToolKindLocal || string(args.Bytes()) != `{"path":"x"}` {
		t.Fatal("direct local resolution changed")
	}
	if _, _, known, err := ResolveToolCall(snapshot, root.ToolCall{Name: "missing"}); err != nil || known {
		t.Fatal("unknown alias gained authority")
	}
}

func TestHostWrapperRejectsBypassAmbiguityAndMalformedInput(t *testing.T) {
	snapshot := append(turnTools(), HostTools()...)
	snapshot = append(snapshot, hostPlugin(t, "memory", "kmp", "kmp_wake"))
	for _, input := range []string{
		`{}`, `[]`, `{"name":"memory"}`, `{"name":"","arguments":{}}`, `{"name":null,"arguments":{}}`, `{"name":"memory","arguments":null}`, `{"name":"memory","arguments":[]}`,
		`{"name":"read","arguments":{}}`, `{"name":"axlr_history","arguments":{}}`, `{"name":"axlr_call_tool","arguments":{}}`, `{"name":"missing","arguments":{}}`, `{"name":"kmp_wake","arguments":{}}`,
		`{"name":"memory","arguments":{},"plugin":"other"}`, `{"name":"memory","name":"read","arguments":{}}`, `{"name":"memory","arguments":{},"arguments":{"bypass":true}}`,
	} {
		_, _, _, err := ResolveToolCall(snapshot, root.ToolCall{Name: HostCallToolName, Arguments: hostJSON(t, input)})
		if err == nil {
			t.Fatalf("wrapper input accepted: %s", input)
		}
	}
	ambiguous := append(append([]domain.AvailableTool(nil), snapshot...), hostPlugin(t, "memory", "made", "ceremony"))
	if _, _, _, err := ResolveToolCall(ambiguous, root.ToolCall{Name: HostCallToolName, Arguments: hostJSON(t, `{"name":"memory","arguments":{}}`)}); err == nil {
		t.Fatal("ambiguous snapshot accepted")
	}
	invalid := append([]domain.AvailableTool(nil), snapshot...)
	invalid[0].Identity = domain.ToolIdentity{}
	if _, _, _, err := ResolveToolCall(invalid, root.ToolCall{Name: HostCallToolName, Arguments: hostJSON(t, `{"name":"memory","arguments":{}}`)}); err == nil {
		t.Fatal("invalid authority snapshot accepted")
	}
	legacy := []domain.AvailableTool{hostPlugin(t, "axlr_call_tool", "legacy", "axlr_call_tool")}
	tool, _, known, err := ResolveToolCall(legacy, root.ToolCall{Name: HostCallToolName, Arguments: hostJSON(t, `{}`)})
	if err != nil || !known || tool.Identity.Kind != domain.ToolKindPlugin {
		t.Fatal("legacy plugin name became implicit host privilege")
	}
	if _, _, known, err := ResolveToolCall(turnTools(), root.ToolCall{Name: HostCallToolName}); known || err != nil {
		t.Fatal("host privilege fabricated outside frozen snapshot")
	}
}

func TestHostDefinitionsRemainStableAndExcludePluginSchemas(t *testing.T) {
	snapshot := append(turnTools(), HostTools()...)
	snapshot = append(snapshot, hostPlugin(t, "memory", "kmp", "kmp_wake"))
	before, _ := json.Marshal(ModelTools(snapshot))
	snapshot[len(snapshot)-1].Definition.Description = "A changed plugin description"
	snapshot = append(snapshot, hostPlugin(t, "ceremony", "made", "run"))
	after, _ := json.Marshal(ModelTools(snapshot))
	if !bytes.Equal(before, after) {
		t.Fatal("plugin refresh broke fixed model schema bytes")
	}
	if len(ModelTools(snapshot)) != 4 || len(ModelTools(turnTools())) != 1 {
		t.Fatal("fixed/legacy model projection")
	}
	definitions := HostTools()
	for _, tool := range definitions {
		if tool.Identity.Validate() != nil || tool.Identity.Kind != domain.ToolKindHost || !json.Valid(tool.Definition.Parameters.Bytes()) {
			t.Fatal("invalid host definition")
		}
	}
	definitions[0].Definition.Description = "changed"
	if reflect.DeepEqual(definitions, HostTools()) {
		t.Fatal("host definitions share mutable caller slice")
	}
}
