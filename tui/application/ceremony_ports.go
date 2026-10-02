package application

import (
	"context"
	"errors"
	"time"

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
	// Claim takes the step with a lease; zero means the engine's default.
	Claim(ctx context.Context, instance, step, key string, lease time.Duration) (fence string, err error)
	Complete(ctx context.Context, instance, step, fence string, output map[string]any) error
	// Transition applies a trigger and returns the instance's new state.
	Transition(ctx context.Context, instance, trigger string) (state string, err error)
	// Inspect reads where the instance is, to reconcile an interrupted advance.
	Inspect(ctx context.Context, instance string) (CeremonyView, error)
}

// CeremonyView is the part of a MADE instance the driver reconciles from.
type CeremonyView struct {
	State string
	// Enabled lists the triggers MADE would accept now.
	Enabled []string
	// Claimable lists the steps that can be claimed now.
	Claimable []string
	// Completed holds the outputs of the steps completed in this visit of
	// the current state, so a completion whose answer was lost is found.
	Completed map[string]map[string]any
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

// WorkspaceFilesPort reads and writes workspace files for the console through
// the same runtime as the model's tools, so the workspace boundary holds.
type WorkspaceFilesPort interface {
	// Read returns at most maxBytes; found is false when the file is missing.
	Read(ctx context.Context, path string, maxBytes int) (content []byte, found bool, err error)
	// Write creates or replaces the file.
	Write(ctx context.Context, path string, content []byte) error
	MakeDir(ctx context.Context, path string) error
}

// CeremonyReviewerPort judges a draft in a fresh context: no transcript and
// no tools other than the verdict.
type CeremonyReviewerPort interface {
	Review(ctx context.Context, request ReviewRequest) (ReviewVerdict, error)
}

type ReviewRequest struct {
	// SessionModel is the reviewer's model unless reviewer_model is set.
	SessionModel string
	Rubric       string
	Draft        string
	// ReturnReason is the person's reason when the draft came back to review.
	ReturnReason string
}

type ReviewVerdict struct {
	Accepted bool
	Findings []string
	// Model is the model that judged, for the record.
	Model string
}

// ApproverPort grants a human guard as the person's approver identity, which
// the work identity cannot do.
type ApproverPort interface {
	ApproveGuard(ctx context.Context, instance, guard string) error
}
