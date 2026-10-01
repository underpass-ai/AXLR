package application

import "github.com/underpass-ai/AXLR/tui/domain"

// ModeDenial is a call the session's work mode refuses. Its text reaches the
// model as the tool result so it can change course. Measured on 2 Oct 2026:
// told only "denied", the model made the same edit through local_exec.
type ModeDenial struct {
	Mode   domain.WorkMode
	Reason string
}

func (d ModeDenial) Error() string {
	return "denied by " + string(d.Mode) + " mode: " + d.Reason + ". This refusal is final for this mode: do not make the same change through local_exec or any other tool. Tell the user what change is needed and that /normal allows it."
}
