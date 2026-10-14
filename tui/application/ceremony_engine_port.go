package application

import (
	"context"
	"time"
)

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
	// Report tells MADE where the host stands on a claimed step (decision
	// H5); Renew extends a claim's lease; Operation reads a claim's
	// operation id and step attempt, which a report must repeat.
	Report(ctx context.Context, report HostReport) error
	Renew(ctx context.Context, instance, step, fence string, lease time.Duration) error
	Operation(ctx context.Context, instance, step, fence string) (ClaimOperation, error)
	// Handoff journals that a claim's host incarnation is gone (decision
	// H6): a console that finds a claim of an earlier incarnation of
	// itself declares it lost, with the evidence, before completing it.
	Handoff(ctx context.Context, handoff HostHandoff) error
}

// ClaimOperation is what MADE checks a host report against besides the
// fence and the owner: the claim's operation id and the attempt of the
// claimed step record. On 14 October 2026 every report of v0.8.1 was
// refused as "claim was changed by someone else first" because none
// carried the attempt and MADE read the missing field as zero.
type ClaimOperation struct {
	ID      string
	Attempt int
}
