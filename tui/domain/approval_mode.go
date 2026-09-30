package domain

import "errors"

// ApprovalMode is the configured authorization policy for one registered plugin.
type ApprovalMode string

const (
	ApprovalManual ApprovalMode = "manual"
	ApprovalAuto   ApprovalMode = "auto"
)

func (m ApprovalMode) Validate() error {
	if m != ApprovalManual && m != ApprovalAuto {
		return errors.New("invalid plugin approval mode")
	}
	return nil
}
