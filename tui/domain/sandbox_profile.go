package domain

import "errors"

// SandboxProfile is how an execution's commands are confined
// (docs/runtime-contract.md, "Sandbox per execution").
type SandboxProfile string

const (
	// SandboxProfileNone runs them as the console does: trusted-local, under the
	// settings' exec_sandbox if any.
	SandboxProfileNone SandboxProfile = "none"
	// SandboxProfileProcess runs each under bubblewrap: the workspace writable,
	// the system read-only, homes and daemon sockets hidden.
	SandboxProfileProcess SandboxProfile = "process"
	// SandboxProfileContainer runs each in a container of a host image.
	SandboxProfileContainer SandboxProfile = "container"
)

func (p SandboxProfile) Validate() error {
	switch p {
	case SandboxProfileNone, SandboxProfileProcess, SandboxProfileContainer:
		return nil
	}
	return errors.New("a sandbox's profile is none, process or container")
}
