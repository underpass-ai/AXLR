package terminal

import (
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/tui/domain"
)

func TestSessionPanelShowsTheInstanceAndTheLastTurn(t *testing.T) {
	theme := Theme{Monochrome: true}
	state := domain.SessionState{ID: "0123456789abcdef0123456789abcdef"}
	if text := sessionPanelContent(state, theme, ""); !strings.Contains(text, "No MADE instance yet") {
		t.Fatalf("%s", text)
	}
	state.Instance = &domain.SessionInstance{Unavailable: "MADE definition not published"}
	if text := sessionPanelContent(state, theme, ""); !strings.Contains(text, "No MADE instance: MADE definition not published") {
		t.Fatalf("%s", text)
	}
	record := `{"turn":3,"prompt":"crea hola.txt","answer":"Hecho.","answer_kind":"changed","reads":2,"cost":{"requests":2,"usd":0.0012},"actions":[{"tool":"local_write","target":"hola.txt (create)","why":"create the file","decision":"auto_approve","outcome":"completed","call_id":"c1"},{"tool":"local_exec","target":"cat hola.txt","why":"check it","decision":"deny","reason":"not now","outcome":"denied","exit_code":null,"call_id":"c2"}],"memories":[{"about":"project:Lab","idempotency_key":"axlr:remember:1"}]}`
	state.Instance = &domain.SessionInstance{Instance: "axlr-session-0123", Turns: 3, LastRecord: record, Pending: []string{"{}"}}
	text := sessionPanelContent(state, theme, "")
	for _, want := range []string{"MADE instance axlr-session-0123 · open · 3 turns", "1 memory entry waiting for KMP", "Last turn (#3)", "prompt: crea hola.txt", "answer: Hecho. (changed)", "requests · cost: 2 · $0.0012", "reads: 2", "memory recorded: project:Lab", "local_write hola.txt (create)", "why: create the file", "auto_approve · completed", "deny · denied (not now)"} {
		if !strings.Contains(text, want) {
			t.Fatalf("panel lacks %q:\n%s", want, text)
		}
	}
}
