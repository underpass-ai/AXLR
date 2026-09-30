package application

import (
	"context"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// ProviderActivityObserver is scoped to one model stream, never the whole app.
type ProviderActivityObserver func(domain.ProviderPhase)

func WithProviderActivity(ctx context.Context, observer ProviderActivityObserver) context.Context {
	return context.WithValue(ctx, providerActivityKey{}, observer)
}

func NotifyProviderActivity(ctx context.Context, phase domain.ProviderPhase) {
	if ctx == nil || ctx.Err() != nil {
		return
	}
	observer, _ := ctx.Value(providerActivityKey{}).(ProviderActivityObserver)
	if observer != nil {
		observer(phase)
	}
}
