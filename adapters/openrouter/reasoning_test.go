package openrouter

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/domain"
)

// The fragments claude-haiku-5.5 streamed on 23 October 2026: one
// reasoning.text block in pieces of text, then its signature alone.
func TestStreamJoinsReasoningDetailsIntoTheMessage(t *testing.T) {
	wire := streamEvents(
		`{"choices":[{"index":0,"delta":{"role":"assistant","content":"","reasoning_details":[{"type":"reasoning.text","text":"Comparing the tenths","format":"anthropic-claude-v1","index":0}]}}]}`,
		`{"choices":[{"index":0,"delta":{"reasoning_details":[{"type":"reasoning.text","text":" place","format":"anthropic-claude-v1","index":0}]}}]}`,
		`{"choices":[{"index":0,"delta":{"reasoning_details":[{"type":"reasoning.text","signature":"CAQSqwc","format":"anthropic-claude-v1","index":0}]}}]}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"read","arguments":"{\"path\":\"a\"}"}}]},"finish_reason":"tool_calls"}]}`,
		"[DONE]")
	c, _ := streamClient(t, &trackedStreamBody{Reader: strings.NewReader(wire)}, 200)
	got, err := c.Stream(context.Background(), simpleCompletionRequest(), func(domain.Text) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	var blocks []map[string]any
	if err := json.Unmarshal(got.Message.Reasoning.Bytes(), &blocks); err != nil {
		t.Fatalf("reasoning %s: %v", got.Message.Reasoning.Bytes(), err)
	}
	if len(blocks) != 1 || blocks[0]["text"] != "Comparing the tenths place" || blocks[0]["signature"] != "CAQSqwc" || blocks[0]["type"] != "reasoning.text" || blocks[0]["format"] != "anthropic-claude-v1" {
		t.Fatalf("blocks %+v", blocks)
	}
}

// Blocks of different indexes stay apart; a fragment without an index joins
// the last block of its type.
func TestReasoningAccumulatorKeepsBlocksApart(t *testing.T) {
	var a reasoningAccumulator
	fragments := func(raw string) []map[string]json.RawMessage {
		var out []map[string]json.RawMessage
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	for _, raw := range []string{
		`[{"type":"reasoning.summary","summary":"Looked at","index":0}]`,
		`[{"type":"reasoning.summary","summary":" the files","index":0},{"type":"reasoning.encrypted","data":"abc","index":1}]`,
		`[{"type":"reasoning.encrypted","data":"def","index":1}]`,
		`[{"type":"reasoning.text","text":"x"}]`,
		`[{"type":"reasoning.text","text":"y","signature":null}]`,
	} {
		if err := a.Add(fragments(raw)); err != nil {
			t.Fatal(err)
		}
	}
	var blocks []map[string]any
	if err := json.Unmarshal(a.Result(), &blocks); err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 3 || blocks[0]["summary"] != "Looked at the files" || blocks[1]["data"] != "abcdef" || blocks[2]["text"] != "xy" {
		t.Fatalf("blocks %+v", blocks)
	}
	if (&reasoningAccumulator{}).Result() != nil {
		t.Fatal("a stream without reasoning has some")
	}
}

// The reasoning goes back with its assistant message, unmodified, and a
// message without reasoning sends no field (OpenRouter docs, "Preserving
// Reasoning").
func TestRequestSendsReasoningBackWithItsMessage(t *testing.T) {
	reasoning, err := domain.NewJSONValue([]byte(`[{"type":"reasoning.text","text":"plan","signature":"sig","format":"anthropic-claude-v1","index":0}]`))
	if err != nil {
		t.Fatal(err)
	}
	arguments, _ := domain.NewJSONObject([]byte(`{"path":"a"}`))
	var sent []byte
	c, err := New(ClientConfig{APIKey: "secret", HTTPClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		sent, _ = io.ReadAll(r.Body)
		return testResponse(200, `{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`), nil
	})}})
	if err != nil {
		t.Fatal(err)
	}
	request := domain.CompletionRequest{Model: "anthropic/claude-haiku-5.5", Messages: []domain.Message{
		{Role: domain.RoleUser, Content: "read a"},
		{Role: domain.RoleAssistant, ToolCalls: []domain.ToolCall{{ID: "call_1", Name: "read", Arguments: arguments}}, Reasoning: reasoning},
		{Role: domain.RoleTool, ToolCallID: "call_1", Content: "a"},
	}, Tools: []domain.ToolDefinition{{Name: "read", Description: "Read", Parameters: arguments}}}
	if _, err := c.Complete(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Messages []map[string]json.RawMessage `json:"messages"`
	}
	if err := json.Unmarshal(sent, &wire); err != nil {
		t.Fatal(err)
	}
	if string(wire.Messages[1]["reasoning_details"]) != string(reasoning.Bytes()) {
		t.Fatalf("assistant message %s", wire.Messages[1]["reasoning_details"])
	}
	for _, i := range []int{0, 2} {
		if _, present := wire.Messages[i]["reasoning_details"]; present {
			t.Fatalf("message %d carries reasoning", i)
		}
	}
}

func TestOnlyAnAssistantMessageCarriesAnArrayOfReasoning(t *testing.T) {
	array, _ := domain.NewJSONValue([]byte(`[{"type":"reasoning.text"}]`))
	object, _ := domain.NewJSONValue([]byte(`{"type":"reasoning.text"}`))
	if err := (domain.Message{Role: domain.RoleUser, Content: "hi", Reasoning: array}).Validate(); err == nil {
		t.Fatal("a user message carried reasoning")
	}
	if err := (domain.Message{Role: domain.RoleAssistant, Content: "hi", Reasoning: object}).Validate(); err == nil {
		t.Fatal("reasoning that is not an array was accepted")
	}
	if err := (domain.Message{Role: domain.RoleAssistant, Content: "hi", Reasoning: array}).Validate(); err != nil {
		t.Fatal(err)
	}
}
