package main

import (
	"os"
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/tui/dto"
)

// The run writes its receipt just before the result: every request with
// its model, usage and cost, every call with digests and times, and the
// policy, budget and correlation it ran under.
func TestExecWritesTheReceiptBeforeTheResult(t *testing.T) {
	env := cliEnv(t)
	env["PATH"] = os.Getenv("PATH")
	costly := `{"choices":[{"index":0,"delta":{"content":"done"},"finish_reason":"stop"}],"usage":{"prompt_tokens":12,"completion_tokens":3,"total_tokens":15,"cost":0.0004}}`
	scriptedModel(t, execChunk, costly)
	policy := writePolicy(t, `{"level": "act-within-policy"}`)
	budget := writePolicy(t, `{"requests": 5, "usd": 1}`)
	correlation := writePolicy(t, `{"task_id": "task-9"}`)
	code, stdout, stderr := execRun(t, env, "--root", t.TempDir(), "--model", "test/model", "--exec", "make the file", "--approve", "local", "--policy", policy, "--budget", budget, "--correlation", correlation, "--json")
	if code != execCompleted {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	lines := execLines(t, stdout)
	line := lines[len(lines)-2]
	if line.Type != "receipt" || line.Receipt == nil {
		t.Fatalf("%+v", line)
	}
	receipt := *line.Receipt
	result := execResult(t, stdout)
	if receipt.Schema != dto.ExecutionReceiptSchema || receipt.Status != "completed" || receipt.Session != result.Session || len(receipt.Sessions) != 1 || !receipt.Complete || receipt.StartedAt.IsZero() || receipt.EndedAt.Before(receipt.StartedAt) {
		t.Fatalf("%+v", receipt)
	}
	if receipt.Policy == nil || receipt.Policy.Level != "act-within-policy" || len(receipt.Policy.SHA256) != 64 || receipt.Budget == nil || receipt.Budget.Limits.Requests != 5 || len(receipt.Budget.SHA256) != 64 || receipt.Correlation == nil || receipt.Correlation.TaskID != "task-9" {
		t.Fatalf("policy %+v budget %+v correlation %+v", receipt.Policy, receipt.Budget, receipt.Correlation)
	}
	if len(receipt.Requests) != 2 || receipt.Requests[0].Index != 1 || receipt.Requests[1].Index != 2 || receipt.Requests[0].Model != "test/model" || receipt.Requests[0].Status != "completed" || receipt.Requests[0].CostUSD != nil || receipt.Requests[1].CostUSD == nil || *receipt.Requests[1].CostUSD != 0.0004 || receipt.Requests[1].PromptTokens != 12 || receipt.Requests[1].DurationMS == nil {
		t.Fatalf("requests %+v", receipt.Requests)
	}
	if receipt.Total.Requests != 2 || receipt.Total.CostUSD != 0.0004 || receipt.Total.CostRequests != 1 || receipt.Total != result.Usage {
		t.Fatalf("total %+v usage %+v", receipt.Total, result.Usage)
	}
	if len(receipt.Calls) != 1 {
		t.Fatalf("calls %+v", receipt.Calls)
	}
	call := receipt.Calls[0]
	if call.ID != "exec-1" || call.Tool != "local_exec" || call.Decision != "auto_approve" || call.Status != "completed" || len(call.ArgumentsSHA256) != 64 || len(call.OutcomeSHA256) != 64 || call.StartedAt == nil || call.DurationMS == nil || strings.EqualFold(call.ArgumentsSHA256, call.OutcomeSHA256) {
		t.Fatalf("call %+v", call)
	}
}
