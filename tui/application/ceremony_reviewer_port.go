package application

import "context"

// CeremonyReviewerPort judges a draft in a fresh context: no transcript and
// no tools other than the verdict.
type CeremonyReviewerPort interface {
	Review(ctx context.Context, request ReviewRequest) (ReviewVerdict, error)
}
