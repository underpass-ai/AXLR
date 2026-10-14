package application

import "context"

// MemoryComparePort reads comparisons; it writes nothing.
type MemoryComparePort interface {
	Compare(ctx context.Context, request CompareRequest) (CompareResult, error)
}
