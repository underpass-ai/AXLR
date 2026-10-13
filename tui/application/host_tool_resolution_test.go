package application

import (
	"bytes"
	"encoding/json"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
	"reflect"
	"strings"
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

// A model that lost the earlier examples guessed field names one rejection at
// a time; the refusal must name the accepted shape and stay an argument error.
func TestHostWrapperRefusalNamesAcceptedFields(t *testing.T) {
	snapshot := append(turnTools(), HostTools()...)
	snapshot = append(snapshot, hostPlugin(t, "memory", "kmp", "kmp_wake"))
	for input, want := range map[string]string{
		`{"tool":"memory","arguments":{}}`: `unknown host argument "tool"; accepted: name, arguments, why`,
		`{"name":"memory","args":{}}`:      `unknown host argument "args"; accepted: name, arguments, why`,
		`["memory"]`:                       `host arguments must be an object; accepted: name, arguments, why`,
	} {
		_, _, _, err := ResolveToolCall(snapshot, root.ToolCall{Name: HostCallToolName, Arguments: hostJSON(t, input)})
		if err == nil || err.Error() != want {
			t.Fatalf("%s: got %v, want %q", input, err, want)
		}
		classified := false
		for _, word := range argumentWords {
			classified = classified || strings.Contains(strings.ToLower(err.Error()), word)
		}
		if !classified {
			t.Fatalf("%s: refusal no longer reads as an argument error", input)
		}
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
	if len(ModelTools(snapshot)) != 10 || len(ModelTools(turnTools())) != 1 {
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

// A why on a host tool that asks none is dropped, not refused: a model told
// to give its side effects a why gives one to everything.
func TestHostToolsDropAnUnaskedWhy(t *testing.T) {
	args, err := decodeHostArguments(hostJSON(t, `{"name":"kmp_ask","arguments":{"q":1},"why":"recall the decision"}`), "name", "arguments")
	if err != nil || string(args["name"]) != `"kmp_ask"` || args["why"] != nil {
		t.Fatalf("%v %v", args, err)
	}
	if _, err := decodeHostArguments(hostJSON(t, `{"name":"x","other":1}`), "name"); err == nil {
		t.Fatal("an unknown argument is still refused")
	}
}

// kmp_guide is the console's: the bridge refuses it with what to do
// instead, and discovery neither lists it nor describes it (decision G6
// of the lifecycle integration design).
func TestBridgeAndDiscoveryKeepKMPGuideToTheConsole(t *testing.T) {
	snapshot := append(HostTools(), hostPlugin(t, "kmp_guide", "kmp", "kmp_guide"), hostPlugin(t, "kmp_wake", "kmp", "kmp_wake"))
	call := root.ToolCall{ID: "c1", Name: HostCallToolName, Arguments: hostJSON(t, `{"name":"kmp_guide","arguments":{"registration_key":"session-1"},"why":"start the guide"}`)}
	if _, _, known, err := ResolveToolCall(snapshot, call); !known || err == nil || !strings.Contains(err.Error(), "axlr_recall") || !strings.Contains(err.Error(), "without context_id") {
		t.Fatalf("%v %v", known, err)
	}
	call.Arguments = hostJSON(t, `{"name":"kmp_wake","arguments":{"about":"project:x"},"why":"resume"}`)
	if tool, _, _, err := ResolveToolCall(snapshot, call); err != nil || tool.Identity.Plugin.ToolName != "kmp_wake" {
		t.Fatalf("%+v %v", tool, err)
	}
	if _, err := hostDiscover(snapshot, nil, hostJSON(t, `{"name":"kmp_guide"}`)); err == nil || !strings.Contains(err.Error(), "console") {
		t.Fatalf("schema served: %v", err)
	}
	listed, err := hostDiscover(snapshot, nil, hostJSON(t, `{"query":"guide"}`))
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(listed)
	if strings.Contains(string(encoded), "kmp_guide") {
		t.Fatalf("listed: %s", encoded)
	}
}
