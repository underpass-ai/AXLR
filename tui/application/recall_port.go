package application

import "context"

// RecallPort reads KMP for the console and for axlr_recall.
type RecallPort interface {
	Recall(ctx context.Context, request RecallRequest) (RecallResult, error)
}
