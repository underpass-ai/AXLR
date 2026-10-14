package application

import (
	"errors"
	"fmt"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// ContextCapError says a request would exceed the cap and names the
// successor the console opened; the console continues the prompt there.
type ContextCapError struct {
	Tokens, Cap int
	Predecessor domain.SessionID
	Successor   domain.Session
	// Prompt is the person's prompt the successor takes.
	Prompt string
}

func (e *ContextCapError) Error() string {
	return fmt.Sprintf("the request would reach %d tokens, past the cap of %d: continuing in session %s", e.Tokens, e.Cap, e.Successor.ID())
}

// IsContextCap reports a context cap error and returns it.
func IsContextCap(err error) (*ContextCapError, bool) {
	var capped *ContextCapError
	if errors.As(err, &capped) {
		return capped, true
	}
	return nil, false
}
