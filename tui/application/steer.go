package application

import (
	"context"
	"errors"
	"strings"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// SteerSource holds what the person wrote while a turn was running. The turn
// takes it between model steps; Restore returns text the turn could not keep.
type SteerSource interface {
	Take() (root.Text, bool)
	Restore(root.Text)
}

type steerKey struct{}

// WithSteer scopes a steer source to one operation.
func WithSteer(ctx context.Context, source SteerSource) context.Context {
	return context.WithValue(ctx, steerKey{}, source)
}

func steerFrom(ctx context.Context) SteerSource {
	source, _ := ctx.Value(steerKey{}).(SteerSource)
	return source
}

// steeredNote tells the model why a user message follows tool results: the
// person wrote it while the previous request was still being worked on.
const steeredNote = "\n\n[AXLR] The person sent this while you were still working on their previous request. Answer it and, unless it changes or replaces that request, also finish and answer the previous one."

// applySteer adds a queued message to the running turn after a tool step, so
// the next request carries it instead of the turn being cancelled for it. A
// ceremony keeps its own step structure and is never steered.
func applySteer(ctx context.Context, session *domain.Session, store SessionStorePort, trace DiagnosticPort) (bool, error) {
	source := steerFrom(ctx)
	if source == nil {
		return false, nil
	}
	if _, live := session.Ceremony(); live {
		return false, nil
	}
	messages := session.Messages()
	if len(messages) == 0 || messages[len(messages)-1].Role != root.RoleTool {
		return false, nil
	}
	text, ok := source.Take()
	if !ok {
		return false, nil
	}
	next := *session
	if err := next.Steer(text); err != nil {
		source.Restore(text)
		return false, err
	}
	if err := store.Save(ctx, next); err != nil {
		source.Restore(text)
		return false, err
	}
	*session = next
	if trace != nil {
		_ = trace.Record(DiagnosticEvent{Stage: DiagnosticSteerApplied, Bytes: len(text), Messages: len(next.Messages())})
	}
	return true, nil
}

// markSteered annotates, for the model only, the person's messages that
// arrived between a tool result and the model's next step. Host notes, which
// start with "[AXLR", are not the person's.
func markSteered(messages []root.Message) []root.Message {
	marked := messages
	copied := false
	for i := 1; i < len(messages); i++ {
		if messages[i].Role != root.RoleUser || messages[i-1].Role != root.RoleTool || strings.HasPrefix(string(messages[i].Content), "[AXLR") {
			continue
		}
		if !copied {
			marked = append([]root.Message(nil), messages...)
			copied = true
		}
		marked[i].Content = messages[i].Content + steeredNote
	}
	return marked
}

// joinCause adds err to cause unless it only repeats it, such as the
// cancellation an emit reports after the stream was cancelled.
func joinCause(cause, err error) error {
	if err == nil || (cause != nil && errors.Is(cause, err)) {
		return cause
	}
	return errors.Join(cause, err)
}
