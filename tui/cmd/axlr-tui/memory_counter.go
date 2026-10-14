package main

import "context"

// memoryCounter adapts the KMP store's count to the header's memories.
type memoryCounter func(ctx context.Context, about string) (int, bool, error)

func (c memoryCounter) Count(ctx context.Context, about string) (int, bool, error) {
	return c(ctx, about)
}
