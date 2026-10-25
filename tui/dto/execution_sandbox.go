package dto

// ExecutionSandbox is the runtime contract's sandbox per execution
// (docs/runtime-contract.md): what `axlr-tui --exec --sandbox` reads and
// what CreateSession takes and GetSession shows. Egress defaults to none
// for a confining profile and to full for none.
type ExecutionSandbox struct {
	Profile   string                     `json:"profile"`
	Egress    string                     `json:"egress,omitempty"`
	Allow     []string                   `json:"allow,omitempty"`
	Writable  []string                   `json:"writable,omitempty"`
	Readable  []string                   `json:"readable,omitempty"`
	Container *ExecutionSandboxContainer `json:"container,omitempty"`
}
