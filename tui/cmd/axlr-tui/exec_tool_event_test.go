package main

import (
	"encoding/json"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func toolCall(t *testing.T, name, arguments string) root.ToolCall {
	t.Helper()
	value, err := root.NewJSONObject([]byte(arguments))
	if err != nil {
		t.Fatal(err)
	}
	return root.ToolCall{ID: "call-1", Name: root.ToolName(name), Arguments: value}
}

// A requested call's line carries the arguments the model gave, compact,
// so a reader sees which plugin tool an axlr_call_tool asks for.
func TestExecToolEventCarriesTheArguments(t *testing.T) {
	event := execToolEvent(domain.PendingTool{Call: toolCall(t, "axlr_call_tool", `{ "name": "synchro_status", "arguments": {"env": "pro"} }`)}, nil)
	if event.Type != "tool.requested" || event.Tool != "axlr_call_tool" || event.Call != "call-1" {
		t.Fatalf("event %+v", event)
	}
	if string(event.Arguments) != `{"name":"synchro_status","arguments":{"env":"pro"}}` {
		t.Fatalf("arguments %s", event.Arguments)
	}
	line, _ := json.Marshal(event)
	if !strings.Contains(string(line), `"arguments":{"name":"synchro_status"`) {
		t.Fatalf("line %s", line)
	}
}

// Arguments past the bound are left out whole, never cut into invalid JSON.
func TestExecToolEventLeavesOutArgumentsPastTheBound(t *testing.T) {
	large := `{"content":"` + strings.Repeat("x", maxExecToolArguments) + `"}`
	event := execToolEvent(domain.PendingTool{Call: toolCall(t, "write_file", large)}, nil)
	if event.Arguments != nil {
		t.Fatalf("arguments of %d bytes kept", len(event.Arguments))
	}
	line, _ := json.Marshal(event)
	if strings.Contains(string(line), `"arguments"`) {
		t.Fatalf("line %s", line)
	}
}

// A completed call says how it ended: ok, or failed with the first line of
// what it answered, cut short; its arguments are not repeated.
func TestExecToolEventSaysHowTheCallEnded(t *testing.T) {
	call := toolCall(t, "synchro_status", `{"env":"pro"}`)
	ok := execToolEvent(domain.PendingTool{Call: call, Decision: domain.ToolDecision("approve"), Outcome: &domain.ToolOutcome{Content: "fine"}}, nil)
	if ok.Type != "tool.completed" || ok.Status != "ok" || ok.Error != "" || ok.Arguments != nil || ok.Decision != "approve" {
		t.Fatalf("ok %+v", ok)
	}
	failed := execToolEvent(domain.PendingTool{Call: call, Outcome: &domain.ToolOutcome{Content: root.Text("\n  expired SSO session  \nsecond line"), IsError: true}}, nil)
	if failed.Status != "failed" || failed.Error != "expired SSO session" {
		t.Fatalf("failed %+v", failed)
	}
	long := execToolEvent(domain.PendingTool{Call: call, Outcome: &domain.ToolOutcome{Content: root.Text(strings.Repeat("é", 300)), IsError: true}}, nil)
	if got := []rune(long.Error); len(got) != maxExecToolError || got[len(got)-1] != '…' {
		t.Fatalf("long error of %d runes", len(got))
	}
	empty := execToolEvent(domain.PendingTool{Call: call, Outcome: &domain.ToolOutcome{IsError: true}}, nil)
	if empty.Status != "failed" || empty.Error != "" {
		t.Fatalf("empty %+v", empty)
	}
}

// A plugin's tool is named as plugin/tool however the model reached it:
// through axlr_call_tool by its opaque alias, or by that alias directly; the
// console's own tools and an unknown name carry none.
func TestExecToolEventNamesThePluginToolTheCallReaches(t *testing.T) {
	snapshot := []domain.AvailableTool{
		{Definition: root.ToolDefinition{Name: "axlr_call_tool"}, Identity: domain.ToolIdentity{Kind: domain.ToolKindHost, LocalOperation: domain.HostOperationCallTool}},
		{Definition: root.ToolDefinition{Name: "mcp_0123456789abcdef"}, Identity: domain.ToolIdentity{Kind: domain.ToolKindPlugin, Plugin: root.PluginRef{PluginID: "checks", ToolName: "doctor"}}},
	}
	bridged := execToolEvent(domain.PendingTool{Call: toolCall(t, "axlr_call_tool", `{"name":"mcp_0123456789abcdef","arguments":{"probe":true}}`)}, snapshot)
	if bridged.PluginTool != "checks/doctor" {
		t.Fatalf("bridged %+v", bridged)
	}
	direct := execToolEvent(domain.PendingTool{Call: toolCall(t, "mcp_0123456789abcdef", `{}`), Outcome: &domain.ToolOutcome{Content: "fine"}}, snapshot)
	if direct.PluginTool != "checks/doctor" || direct.Type != "tool.completed" {
		t.Fatalf("direct %+v", direct)
	}
	for _, call := range []root.ToolCall{toolCall(t, "axlr_call_tool", `{"name":"unknown","arguments":{}}`), toolCall(t, "local_read", `{"path":"a"}`)} {
		if event := execToolEvent(domain.PendingTool{Call: call}, snapshot); event.PluginTool != "" {
			t.Fatalf("%s named %q", call.Name, event.PluginTool)
		}
	}
}
