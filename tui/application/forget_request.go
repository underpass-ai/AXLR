package application

import (
	"github.com/underpass-ai/AXLR/tui/domain"
	"time"
)

// ForgetRequest closes one entry: the ref, the person's reason, and what
// they typed, kept as evidence with the time.
type ForgetRequest struct {
	Ref, Why  string
	Command   string
	At        time.Time
	Labels    map[string][]string
	ContextID domain.MemoryContextID
}
