package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/underpass-ai/AXLR/adapters/local"
	"github.com/underpass-ai/AXLR/tui/adapters/storage"
)

// execSandbox turns exec_sandbox into the runtime's sandbox, probed once at
// launch: nil when off, or when auto finds this platform unable to confine
// a command; a sandbox that refuses every command when required and
// unable; bwrap otherwise. Each mode but off says on stderr what it got;
// unavailable is why a configured sandbox cannot confine commands, for the
// console to show once stderr is behind its screen.
func execSandbox(ctx context.Context, settings storage.ExecSandboxSettings, lookPath func(string) (string, error), stderr io.Writer) (sandbox *local.Sandbox, unavailable string, err error) {
	if settings.Mode == "off" {
		return nil, "", nil
	}
	for _, path := range settings.Writable {
		if _, err := os.Stat(path); err != nil {
			return nil, "", fmt.Errorf("exec_sandbox.writable %s: %w", path, err)
		}
	}
	confined := local.Sandbox{Network: settings.AllowsNetwork(), Writable: settings.Writable}
	program, err := lookPath("bwrap")
	if err != nil {
		err = errors.New("bwrap is not installed")
	} else {
		confined.Program = program
		err = local.ProbeSandbox(ctx, confined)
	}
	switch {
	case err == nil:
		network := "on"
		if !confined.Network {
			network = "off"
		}
		writable := "the workspace"
		if len(confined.Writable) > 0 {
			writable += ", " + strings.Join(confined.Writable, ", ")
		}
		fmt.Fprintf(stderr, "axlr-tui: exec sandbox: local commands write only %s and a private /tmp; network %s\n", writable, network)
		return &confined, "", nil
	case settings.Mode == "required":
		fmt.Fprintf(stderr, "axlr-tui: exec_sandbox is required but unavailable (%v): local commands are refused\n", err)
		return &local.Sandbox{Unavailable: err.Error()}, err.Error(), nil
	default:
		fmt.Fprintf(stderr, "axlr-tui: exec sandbox unavailable (%v): local commands run unsandboxed\n", err)
		return nil, err.Error(), nil
	}
}
