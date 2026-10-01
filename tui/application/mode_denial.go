package application

import "github.com/underpass-ai/AXLR/tui/domain"

// ModeDenial is a call the session's work mode refuses. Its text reaches the
// model as the tool result so it can change course.
type ModeDenial struct {
	Mode   domain.WorkMode
	Reason string
}

func (d ModeDenial) Error() string { return "denied by " + string(d.Mode) + " mode: " + d.Reason }
