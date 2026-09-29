package application

import (
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// Event is delivered synchronously. Tool activity always follows persisted history.
type Event struct {
	Kind  EventKind
	Text  root.Text
	State domain.SessionStatus
	Tool  domain.PendingTool
	Usage *root.TokenUsage
}
