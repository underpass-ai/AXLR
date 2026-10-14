package application

import "context"

// MemoryCountPort counts the events a memory store holds under one about,
// for the header's "N memories"; known is false when nothing was recorded
// under it yet.
type MemoryCountPort interface {
	Count(ctx context.Context, about string) (count int, known bool, err error)
}
