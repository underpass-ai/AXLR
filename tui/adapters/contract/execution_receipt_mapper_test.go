package contract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// The wire receipt digests the policy and the budget as their compact
// JSON, writes an unknown cost as null and leaves out the duration of a
// request that never ended and of a call that did not run.
func TestExecutionReceiptWireForm(t *testing.T) {
	policy, err := ParseExecutionPolicy([]byte(`{"level":"observe","tools":{"allow":[{"name":"local_read"}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	budget, err := ParseExecutionBudget([]byte(`{"requests":5}`))
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 25, 9, 0, 0, 0, time.FixedZone("CEST", 2*3600))
	receipt := domain.ExecutionReceipt{Status: domain.ExecutionCompleted, Session: "s", Sessions: []domain.SessionID{"s"}, StartedAt: at, EndedAt: at.Add(time.Minute), Policy: &policy, Budget: &budget, Complete: true,
		Requests: []domain.ReceiptRequest{
			{Session: "s", Index: 1, StartedAt: at, Duration: 1500 * time.Millisecond, Status: domain.ReceiptRequestCompleted, Model: "m", Usage: domain.UsageSample{Cost: 0.5, CostKnown: true}},
			{Session: "s", Index: 2, StartedAt: at, Status: domain.ReceiptRequestInterrupted, Model: "m", Usage: domain.UsageSample{Unmetered: true}},
		},
		Calls: []domain.ReceiptCall{
			{Session: "s", Call: domain.ExecutionCall{ID: "c1", Tool: "local_read", Decision: domain.DecisionAutoApprove, Status: domain.ExecutionCallCompleted}, ArgumentsSHA256: "a", OutcomeSHA256: "b", StartedAt: at, Duration: 3 * time.Millisecond, Ended: true},
			{Session: "s", Call: domain.ExecutionCall{ID: "c2", Tool: "local_exec", Status: domain.ExecutionCallPending}, ArgumentsSHA256: "c"},
		},
	}
	wire := ExecutionReceipt(receipt)
	digest := func(raw string) string {
		sum := sha256.Sum256([]byte(raw))
		return hex.EncodeToString(sum[:])
	}
	if wire.Policy == nil || wire.Policy.Level != "observe" || wire.Policy.SHA256 != digest(`{"level":"observe","tools":{"allow":[{"name":"local_read"}]}}`) {
		t.Fatalf("policy %+v", wire.Policy)
	}
	if wire.Budget == nil || wire.Budget.SHA256 != digest(`{"requests":5}`) || wire.Budget.Limits.Requests != 5 {
		t.Fatalf("budget %+v", wire.Budget)
	}
	encoded, _ := json.Marshal(wire)
	var raw struct {
		StartedAt string           `json:"started_at"`
		Requests  []map[string]any `json:"requests"`
		Calls     []map[string]any `json:"tool_calls"`
	}
	if err := json.Unmarshal(encoded, &raw); err != nil {
		t.Fatal(err)
	}
	if raw.StartedAt != "2026-10-25T07:00:00Z" || raw.Requests[0]["duration_ms"] != 1500.0 || raw.Requests[0]["cost_usd"] != 0.5 || raw.Requests[1]["cost_usd"] != nil || raw.Requests[1]["duration_ms"] != nil {
		t.Fatalf("%s", encoded)
	}
	if _, has := raw.Requests[1]["cost_usd"]; !has {
		t.Fatalf("an unknown cost is null, not absent: %s", encoded)
	}
	if raw.Calls[0]["duration_ms"] != 3.0 || raw.Calls[1]["started_at"] != nil || raw.Calls[1]["outcome_sha256"] != nil {
		t.Fatalf("%s", encoded)
	}
}
