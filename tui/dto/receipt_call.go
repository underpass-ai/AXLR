package dto

import "time"

// ReceiptCall is one tool call in a receipt: the result's call with the
// digests of its arguments and outcome, and when it ran.
type ReceiptCall struct {
	Session         string     `json:"session_id"`
	ID              string     `json:"call_id"`
	Tool            string     `json:"tool"`
	ArgumentsSHA256 string     `json:"arguments_sha256"`
	OutcomeSHA256   string     `json:"outcome_sha256,omitempty"`
	Decision        string     `json:"decision,omitempty"`
	Status          string     `json:"status"`
	StartedAt       *time.Time `json:"started_at,omitempty"`
	DurationMS      *int64     `json:"duration_ms,omitempty"`
}
