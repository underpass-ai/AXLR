package terminal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/adapters/openrouter"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// A failed turn shows the provider's message and what to do about it in
// the footer, and leaves the same row in the transcript, saved as a console
// line, so reopening the session still says why the prompt has no reply.
func TestAFailedTurnLeavesAnErrorRow(t *testing.T) {
	s := navSession(t)
	if err := s.BeginTurn("hola", nil); err != nil {
		t.Fatal(err)
	}
	if err := s.InterruptDraft(""); err != nil {
		t.Fatal(err)
	}
	m := navModel(t, &s)
	m.Busy, m.operationID, m.providerWaiting = true, 1, true
	failure := fmt.Errorf("model request: %w", &openrouter.ProviderError{StatusCode: 400, Category: openrouter.CategoryInvalidRequest, Message: "anthropic/claude-nonexistent-9 is not a valid model ID"})
	next, cmd := m.Update(operationComplete{ID: 1, Session: s, Err: failure})
	m = drain(t, next.(AppModel), cmd)
	want := "model request: OpenRouter invalid_request (HTTP 400): anthropic/claude-nonexistent-9 is not a valid model ID · choose a model the provider serves with /model"
	if m.Status.Error != want {
		t.Fatalf("footer %q", m.Status.Error)
	}
	lines := m.deps.Session.ConsoleLines()
	if len(lines) != 1 || lines[0].Failure == nil || lines[0].Failure.Hint != "choose_model" || lines[0].AfterMessage != 1 {
		t.Fatalf("lines %+v", lines)
	}
	if text := m.Transcript.Text(); !strings.Contains(text, "x OpenRouter invalid_request (HTTP 400): anthropic/claude-nonexistent-9") || !strings.Contains(text, "· choose a model") {
		t.Fatalf("transcript:\n%s", text)
	}
	loaded, err := m.deps.Store.Load(context.Background(), s.Export().ID)
	if err != nil || len(loaded.ConsoleLines()) != 1 {
		t.Fatalf("not saved: %v", err)
	}
	m.Theme.Locale = Spanish
	if row := consoleLineRow(lines[0], m.Theme); !strings.HasSuffix(row.Text, "· elige con /model un modelo que sirva el proveedor") {
		t.Fatalf("spanish row %q", row.Text)
	}
}

// Only failures leave a line: the person's cancel, the tool-call limit and
// the budget pause say how to go on in the footer. A refused key of a local
// endpoint gets its own hint, and an unknown hint kind none.
func TestTurnFailureKinds(t *testing.T) {
	for _, err := range []error{nil, context.Canceled, fmt.Errorf("x: %w", domain.ErrToolCallLimit), &domain.SessionBudgetError{Limit: 1}} {
		if _, ok := turnFailure(err); ok {
			t.Fatalf("%v left a line", err)
		}
	}
	got, ok := turnFailure(&openrouter.ProviderError{StatusCode: 401, Category: openrouter.CategoryAuthentication, Provider: "local/qwen"})
	if !ok || got.Hint != "check_key_local" || !strings.HasPrefix(got.Message, "local/qwen authentication (HTTP 401)") {
		t.Fatalf("local key: %+v", got)
	}
	if got, ok := turnFailure(errors.New(strings.Repeat("é", domain.MaxTurnFailureRunes+5) + "\nsecond")); !ok || len([]rune(got.Message)) != domain.MaxTurnFailureRunes || got.Hint != "" {
		t.Fatalf("long: %d runes", len([]rune(got.Message)))
	}
	if hint := (Theme{Locale: English}).providerHint("unknown_kind"); hint != "" {
		t.Fatalf("unknown hint %q", hint)
	}
}
