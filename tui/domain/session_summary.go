package domain

import axlr "github.com/underpass-ai/AXLR/domain"

type SessionSummary struct {
	ID        SessionID
	Workspace Workspace
	Model     axlr.ModelID
	Status    SessionStatus
}
