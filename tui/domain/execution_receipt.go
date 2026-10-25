package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"time"

	axlr "github.com/underpass-ai/AXLR/domain"
)

// ExecutionReceipt is an execution's account for an audit (the runtime
// contract's receipt): every model request with its model, provider, usage
// and cost, every tool call with digests of its arguments and outcome, and
// the policy, budget and correlation it ran under. It is read from the
// sessions and their receipt journals, so it survives a restart.
type ExecutionReceipt struct {
	Status   ExecutionStatus
	Session  SessionID
	Sessions []SessionID
	// StartedAt is when the execution began: its budget's start, else the
	// first message or entry of its first session; EndedAt its last
	// message or entry.
	StartedAt, EndedAt time.Time
	// Policy, Budget and Correlation are the ones the execution ran under,
	// as the last session carries them; nil without.
	Policy      *ExecutionPolicy
	Budget      *ExecutionBudget
	Correlation *ExecutionCorrelation
	Requests    []ReceiptRequest
	Calls       []ReceiptCall
	// Total sums the requests listed, as the result's usage sums ledgers.
	Total ExecutionUsage
	// Complete is true when every request the ledgers counted and every
	// call that ran is itemized.
	Complete bool
}

// NewExecutionReceipt reads the receipt of the execution result describes
// from the sessions it spanned, first to last.
func NewExecutionReceipt(result ExecutionResult, sessions []ReceiptSession) ExecutionReceipt {
	receipt := ExecutionReceipt{Status: result.Status, Session: result.Session, Sessions: append([]SessionID(nil), result.Sessions...), Complete: true}
	for _, read := range sessions {
		state := read.Session.Export()
		requests, ended := receiptRequests(state.ID, read.Entries)
		receipt.Requests = append(receipt.Requests, requests...)
		calls, timed := receiptCalls(state, read.Entries)
		receipt.Calls = append(receipt.Calls, calls...)
		if ended < read.Ledger.Requests || !timed {
			receipt.Complete = false
		}
		for _, at := range state.MessageTimes {
			receipt.span(at)
		}
		for _, entry := range read.Entries {
			receipt.span(entry.At)
		}
	}
	for _, request := range receipt.Requests {
		receipt.Total.addRequest(request)
	}
	if len(sessions) > 0 {
		last := sessions[len(sessions)-1].Session
		if policy, has := last.Policy(); has {
			receipt.Policy = &policy
		}
		if account, has := last.Budget(); has {
			budget := account.Budget
			receipt.Budget = &budget
			if !account.StartedAt.IsZero() {
				receipt.StartedAt = account.StartedAt
			}
		}
		if correlation, has := last.Correlation(); has {
			receipt.Correlation = &correlation
		}
	}
	return receipt
}

// span widens the receipt's times to at; a zero time is unknown.
func (r *ExecutionReceipt) span(at time.Time) {
	if at.IsZero() {
		return
	}
	if r.StartedAt.IsZero() || at.Before(r.StartedAt) {
		r.StartedAt = at
	}
	if at.After(r.EndedAt) {
		r.EndedAt = at
	}
}

// receiptRequests pairs the journal's request lines in order: requests run
// one at a time in a session, so an end closes the last start, and a start
// still open at the next start, or at the journal's end, never ended. It
// also counts the requests that ended, which the ledger counted too.
func receiptRequests(session SessionID, entries []ReceiptEntry) ([]ReceiptRequest, int) {
	var requests []ReceiptRequest
	var open *ReceiptRequest
	ended := 0
	closeOpen := func() {
		if open != nil {
			open.Status = ReceiptRequestInterrupted
			open.Usage.Unmetered = true
			requests = append(requests, *open)
			open = nil
		}
	}
	for _, entry := range entries {
		switch entry.Kind {
		case ReceiptRequestStarted:
			closeOpen()
			open = &ReceiptRequest{Session: session, Index: len(requests) + 1, StartedAt: entry.At, Model: entry.Model}
		case ReceiptRequestEnded:
			ended++
			request := ReceiptRequest{Session: session, Index: len(requests) + 1, StartedAt: entry.At.Add(-entry.Usage.Total)}
			if open != nil {
				request = *open
				open = nil
			}
			request.Status, request.Usage = entry.Status, entry.Usage
			request.Duration = entry.At.Sub(request.StartedAt)
			requests = append(requests, request)
		}
	}
	closeOpen()
	return requests, ended
}

// receiptCalls lists the session's calls in order, with their digests and
// the times the journal kept. It reports whether every call that ran has
// its start in the journal.
func receiptCalls(state SessionState, entries []ReceiptEntry) ([]ReceiptCall, bool) {
	started, ended := map[axlr.ToolCallID]time.Time{}, map[axlr.ToolCallID]time.Time{}
	for _, entry := range entries {
		switch entry.Kind {
		case ReceiptCallStarted:
			started[entry.Call] = entry.At
		case ReceiptCallEnded:
			ended[entry.Call] = entry.At
		}
	}
	timed := true
	calls := make([]ReceiptCall, 0, len(state.Activity))
	for _, record := range state.Activity {
		call := ReceiptCall{Session: state.ID, Call: executionCallOf(record), ArgumentsSHA256: digest(record.Call.Arguments.Bytes())}
		if record.Outcome != nil {
			call.OutcomeSHA256 = digest([]byte(record.Outcome.Content))
		}
		ran := record.Outcome != nil && (record.Decision == DecisionApprove || record.Decision == DecisionAutoApprove)
		if at, has := started[record.Call.ID]; has {
			call.StartedAt = at
			if end, done := ended[record.Call.ID]; done {
				call.Duration, call.Ended = end.Sub(at), true
			}
		} else if ran && !refusedWithoutRunning(record) {
			timed = false
		}
		calls = append(calls, call)
	}
	return calls, timed
}

// refusedWithoutRunning reports an approved decision whose call was still
// answered without running: past the turn's or the budget's limit.
func refusedWithoutRunning(record PendingTool) bool {
	return record.Outcome != nil && overBudget(record.Decision, *record.Outcome)
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
