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
	ref := root.PluginRef{PluginID: root.PluginID(plugin), ToolName: root.PluginToolName(native)}
	id, err := domain.NewPluginToolIdentity(ref)
	if err != nil {
		t.Fatal(err)
	}
	return domain.AvailableTool{Identity: id, Definition: root.ToolDefinition{Name: root.ToolName(alias), Description: "Memory wake guide", Parameters: hostJSON(t, `{"type":"object","properties":{"about":{"type":"string"}},"required":["about"]}`)}, Surface: engineSurface(ref)}
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

// A tool whose surface the engine adapter hides (kmp_guide here) is the
// console's: the bridge refuses it with the surface's refusal, and
// discovery neither lists it nor describes it (decision G6 of the
// lifecycle integration design).
func TestBridgeAndDiscoveryKeepAHiddenToolToTheConsole(t *testing.T) {
	snapshot := append(HostTools(), hostPlugin(t, "kmp_guide", "kmp", "kmp_guide"), hostPlugin(t, "kmp_wake", "kmp", "kmp_wake"))
	call := root.ToolCall{ID: "c1", Name: HostCallToolName, Arguments: hostJSON(t, `{"name":"kmp_guide","arguments":{"registration_key":"session-1"},"why":"start the guide"}`)}
	if _, _, known, err := ResolveToolCall(snapshot, call); !known || err == nil || !strings.Contains(err.Error(), engineSurface(root.PluginRef{PluginID: "kmp", ToolName: "kmp_guide"}).Hidden) {
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

// A hidden tool called by its own name, not through the bridge, is
// refused the same way (until 14 October 2026 it resolved and ran).
func TestADirectCallOfAHiddenToolIsRefused(t *testing.T) {
	guide := hostPlugin(t, "kmp_guide", "kmp", "kmp_guide")
	guide.Surface = engineSurface(root.PluginRef{PluginID: "kmp", ToolName: "kmp_guide"})
	wake := hostPlugin(t, "kmp_wake", "kmp", "kmp_wake")
	snapshot := append(HostTools(), guide, wake)
	call := root.ToolCall{ID: "c1", Name: "kmp_guide", Arguments: hostJSON(t, `{"registration_key":"session-1"}`)}
	if _, _, known, err := ResolveToolCall(snapshot, call); !known || err == nil || !strings.Contains(err.Error(), guide.Surface.Hidden) {
		t.Fatalf("%v %v", known, err)
	}
	call = root.ToolCall{ID: "c2", Name: "kmp_wake", Arguments: hostJSON(t, `{"about":"project:x"}`)}
	if tool, _, _, err := ResolveToolCall(snapshot, call); err != nil || tool.Identity.Plugin.ToolName != "kmp_wake" {
		t.Fatalf("%+v %v", tool, err)
	}
}

// The orchestrator's write surface is the console's (decision D6): the
// verbs the adapter leaves visible are bridged and described; the ones it
// hides are refused by the bridge and hidden by discovery, which also
// stops counting them. Which MADE verb is which is ceremonyhost's table.
func TestBridgeAndDiscoveryKeepTheOrchestratorsWritesToTheConsole(t *testing.T) {
	allowed := []string{"made_get_help", "made_discover_capabilities", "made_list_ceremony_definitions", "made_get_ceremony_instance", "made_read_ceremony_events", "made_stream_ceremony", "made_search_ceremony_instances", "made_inspect_ceremony_resume", "made_design_ceremony", "made_validate_ceremony_draft", "made_explain_ceremony_draft", "made_diff_ceremony_definitions", "made_render_agentic_system_diagram", "made_generate_ceremony_report", "made_verify_ceremony_journal"}
	hidden := []string{"made_start_published_ceremony", "made_start_ceremony", "made_claim_ceremony_step", "made_complete_ceremony_step", "made_apply_ceremony_transition", "made_cancel_ceremony", "made_pause_ceremony", "made_resume_ceremony", "made_run_ceremony", "made_run_ceremony_step", "made_approve_ceremony_guard", "made_defer_ceremony_guard", "made_issue_authorization_grant", "made_revoke_authorization_grant", "made_publish_ceremony_definition", "made_register_agent", "made_unregister_agent", "made_delete_contract", "made_begin_artifact_upload", "made_put_artifact_chunk", "made_report_ceremony_agent_status", "made_renew_ceremony_step_lease", "made_record_ceremony_host_handoff", "made_orchestrate", "made_deliberate", "made_create_council", "made_design_agentic_system", "made_lease_council_events", "made_pull_ceremony_events", "made_request_ceremony_intervention"}
	var snapshot []domain.AvailableTool
	for _, name := range allowed {
		tool := hostPlugin(t, name, "made", name)
		tool.Surface = domain.ToolSurface{Role: domain.PluginRoleOrchestrator}
		snapshot = append(snapshot, tool)
	}
	for _, name := range hidden {
		tool := hostPlugin(t, name, "made", name)
		tool.Surface = domain.ToolSurface{Role: domain.PluginRoleOrchestrator, Hidden: "the console runs ceremonies"}
		snapshot = append(snapshot, tool)
	}
	snapshot = append(HostTools(), snapshot...)
	for _, name := range allowed {
		call := root.ToolCall{ID: "c", Name: HostCallToolName, Arguments: hostJSON(t, `{"name":"`+name+`","arguments":{},"why":"read"}`)}
		if tool, _, _, err := ResolveToolCall(snapshot, call); err != nil || string(tool.Identity.Plugin.ToolName) != name {
			t.Fatalf("%s refused: %v", name, err)
		}
		if _, err := hostDiscover(snapshot, nil, hostJSON(t, `{"name":"`+name+`"}`)); err != nil {
			t.Fatalf("%s undescribed: %v", name, err)
		}
	}
	for _, name := range hidden {
		call := root.ToolCall{ID: "c", Name: HostCallToolName, Arguments: hostJSON(t, `{"name":"`+name+`","arguments":{},"why":"write"}`)}
		if _, _, known, err := ResolveToolCall(snapshot, call); !known || err == nil || !strings.Contains(err.Error(), "the console runs ceremonies") {
			t.Fatalf("%s allowed: %v", name, err)
		}
		if _, err := hostDiscover(snapshot, nil, hostJSON(t, `{"name":"`+name+`"}`)); err == nil {
			t.Fatalf("%s described", name)
		}
	}
	listed, err := hostDiscover(snapshot, nil, hostJSON(t, `{"query":"made","limit":20}`))
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(listed)
	for _, name := range hidden {
		if strings.Contains(string(encoded), `"`+name+`"`) {
			t.Fatalf("%s listed", name)
		}
	}
	s := turnSession(t)
	if err := s.BeginTurn("hello", snapshot); err != nil {
		t.Fatal(err)
	}
	guidance := string(modelHostGuidance(&s, engineParagraphs{}).Content)
	if !strings.Contains(guidance, "Registered MCP plugin made: "+itoa(len(allowed))+" tools") {
		t.Fatalf("the hidden verbs are counted: %s", guidance)
	}
}
