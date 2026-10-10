package terminal

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type readToolCatalog struct{}

func (readToolCatalog) Snapshot(context.Context) ([]domain.AvailableTool, error) {
	schema, _ := root.NewJSONValue([]byte(`{"type":"object"}`))
	return []domain.AvailableTool{{Definition: root.ToolDefinition{Name: "local_read", Description: "read", Parameters: schema}, Identity: domain.ToolIdentity{Kind: domain.ToolKindLocal, LocalOperation: "read"}}}, nil
}

// overLimitStream answers the first request with more calls than a turn
// may make, and every later one with plain text.
type overLimitStream struct{ requests *int }

func (s overLimitStream) Stream(_ context.Context, _ root.CompletionRequest, emit func(root.Text) error) (root.CompletionResult, error) {
	*s.requests++
	if *s.requests > 1 {
		return root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, Content: "finished after the limit"}}, nil
	}
	args, _ := root.NewJSONObject([]byte(`{}`))
	calls := make([]root.ToolCall, domain.MaxTurnToolCalls+1)
	for i := range calls {
		calls[i] = root.ToolCall{ID: root.ToolCallID(fmt.Sprintf("c%d", i)), Name: "local_read", Arguments: args}
	}
	const text = "I found the cause; reading every file"
	_ = emit(text)
	return root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, Content: text, ToolCalls: calls}}, nil
}

// The turn pauses at its call limit with the answer kept; the footer says,
// in the person's language, that a message or Ctrl+R goes on, and Ctrl+R
// does go on with a new budget.
func TestTheToolCallLimitSaysHowToContinueAndCtrlRContinues(t *testing.T) {
	s := navSession(t)
	m := navModel(t, &s)
	m.Theme.Locale = Spanish
	requests := 0
	continuation := application.ContinueTurnUseCase{Store: m.deps.Store, Models: overLimitStream{&requests}}
	m.deps.Start = application.StartTurnUseCase{Catalog: readToolCatalog{}, Store: m.deps.Store, Continue: continuation, Tools: navTool{}}
	m.deps.Agent = application.AgentTurnUseCase{Continue: continuation, Tools: navTool{}}
	m.Composer.Input.SetValue("find the cause")
	next, cmd := m.Update(ControlIntent("send"))
	m = drain(t, next.(AppModel), cmd)
	if s.Status() != domain.StatusInterrupted || !strings.Contains(m.Transcript.Text(), "I found the cause") {
		t.Fatalf("the turn did not pause with its answer kept: %s", s.Status())
	}
	// Reads need no card, so the calls that fit the budget ran and only the
	// one past it was refused, labelled as over budget rather than denied.
	ran, refused := 0, 0
	for _, msg := range s.Messages() {
		switch {
		case msg.Role != root.RoleTool:
		case strings.HasPrefix(string(msg.Content), domain.OverBudgetOutcomePrefix):
			refused++
		default:
			ran++
		}
	}
	if ran != domain.MaxTurnToolCalls || refused != 1 {
		t.Fatalf("ran %d and refused %d of %d calls", ran, refused, domain.MaxTurnToolCalls+1)
	}
	if text := m.Transcript.Text(); !strings.Contains(text, Translate(Spanish, "transcript.toolOverBudget")) {
		t.Fatalf("the refused call is not labelled over budget:\n%s", text)
	}
	if want := Translate(Spanish, "error.toolCallLimit"); m.Status.Error != want {
		t.Fatalf("footer error %q; want %q", m.Status.Error, want)
	}
	if m.Composer.Input.Value() != "" {
		t.Fatalf("the accepted prompt came back to the composer: %q", m.Composer.Input.Value())
	}
	next, cmd = m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	m = drain(t, next.(AppModel), cmd)
	if requests != 2 || s.Status() != domain.StatusComplete || m.Status.Error != "" || !strings.Contains(m.Transcript.Text(), "finished after the limit") {
		t.Fatalf("Ctrl+R did not continue: requests %d, status %s, error %q", requests, s.Status(), m.Status.Error)
	}
}
