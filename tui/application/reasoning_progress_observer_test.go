package application

import (
	"context"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
)

// A turn passes the stream's reasoning progress on to the console as
// events, so the footer can count it and the transcript show its text.
func TestATurnEmitsReasoningProgress(t *testing.T) {
	s := turnSession(t)
	store := &memoryStore{}
	reasoning := streamFunc(func(ctx context.Context, _ root.CompletionRequest, onText func(root.Text) error) (root.CompletionResult, error) {
		NotifyReasoningProgress(ctx, 700, "Plan the ")
		NotifyReasoningProgress(ctx, 1300, "page.")
		_ = onText("hola")
		return root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, Content: "hola"}}, nil
	})
	var told []int
	var text string
	emit := func(e Event) error {
		if e.Kind == EventReasoningProgress {
			told = append(told, e.ReasoningBytes)
			text += e.ReasoningText
		}
		return nil
	}
	u := StartTurnUseCase{Catalog: &catalogStub{}, Store: store, Continue: ContinueTurnUseCase{Models: reasoning, Store: store}}
	if err := u.Execute(context.Background(), &s, "di hola", emit); err != nil {
		t.Fatal(err)
	}
	if len(told) != 2 || told[0] != 700 || told[1] != 1300 || text != "Plan the page." {
		t.Fatalf("reasoning progress %v %q", told, text)
	}
	NotifyReasoningProgress(nil, 1, "")
	NotifyReasoningProgress(context.Background(), 1, "")
}
