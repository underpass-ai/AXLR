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
)

func ParseWorkMode(raw string) (WorkMode, error) {
	mode := WorkMode(raw)
	return mode, mode.Validate()
}

func (m WorkMode) Validate() error {
	switch m {
	case ModeNormal, ModeReview, ModeWriter, ModeResearch, ModeDebug, ModeDelivery:
		return nil
	}
	return errors.New("unknown work mode")
}

// StartsCeremony reports modes whose first prompt starts a MADE ceremony.
func (m WorkMode) StartsCeremony() bool { return m == ModeDebug || m == ModeDelivery }
