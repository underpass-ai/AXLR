package application

import (
	"context"
	"errors"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// memoryReceipts keeps the journal entries a turn writes.
type memoryReceipts struct {
	entries []domain.ReceiptEntry
	err     error
}

func (m *memoryReceipts) AppendReceipt(_ context.Context, _ domain.SessionID, entry domain.ReceiptEntry) error {
	if m.err != nil {
		return m.err
	}
	m.entries = append(m.entries, entry)
	return nil
}

func (m *memoryReceipts) LoadReceipt(context.Context, domain.SessionID) ([]domain.ReceiptEntry, error) {
	return m.entries, nil
}

// A request is journaled before it is sent and when it ends, failed ones
// included; a call when it starts and when its outcome is saved. A journal
// that cannot be written never fails the turn.
func TestTurnsJournalTheirRequestsAndCalls(t *testing.T) {
	receipts := &memoryReceipts{}
	requests := 0
	model := streamFunc(func(context.Context, root.CompletionRequest, func(root.Text) error) (root.CompletionResult, error) {
		requests++
		if requests == 1 {
			return assistant("", call("c1", "read")), nil
		}
		return root.CompletionResult{}, errors.New("provider unreachable")
	})
	store := &memoryStore{}
	continuation := ContinueTurnUseCase{Models: model, Store: store, Receipts: receipts}
	approveAll := approvalFunc(func(domain.ToolIdentity) bool { return true })
	start := StartTurnUseCase{Catalog: &catalogStub{}, Store: store, Continue: continuation, Tools: fixedOutcome{}, Approval: approveAll}
	s := turnSession(t)
	if err := start.Execute(context.Background(), &s, "go", ignoreEvent); err == nil {
		t.Fatal("the second request should have failed")
	}
	var kinds []domain.ReceiptEntryKind
	for _, entry := range receipts.entries {
		kinds = append(kinds, entry.Kind)
	}
	want := []domain.ReceiptEntryKind{domain.ReceiptRequestStarted, domain.ReceiptRequestEnded, domain.ReceiptCallStarted, domain.ReceiptCallEnded, domain.ReceiptRequestStarted, domain.ReceiptRequestEnded}
	if len(kinds) != len(want) {
		t.Fatalf("entries %v", kinds)
	}
	for i := range want {
		if kinds[i] != want[i] {
			t.Fatalf("entries %v", kinds)
		}
	}
	if first := receipts.entries[0]; first.Model != "test/model" || first.At.Location() != nil && first.At.Location().String() != "UTC" {
		t.Fatalf("%+v", first)
	}
	if receipts.entries[1].Status != domain.ReceiptRequestCompleted || receipts.entries[5].Status != domain.ReceiptRequestFailed || receipts.entries[2].Call != "c1" || receipts.entries[3].Call != "c1" {
		t.Fatalf("%+v", receipts.entries)
	}
	if requestStatus(context.Canceled) != domain.ReceiptRequestCancelled || requestStatus(context.DeadlineExceeded) != domain.ReceiptRequestCancelled {
		t.Fatal("a cancelled request reads cancelled")
	}
	failing := &memoryReceipts{err: errors.New("disk full")}
	requests = 0
	continuation.Receipts = failing
	model = streamFunc(func(context.Context, root.CompletionRequest, func(root.Text) error) (root.CompletionResult, error) {
		return assistant("done"), nil
	})
	continuation.Models = model
	start.Continue = continuation
	s = turnSession(t)
	if err := start.Execute(context.Background(), &s, "go", ignoreEvent); err != nil || s.Status() != domain.StatusComplete {
		t.Fatalf("a journal that cannot be written failed the turn: %v", err)
	}
}

// fixedOutcome runs any call and returns its outcome.
type fixedOutcome struct{}

func (fixedOutcome) Execute(context.Context, domain.ToolIdentity, root.JSONValue) (domain.ToolOutcome, error) {
	return domain.ToolOutcome{Content: "ran"}, nil
}
