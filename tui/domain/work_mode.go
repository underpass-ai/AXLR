package domain

import "errors"

// WorkMode is how AXLR works in a session: a policy over guidance and tools,
// not a ceremony. The zero value reads as ModeNormal.
type WorkMode string

const (
	ModeNormal   WorkMode = "normal"
	ModeReview   WorkMode = "review"
	ModeWriter   WorkMode = "writer"
	ModeResearch WorkMode = "research"
	// ModeDebug and ModeDelivery start a console-driven MADE ceremony; they
	// limit no tool, because the ceremony's checks decide success.
	ModeDebug    WorkMode = "debug"
	ModeDelivery WorkMode = "delivery"
	// ModeIncident reviews a production incident through a MADE ceremony
	// with a fresh-context reviewer and a person's approval.
	ModeIncident WorkMode = "incident"
	// ModeRepair repairs the configured repository in a disposable clone
	// through a MADE ceremony: the console opens, watches and merges the
	// pull request itself.
	ModeRepair WorkMode = "repair"
	// ModePlan decomposes a brief into atomic tasks the console verifies
	// and the person approves, then runs them.
	ModePlan WorkMode = "plan"
	// ModeTask is a worker session the console starts for one task of an
	// approved plan; it is never selected by the person.
	ModeTask WorkMode = "task"
)

func ParseWorkMode(raw string) (WorkMode, error) {
	mode := WorkMode(raw)
	return mode, mode.Validate()
}

func (m WorkMode) Validate() error {
	switch m {
	case ModeNormal, ModeReview, ModeWriter, ModeResearch, ModeDebug, ModeDelivery, ModeIncident, ModeRepair, ModePlan, ModeTask:
		return nil
	}
	return errors.New("unknown work mode")
}

// StartsCeremony reports modes whose first prompt starts a MADE ceremony.
func (m WorkMode) StartsCeremony() bool {
	return m == ModeDebug || m == ModeDelivery || m == ModeIncident || m == ModeRepair || m == ModePlan
}
