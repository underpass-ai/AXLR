package domain

import (
	"strings"

	axlr "github.com/underpass-ai/AXLR/domain"
)

// ForbiddenOutcomePrefix starts the result of a call the execution's
// policy refused; the execution's result reads the call's status from it
// (ExecutionCallForbidden).
const ForbiddenOutcomePrefix = "forbidden by the execution's policy"

// ForbiddenOutcome is what the model reads for a call the policy refused:
// the reason, and that the refusal is final. Measured on 2 October 2026
// with the modes' denials: told only "denied", a model makes the same
// change through another tool.
func ForbiddenOutcome(reason string) axlr.Text {
	return axlr.Text(ForbiddenOutcomePrefix + ": " + reason + ". This refusal is final for this execution: do not try the same through another tool; tell the user what was refused.")
}

func forbidden(decision ToolDecision, outcome ToolOutcome) bool {
	return decision == DecisionDeny && outcome.IsError && strings.HasPrefix(string(outcome.Content), ForbiddenOutcomePrefix)
}

// Policy is the execution's policy in force, if the session has one.
func (s Session) Policy() (ExecutionPolicy, bool) {
	if s.state.Policy == nil {
		return ExecutionPolicy{}, false
	}
	return s.state.Policy.clone(), true
}

// SetPolicy puts the session under an execution's policy from now on: the
// next judgement applies it, a pending call's decision included, so a
// decision taken under a new policy cannot run what it forbids.
func (s *Session) SetPolicy(policy ExecutionPolicy) error {
	if err := policy.Validate(); err != nil {
		return err
	}
	clone := policy.clone()
	s.state.Policy = &clone
	return nil
}

// ClearPolicy lifts the execution's policy.
func (s *Session) ClearPolicy() { s.state.Policy = nil }
