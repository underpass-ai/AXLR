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
)

func ParseWorkMode(raw string) (WorkMode, error) {
	mode := WorkMode(raw)
	return mode, mode.Validate()
}

func (m WorkMode) Validate() error {
	switch m {
	case ModeNormal, ModeReview, ModeWriter, ModeResearch:
		return nil
	}
	return errors.New("unknown work mode")
}
