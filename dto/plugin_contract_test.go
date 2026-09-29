package dto

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPluginCallArgsWireShape(t *testing.T) {
	b, err := json.Marshal(PluginCallArgs{PluginID: "search", ToolName: "find", Arguments: json.RawMessage(`{"query":"go"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(b); got != `{"plugin_id":"search","tool_name":"find","arguments":{"query":"go"}}` {
		t.Fatalf("unexpected wire shape: %s", got)
	}
}

func TestPluginOutputsWireShape(t *testing.T) {
	list, err := json.Marshal(PluginToolOutput{PluginID: "search", ToolName: "find", Description: "Find", InputSchema: json.RawMessage(`{"type":"object"}`)})
	if err != nil || !strings.Contains(string(list), `"plugin_id":"search"`) || !strings.Contains(string(list), `"input_schema":{"type":"object"}`) {
		t.Fatalf("list output: %s, %v", list, err)
	}
	call, err := json.Marshal(PluginCallOutput{Content: []json.RawMessage{json.RawMessage(`{"type":"text","text":"ok"}`)}, IsError: true})
	if err != nil || !strings.Contains(string(call), `"is_error":true`) {
		t.Fatalf("call output: %s, %v", call, err)
	}
}
