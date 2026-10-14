package application

import "time"

// TurnAction is one side effect of a turn.
type TurnAction struct {
	Tool       string    `json:"tool"`
	Target     string    `json:"target"`
	Why        string    `json:"why"`
	Decision   string    `json:"decision"`
	Reason     string    `json:"reason,omitempty"`
	Outcome    string    `json:"outcome"`
	ExitCode   *int      `json:"exit_code,omitempty"`
	OutputTail string    `json:"output_tail,omitempty"`
	OccurredAt time.Time `json:"occurred_at,omitempty"`
	CallID     string    `json:"call_id"`
}
