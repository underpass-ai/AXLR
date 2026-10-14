package application

import "context"

// ApproverPort grants a human guard as the person's approver identity, which
// the work identity cannot do.
type ApproverPort interface {
	ApproveGuard(ctx context.Context, instance, guard string) error
}
