package openrouter

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/underpass-ai/AXLR/domain"
)

func assertJSONEqual(t *testing.T, got []byte, want string) {
	t.Helper()
	var actual, expected any
	if err := json.Unmarshal(got, &actual); err != nil {
		t.Fatalf("invalid output JSON: %v", err)
	}
	if err := json.Unmarshal([]byte(want), &expected); err != nil {
		t.Fatalf("invalid expected JSON: %v", err)
	}
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("JSON mismatch\ngot:  %s\nwant: %s", got, want)
	}
}

func testObject(t *testing.T, raw string) domain.JSONObject {
	t.Helper()
	object, err := domain.NewJSONObject([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	return object
}

func TestMapRequestTextOnly(t *testing.T) {
	wire, err := mapRequest(domain.CompletionRequest{
		Model:    "openai/gpt-4o",
		Messages: []domain.Message{{Role: domain.RoleUser, Content: "Hi"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	assertJSONEqual(t, got, `{"model":"openai/gpt-4o","messages":[{"role":"user","content":"Hi"}],"stream":false}`)
}

func TestMapRequestToolResultFollowUp(t *testing.T) {
	schema := testObject(t, `{"type":"object","properties":{"query":{"type":"string"}}}`)
	first := testObject(t, `{"query":"first"}`)
	second := testObject(t, `{"query":"second"}`)
	wire, err := mapRequest(domain.CompletionRequest{
		Model: "openai/gpt-4o",
		Tools: []domain.ToolDefinition{{Name: "search", Description: "Search documents", Parameters: schema}},
		Messages: []domain.Message{
			{Role: domain.RoleUser, Content: "Find both"},
			{Role: domain.RoleAssistant, ToolCalls: []domain.ToolCall{
				{ID: "call_1", Name: "search", Arguments: first},
				{ID: "call_2", Name: "search", Arguments: second},
			}},
			{Role: domain.RoleTool, ToolCallID: "call_1", Content: "first result"},
			{Role: domain.RoleTool, ToolCallID: "call_2", Content: "second result"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	assertJSONEqual(t, got, `{
		"model":"openai/gpt-4o",
		"messages":[
			{"role":"user","content":"Find both"},
			{"role":"assistant","content":null,"tool_calls":[
				{"id":"call_1","type":"function","function":{"name":"search","arguments":"{\"query\":\"first\"}"}},
				{"id":"call_2","type":"function","function":{"name":"search","arguments":"{\"query\":\"second\"}"}}
			]},
			{"role":"tool","tool_call_id":"call_1","content":"first result"},
			{"role":"tool","tool_call_id":"call_2","content":"second result"}
		],
		"tools":[{"type":"function","function":{"name":"search","description":"Search documents","parameters":{"type":"object","properties":{"query":{"type":"string"}}}}}],
		"stream":false
	}`)
}

func TestMapRequestKeepsAssistantTextAndCalls(t *testing.T) {
	args := testObject(t, `{}`)
	wire, err := mapRequest(domain.CompletionRequest{
		Model: "openai/gpt-4o",
		Messages: []domain.Message{
			{Role: domain.RoleUser, Content: "Hi"},
			{Role: domain.RoleAssistant, Content: "Checking", ToolCalls: []domain.ToolCall{{ID: "call_1", Name: "search", Arguments: args}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	assertJSONEqual(t, got, `{"model":"openai/gpt-4o","messages":[{"role":"user","content":"Hi"},{"role":"assistant","content":"Checking","tool_calls":[{"id":"call_1","type":"function","function":{"name":"search","arguments":"{}"}}]}],"stream":false}`)
}

func TestMapRequestRejectsInvalidDomain(t *testing.T) {
	if _, err := mapRequest(domain.CompletionRequest{}); err == nil {
		t.Fatal("invalid domain request mapped")
	}
}
