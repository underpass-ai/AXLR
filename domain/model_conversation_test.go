package domain

import (
	"encoding/json"
	"testing"
)

func TestModelIdentifiersRejectBlankAndControl(t *testing.T) {
	for _, input := range []string{"  ", "a\n"} {
		if _, err := NewModelID(input); err == nil {
			t.Errorf("model ID %q accepted", input)
		}
		if _, err := NewToolName(input); err == nil {
			t.Errorf("tool name %q accepted", input)
		}
		if _, err := NewToolCallID(input); err == nil {
			t.Errorf("tool call ID %q accepted", input)
		}
	}
	for _, input := range []string{"openai/gpt-4o", "lookup", "call_1"} {
		if _, err := NewModelID(input); err != nil {
			t.Errorf("model ID %q rejected: %v", input, err)
		}
	}
}

func TestJSONObjectCopiesAndRejectsNonObjects(t *testing.T) {
	for _, input := range []string{"[]", `"text"`, "{bad"} {
		if _, err := NewJSONObject([]byte(input)); err == nil {
			t.Errorf("non-object %q accepted", input)
		}
	}
	raw := []byte(`{"query":"first"}`)
	object, err := NewJSONObject(raw)
	if err != nil {
		t.Fatal(err)
	}
	raw[10] = 'X'
	read := object.Bytes()
	if string(read) != `{"query":"first"}` {
		t.Fatalf("stored JSON changed with input: %s", read)
	}
	read[10] = 'Y'
	if string(object.Bytes()) != `{"query":"first"}` {
		t.Fatal("stored JSON changed with output")
	}
	encoded, err := json.Marshal(object)
	if err != nil || string(encoded) != `{"query":"first"}` {
		t.Fatalf("object marshaled as %s, %v", encoded, err)
	}
}

func TestCompletionRequestValidatesConversation(t *testing.T) {
	object, err := NewJSONObject([]byte(`{"type":"object"}`))
	if err != nil {
		t.Fatal(err)
	}
	request := CompletionRequest{
		Model: "openai/gpt-4o",
		Tools: []ToolDefinition{{Name: "search", Description: "Search", Parameters: object}},
		Messages: []Message{
			{Role: RoleUser, Content: "Find this"},
			{Role: RoleAssistant, Content: "Checking", ToolCalls: []ToolCall{
				{ID: "call_1", Name: "search", Arguments: object},
				{ID: "call_2", Name: "search", Arguments: object},
			}},
			{Role: RoleTool, ToolCallID: "call_1", Content: "result one"},
			{Role: RoleTool, ToolCallID: "call_2"},
		},
	}
	if err := request.Validate(); err != nil {
		t.Fatalf("valid conversation rejected: %v", err)
	}

	noMessages := request
	noMessages.Messages = nil
	if err := noMessages.Validate(); err == nil {
		t.Fatal("empty conversation accepted")
	}

	duplicateNames := request
	duplicateNames.Tools = append([]ToolDefinition{}, request.Tools...)
	duplicateNames.Tools = append(duplicateNames.Tools, request.Tools[0])
	if err := duplicateNames.Validate(); err == nil {
		t.Fatal("duplicate tool names accepted")
	}

	duplicateCalls := request
	duplicateCalls.Messages = append([]Message{}, request.Messages...)
	duplicateCalls.Messages[1].ToolCalls = append([]ToolCall{}, request.Messages[1].ToolCalls...)
	duplicateCalls.Messages[1].ToolCalls[1].ID = "call_1"
	if err := duplicateCalls.Validate(); err == nil {
		t.Fatal("duplicate assistant call IDs accepted")
	}

	unmatchedResult := request
	unmatchedResult.Messages = append([]Message{}, request.Messages...)
	unmatchedResult.Messages[3].ToolCallID = "call_missing"
	if err := unmatchedResult.Validate(); err == nil {
		t.Fatal("unmatched tool result accepted")
	}

	repeatedResult := request
	repeatedResult.Messages = append(append([]Message{}, request.Messages...), request.Messages[2])
	if err := repeatedResult.Validate(); err == nil {
		t.Fatal("repeated tool result accepted")
	}
}

func TestJSONObjectCanServePluginAndModelToolArguments(t *testing.T) {
	object, err := NewJSONObject([]byte(`{"query":"shared"}`))
	if err != nil {
		t.Fatal(err)
	}
	plugin := PluginCall{Arguments: object}
	model := ToolCall{ID: "call_1", Name: "search", Arguments: object}
	if string(plugin.Arguments.Bytes()) != `{"query":"shared"}` || model.validate() != nil {
		t.Fatal("shared object is invalid for plugin or model call")
	}
	array, err := NewJSONValue([]byte(`[]`))
	if err != nil {
		t.Fatal(err)
	}
	if err := (ToolCall{ID: "call_2", Name: "search", Arguments: array}).validate(); err == nil {
		t.Fatal("model tool call accepted a JSON array")
	}
}
