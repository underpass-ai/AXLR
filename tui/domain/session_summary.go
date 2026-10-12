package domain

import (
	"time"

	axlr "github.com/underpass-ai/AXLR/domain"
)

type SessionSummary struct {
	ID        SessionID
	Workspace Workspace
	Model     axlr.ModelID
	Status    SessionStatus
	// Title is the first prompt; UpdatedAt is when the session was last saved.
	Title        axlr.Text
	UpdatedAt    time.Time
	MessageCount int
	// Mode is the session's work mode, empty for a normal session.
	Mode WorkMode
}
