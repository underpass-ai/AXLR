package application

import "context"

// MemoryBundlePort exports one about as the checkout's bundle.
type MemoryBundlePort interface {
	Save(ctx context.Context, about string) (BundleSaved, error)
}
