package contract

import (
	"encoding/json"
	"testing"

	axlr "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// The wire form keeps the field names of the contract and never writes a
// null array; the pending call carries its arguments.
func TestExecutionResultWireForm(t *testing.T) {
	arguments, err := axlr.NewJSONObject([]byte(`{"path":"a.txt"}`))
	if err != nil {
		t.Fatal(err)
	}
	result := domain.ExecutionResult{
		Status:   domain.ExecutionNeedsDecision,
		Session:  "second",
		Sessions: []domain.SessionID{"first", "second"},
		Answer:   "so far",
		Pending:  &axlr.ToolCall{ID: "c2", Name: "local_read", Arguments: arguments},
		Calls:    []domain.ExecutionCall{{ID: "c1", Tool: "local_exec", Decision: domain.DecisionAutoApprove, Status: domain.ExecutionCallCompleted}, {ID: "c2", Tool: "local_read", Status: domain.ExecutionCallPending}},
		Usage:    domain.ExecutionUsage{Requests: 2, PromptTokens: 10, CompletionTokens: 3, CostUSD: 0.5, CostRequests: 2},
	}
	bytes, err := json.Marshal(ExecutionResult(result))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"schema":"axlr.execution-result/v1","status":"needs_decision","session_id":"second","sessions":["first","second"],"answer":"so far","pending":{"call_id":"c2","tool":"local_read","arguments":{"path":"a.txt"}},"tool_calls":[{"call_id":"c1","tool":"local_exec","decision":"auto_approve","status":"completed"},{"call_id":"c2","tool":"local_read","status":"pending"}],"usage":{"requests":2,"prompt_tokens":10,"cached_tokens":0,"cache_write_tokens":0,"completion_tokens":3,"reasoning_tokens":0,"cost_usd":0.5,"cost_requests":2,"unmetered_requests":0},"artifacts":[]}`
	if string(bytes) != want {
		t.Fatalf("got  %s\nwant %s", bytes, want)
	}
	empty, _ := json.Marshal(ExecutionResult(domain.ExecutionResult{Status: domain.ExecutionFailed, Error: "x"}))
	if string(empty) != `{"schema":"axlr.execution-result/v1","status":"failed","session_id":"","sessions":[],"answer":"","error":"x","tool_calls":[],"usage":{"requests":0,"prompt_tokens":0,"cached_tokens":0,"cache_write_tokens":0,"completion_tokens":0,"reasoning_tokens":0,"cost_usd":0,"cost_requests":0,"unmetered_requests":0},"artifacts":[]}` {
		t.Fatalf("%s", empty)
	}
}
