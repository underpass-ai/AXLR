package storage

// PlanSettings is the plan section: Model plans (default, and "session",
// the session's model) and AutoApprove starts a verified plan without the
// person (default false).
type PlanSettings struct {
	Model       string `json:"model,omitempty"`
	AutoApprove bool   `json:"auto_approve,omitempty"`
}
