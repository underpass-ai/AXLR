package openrouter

import (
	"encoding/json"
	"testing"
)

func mapFixture(t *testing.T, raw string) (responseDTO, error) {
	t.Helper()
	var wire responseDTO
	if err := json.Unmarshal([]byte(raw), &wire); err != nil {
		t.Fatal(err)
	}
	return wire, nil
}

func TestMapResponseTextAndUsage(t *testing.T) {
	wire, _ := mapFixture(t, `{
		"choices":[
			{"message":{"role":"assistant","content":"Hello"},"finish_reason":"stop"},
			{"message":{"role":"assistant","content":"Ignore"},"finish_reason":"stop"}
		],
		"usage":{"prompt_tokens":3,"completion_tokens":4,"total_tokens":7}
	}`)
	got, err := mapResponse(wire)
	if err != nil {
		t.Fatal(err)
	}
	if got.Message.Content != "Hello" || got.FinishReason != "stop" || got.Usage == nil || got.Usage.PromptTokens != 3 || got.Usage.CompletionTokens != 4 || got.Usage.TotalTokens != 7 {
		t.Fatalf("wrong first completion: %+v", got)
	}
}

func TestMapResponseKeepsTextAndParallelToolCalls(t *testing.T) {
	wire, _ := mapFixture(t, `{
		"choices":[{"message":{"role":"assistant","content":"Checking","tool_calls":[
			{"id":"call_1","type":"function","function":{"name":"search","arguments":"{\"query\":\"first\"}"}},
			{"id":"call_2","type":"function","function":{"name":"search","arguments":"{\"query\":\"second\"}"}}
		]},"finish_reason":"tool_calls"}]
	}`)
	got, err := mapResponse(wire)
	if err != nil {
		t.Fatal(err)
	}
	if got.Message.Content != "Checking" || got.FinishReason != "tool_calls" || len(got.Message.ToolCalls) != 2 || got.Message.ToolCalls[0].ID != "call_1" || got.Message.ToolCalls[1].ID != "call_2" || got.Message.ToolCalls[0].Name != "search" || string(got.Message.ToolCalls[1].Arguments.Bytes()) != `{"query":"second"}` || got.Usage != nil {
		t.Fatalf("wrong tool completion: %+v", got)
	}
}

func TestMapResponseAcceptsNullContentWithToolCall(t *testing.T) {
	wire, _ := mapFixture(t, `{"choices":[{"message":{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"search","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`)
	got, err := mapResponse(wire)
	if err != nil || got.Message.Content != "" || len(got.Message.ToolCalls) != 1 {
		t.Fatalf("null-content tool call = %+v, %v", got, err)
	}
}

func TestMapResponseRejectsMalformedChoices(t *testing.T) {
	for name, raw := range map[string]string{
		"no choices":           `{"choices":[]}`,
		"wrong role":           `{"choices":[{"message":{"role":"user","content":"Hi"},"finish_reason":"stop"}]}`,
		"blank call ID":        `{"choices":[{"message":{"role":"assistant","content":null,"tool_calls":[{"id":"","type":"function","function":{"name":"search","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`,
		"blank function name":  `{"choices":[{"message":{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`,
		"duplicate call ID":    `{"choices":[{"message":{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"search","arguments":"{}"}},{"id":"call_1","type":"function","function":{"name":"search","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`,
		"non-object arguments": `{"choices":[{"message":{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"search","arguments":"[]"}}]},"finish_reason":"tool_calls"}]}`,
		"malformed arguments":  `{"choices":[{"message":{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"search","arguments":"{"}}]},"finish_reason":"tool_calls"}]}`,
		"empty assistant":      `{"choices":[{"message":{"role":"assistant","content":null},"finish_reason":"stop"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			wire, _ := mapFixture(t, raw)
			if _, err := mapResponse(wire); err == nil {
				t.Fatal("malformed completion accepted")
			}
		})
	}
}
