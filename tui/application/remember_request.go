package application

import "github.com/underpass-ai/AXLR/tui/domain"

// RememberRequest is one memory for KMP as the console writes it.
type RememberRequest struct {
	About, Kind, Text string
	// SummaryEN is the English summary of a text written in another
	// language, which a strict store requires (decision E13).
	SummaryEN      string
	Evidence       []string
	Links          []MemoryLink
	Labels         map[string][]string
	IdempotencyKey string
	// Continuation resumes a write KMP returned for review; the other
	// fields are then empty.
	Continuation string
	// ContextID is the session's memory context, when the console opened
	// one.
	ContextID domain.MemoryContextID
}
