package contract

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseExecutionRoutingReadsTheContractsObject(t *testing.T) {
	for _, raw := range []string{
		`{"candidates": ["qwen-local", "anthropic/claude-haiku-5.5"], "objective": "cost", "allow_remote_providers": false, "max_context_tokens": 32000, "max_request_usd": 0.05}`,
		`{"routing": {"candidates": ["qwen-local", "anthropic/claude-haiku-5.5"], "objective": "cost", "allow_remote_providers": false, "max_context_tokens": 32000, "max_request_usd": 0.05}}`,
	} {
		routing, err := ParseExecutionRouting([]byte(raw))
		if err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		if routing.Objective() != "cost" || routing.AllowsRemote() || routing.MaxContextTokens() != 32000 || routing.MaxRequestUSD() != 0.05 || len(routing.Candidates()) != 2 {
			t.Fatalf("%s: read %+v", raw, routing)
		}
		wire, _ := json.Marshal(ExecutionRouting(routing))
		if string(wire) != `{"candidates":["qwen-local","anthropic/claude-haiku-5.5"],"objective":"cost","allow_remote_providers":false,"max_context_tokens":32000,"max_request_usd":0.05}` {
			t.Fatalf("wire = %s", wire)
		}
	}
	routing, err := ParseExecutionRouting([]byte(`{"candidates": ["a"], "objective": "quality-first"}`))
	if err != nil || !routing.AllowsRemote() {
		t.Fatalf("absent allow_remote_providers must allow them: %+v, %v", routing, err)
	}
}

func TestParseExecutionRoutingRefusesWhatIsNotTheContract(t *testing.T) {
	for raw, want := range map[string]string{
		`{"candidates": ["a"], "objective": "cost", "routingPolicy": "cost-optimized"}`: "no other field",
		`{"candidates": [], "objective": "cost"}`:                                       "between 1 and 8",
		`{"candidates": ["a"]}`:                                                         "objective",
		`{"candidates": ["a"], "objective": "cheapest"}`:                                "objective",
		`{"candidates": ["a", "a"], "objective": "cost"}`:                               "twice",
		`{"candidates": ["a"], "objective": "cost", "max_request_usd": -1}`:             "max_request_usd",
		`{"candidates": ["a"], "objective": "cost"} {}`:                                 "trailing",
		`{"routing": {"candidates": ["a"], "objective": "cost"}, "extra": 1}`:           "no other field",
	} {
		_, err := ParseExecutionRouting([]byte(raw))
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: error %v, want %q", raw, err, want)
		}
	}
}
