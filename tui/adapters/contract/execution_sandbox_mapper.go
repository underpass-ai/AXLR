package contract

import (
	"encoding/json"
	"errors"
	"io"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
	"github.com/underpass-ai/AXLR/tui/dto"
)

// MaxExecutionSandboxBytes bounds the sandbox object a surface reads: an
// allow list and two path lists.
const MaxExecutionSandboxBytes = 32 << 10

// ParseExecutionSandbox reads the contract's sandbox object, strictly: an
// unknown field refuses it, as the service refuses unknown fields. The
// object may come wrapped as {"sandbox": {...}}.
func ParseExecutionSandbox(raw []byte) (domain.ExecutionSandbox, error) {
	if len(raw) > MaxExecutionSandboxBytes {
		return domain.ExecutionSandbox{}, errors.New("the sandbox exceeds 32 KiB")
	}
	var wrapped struct {
		Sandbox json.RawMessage `json:"sandbox"`
	}
	if decoder := strictDecoder(raw); decoder.Decode(&wrapped) == nil && wrapped.Sandbox != nil {
		raw = wrapped.Sandbox
	}
	var wire dto.ExecutionSandbox
	decoder := strictDecoder(raw)
	if err := decoder.Decode(&wire); err != nil {
		return domain.ExecutionSandbox{}, errors.New("the sandbox must be the contract's object: profile, egress, allow, writable, readable and container, with no other field")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return domain.ExecutionSandbox{}, errors.New("trailing data after the sandbox")
	}
	return DomainExecutionSandbox(wire)
}

// DomainExecutionSandbox validates a wire sandbox into the domain's: the
// egress defaults to none for a confining profile and to full for none,
// and each destination is written as the contract reads it.
func DomainExecutionSandbox(wire dto.ExecutionSandbox) (domain.ExecutionSandbox, error) {
	sandbox := domain.ExecutionSandbox{Profile: domain.SandboxProfile(wire.Profile), Egress: domain.SandboxEgress(wire.Egress), Writable: append([]string(nil), wire.Writable...), Readable: append([]string(nil), wire.Readable...)}
	if wire.Profile == "" {
		return domain.ExecutionSandbox{}, errors.New("a sandbox names its profile: none, process or container")
	}
	if sandbox.Egress == "" {
		sandbox.Egress = domain.SandboxEgressNone
		if sandbox.Profile == domain.SandboxProfileNone {
			sandbox.Egress = domain.SandboxEgressFull
		}
	}
	for _, raw := range wire.Allow {
		rule, err := root.ParseEgressRule(raw)
		if err != nil {
			return domain.ExecutionSandbox{}, err
		}
		sandbox.Allow = append(sandbox.Allow, rule.String())
	}
	if wire.Container != nil {
		sandbox.Container = &domain.SandboxContainer{Image: wire.Container.Image, Runtime: wire.Container.Runtime, MemoryMB: wire.Container.MemoryMB, CPUs: wire.Container.CPUs, PIDs: wire.Container.PIDs}
	}
	return sandbox, sandbox.Validate()
}

// ExecutionSandbox is the wire form of a sandbox, as GetSession shows it
// and the receipt digests it: the egress spelled out.
func ExecutionSandbox(sandbox domain.ExecutionSandbox) dto.ExecutionSandbox {
	wire := dto.ExecutionSandbox{Profile: string(sandbox.Profile), Egress: string(sandbox.Egress), Allow: append([]string(nil), sandbox.Allow...), Writable: append([]string(nil), sandbox.Writable...), Readable: append([]string(nil), sandbox.Readable...)}
	if sandbox.Container != nil {
		wire.Container = &dto.ExecutionSandboxContainer{Image: sandbox.Container.Image, Runtime: sandbox.Container.Runtime, MemoryMB: sandbox.Container.MemoryMB, CPUs: sandbox.Container.CPUs, PIDs: sandbox.Container.PIDs}
	}
	return wire
}
