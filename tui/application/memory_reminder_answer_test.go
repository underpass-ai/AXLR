package application

import (
	"context"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func TestAnswerThatAddressesRecordingIsNotReminded(t *testing.T) {
	for _, tc := range []struct {
		answer string
		want   bool
	}{
		// claude-haiku-5.5's final answer on 10 October 2026.
		{"I did not record anything in KMP. This was a small error-message change with no lasting decision.", true},
		{"Nothing durable to record: a one-line fix.", true},
		{"I didn’t record this in memory; it was a typo.", true},
		{"Recorded in KMP as a success_path with the test output as evidence.", true},
		{"I recorded the fix with axlr_remember.", true},
		{"No he registrado nada en KMP: era un cambio menor.", true},
		{"No hay nada que registrar en la memoria del proyecto.", true},
		{"Lo registré en KMP con la salida del test como evidencia.", true},
		{"Done: the parser splits on whitespace.", false},
		{"Fixed the record type in parser.go; tests pass.", false},
		{"He corregido el registro de errores en el parser.", false},
		{"The payload recorder did not record the stream body because the reader closed early; fixed in payload_recorder.go.", false},
		{"No hay nada que guardar, el fichero ya está actualizado.", false},
	} {
		if got := answerAddressesMemory(root.Text(tc.answer)); got != tc.want {
			t.Fatalf("%q: got %v", tc.answer, got)
		}
	}
}

// On 10 October 2026 claude-haiku-5.5 ended an edit and test turn with "I did
// not record anything in KMP…" and was reminded anyway; it then recorded a
// low-value success_path, one extra request for nothing.
func TestMemoryReminderRespectsTheModelsOwnAnswer(t *testing.T) {
	requests := 0
	u := agentWith(nil, func(_ context.Context, r root.CompletionRequest, _ func(root.Text) error) (root.CompletionResult, error) {
		requests++
		if requests == 1 {
			return assistant("", root.ToolCall{ID: "e1", Name: "local_edit", Arguments: mustObject(t, `{}`)}), nil
		}
		if last := string(r.Messages[len(r.Messages)-1].Content); strings.HasPrefix(last, MemoryReminderPrefix) {
			t.Fatalf("reminded after the model said it recorded nothing: %q", last)
		}
		return assistant("I did not record anything in KMP. This was a small error-message change."), nil
	})
	u.Approval = approvesAll{}
	u.Tools = toolFunc(func(context.Context, domain.ToolIdentity, root.JSONValue) (domain.ToolOutcome, error) {
		return domain.ToolOutcome{Content: `{"status":"completed","output":{}}`}, nil
	})
	s := turnSession(t)
	if err := s.BeginTurn("fix the parser", memoryTools(t)); err != nil {
		t.Fatal(err)
	}
	if err := u.Execute(context.Background(), &s, ignoreEvent); err != nil {
		t.Fatal(err)
	}
	if requests != 2 || s.Status() != domain.StatusComplete {
		t.Fatalf("requests=%d status=%s", requests, s.Status())
	}
	if MemoryReminderPrefix != "[AXLR · memory]" {
		t.Fatalf("the console renders reminders by this prefix: %q", MemoryReminderPrefix)
	}
}
