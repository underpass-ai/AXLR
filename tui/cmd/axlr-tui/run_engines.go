package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/underpass-ai/AXLR/buildinfo"
	"github.com/underpass-ai/AXLR/plugins"
	"github.com/underpass-ai/AXLR/tui/adapters/engines"
)

// engineServeFlag runs the process as the daemon of one shared engine; the
// console starts it with the engine's spec on stdin.
const engineServeFlag = "--engines-serve"

// serveEngine is the daemon's whole life.
func serveEngine(ctx context.Context, socket string, spec io.Reader) int {
	if err := engines.Serve(ctx, socket, spec, buildinfo.Version, engines.IdleExit); err != nil {
		return 1
	}
	return 0
}

// sharedEngine reports whether a registration's process may be shared:
// KMP and MADE launched as stdio commands.
func sharedEngine(r plugins.Registration) bool {
	id := r.Manifest.ID.String()
	return (id == "kmp" || id == "made") && r.Manifest.Command != ""
}

// engineSocketDir is where the shared engines' sockets live: the user's
// runtime directory, else the state directory.
func engineSocketDir(getenv func(string) string, stateBase string) string {
	if runtimeDir := getenv("XDG_RUNTIME_DIR"); filepath.IsAbs(runtimeDir) {
		return filepath.Join(runtimeDir, "axlr", "engines")
	}
	return filepath.Join(stateBase, "axlr", "engines")
}

// shareEngines, when settings ask for it, makes the manager reach KMP and
// MADE through a daemon shared by every console with the same launch, and
// starts those daemons now so the launch can say in one line what it got.
// Anything that fails leaves the console starting its own engine.
func shareEngines(ctx context.Context, manager *plugins.Manager, registrations []plugins.Registration, supervisor engines.Supervisor, stderr io.Writer) {
	manager.SetShare(func(ctx context.Context, r plugins.Registration) (string, error) {
		if !sharedEngine(r) {
			return "", fmt.Errorf("%s is not shared", r.Manifest.ID)
		}
		return supervisor.Socket(ctx, engines.Spec{Command: r.Manifest.Command, Args: r.Manifest.Args, Env: r.Env})
	})
	var shared []string
	for _, r := range registrations {
		if !sharedEngine(r) {
			continue
		}
		if _, err := supervisor.Socket(ctx, engines.Spec{Command: r.Manifest.Command, Args: r.Manifest.Args, Env: r.Env}); err != nil {
			fmt.Fprintf(stderr, "axlr-tui: shared engine %s unavailable (%v): this console starts its own\n", r.Manifest.ID, err)
			continue
		}
		shared = append(shared, r.Manifest.ID.String())
	}
	if len(shared) > 0 {
		fmt.Fprintf(stderr, "axlr-tui: shared engines: %s, through %s\n", strings.Join(shared, ", "), supervisor.Dir)
	}
}

// engineSupervisor is the console's supervisor of shared engines.
func engineSupervisor(getenv func(string) string, stateBase string) (engines.Supervisor, error) {
	executable, err := os.Executable()
	if err != nil {
		return engines.Supervisor{}, err
	}
	return engines.Supervisor{Dir: engineSocketDir(getenv, stateBase), Executable: executable, Version: buildinfo.Version}, nil
}
