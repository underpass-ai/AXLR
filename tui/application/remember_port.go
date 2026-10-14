package application

import "context"

// RememberPort writes a RememberRequest and returns KMP's answer, compact.
type RememberPort interface {
	Remember(ctx context.Context, request RememberRequest) (map[string]any, error)
}
