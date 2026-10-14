package terminal

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// fakeFiles plays the local runtime's read and write over a map.
type fakeFiles map[string]string

func (f fakeFiles) Execute(_ context.Context, id domain.ToolIdentity, args root.JSONValue) (domain.ToolOutcome, error) {
	var in struct {
		Path, Content string
		Expected      string `json:"expected_sha256"`
	}
	_ = json.Unmarshal(args.Bytes(), &in)
	content := f[in.Path]
	output := map[string]any{"content": content, "content_sha256": string(root.DigestOf([]byte(content)))}
	if id.LocalOperation == "write" {
		if in.Expected != string(root.DigestOf([]byte(content))) {
			data, _ := json.Marshal(map[string]any{"status": "rejected", "error": map[string]string{"code": "conflict", "message": "changed"}})
			return domain.ToolOutcome{Content: root.Text(data), IsError: true}, nil
		}
		f[in.Path] = in.Content
	}
	data, _ := json.Marshal(map[string]any{"status": "completed", "output": output})
	return domain.ToolOutcome{Content: root.Text(data)}, nil
}

// restoreModel has a session whose one turn edited a.go twice and wrote
// b.go, with the files on "disk" as the turn left them.
func restoreModel(t *testing.T) (AppModel, fakeFiles) {
	t.Helper()
	s := navSession(t)
	schema, _ := root.NewJSONValue([]byte(`{"type":"object"}`))
	tools := []domain.AvailableTool{{Definition: root.ToolDefinition{Name: "local_write", Description: "write", Parameters: schema}, Identity: domain.ToolIdentity{Kind: domain.ToolKindLocal, LocalOperation: "write"}}}
	steps := []struct {
		id     root.ToolCallID
		change domain.FileChange
	}{
		{"w1", domain.FileChange{Path: "a.go", Before: "one\n", After: "two\n"}},
		{"w2", domain.FileChange{Path: "a.go", Before: "two\n", After: "three\n"}},
		{"w3", domain.FileChange{Path: "b.go", Before: "x\n", After: "y\n"}},
	}
	if err := s.BeginTurn("edit them", tools); err != nil {
		t.Fatal(err)
	}
	args, _ := root.NewJSONValue([]byte(`{"path":"a.go","content":"x"}`))
	var calls []root.ToolCall
	for _, step := range steps {
		calls = append(calls, root.ToolCall{ID: step.id, Name: "local_write", Arguments: args})
	}
	if err := s.CompleteAssistant(root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, ToolCalls: calls}}); err != nil {
		t.Fatal(err)
	}
	for _, step := range steps {
		change := step.change
		if err := s.RecordToolOutcome(step.id, domain.DecisionApprove, domain.ToolOutcome{Content: "{}", Change: &change}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.CompleteAssistant(root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, Content: "done"}}); err != nil {
		t.Fatal(err)
	}
	m := navModel(t, &s)
	files := fakeFiles{"a.go": "three\n", "b.go": "y\n"}
	m.deps.Start = application.StartTurnUseCase{Store: m.deps.Store, Tools: files}
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 30})
	return m, files
}

func typed(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r, Text: string(r)} }

// u in Ctrl+D asks, then y puts the selected step's file back to what it
// held before the step; the restore shows in the transcript and as a step
// of its own in Ctrl+D. A file with several steps asks for one.
func TestUInChangesRestoresTheSelectedStep(t *testing.T) {
	m, files := restoreModel(t)
	defer m.zones.Close()
	m = update(m, ControlIntent("changes"))
	// The newest file first, b.go; then a.go with two steps.
	m = update(m, tea.KeyPressMsg{Code: tea.KeyDown})
	m = update(m, typed('u'))
	if m.Changes.confirm != "" || !strings.Contains(ansi.Strip(m.View().Content), "several steps") {
		t.Fatalf("u on a file of two steps did not ask for a step:\n%s", ansi.Strip(m.View().Content))
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyRight})
	m = update(m, tea.KeyPressMsg{Code: tea.KeyDown})
	m = update(m, typed('u'))
	if m.Changes.confirm != "w2" || !strings.Contains(ansi.Strip(m.View().Content), "Put a.go back to before step 2? y restores") {
		t.Fatalf("no confirmation for w2:\n%s", ansi.Strip(m.View().Content))
	}
	m = update(m, typed('n'))
	if m.Changes.confirm != "" || m.Busy || files["a.go"] != "three\n" {
		t.Fatal("another key restored")
	}
	m = update(m, typed('u'))
	next, cmd := m.Update(typed('y'))
	m = drain(t, next.(AppModel), cmd)
	if files["a.go"] != "two\n" || m.Status.Error != "" {
		t.Fatalf("a.go %q, error %q", files["a.go"], m.Status.Error)
	}
	if !strings.Contains(m.Status.Notice, "restored a.go") || !strings.Contains(m.Transcript.Text(), "restored a.go to before the selected step") {
		t.Fatalf("notice %q, transcript:\n%s", m.Status.Notice, m.Transcript.Text())
	}
	if m.overlay != "changes" || len(m.Changes.records) != 4 || !m.Changes.records[3].Restore || m.Changes.records[3].Change.After != "two\n" {
		t.Fatalf("records %+v", m.Changes.records)
	}
	// A file changed since is refused, with the reason.
	files["b.go"] = "edited by hand\n"
	m.Changes.Open(m.Header.State)
	for m.Changes.files[m.Changes.rows[m.Changes.selected].File].Path != "b.go" {
		m = update(m, tea.KeyPressMsg{Code: tea.KeyDown})
	}
	m = update(m, typed('u'))
	next, cmd = m.Update(typed('y'))
	m = drain(t, next.(AppModel), cmd)
	if files["b.go"] != "edited by hand\n" || m.Status.Error != "Not restored: b.go changed after the session wrote it, and nothing was put back" {
		t.Fatalf("b.go %q, error %q", files["b.go"], m.Status.Error)
	}
}

