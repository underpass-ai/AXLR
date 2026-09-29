package application

import (
	"context"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type diagnosticCollector struct{ events []DiagnosticEvent }

func (c *diagnosticCollector) Record(event DiagnosticEvent) error {
	c.events = append(c.events, event)
	return nil
}

func (c *diagnosticCollector) contains(stage DiagnosticStage) bool {
	for _, event := range c.events {
		if event.Stage == stage {
			return true
		}
	}
	return false
}

func TestModelCatalogRecordsLifecycle(t *testing.T) {
	trace := &diagnosticCollector{}
	u := ListModelsUseCase{Catalog: modelCatalogFunc(func(context.Context) ([]domain.AvailableModel, error) {
		return []domain.AvailableModel{catalogModel("provider/model", "Model", true, true)}, nil
	}), Diagnostics: trace}
	if _, err := u.Execute(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !trace.contains(DiagnosticModelCatalogStart) || !trace.contains(DiagnosticModelCatalogDone) {
		t.Fatalf("catalog lifecycle absent: %+v", trace.events)
	}
}

func TestToolLifecycleRecordsRequestDecisionAndCompletion(t *testing.T) {
	trace := &diagnosticCollector{}
	store := &memoryStore{}
	s := turnSession(t)
	if err := s.BeginTurn("go", turnTools()); err != nil {
		t.Fatal(err)
	}
	continuation := ContinueTurnUseCase{Store: store, Diagnostics: trace, Models: streamFunc(func(_ context.Context, _ root.CompletionRequest, _ func(root.Text) error) (root.CompletionResult, error) {
		if !trace.contains(DiagnosticToolRequested) {
			return assistant("", call("a", "read")), nil
		}
		return assistant("done"), nil
	})}
	if err := continuation.Execute(context.Background(), &s, nil); err != nil {
		t.Fatal(err)
	}
	u := ResolveToolUseCase{Store: store, Continue: continuation, Diagnostics: trace}
	if err := u.Execute(context.Background(), &s, "a", domain.DecisionDeny, nil); err != nil {
		t.Fatal(err)
	}
	for _, stage := range []DiagnosticStage{DiagnosticToolRequested, DiagnosticToolRejected, DiagnosticToolCompleted} {
		if !trace.contains(stage) {
			t.Fatalf("missing %s: %+v", stage, trace.events)
		}
	}
}
