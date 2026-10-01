package application

import (
	"context"
	"errors"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// ErrCeremonyNotPrepared means the pinned 2.0 definition is not published in
// the connected MADE store; only the explicit /mcp → P action publishes it.
var ErrCeremonyNotPrepared = errors.New("prepare MADE first: /mcp → P")

// CeremonyEnginePort is MADE as the ceremony driver sees it. Implementations
// keep MCP shapes out of the application layer.
type CeremonyEnginePort interface {
	// Ready reports ErrCeremonyNotPrepared unless the exact pinned definition
	// is published.
	Ready(ctx context.Context, definition, version string) error
	Start(ctx context.Context, definition, version, instance string, inputs map[string]string) error
	Claim(ctx context.Context, instance, step, key string) (fence string, err error)
	Complete(ctx context.Context, instance, step, fence string, output map[string]any) error
	// Transition applies a trigger and returns the instance's new state.
	Transition(ctx context.Context, instance, trigger string) (state string, err error)
}

// CheckRunnerPort runs one command in the workspace with a time limit and no
// shell.
type CheckRunnerPort interface {
	Run(ctx context.Context, command domain.CheckCommand) (CheckResult, error)
}

type CheckResult struct {
	// Ran is false when the program never started (not found, timed out,
	// refused); ExitCode is then meaningless.
	Ran      bool
	ExitCode int
	// Output is the bounded tail of stdout and stderr.
	Output string
}

// MemoryPort is KMP as the ceremony driver sees it.
type MemoryPort interface {
	// Wake returns bounded context for the about, or "" when it has none yet.
	Wake(ctx context.Context, about string) (string, error)
	Record(ctx context.Context, about string, labels map[string][]string, id, summary, evidence string) error
}
