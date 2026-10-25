package domain

import (
	"errors"
	"fmt"
	pathpkg "path"
	"strings"

	root "github.com/underpass-ai/AXLR/domain"
)

// ExecutionSandbox confines the commands of one execution, local_exec and
// the forged tools (docs/runtime-contract.md, "Sandbox per execution"):
// its profile, its egress with the allow list, the paths outside the
// workspace its commands may write or read, and the container for the
// container profile. AXLR's own model requests and MCP servers run outside
// it.
type ExecutionSandbox struct {
	Profile SandboxProfile
	Egress  SandboxEgress
	// Allow lists the destinations of the allowlist egress, as
	// root.ParseEgressRule reads them.
	Allow     []string
	Writable  []string
	Readable  []string
	Container *SandboxContainer
}

// Bounds of a sandbox.
const (
	MaxSandboxAllow = 64
	MaxSandboxPaths = 32
)

func (s ExecutionSandbox) Validate() error {
	if err := s.Profile.Validate(); err != nil {
		return err
	}
	if err := s.Egress.Validate(); err != nil {
		return err
	}
	if s.Profile == SandboxProfileNone && (s.Egress != SandboxEgressFull || len(s.Writable)+len(s.Readable) > 0) {
		return errors.New("the profile none confines nothing: its egress is full and it takes no writable or readable paths")
	}
	if (s.Egress == SandboxEgressAllowlist) != (len(s.Allow) > 0) {
		return errors.New("a sandbox's allow list goes with the egress allowlist, which needs one")
	}
	if len(s.Allow) > MaxSandboxAllow {
		return fmt.Errorf("a sandbox's allow list holds at most %d destinations", MaxSandboxAllow)
	}
	for _, raw := range s.Allow {
		if _, err := root.ParseEgressRule(raw); err != nil {
			return err
		}
	}
	for name, paths := range map[string][]string{"writable": s.Writable, "readable": s.Readable} {
		if len(paths) > MaxSandboxPaths {
			return fmt.Errorf("a sandbox's %s list holds at most %d paths", name, MaxSandboxPaths)
		}
		for _, path := range paths {
			// A path of the Linux host the sandbox runs on, whatever the
			// platform that reads the object.
			if !pathpkg.IsAbs(path) || pathpkg.Clean(path) != path || path == "/" || len(path) > 4096 || strings.ContainsAny(path, ",\n\r\x00") {
				return fmt.Errorf("a sandbox's %s path %q must be a clean absolute path other than the root, without commas", name, path)
			}
		}
	}
	if (s.Profile == SandboxProfileContainer) != (s.Container != nil) {
		return errors.New("a sandbox's container goes with the profile container, which needs one")
	}
	if s.Container != nil {
		return s.Container.Validate()
	}
	return nil
}

// Clone copies the sandbox's lists, so a session keeps its own.
func (s ExecutionSandbox) Clone() ExecutionSandbox {
	clone := s
	clone.Allow = append([]string(nil), s.Allow...)
	clone.Writable = append([]string(nil), s.Writable...)
	clone.Readable = append([]string(nil), s.Readable...)
	if s.Container != nil {
		container := *s.Container
		clone.Container = &container
	}
	return clone
}
