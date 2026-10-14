package application

import "context"

// ReasoningProgressObserver receives how many bytes of reasoning the model
// has streamed so far in one request. The reasoning text never leaves the
// stream. On 14 October 2026 glm-5.3-flash reasoned for four to five
// minutes and the console showed only "Model is reasoning" and a clock.
type ReasoningProgressObserver func(bytes int)

type reasoningProgressKey struct{}

// WithReasoningProgress scopes the observer to one model stream.
func WithReasoningProgress(ctx context.Context, observer ReasoningProgressObserver) context.Context {
	return context.WithValue(ctx, reasoningProgressKey{}, observer)
}

// NotifyReasoningProgress reports progress unless the stream was cancelled.
func NotifyReasoningProgress(ctx context.Context, bytes int) {
	if ctx == nil || ctx.Err() != nil {
		return
	}
	observer, _ := ctx.Value(reasoningProgressKey{}).(ReasoningProgressObserver)
	if observer != nil {
		observer(bytes)
	}
}
