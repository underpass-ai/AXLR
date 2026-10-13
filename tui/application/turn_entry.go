package application

import "time"

// TurnEntry is a turn's results as the console writes them to KMP under
// the session's about: the summary and its evidence, one line per action,
// never the prompt or the answer (decision H8).
type TurnEntry struct {
	About      string
	Labels     map[string][]string
	Key        string
	Summary    string
	Evidence   []string
	OccurredAt time.Time
	// ContextID is the session's KMP context, when the console opened one.
	ContextID string
	// Kind is the memory's kind: turn, or observation for the entry the
	// close writes (decision F3); "" is turn.
	Kind string
}
