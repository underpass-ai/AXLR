package application

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// workspaceFiles plays the local runtime's read and write over a map.
type workspaceFiles struct {
	files  map[string]string
	writes int
}

func (w *workspaceFiles) Execute(_ context.Context, id domain.ToolIdentity, args root.JSONValue) (domain.ToolOutcome, error) {
	var in struct {
		Path, Content, Mode string
		Expected            string `json:"expected_sha256"`
	}
	_ = json.Unmarshal(args.Bytes(), &in)
	reply := func(status string, output any, code string) (domain.ToolOutcome, error) {
		response := map[string]any{"status": status, "output": output}
		if code != "" {
			response["error"] = map[string]string{"code": code, "message": code + " " + in.Path}
		}
		data, _ := json.Marshal(response)
		return domain.ToolOutcome{Content: root.Text(data), IsError: status != "completed"}, nil
	}
	content, exists := w.files[in.Path]
	switch id.LocalOperation {
	case "read":
		if !exists {
			return reply("failed", nil, "not_found")
		}
		return reply("completed", map[string]any{"content": content, "truncated": false, "content_sha256": string(root.DigestOf([]byte(content)))}, "")
	case "write":
		if in.Mode != "replace" || !exists || string(root.DigestOf([]byte(content))) != in.Expected {
			return reply("rejected", nil, "conflict")
		}
		w.files[in.Path] = in.Content
		w.writes++
		return reply("completed", map[string]any{"content_sha256": string(root.DigestOf([]byte(in.Content)))}, "")
	}
	return reply("rejected", nil, "unsupported")
}

type changeStep struct {
	id     string
	change domain.FileChange
}

