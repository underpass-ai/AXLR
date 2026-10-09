package application

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// ErrCeremonyNotPrepared means the pinned 2.0 definition is not published in
// the connected MADE store; only the explicit /mcp → P action publishes it.
var ErrCeremonyNotPrepared = errors.New("prepare MADE first: open /mcp, select MADE and press p")

// MissingDefinition is a pinned MADE definition the connected store does not
// publish yet.
type MissingDefinition struct {
	Name, Version string
}

// notPreparedError names the missing definitions and the one-time action; it
// matches ErrCeremonyNotPrepared with errors.Is.
type notPreparedError struct {
	missing []MissingDefinition
}

// NotPreparedError returns ErrCeremonyNotPrepared's failure naming each missing
// definition with its version and the exact action to publish it.
func NotPreparedError(missing ...MissingDefinition) error {
	return notPreparedError{missing: missing}
}

func (e notPreparedError) Error() string {
	names := make([]string, 0, len(e.missing))
	for _, m := range e.missing {
		names = append(names, m.Name+" "+m.Version)
	}
	list := strings.Join(names, ", ")
	if list == "" {
		list = "the MADE definition"
	}
	// Where the person's prompt went is the console's to say: this text
	// also reaches plan tasks and the model's tool results.
	return "MADE definition not published: " + list +
		". Preparing MADE is a one-time setup: open /mcp, select MADE and press p."
}

func (e notPreparedError) Is(target error) bool {
	return target == ErrCeremonyNotPrepared
}

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
	// Cancel ends the instance irreversibly with a recorded reason: a stalled
	// step the model will not hand back, or the person's stop.
	Cancel(ctx context.Context, instance, reason string) error
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
	// Outputs holds the latest completed output of every step, so a resumed
	// console step finds what an earlier one recorded.
	Outputs map[string]map[string]any
	// Live maps a step with an unexpired claim of the console's own to its
	// fence: MADE refuses a second claim while the lease runs, so the
	// console completes with the original fence instead.
	Live map[string]string
	// LiveErr is why Live could not be read: MADE's resume inspection
	// failed, so whether a claim of the console's own is live is unknown,
	// and a console step must be neither claimed again nor passed over on
	// that guess.
	LiveErr error
}

// CheckRunnerPort runs one command in the workspace with a time limit and no
// shell.
type CheckRunnerPort interface {
	Run(ctx context.Context, command domain.CheckCommand) (CheckResult, error)
}

// ConsoleOutputBytes is the output cap the console asks for when it parses a
// command's output itself: the runtime's hard limit (hardFileBytes in
// runtime/config.go). An output still cut there is reported by
// CheckResult.Truncated and refused, never parsed in part.
const ConsoleOutputBytes = 1 << 20

type CheckResult struct {
	// Ran is false when the program never started (not found, timed out,
	// refused); ExitCode is then meaningless.
	Ran      bool
	ExitCode int
	// Output is the bounded tail of stdout and stderr, for showing to the model.
	Output string
	// Stdout is the whole stdout within the runtime's output cap, for callers
	// that parse it: stderr may carry notices (gh's release notice, for one)
	// that are not part of the parsed value. Empty when the runner only has
	// Output.
	Stdout string
	// Truncated is true when the runtime's output cap, which stdout and
	// stderr share, cut the output: Stdout then lacks its end, so a caller
	// that parses it must refuse it.
	Truncated bool
}

