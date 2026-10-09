package openrouter

import (
	"encoding/json"
	"strings"
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
		"no choices":          `{"choices":[]}`,
		"wrong role":          `{"choices":[{"message":{"role":"user","content":"Hi"},"finish_reason":"stop"}]}`,
		"blank call ID":       `{"choices":[{"message":{"role":"assistant","content":null,"tool_calls":[{"id":"","type":"function","function":{"name":"search","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`,
		"blank function name": `{"choices":[{"message":{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`,
		"duplicate call ID":   `{"choices":[{"message":{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"search","arguments":"{}"}},{"id":"call_1","type":"function","function":{"name":"search","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`,
		"empty assistant":     `{"choices":[{"message":{"role":"assistant","content":null},"finish_reason":"stop"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			wire, _ := mapFixture(t, raw)
			if _, err := mapResponse(wire); err == nil {
				t.Fatal("malformed completion accepted")
			}
		})
	}
}

func TestMapResponseReadsPromptCacheUsage(t *testing.T) {
	content := "ok"
	wire := responseDTO{Choices: []choiceDTO{{Message: messageDTO{Role: "assistant", Content: &content}, FinishReason: "stop"}}, Usage: &usageDTO{PromptTokens: 1000, CompletionTokens: 10, TotalTokens: 1010, PromptTokensDetails: &promptTokensDetailsDTO{CachedTokens: 800, CacheWriteTokens: 150}}}
	result, err := mapResponse(wire)
	if err != nil {
		t.Fatal(err)
	}
	if result.Usage == nil || result.Usage.CachedTokens != 800 || result.Usage.CacheWriteTokens != 150 || result.Usage.PromptTokens != 1000 {
		t.Fatalf("usage = %+v", result.Usage)
	}
	wire.Usage.PromptTokensDetails = nil
	if result, err := mapResponse(wire); err != nil || result.Usage.CachedTokens != 0 || result.Usage.CacheWriteTokens != 0 {
		t.Fatalf("usage without details = %+v (%v)", result.Usage, err)
	}
}

// A call whose arguments are not a JSON object (cut short, as claude-haiku
// sent 1 in 24 times, or not an object) is kept with a valid object naming
// the error, so the turn can answer the model with a tool error instead of
// failing; empty arguments are the empty object of a call without
// parameters. The kept prefix ends on a rune boundary.
func TestMapResponseKeepsUnreadableToolArgumentsForAToolError(t *testing.T) {
	long := strings.Repeat("x", 499) + "ñ" + strings.Repeat("y", 100)
	for _, tc := range []struct{ name, arguments, finish, want string }{
		{"empty", ``, "tool_calls", `{}`},
		{"blank", ` `, "tool_calls", `{}`},
		{"missing quote", `{"path":"a.go`, "tool_calls", `{"axlr_malformed_arguments":{"error":"unexpected end of JSON input","raw_prefix":"{\"path\":\"a.go"}}`},
		{"missing brace", `{"path":"a.go"`, "tool_calls", `{"axlr_malformed_arguments":{"error":"unexpected end of JSON input","raw_prefix":"{\"path\":\"a.go\""}}`},
		{"not an object", `[]`, "tool_calls", `{"axlr_malformed_arguments":{"error":"arguments must be a JSON object","raw_prefix":"[]"}}`},
		{"cut by the output limit", `{"path":"a.go","content":"pack`, "length", `{"axlr_malformed_arguments":{"error":"unexpected end of JSON input","raw_prefix":"{\"path\":\"a.go\",\"content\":\"pack","cut_by_output_limit":true}}`},
		{"long", `{"content":"` + long, "length", `{"axlr_malformed_arguments":{"error":"unexpected end of JSON input","raw_prefix":"{\"content\":\"` + strings.Repeat("x", 499) + `","cut_by_output_limit":true}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			arguments, _ := json.Marshal(tc.arguments)
			wire, _ := mapFixture(t, `{"choices":[{"message":{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"local_read","arguments":`+string(arguments)+`}}]},"finish_reason":"`+tc.finish+`"}]}`)
			got, err := mapResponse(wire)
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Message.ToolCalls) != 1 || got.Message.ToolCalls[0].ID != "call_1" || got.Message.ToolCalls[0].Name != "local_read" {
				t.Fatalf("calls = %+v", got.Message.ToolCalls)
			}
			if raw := string(got.Message.ToolCalls[0].Arguments.Bytes()); raw != tc.want {
				t.Fatalf("arguments = %s\nwant        %s", raw, tc.want)
			}
		})
	}
}

// An answer the output limit cut short (finish_reason "length") says so
// where the person and the model read it, instead of passing for complete.
func TestMapResponseMarksAnAnswerCutByTheOutputLimit(t *testing.T) {
	for _, tc := range []struct{ name, message, finish, want string }{
		{"cut text", `{"role":"assistant","content":"The three steps are: 1. Back up the"}`, "length", "The three steps are: 1. Back up the\n\n" + outputLimitNote},
		{"cut before any text", `{"role":"assistant","content":null}`, "length", outputLimitNote},
		{"complete text", `{"role":"assistant","content":"Done."}`, "stop", "Done."},
		{"cut after a whole call", `{"role":"assistant","content":"Reading","tool_calls":[{"id":"call_1","type":"function","function":{"name":"read","arguments":"{}"}}]}`, "length", "Reading"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wire, _ := mapFixture(t, `{"choices":[{"message":`+tc.message+`,"finish_reason":"`+tc.finish+`"}]}`)
			got, err := mapResponse(wire)
			if err != nil {
				t.Fatal(err)
			}
			if string(got.Message.Content) != tc.want || string(got.FinishReason) != tc.finish {
				t.Fatalf("content = %q, finish = %q", got.Message.Content, got.FinishReason)
			}
		})
	}
}
