package domain

import (
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
)

func TestAvailableModelTypedValues(t *testing.T) {
	id, err := root.NewModelID("openai/gpt-4.1")
	if err != nil {
		t.Fatal(err)
	}
	context, err := NewContextWindow(128000)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := NewModelRate("0.0000025")
	if err != nil {
		t.Fatal(err)
	}
	completion, err := NewModelRate("0.00001")
	if err != nil {
		t.Fatal(err)
	}
	model := AvailableModel{ID: id, Name: "GPT 4.1", Context: context, PromptRate: prompt, CompletionRate: completion, SupportsTools: true, TextOutput: true}
	if model.ID != "openai/gpt-4.1" || model.Context.Tokens() != 128000 || model.PromptRate.Display() != "$2.50 / 1M tokens" || model.CompletionRate.Display() != "$10 / 1M tokens" {
		t.Fatalf("unexpected typed model: %+v", model)
	}
}

func TestAvailableModelRejectsInvalidMetadata(t *testing.T) {
	for _, tokens := range []int{0, -1} {
		if _, err := NewContextWindow(tokens); err == nil {
			t.Fatalf("accepted context %d", tokens)
		}
	}
	for _, rate := range []string{"", "-0.01", "NaN", "1e-6", "1.2.3"} {
		if _, err := NewModelRate(rate); err == nil {
			t.Fatalf("accepted rate %q", rate)
		}
	}
	if _, err := NewModelRate("0"); err != nil {
		t.Fatalf("rejected free rate: %v", err)
	}
	if (ModelRate{}).Display() != "" {
		t.Fatal("unavailable rate should have no display")
	}
	if (ContextWindow(0)).Tokens() != 0 {
		t.Fatal("unavailable context should have zero tokens")
	}
}
