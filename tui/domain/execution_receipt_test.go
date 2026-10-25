package domain

import (
	"testing"
	"time"

	axlr "github.com/underpass-ai/AXLR/domain"
)

// The receipt pairs each request's start with its end, lists a request
// the process died in as interrupted, digests every call and times the
// ones that ran, and is complete only when the journal itemizes all the
// ledger counted and every call that ran.
func TestExecutionReceiptReadsTheJournal(t *testing.T) {
	s := session(t)
	must(t, s.BeginTurn("hello", catalog(t)))
	must(t, s.CompleteAssistant(completion(t, "one", "two")))
	must(t, s.RecordToolOutcome("one", DecisionAutoApprove, ToolOutcome{Content: "first"}))
	must(t, s.RecordToolOutcome("two", DecisionDeny, ToolOutcome{Content: "denied", IsError: true}))
	must(t, s.CompleteAssistant(axlr.CompletionResult{Message: axlr.Message{Role: axlr.RoleAssistant, Content: "done"}}))
	base := time.Now().UTC().Add(time.Hour)
	at := func(ms int) time.Time { return base.Add(time.Duration(ms) * time.Millisecond) }
	entries := []ReceiptEntry{
		{Kind: ReceiptRequestStarted, At: at(0), Model: "test/model"},
		{Kind: ReceiptRequestEnded, At: at(900), Status: ReceiptRequestCompleted, Usage: UsageSample{Provider: "Anthropic", PromptTokens: 100, CompletionTokens: 10, Cost: 0.002, CostKnown: true}},
		{Kind: ReceiptCallStarted, At: at(1000), Call: "one"},
		{Kind: ReceiptCallEnded, At: at(1012), Call: "one"},
		{Kind: ReceiptRequestStarted, At: at(1100), Model: "test/model"},
		{Kind: ReceiptRequestStarted, At: at(5000), Model: "test/model"},
		{Kind: ReceiptRequestEnded, At: at(5400), Status: ReceiptRequestCompleted, Usage: UsageSample{PromptTokens: 120, CompletionTokens: 4}},
	}
	result := NewExecutionResult(s, nil)
	receipt := NewExecutionReceipt(result, []ReceiptSession{{Session: s, Entries: entries, Ledger: UsageLedger{Requests: 2}}})
	if receipt.Status != ExecutionCompleted || receipt.Session != s.ID() || !receipt.Complete || !receipt.StartedAt.Equal(s.Export().MessageTimes[0]) || !receipt.EndedAt.Equal(at(5400)) {
		t.Fatalf("%+v", receipt)
	}
	if len(receipt.Requests) != 3 {
		t.Fatalf("requests %+v", receipt.Requests)
	}
	first, cut, last := receipt.Requests[0], receipt.Requests[1], receipt.Requests[2]
	if first.Index != 1 || first.Duration != 900*time.Millisecond || first.Status != ReceiptRequestCompleted || first.Model != "test/model" || first.Usage.Provider != "Anthropic" {
		t.Fatalf("first %+v", first)
	}
	if cut.Index != 2 || cut.Status != ReceiptRequestInterrupted || !cut.Usage.Unmetered || cut.Duration != 0 || !cut.StartedAt.Equal(at(1100)) {
		t.Fatalf("cut %+v", cut)
	}
	if last.Index != 3 || last.Duration != 400*time.Millisecond {
		t.Fatalf("last %+v", last)
	}
	if receipt.Total != (ExecutionUsage{Requests: 3, PromptTokens: 220, CompletionTokens: 14, CostUSD: 0.002, CostRequests: 1, Unmetered: 1}) {
		t.Fatalf("total %+v", receipt.Total)
	}
	if len(receipt.Calls) != 2 {
		t.Fatalf("calls %+v", receipt.Calls)
	}
	ran, denied := receipt.Calls[0], receipt.Calls[1]
	if ran.Call.Status != ExecutionCallCompleted || ran.ArgumentsSHA256 != digest([]byte(`{}`)) || ran.OutcomeSHA256 != digest([]byte("first")) || !ran.StartedAt.Equal(at(1000)) || ran.Duration != 12*time.Millisecond || !ran.Ended {
		t.Fatalf("ran %+v", ran)
	}
	if denied.Call.Status != ExecutionCallDenied || !denied.StartedAt.IsZero() || denied.OutcomeSHA256 != digest([]byte("denied")) {
		t.Fatalf("denied %+v", denied)
	}
	// A ledger that counted more than the journal ended, and a call that ran
	// without a start in the journal, leave the receipt incomplete.
	if NewExecutionReceipt(result, []ReceiptSession{{Session: s, Entries: entries, Ledger: UsageLedger{Requests: 3}}}).Complete {
		t.Fatal("an unitemized request left the receipt complete")
	}
	if NewExecutionReceipt(result, []ReceiptSession{{Session: s, Entries: entries[:2], Ledger: UsageLedger{Requests: 1}}}).Complete {
		t.Fatal("an untimed call left the receipt complete")
	}
}

// The receipt names the policy, the budget and the correlation the last
// session carries, and starts when the budget's execution began.
func TestExecutionReceiptNamesWhatTheExecutionRanUnder(t *testing.T) {
	s := session(t)
	policy, err := NewExecutionPolicy(PolicyObserve, nil, nil)
	must(t, err)
	must(t, s.SetPolicy(policy))
	began := time.Date(2026, 10, 25, 8, 0, 0, 0, time.UTC)
	budget, err := NewExecutionBudget(0, 3, 0, 0)
	must(t, err)
	must(t, s.SetBudget(budget, began, UsageLedger{}))
	must(t, s.SetCorrelation(ExecutionCorrelation{TaskID: "task-1"}))
	must(t, s.BeginTurn("hello", catalog(t)))
	must(t, s.CompleteAssistant(axlr.CompletionResult{Message: axlr.Message{Role: axlr.RoleAssistant, Content: "done"}}))
	receipt := NewExecutionReceipt(NewExecutionResult(s, nil), []ReceiptSession{{Session: s}})
	if receipt.Policy == nil || receipt.Policy.Level() != PolicyObserve || receipt.Budget == nil || receipt.Budget.Requests() != 3 || receipt.Correlation == nil || receipt.Correlation.TaskID != "task-1" || !receipt.StartedAt.Equal(began) || !receipt.Complete || len(receipt.Requests) != 0 {
		t.Fatalf("%+v", receipt)
	}
}
