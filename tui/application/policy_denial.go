package application

import "github.com/underpass-ai/AXLR/tui/domain"

// PolicyDenial is a call the execution's policy refuses, outside the model
// and before any card. Its text reaches the model as the tool result, as a
// ModeDenial's does, so it can change course; the result reads the call's
// status "forbidden" from it.
type PolicyDenial struct {
	Reason string
}

func (d PolicyDenial) Error() string { return string(domain.ForbiddenOutcome(d.Reason)) }
