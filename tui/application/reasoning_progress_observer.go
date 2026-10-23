package application

import "context"

// ReasoningProgressObserver receives how many bytes of reasoning the model
// has streamed so far in one request, and the readable reasoning text
// streamed since the previous call, for the console to show as it arrives;
// it is never saved (the session keeps the provider's own record, not this
// text). On 14 October 2026 glm-5.3-flash reasoned for four to five minutes
// and the console showed only "Model is reasoning" and a clock; on 23
// October 2026 claude-haiku-5.5 thought for two minutes before each file
// and the console could only count the bytes.
type ReasoningProgressObserver func(bytes int, text string)

type reasoningProgressKey struct{}

// WithReasoningProgress scopes the observer to one model stream.
func WithReasoningProgress(ctx context.Context, observer ReasoningProgressObserver) context.Context {
	return context.WithValue(ctx, reasoningProgressKey{}, observer)
}

// NotifyReasoningProgress reports progress unless the stream was cancelled.
func NotifyReasoningProgress(ctx context.Context, bytes int, text string) {
	if ctx == nil || ctx.Err() != nil {
		return
	}
	observer, _ := ctx.Value(reasoningProgressKey{}).(ReasoningProgressObserver)
	if observer != nil {
		observer(bytes, text)
	}
}
