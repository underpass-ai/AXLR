package application

import "github.com/underpass-ai/AXLR/tui/domain"

// StepResult is what one axlr_step_done call did. When accepted, Run is nil
// once the ceremony reached a terminal state. A refusal leaves Run nil unless
// the console must still remember something, such as a reviewer failure.
type StepResult struct {
	Outcome  domain.ToolOutcome
	Run      *domain.CeremonyRun
	Accepted bool
}
