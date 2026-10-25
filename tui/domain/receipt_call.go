package domain

import "time"

// ReceiptCall is one tool call of an execution: what became of it, as the
// result's call says, digests of what it was given and returned, and when
// it ran.
type ReceiptCall struct {
	Session SessionID
	Call    ExecutionCall
	// ArgumentsSHA256 digests the call's arguments as the session keeps
	// them, compact JSON; OutcomeSHA256 its result's text, "" while it is
	// pending.
	ArgumentsSHA256 string
	OutcomeSHA256   string
	// StartedAt is zero for a call that did not run; Duration zero while it
	// has not ended.
	StartedAt time.Time
	Duration  time.Duration
	Ended     bool
}
