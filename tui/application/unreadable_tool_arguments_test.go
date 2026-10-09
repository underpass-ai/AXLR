package application

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/adapters/openrouter"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestResolveToolCallRefusesUnreadableArguments(t *testing.T) {
	for _, tc := range []struct{ arguments, want string }{
		{`{"axlr_malformed_arguments":{"error":"unexpected end of JSON input","raw_prefix":"{\"path\":\"a.go"}}`, `the call's arguments were not a valid JSON object (unexpected end of JSON input); send the complete call again`},
		{`{"axlr_malformed_arguments":{"error":"unexpected end of JSON input","raw_prefix":"{\"path\":","cut_by_output_limit":true}}`, `the model's output limit cut the call's arguments (unexpected end of JSON input); send the complete call again, shorter`},
		{` { "axlr_malformed_arguments" : { } } `, `the call's arguments were not a valid JSON object (unknown error); send the complete call again`},
	} {
		call := root.ToolCall{ID: "a", Name: "read", Arguments: hostJSON(t, tc.arguments)}
		if _, _, _, err := ResolveToolCall(turnTools(), call); err == nil || err.Error() != tc.want {
			t.Fatalf("%s: error = %v", tc.arguments, err)
		}
	}
	if _, _, known, err := ResolveToolCall(turnTools(), root.ToolCall{ID: "a", Name: "read", Arguments: hostJSON(t, `{"path":"axlr_malformed_arguments"}`)}); !known || err != nil {
		t.Fatalf("ordinary arguments refused: %v %v", known, err)
	}
}

// End to end: a model stream whose tool call the output limit cut short
// (claude-haiku sent such a call 1 in 24 requests) answers the model with a
// tool error in the same turn: the session keeps the call and its error, the
// next request carries the error and the operation is not interrupted.
func TestCutToolCallIsAnsweredWithAToolError(t *testing.T) {
	var requests []map[string]any
	models, err := openrouter.New(openrouter.ClientConfig{Endpoint: "http://127.0.0.1:8080/v1/chat/completions", HTTPClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		var request map[string]any
		if err := json.Unmarshal(raw, &request); err != nil {
			t.Fatal(err)
		}
		requests = append(requests, request)
		events := []string{`{"choices":[{"index":0,"delta":{"role":"assistant","content":"done"},"finish_reason":"stop"}]}`}
		if len(requests) == 1 {
			events = []string{
				`{"choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"read","arguments":"{\"path\":\"a.go\","}}]}}]}`,
				`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"content\":\"package"}}]},"finish_reason":"length"}]}`,
			}
		}
		body := "data: " + strings.Join(append(events, "[DONE]"), "\n\ndata: ") + "\n\n"
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}})
	if err != nil {
		t.Fatal(err)
	}
	s := turnSession(t)
	if err := s.BeginTurn("write a.go", turnTools()); err != nil {
		t.Fatal(err)
	}
	store := &memoryStore{}
	if err := (AgentTurnUseCase{Continue: ContinueTurnUseCase{Store: store, Models: models}}).Execute(context.Background(), &s, ignoreEvent); err != nil {
		t.Fatalf("turn failed: %v (status %s)", err, s.Status())
	}
	if s.Status() != domain.StatusComplete || len(requests) != 2 {
		t.Fatalf("status %s after %d requests", s.Status(), len(requests))
	}
	activity := s.Export().Activity
	if len(activity) != 1 || activity[0].Outcome == nil || !activity[0].Outcome.IsError || !strings.Contains(string(activity[0].Outcome.Content), "the model's output limit cut the call's arguments") {
		t.Fatalf("activity = %+v", activity)
	}
	messages, _ := requests[1]["messages"].([]any)
	last, _ := messages[len(messages)-1].(map[string]any)
	if last["role"] != "tool" || last["tool_call_id"] != "call_1" || !strings.Contains(last["content"].(string), "send the complete call again") {
		t.Fatalf("next request ends with %v", last)
	}
}

// The person's decision path refuses such a call too: approving it records
// the error for the model and runs nothing; the next call waits its turn.
func TestApprovingAnUnreadableToolCallRecordsAToolError(t *testing.T) {
	cut := root.ToolCall{ID: "a", Name: "read", Arguments: hostJSON(t, `{"axlr_malformed_arguments":{"error":"unexpected end of JSON input","raw_prefix":"{","cut_by_output_limit":true}}`)}
	s := queued(t, cut, call("b", "read"))
	executions := 0
	u := ResolveToolUseCase{Store: &memoryStore{}, Tools: executionFunc(func(context.Context, domain.ToolIdentity, root.JSONValue) (domain.ToolOutcome, error) {
		executions++
		return domain.ToolOutcome{Content: "ok"}, nil
	})}
	if err := u.Execute(context.Background(), &s, "a", domain.DecisionApprove, nil); err != nil {
		t.Fatal(err)
	}
	outcome := s.Export().Activity[0].Outcome
	if executions != 0 || outcome == nil || !outcome.IsError || !strings.Contains(string(outcome.Content), "the model's output limit cut the call's arguments") || len(s.Pending()) != 1 || s.Pending()[0].Call.ID != "b" {
		t.Fatalf("executions %d, outcome %+v, pending %+v", executions, outcome, s.Pending())
	}
}
