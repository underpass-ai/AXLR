package domain

import (
	"fmt"

	axlr "github.com/underpass-ai/AXLR/domain"
)

// ToolFailureStreakError ends an execution whose model kept sending a call
// the console or the tool refused the same way: on 12 November 2026 a dev
// bot on codex/gpt-6.1-terra-high sent axlr_remember ten times with an
// invented about, each refused alike, until someone cancelled the run by
// hand. The execution ends failed with this error; the session is kept,
// its turn interrupted, for a person to read or resume.
type ToolFailureStreakError struct {
	Tool    axlr.ToolName
	Count   int
	Failure string
}

func (e *ToolFailureStreakError) Error() string {
	return fmt.Sprintf("tool loop: %s failed %d times in a row with the same error, so the run stopped: %s", e.Tool, e.Count, e.Failure)
}
