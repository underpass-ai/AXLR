package application

import (
	"context"
	"encoding/json"
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
	if _, err := u.Restore(context.Background(), &s, "w1"); err == nil || !strings.Contains(err.Error(), "changed after step w1") {
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
