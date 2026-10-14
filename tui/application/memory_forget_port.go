package application

import "context"

// MemoryForgetPort closes an entry by superseding it.
type MemoryForgetPort interface {
	Forget(ctx context.Context, request ForgetRequest) (ForgetReceipt, error)
}
