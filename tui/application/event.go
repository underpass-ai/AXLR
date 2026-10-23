package application

import (
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// Event is delivered synchronously. Tool activity always follows persisted history.
type Event struct {
	ProviderPhase domain.ProviderPhase
	Snapshot      *domain.SessionState
	Memory        bool
	Kind          EventKind
	Text          root.Text
	MessageCount  int
	State         domain.SessionStatus
	Tool          domain.PendingTool
	Usage         *root.TokenUsage
	// ToolCallName and ToolCallBytes describe the call being streamed.
	ToolCallName  string
	ToolCallBytes int
	// ReasoningBytes is how much reasoning the request has streamed, and
	// ReasoningText the readable reasoning since the previous event.
	ReasoningBytes int
	ReasoningText  string
	// SessionUsage is the session's ledger after a request, and
	// BudgetWarning marks the request that took it past 80 % of its budget.
	SessionUsage  *domain.UsageLedger
	BudgetWarning bool
}
