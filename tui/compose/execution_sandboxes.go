package compose

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/underpass-ai/AXLR/adapters/local"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/adapters/storage"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// ExecutionSandboxes turns an execution's sandbox into the runtime's on
// this host (docs/runtime-contract.md, "Sandbox per execution"). Settings
// are the console's exec_sandbox, which an execution may not loosen when
// required, and Default the sandbox they gave: the profile none runs
// under it. Relay is the absolute path of this binary, which runs the
// egress relay inside a sandbox whose egress is an allow list.
type ExecutionSandboxes struct {
	Settings storage.ExecSandboxSettings
	Default  *local.Sandbox
	LookPath func(string) (string, error)
	Relay    string
}

// NewExecutionSandboxes is the sandboxes of this binary, with the settings'
// sandbox as ExecSandbox gave it.
func NewExecutionSandboxes(settings storage.ExecSandboxSettings, sandbox *local.Sandbox, lookPath func(string) (string, error)) ExecutionSandboxes {
	relay, err := os.Executable()
	if err == nil {
		if resolved, err := filepath.EvalSymlinks(relay); err == nil {
			relay = resolved
		}
	} else {
		relay = ""
	}
	return ExecutionSandboxes{Settings: settings, Default: sandbox, LookPath: lookPath, Relay: relay}
}

// Confine is the runtime's sandbox for an execution's sandbox, or why this
// host cannot give it: never a weaker one.
func (x ExecutionSandboxes) Confine(s domain.ExecutionSandbox) (*local.Sandbox, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	if x.Settings.Mode == "required" {
		if s.Profile == domain.SandboxProfileNone {
			return nil, errors.New("settings require the exec sandbox (exec_sandbox.mode required): an execution's sandbox cannot be none")
		}
		if !x.Settings.AllowsNetwork() && s.Egress == domain.SandboxEgressFull {
			return nil, errors.New("settings keep sandboxed commands off the network (exec_sandbox.network false): an execution's egress cannot be full")
		}
	}
	if s.Profile == domain.SandboxProfileNone {
		return x.Default, nil
	}
	sandbox := &local.Sandbox{Isolate: true, Network: s.Egress == domain.SandboxEgressFull, Writable: s.Writable, Readable: s.Readable}
	if s.Egress == domain.SandboxEgressAllowlist {
		if x.Relay == "" {
			return nil, errors.New("the egress relay is unavailable: the path of this binary is unknown")
		}
		rules := make([]root.EgressRule, 0, len(s.Allow))
		for _, raw := range s.Allow {
			rule, err := root.ParseEgressRule(raw)
			if err != nil {
				return nil, err
			}
			rules = append(rules, rule)
		}
		sandbox.Egress = &local.Egress{Allow: rules, Relay: x.Relay}
	}
	if s.Profile == domain.SandboxProfileContainer {
		container, err := x.container(*s.Container)
		if err != nil {
			return nil, err
		}
		sandbox.Container = &container
		return sandbox, nil
	}
	program, err := x.LookPath("bwrap")
	if err != nil {
		return nil, errors.New("the profile process needs bwrap, which is not installed")
	}
	sandbox.Program = program
	return sandbox, nil
}

// container finds the runtime the container asks for, else podman, else
// docker.
func (x ExecutionSandboxes) container(c domain.SandboxContainer) (local.Container, error) {
	kinds := []string{local.ContainerPodman, local.ContainerDocker}
	if c.Runtime != "" {
		kinds = []string{c.Runtime}
	}
	for _, kind := range kinds {
		path, err := x.LookPath(kind)
		if err != nil {
			continue
		}
		return local.Container{Runtime: path, Kind: kind, Image: c.Image, MemoryMB: c.MemoryMB, CPUs: c.CPUs, PIDs: c.PIDs, User: strconv.Itoa(os.Getuid()) + ":" + strconv.Itoa(os.Getgid())}, nil
	}
	return local.Container{}, fmt.Errorf("the profile container needs %s, which is not installed", strings.Join(kinds, " or "))
}

// Probe checks, before an execution runs, that its sandbox can confine a
// command on this host, and says how: one line for the run's stderr. It
// runs a trivial command under bwrap, or asks the container runtime for
// the image, so a refusal ends the execution before its first request.
func (x ExecutionSandboxes) Probe(ctx context.Context, s domain.ExecutionSandbox) (string, error) {
	sandbox, err := x.Confine(s)
	if err != nil {
		return "", err
	}
	if s.Profile == domain.SandboxProfileNone {
		if sandbox == nil {
			return "sandbox none: local commands run unconfined", nil
		}
		return "sandbox none: local commands run under the settings' exec sandbox", nil
	}
	for _, path := range append(append([]string(nil), s.Writable...), s.Readable...) {
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("sandbox path %s: %w", path, err)
		}
	}
	if sandbox.Egress != nil {
		info, err := os.Stat(sandbox.Egress.Relay)
		if err != nil || !info.Mode().IsRegular() {
			return "", fmt.Errorf("the egress relay %s is not a program", sandbox.Egress.Relay)
		}
	}
	if err := local.ProbeSandbox(ctx, *sandbox); err != nil {
		return "", err
	}
	egress := string(s.Egress)
	if s.Egress == domain.SandboxEgressAllowlist {
		egress += " (" + strings.Join(s.Allow, ", ") + ")"
	}
	if sandbox.Container == nil {
		return "sandbox process (bwrap): local commands write only the workspace, the writable paths and a private /tmp; homes and /run hidden; egress " + egress, nil
	}
	rootful := "rootful"
	if rootless, err := local.ContainerRootless(ctx, *sandbox.Container); err == nil && rootless {
		rootful = "rootless"
	}
	return fmt.Sprintf("sandbox container (%s %s, image %s): local commands write only the workspace and the writable paths; egress %s", rootful, sandbox.Container.Kind, s.Container.Image, egress), nil
}
