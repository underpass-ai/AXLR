package application

import "context"

// ToolCallProgressObserver receives the name and cumulative argument bytes of the
// tool call the model is streaming. Argument text never leaves the stream.
type ToolCallProgressObserver func(name string, bytes int)

type toolCallProgressKey struct{}

// WithToolCallProgress scopes the observer to one model stream.
func WithToolCallProgress(ctx context.Context, observer ToolCallProgressObserver) context.Context {
	return context.WithValue(ctx, toolCallProgressKey{}, observer)
}

// NotifyToolCallProgress reports progress unless the stream was cancelled.
func NotifyToolCallProgress(ctx context.Context, name string, bytes int) {
	if ctx == nil || ctx.Err() != nil {
		return
	}
	observer, _ := ctx.Value(toolCallProgressKey{}).(ToolCallProgressObserver)
	if observer != nil {
		observer(name, bytes)
	}
}
