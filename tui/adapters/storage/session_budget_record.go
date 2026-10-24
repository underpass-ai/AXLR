package storage

import (
	"time"

	"github.com/underpass-ai/AXLR/tui/dto"
)

// sessionBudget is the domain's ExecutionAccount on disk.
type sessionBudget struct {
	Limits    dto.ExecutionBudget `json:"limits"`
	StartedAt time.Time           `json:"started_at"`
	Carried   sessionSpend        `json:"carried"`
	Base      sessionSpend        `json:"base"`
	Stopped   string              `json:"stopped,omitempty"`
}