// changedSession is a session whose turns wrote the given changes.
func changedSession(t *testing.T, turns ...[]changeStep) domain.Session {
	t.Helper()
	s := turnSession(t)
	for n, turn := range turns {
		if err := s.BeginTurn(root.Text("turn "+string(rune('1'+n))), repairTools(t)); err != nil {
			t.Fatal(err)
		}
		var calls []root.ToolCall
		for _, c := range turn {
			calls = append(calls, root.ToolCall{ID: root.ToolCallID(c.id), Name: "local_write", Arguments: mustObject(t, `{}`)})
		}
		if err := s.CompleteAssistant(assistant("", calls...)); err != nil {
			t.Fatal(err)
		}
		for _, c := range turn {
			change := c.change
			if err := s.RecordToolOutcome(root.ToolCallID(c.id), domain.DecisionApprove, domain.ToolOutcome{Content: "{}", Change: &change}); err != nil {
				t.Fatal(err)
			}
		}
		if err := s.CompleteAssistant(assistant("done")); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

// Ctrl+D puts one file back to before a step, through the local write, and
// records it; a file changed since the step is never overwritten.
func TestRestoreFilePutsAStepsFileBack(t *testing.T) {
	s := changedSession(t, []changeStep{{"w1", domain.FileChange{Path: "a.go", Before: "one", After: "two"}}, {"w2", domain.FileChange{Path: "b.go", Created: true, After: "new"}}})
	files := &workspaceFiles{files: map[string]string{"a.go": "two", "b.go": "new"}}
	store := &memoryStore{}
	u := RestoreFileUseCase{Tools: files, Store: store, Workspace: s.Export().Workspace}
	restored, err := u.Restore(context.Background(), &s, "w1")
	if err != nil || restored != (domain.RestoredFile{Path: "a.go", From: "w1", To: "w1"}) || files.files["a.go"] != "one" {
		t.Fatalf("restore %+v %v, file %q", restored, err, files.files["a.go"])
	}
	lines := s.ConsoleLines()
	if len(lines) != 1 || lines[0].Restore == nil || len(store.states) != 1 {
		t.Fatalf("restore not recorded: %+v", lines)
	}
	if change, ok := s.Export().RestoreChange(lines[0].Restore.Files[0]); !ok || change.Before != "two" || change.After != "one" {
		t.Fatalf("restore change %+v", change)
	}
	// The file now holds "one", not what w1 wrote: a second restore refuses.
	if _, err := u.Restore(context.Background(), &s, "w1"); !errors.As(err, new(FileChangedError)) || !strings.Contains(err.Error(), "changed after the session wrote it") {
		t.Fatalf("restored a changed file: %v", err)
	}
	if _, err := u.Restore(context.Background(), &s, "w2"); err == nil || !strings.Contains(err.Error(), "created it") {
		t.Fatalf("restored a created file: %v", err)
	}
	if _, err := u.Restore(context.Background(), &s, "nope"); err == nil {
		t.Fatal("restored an unknown step")
	}
	other := u
	other.Workspace = "/elsewhere"
	if _, err := other.Restore(context.Background(), &s, "w1"); err == nil {
		t.Fatal("restored a session of another workspace")
	}
	if files.writes != 1 {
		t.Fatalf("writes %d", files.writes)
	}
}

// A truncated or unavailable snapshot is not restored.
func TestRestoreFileRefusesASnapshotThatWasNotKept(t *testing.T) {
	s := changedSession(t, []changeStep{{"w1", domain.FileChange{Path: "big.go", Unavailable: "too_large"}}})
	u := RestoreFileUseCase{Tools: &workspaceFiles{files: map[string]string{"big.go": "x"}}, Store: &memoryStore{}}
	if _, err := u.Restore(context.Background(), &s, "w1"); err == nil || !strings.Contains(err.Error(), "too_large") {
		t.Fatalf("restored a snapshot that was not kept: %v", err)
	}
}

// Undo puts back every file the last turn changed, to before its first step
// on each file, and writes nothing unless every file still holds what the
// turn wrote.
func TestUndoLastTurnIsAllOrNothing(t *testing.T) {
	earlier := []changeStep{{"e1", domain.FileChange{Path: "a.go", Before: "zero", After: "one"}}}
	last := []changeStep{
		{"l1", domain.FileChange{Path: "a.go", Before: "one", After: "two"}},
		{"l2", domain.FileChange{Path: "b.go", Before: "b0", After: "b1"}},
		{"l3", domain.FileChange{Path: "a.go", Before: "two", After: "three"}},
	}
	s := changedSession(t, earlier, last)
	files := &workspaceFiles{files: map[string]string{"a.go": "three", "b.go": "changed by hand"}}
	u := RestoreFileUseCase{Tools: files, Store: &memoryStore{}}
	if _, err := u.UndoLastTurn(context.Background(), &s); err == nil || !strings.Contains(err.Error(), "b.go") || files.writes != 0 || files.files["a.go"] != "three" {
		t.Fatalf("undo over a changed file: %v, writes %d", err, files.writes)
	}
	files.files["b.go"] = "b1"
	restored, err := u.UndoLastTurn(context.Background(), &s)
	if err != nil || len(restored) != 2 || files.files["a.go"] != "one" || files.files["b.go"] != "b0" {
		t.Fatalf("undo %+v %v, files %v", restored, err, files.files)
	}
	if restored[0] != (domain.RestoredFile{Path: "a.go", From: "l3", To: "l1"}) || restored[1] != (domain.RestoredFile{Path: "b.go", From: "l2", To: "l2"}) {
		t.Fatalf("restored %+v", restored)
	}
	lines := s.ConsoleLines()
	if len(lines) != 1 || !lines[0].Restore.Undo {
		t.Fatalf("undo not recorded: %+v", lines)
	}
	// The files no longer hold what the turn wrote: a second undo refuses.
	if _, err := u.UndoLastTurn(context.Background(), &s); err == nil {
		t.Fatal("undid twice")
	}
	// A turn that created a file is not undone, whatever else it did.
	created := changedSession(t, []changeStep{{"c1", domain.FileChange{Path: "c.go", Created: true, After: "c"}}, {"c2", domain.FileChange{Path: "a.go", Before: "x", After: "y"}}})
	files = &workspaceFiles{files: map[string]string{"c.go": "c", "a.go": "y"}}
	u.Tools = files
	if _, err := u.UndoLastTurn(context.Background(), &created); err == nil || !strings.Contains(err.Error(), "created c.go") || files.writes != 0 {
		t.Fatalf("undid a created file: %v", err)
	}
}

// A restore is told to the model once, on the next prompt, as a console
// note: the history stays as it was, and the model no longer takes the
// restored file for someone else's change (14 October 2026: glm-5.3-flash
// found calc/calc.go "reverted by someone" and spent a 382k-token debug
// ceremony on it).
func TestTheNextPromptTellsTheModelWhatThePersonRestored(t *testing.T) {
	s := changedSession(t, []changeStep{{"w1", domain.FileChange{Path: "calc/calc.go", Before: "one", After: "two"}}}, []changeStep{{"l1", domain.FileChange{Path: "a.go", Before: "a0", After: "a1"}}, {"l2", domain.FileChange{Path: "b.go", Before: "b0", After: "b1"}}})
	files := &workspaceFiles{files: map[string]string{"calc/calc.go": "two", "a.go": "a1", "b.go": "b1"}}
	store := &memoryStore{}
	restore := RestoreFileUseCase{Tools: files, Store: store}
	if _, err := restore.Restore(context.Background(), &s, "w1"); err != nil {
		t.Fatal(err)
	}
	if _, err := restore.UndoLastTurn(context.Background(), &s); err != nil {
		t.Fatal(err)
	}
	var prompts []string
	model := streamFunc(func(_ context.Context, r root.CompletionRequest, _ func(root.Text) error) (root.CompletionResult, error) {
		prompts = append(prompts, string(r.Messages[len(r.Messages)-1].Content))
		return assistant("ok"), nil
	})
	u := StartTurnUseCase{Catalog: &catalogStub{}, Store: store, Continue: ContinueTurnUseCase{Models: model, Store: store}}
	for _, prompt := range []root.Text{"sigue", "y ahora"} {
		if err := u.Execute(context.Background(), &s, prompt, ignoreEvent); err != nil {
			t.Fatal(err)
		}
	}
	want := "sigue\n\n" + RestoreNotePrefix + " Since your last answer the person restored calc/calc.go to what it held before your call w1 (from the file-changes panel); and b.go, a.go to what they held before your last turn (with /undo). That was the person's decision, not a fault"
	if len(prompts) != 2 || !strings.HasPrefix(prompts[0], want) {
		t.Fatalf("first prompt %q", prompts[0])
	}
	if prompts[1] != "y ahora" {
		t.Fatalf("the note was repeated: %q", prompts[1])
	}
}

// /undo reaches the most recent turn that changed files, which need not be
// the last one; the note said "before your last turn" whatever turn it
// undid. It now names the turn and the person's prompt that began it.
func TestTheRestoreNoteNamesTheTurnAnUndoReached(t *testing.T) {
	s := changedSession(t, []changeStep{{"w1", domain.FileChange{Path: "calc/calc.go", Before: "one", After: "two"}}}, []changeStep{{"w2", domain.FileChange{Path: "b.go", Before: "b0", After: "b1"}}})
	files := &workspaceFiles{files: map[string]string{"calc/calc.go": "two", "b.go": "b1"}}
	store := &memoryStore{}
	restore := RestoreFileUseCase{Tools: files, Store: store}
	if _, err := restore.UndoTurn(context.Background(), &s, 1); err != nil {
		t.Fatal(err)
	}
	note := restoreNote(s.Export())
	if !strings.Contains(note, `calc/calc.go to what it held before your turn 1 of 2, not your last, the one the person began with "turn 1" (with /undo)`) || strings.Contains(note, "your last turn") {
		t.Fatalf("note %q", note)
	}
}

// Confirmed live on 14 October 2026: a turn wrote calc.go and
// calc_test.go, the person restored calc_test.go from Ctrl+D, and /undo
// offered the turn, found calc_test.go "changed after the session wrote
// it" and undid nothing, calc.go included. A change restored from the
// panel counts as undone: the undo puts back what remains.
func TestUndoSkipsAChangeRestoredFromThePanel(t *testing.T) {
	s := changedSession(t, []changeStep{
		{"w1", domain.FileChange{Path: "calc/calc.go", Before: "calc0", After: "calc1"}},
		{"w2", domain.FileChange{Path: "calc/calc_test.go", Before: "test0", After: "test1"}},
	})
	// A later turn ran only reads: the session's last turn changed no file.
	if err := s.BeginTurn("¿pasan los tests?", repairTools(t)); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteAssistant(assistant("sí")); err != nil {
		t.Fatal(err)
	}
	files := &workspaceFiles{files: map[string]string{"calc/calc.go": "calc1", "calc/calc_test.go": "test1"}}
	u := RestoreFileUseCase{Tools: files, Store: &memoryStore{}}
	if _, err := u.Restore(context.Background(), &s, "w2"); err != nil {
		t.Fatal(err)
	}
	// /undo offers turn 1, without the restored test file.
	state := s.Export()
	if turn, changes := state.LatestChangingTurn(), state.TurnChanges(1); turn != 1 || len(changes) != 1 || changes[0].Call != "w1" {
		t.Fatalf("turn %d changes %+v", turn, changes)
	}
	restored, err := u.UndoTurn(context.Background(), &s, 1)
	if err != nil || len(restored) != 1 || restored[0] != (domain.RestoredFile{Path: "calc/calc.go", From: "w1", To: "w1"}) {
		t.Fatalf("undo %+v %v", restored, err)
	}
	if files.files["calc/calc.go"] != "calc0" || files.files["calc/calc_test.go"] != "test0" {
		t.Fatalf("files %v", files.files)
	}
	// Everything turn 1 changed is back: it is not offered again.
	if turn := s.Export().LatestChangingTurn(); turn == 1 {
		t.Fatal("turn 1 offered again")
	}
	// A turn whose every change the panel restored has nothing to undo.
	only := changedSession(t, []changeStep{{"o1", domain.FileChange{Path: "a.go", Before: "a0", After: "a1"}}})
	files = &workspaceFiles{files: map[string]string{"a.go": "a1"}}
	u.Tools = files
	if _, err := u.Restore(context.Background(), &only, "o1"); err != nil {
		t.Fatal(err)
	}
	if turn := only.Export().LatestChangingTurn(); turn != 0 || len(only.Export().LastTurnChanges()) != 0 {
		t.Fatalf("restored turn still offered: %d", turn)
	}
}

// A restore made while the turn is interrupted reached the model only with
// a new prompt: Ctrl+R resumed without one and the model went on over
// files that no longer held what it wrote. The resumed turn's next request
// carries the note once, as a console note that joins the turn, and the
// saved session replays.
func TestAResumedTurnIsToldWhatThePersonRestored(t *testing.T) {
	s := turnSession(t)
	if err := s.BeginTurn("cambia calc", repairTools(t)); err != nil {
		t.Fatal(err)
	}
	call := root.ToolCall{ID: "w1", Name: "local_write", Arguments: mustObject(t, `{}`)}
	if err := s.CompleteAssistant(assistant("", call)); err != nil {
		t.Fatal(err)
	}
	change := domain.FileChange{Path: "calc/calc.go", Before: "one", After: "two"}
	if err := s.RecordToolOutcome("w1", domain.DecisionApprove, domain.ToolOutcome{Content: "{}", Change: &change}); err != nil {
		t.Fatal(err)
	}
	if err := s.PauseTurn(); err != nil {
		t.Fatal(err)
	}
	store := &memoryStore{}
	files := &workspaceFiles{files: map[string]string{"calc/calc.go": "two"}}
	if _, err := (RestoreFileUseCase{Tools: files, Store: store}).Restore(context.Background(), &s, "w1"); err != nil {
		t.Fatal(err)
	}
	var requests [][]root.Message
	model := streamFunc(func(_ context.Context, r root.CompletionRequest, _ func(root.Text) error) (root.CompletionResult, error) {
		requests = append(requests, r.Messages)
		return assistant("listo"), nil
	})
	u := SessionControlUseCase{Store: store, Agent: AgentTurnUseCase{Continue: ContinueTurnUseCase{Models: model, Store: store}}}
	if err := u.Resume(context.Background(), &s, ignoreEvent); err != nil {
		t.Fatal(err)
	}
	if len(requests) != 1 {
		t.Fatalf("%d requests", len(requests))
	}
	last := requests[0][len(requests[0])-1]
	if last.Role != root.RoleUser || !strings.HasPrefix(string(last.Content), RestoreNotePrefix+" Since your last answer the person restored calc/calc.go to what it held before your call w1") {
		t.Fatalf("last message %+v", last)
	}
	if note := restoreNote(s.Export()); note != "" {
		t.Fatalf("the note is still pending: %q", note)
	}
	if _, err := domain.RestoreSession(s.Export()); err != nil {
		t.Fatalf("the saved session does not replay: %v", err)
	}
	if s.Export().TurnCallCount != 1 {
		t.Fatalf("the note began a turn: count %d", s.Export().TurnCallCount)
	}
}
