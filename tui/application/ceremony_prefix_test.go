package application

import (
	"context"
	"strings"
	"testing"
	"time"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// A standard ceremony's system prompt names the ceremony and nothing that
// changes while it runs, so the provider's cached prefix survives its steps,
// attempts and check command.
func TestStandardCeremonyKeepsTheSystemPromptAcrossSteps(t *testing.T) {
	_, _, s := stalledDelivery(t)
	before := string(modelHostGuidance(&s).Content)
	run, _ := s.Ceremony()
	run.Step, run.Iteration, run.Memory = "build", 2, "recalled evidence"
	run.Check = domain.CheckCommand{Program: "go", Args: []string{"test", "./..."}}
	if err := s.SetCeremony(run); err != nil {
		t.Fatal(err)
	}
	after := string(modelHostGuidance(&s).Content)
	if before != after || !strings.Contains(after, "Ceremony axlr_delivery 2.0 is running") || strings.Contains(after, "Current step") || strings.Contains(after, "recalled evidence") {
		t.Fatalf("guidance changed or carries the step:\n%s\n---\n%s", before, after)
	}
	note := CurrentStepNote(run, false)
	if !strings.HasPrefix(note, "[AXLR] Ceremony axlr_delivery, current step: build (attempt 2 of 3).") || !strings.Contains(note, "Approved check command: go test ./....") || strings.Contains(note, "recalled evidence") {
		t.Fatalf("note = %s", note)
	}
	if !strings.Contains(CurrentStepNote(run, true), "KMP recall for this session") {
		t.Fatal("the first note lacks the recall")
	}
	if progress := stepProgress(run); progress != " This is attempt 2 of 3. Approved check command: go test ./...." {
		t.Fatalf("progress = %q", progress)
	}
}

// The step reaches the model on the person's prompt: when the ceremony
// begins and on a later prompt while it runs.
func TestCeremonyStepTravelsOnThePersonsPrompt(t *testing.T) {
	engine := &fakeEngine{}
	d := &CeremonyDriver{Engine: engine, Checks: &fakeChecks{}, Now: func() time.Time { return time.Unix(1, 0) }}
	s := turnSession(t)
	if err := s.SetMode(domain.ModeDelivery); err != nil {
		t.Fatal(err)
	}
	var last []string
	store := &memoryStore{}
	u := StartTurnUseCase{Catalog: &catalogStub{}, Store: store, Continue: ContinueTurnUseCase{Store: store, Ceremonies: d, Models: streamFunc(func(_ context.Context, r root.CompletionRequest, _ func(root.Text) error) (root.CompletionResult, error) {
		last = append(last, string(r.Messages[len(r.Messages)-1].Content))
		return assistant("trabajando"), nil
	})}}
	_ = u.Execute(context.Background(), &s, "arregla el test", ignoreEvent)
	if len(last) == 0 || !strings.HasPrefix(last[0], "arregla el test\n\n[AXLR] Ceremony axlr_delivery, current step: brief.") {
		t.Fatalf("first request ends with %q", last)
	}
	// A later prompt while the step is open carries it again, without the
	// recall the first one brought.
	_, d, s = stalledDelivery(t)
	u.Continue.Ceremonies = d
	if err := s.CompleteAssistant(assistant("pausa")); err != nil {
		t.Fatal(err)
	}
	last = nil
	_ = u.Execute(context.Background(), &s, "sigue", ignoreEvent)
	if len(last) == 0 || !strings.HasPrefix(last[0], "sigue\n\n[AXLR] Ceremony axlr_delivery, current step: brief.") {
		t.Fatalf("later request ends with %q", last)
	}
}