// RanNoTests reports a Go test run that exited 0 without running a test: a
// -run pattern that matches nothing passes with "[no tests to run]" on every
// package line, or with "testing: warning: no tests to run" in a single
// directory. Such a check proves nothing, so it never passes, and before a
// change it shows the improvement missing. It is narrow on purpose: only
// those Go markers with exit 0, never when one package ran a test, and never
// on output whose head or end is gone.
func (r CheckResult) RanNoTests() bool {
	if !r.Ran || r.ExitCode != 0 || r.Truncated {
		return false
	}
	text := r.Stdout
	if text == "" {
		if strings.HasPrefix(r.Output, "…") {
			return false
		}
		text = r.Output
	}
	warned := strings.Contains(text, "testing: warning: no tests to run")
	if !warned && !strings.Contains(text, "[no tests to run]") {
		return false
	}
	if strings.Contains(text, "=== RUN") || strings.Contains(text, `"Action":"run"`) {
		return false // a test started (-v or -json)
	}
	packages, empty := 0, 0
	for _, line := range strings.Split(text, "\n") {
		if !strings.HasPrefix(line, "ok ") && !strings.HasPrefix(line, "ok\t") {
			continue
		}
		packages++
		if strings.HasSuffix(strings.TrimRight(line, " \r"), "[no tests to run]") {
			empty++
		}
	}
	return packages > 0 && empty == packages || warned && packages <= 1
}

// noTestsFeedback is what the model is told when its check ran no tests.
const noTestsFeedback = "the check ran no tests: go test matched no test (\"[no tests to run]\"), so its zero exit proves nothing; add the test or fix the -run pattern so the check runs it"

// MemoryPort is KMP as the ceremony driver sees it.
type MemoryPort interface {
	// Wake returns bounded context for the about, or "" when it has none yet.
	Wake(ctx context.Context, about string) (string, error)
	// WakeFocused is Wake with an intent the store may focus the recall on;
	// it also returns the refs the recall exposed, which links may name.
	WakeFocused(ctx context.Context, about, intent string) (text string, refs []string, err error)
	Record(ctx context.Context, about string, labels map[string][]string, id, summary, evidence string) error
	// RecordLinked writes one memory with its relations to existing refs and
	// returns the stored ref, so later memories can link to it exactly.
	RecordLinked(ctx context.Context, about string, labels map[string][]string, record MemoryRecord) (ref string, err error)
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

// ForgePort is the Git hosting side of the repair ceremony as the driver sees
// it: branches, pull requests, checks and merges, never the model.
type ForgePort interface {
	// Propose commits the clone's changes on the branch, pushes it and opens
	// the pull request; with Number set it pushes to the open one instead.
	Propose(ctx context.Context, proposal RepairProposal) (PullRequest, error)
	// Status reads the pull request's checks and merge state.
	Status(ctx context.Context, repository string, number int) (PullRequestStatus, error)
	// UpdateBranch merges the base into the pull request branch on the forge.
	UpdateBranch(ctx context.Context, repository string, number int) error
	// Merge squash-merges the pull request, deletes its branch and returns
	// the merge commit.
	Merge(ctx context.Context, repository string, number int) (string, error)
}

type RepairProposal struct {
	Repository, Base, Branch string
	Title, Body, Trailer     string
	// Number is the open pull request on a later round, zero on the first.
	Number int
}

type PullRequest struct {
	Number       int
	URL, HeadSHA string
}

// PullRequestStatus is what the forge reports about an open pull request.
type PullRequestStatus struct {
	// State is OPEN, MERGED or CLOSED.
	State string
	// MergeState is the forge's merge state: CLEAN, BEHIND, BLOCKED, DIRTY,
	// UNSTABLE, HAS_HOOKS or UNKNOWN.
	MergeState string
	HeadSHA    string
	// Pending, Passed and Failed count the head commit's checks; Failed names
	// each failing check with its conclusion.
	Pending, Passed int
	Failed          []string
}

// RepairPolicy is what settings.json decides about the repair ceremony.
type RepairPolicy struct {
	// AutoMerge lets the console merge a green pull request without the
	// person; otherwise the merge waits on the approval card.
	AutoMerge bool
	// WatchDeadline bounds one check round; Poll spaces the status reads.
	WatchDeadline, Poll time.Duration
}

// MemoryLink is a relation the model proposes from a new memory to a ref the
// recall exposed; the console refuses refs the recall did not show.
type MemoryLink struct {
	Ref, Rel, Why, Evidence string
}

// MemoryRecord is one memory with its links, written by the console.
type MemoryRecord struct {
	ID, Kind, Summary, Evidence string
	Links                       []MemoryLink
}