// /undo asks, then puts back every file the last turn changed.
func TestUndoPutsBackTheLastTurn(t *testing.T) {
	m, files := restoreModel(t)
	defer m.zones.Close()
	m.Composer.Input.SetValue("/undo")
	m = update(m, ControlIntent("send"))
	if !m.undoAsking || !strings.Contains(m.Status.Notice, "Undo the last turn's change to b.go, a.go? y restores") {
		t.Fatalf("notice %q", m.Status.Notice)
	}
	m = update(m, typed('x'))
	if m.undoAsking || m.Busy || files["a.go"] != "three\n" || m.Status.Notice != "Nothing undone" {
		t.Fatal("another key undid")
	}
	m.Composer.Input.SetValue("/deshacer")
	m = update(m, ControlIntent("send"))
	next, cmd := m.Update(typed('y'))
	m = drain(t, next.(AppModel), cmd)
	if files["a.go"] != "one\n" || files["b.go"] != "x\n" || !strings.Contains(m.Transcript.Text(), "undid the last turn: restored b.go, a.go") {
		t.Fatalf("files %v, error %q", files, m.Status.Error)
	}
	m.Composer.Input.SetValue("/undo")
	m = update(m, ControlIntent("send"))
	if m.undoAsking || m.Status.Notice != "The last turn is already undone" {
		t.Fatalf("a second /undo: %q", m.Status.Notice)
	}
}

// On 14 October 2026 /undo after a turn that only read said "The last turn
// changed no file": it now offers the most recent turn that changed files,
// with the same y, and an undone turn is not offered again.
func TestUndoOffersTheLatestTurnThatChangedFiles(t *testing.T) {
	m, files := restoreModel(t)
	defer m.zones.Close()
	if err := m.deps.Session.BeginTurn("just read", nil); err != nil {
		t.Fatal(err)
	}
	if err := m.deps.Session.CompleteAssistant(root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, Content: "read"}}); err != nil {
		t.Fatal(err)
	}
	m.Header.State = m.deps.Session.Export()
	m.Composer.Input.SetValue("/undo")
	m = update(m, ControlIntent("send"))
	if !m.undoAsking || !strings.Contains(m.Status.Notice, "The last turn changed no file: undo turn 1's changes to b.go, a.go? y restores") {
		t.Fatalf("notice %q", m.Status.Notice)
	}
	next, cmd := m.Update(typed('y'))
	m = drain(t, next.(AppModel), cmd)
	if files["a.go"] != "one\n" || files["b.go"] != "x\n" || !strings.Contains(m.Transcript.Text(), "undid turn 1: restored b.go, a.go") || !strings.Contains(m.Status.Notice, "undid turn 1") {
		t.Fatalf("files %v, error %q, notice %q", files, m.Status.Error, m.Status.Notice)
	}
	if turn := m.Header.State.LatestChangingTurn(); turn != 0 {
		t.Fatalf("an undone turn is offered again: %d", turn)
	}
}

// absentFiles plays a workspace without the session's files, as one a
// session was imported into.
type absentFiles struct{}

func (absentFiles) Execute(context.Context, domain.ToolIdentity, root.JSONValue) (domain.ToolOutcome, error) {
	data, _ := json.Marshal(map[string]any{"status": "failed", "error": map[string]string{"code": "not_found", "message": "file or directory not found"}})
	return domain.ToolOutcome{Content: root.Text(data), IsError: true}, nil
}

// On 14 October 2026 a restore of a file the workspace did not have read
// "! cannot restore calc/calc.go: local_read not_found: file or directory
// not found" in a Spanish console.
func TestRestoreOfAMissingFileIsRefusedInThePersonsLanguage(t *testing.T) {
	m, _ := restoreModel(t)
	defer m.zones.Close()
	m.Theme.Locale, m.Changes.Theme.Locale = Spanish, Spanish
	m.deps.Start.Tools = absentFiles{}
	m = update(m, ControlIntent("changes"))
	m = update(m, typed('u'))
	next, cmd := m.Update(typed('y'))
	m = drain(t, next.(AppModel), cmd)
	if m.Status.Error != "No se ha restaurado: b.go no está en este espacio de trabajo (se borró después, o cambió en otra carpeta), y no se ha devuelto nada" {
		t.Fatalf("error %q", m.Status.Error)
	}
}
