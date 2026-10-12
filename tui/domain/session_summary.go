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
	// Text is the session's prompts and replies, one after another, cut at
	// MaxSummaryTextBytes: what the session picker's filter searches
	// besides the title. A store may leave it empty.
	Text axlr.Text
}

// MaxSummaryTextBytes bounds SessionSummary.Text, so listing many long
// sessions stays cheap.
const MaxSummaryTextBytes = 64 << 10
