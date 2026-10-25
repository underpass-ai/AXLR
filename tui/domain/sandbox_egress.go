package domain

import "errors"

// SandboxEgress is what of the network an execution's commands reach.
type SandboxEgress string

const (
	// SandboxEgressNone gives them a network namespace with nothing in it.
	SandboxEgressNone SandboxEgress = "none"
	// SandboxEgressAllowlist lets them reach the sandbox's allow list
	// through AXLR's egress proxy, and nothing else.
	SandboxEgressAllowlist SandboxEgress = "allowlist"
	// SandboxEgressFull leaves them the host's network.
	SandboxEgressFull SandboxEgress = "full"
)

func (e SandboxEgress) Validate() error {
	switch e {
	case SandboxEgressNone, SandboxEgressAllowlist, SandboxEgressFull:
		return nil
	}
	return errors.New("a sandbox's egress is none, allowlist or full")
}
