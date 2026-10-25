package application

import (
	"context"

	"github.com/underpass-ai/AXLR/tui/domain"
)

type executionSandboxKey struct{}

// WithSessionSandbox carries the session's sandbox to the tool runtime, so
// a call of an execution runs its commands under the execution's sandbox
// while one runtime serves several sessions; the context as it is for a
// session without one.
func WithSessionSandbox(ctx context.Context, s domain.Session) context.Context {
	sandbox, has := s.Sandbox()
	if !has {
		return ctx
	}
	return context.WithValue(ctx, executionSandboxKey{}, sandbox)
}

// ExecutionSandboxOf is the sandbox of the execution whose call the runtime
// is executing, if it has one.
func ExecutionSandboxOf(ctx context.Context) (domain.ExecutionSandbox, bool) {
	sandbox, ok := ctx.Value(executionSandboxKey{}).(domain.ExecutionSandbox)
	return sandbox, ok
}
