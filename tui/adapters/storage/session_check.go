package storage

// sessionCheck is a command the person approved; sidecars written before
// approvals were kept have none.
type sessionCheck struct {
	Program string   `json:"program"`
	Args    []string `json:"args,omitempty"`
}
