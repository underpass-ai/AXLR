package dto

// ReceiptSandbox names the sandbox an execution's commands ran under: the
// sha256 of its compact JSON, its profile, its egress and the allow list.
type ReceiptSandbox struct {
	SHA256  string   `json:"sha256"`
	Profile string   `json:"profile"`
	Egress  string   `json:"egress"`
	Allow   []string `json:"allow,omitempty"`
}
