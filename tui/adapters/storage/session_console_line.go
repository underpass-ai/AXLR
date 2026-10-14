package storage

import "time"

type sessionConsoleLine struct {
	AfterMessage int                 `json:"after_message"`
	At           time.Time           `json:"at"`
	Plan         *sessionPlanOutcome `json:"plan,omitempty"`
	Restore      *sessionRestore     `json:"restore,omitempty"`
	Failure      *sessionFailure     `json:"failure,omitempty"`
}
