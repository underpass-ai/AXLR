package application

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
	"time"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type exportedFiles map[string]string

func (e exportedFiles) Create(_ context.Context, path string, data []byte) error {
	e[path] = string(data)
	return nil
}

// /export writes the session as Markdown: title, model and date, each
// prompt and reply, a row per tool call and the console lines, and the
// cost the ledger knows, to <workspace>/.axlr/exports by default.
func TestExportSessionWritesMarkdown(t *testing.T) {
	s := changedSession(t, []changeStep{{"w1", domain.FileChange{Path: "a.go", Before: "one", After: "two"}}})
	if err := s.AddRestore(domain.FileRestore{Files: []domain.RestoredFile{{Path: "a.go", From: "w1", To: "w1"}}}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddTurnFailure(domain.TurnFailure{Message: "OpenRouter provider_failure (HTTP 502)"}); err != nil {
		t.Fatal(err)
	}
	state := s.Export()
	files := exportedFiles{}
	ledgers := usageLedgers{state.ID: {Requests: 3, PromptTokens: 9000, CompletionTokens: 400, Cost: 0.0147, CostRequests: 2, Unmetered: 1}}
	labels := &contextLabels{labels: map[domain.SessionID]domain.SessionLabel{state.ID: {Title: "Añade IsDev() a buildinfo"}}}
	now := time.Date(2026, 10, 12, 18, 30, 0, 0, time.Local)
	u := ExportSessionUseCase{Files: files, Usage: ledgers, Labels: labels, Now: func() time.Time { return now }}
	path, err := u.Execute(context.Background(), state, "")
	want := filepath.Join(string(state.Workspace), ".axlr", "exports", "anade-isdev-a-buildinfo-20261012-1830.md")
	if err != nil || path != want {
		t.Fatalf("path %q %v, want %q", path, err, want)
	}
	text := files[path]
	for _, part := range []string{
		"# Añade IsDev() a buildinfo\n",
		"- Model: `test/model`",
		"- Exported: 2026-10-12 18:30",
		"- Cost: $0.0147 for 2 of 3 requests",
		"- Unmetered: 1 cancelled or failed requests",
		"## You",
		"turn 1",
		"| `local_write` |  | ok | 2 B |",
		"## Assistant",
		"done",
		"> Restored `a.go` (to before w1)",
		"> Failed: OpenRouter provider_failure (HTTP 502)",
	} {
		if !strings.Contains(text, part) {
			t.Fatalf("export lacks %q:\n%s", part, text)
		}
	}
	// A relative path is the workspace's; nothing to export is refused.
	if path, err := u.Execute(context.Background(), state, "notes/out.md"); err != nil || path != filepath.Join(string(state.Workspace), "notes", "out.md") {
		t.Fatalf("relative path %q %v", path, err)
	}
	if _, err := u.Execute(context.Background(), turnSession(t).Export(), ""); err == nil {
		t.Fatal("exported an empty session")
	}
	if got := exportSlug("¿?"); got != "session" {
		t.Fatalf("slug %q", got)
	}
	call := root.ToolCall{ID: "x", Name: "local_exec", Arguments: mustObject(t, `{"program":"sh","args":["-c","a | b"]}`)}
	if got := tableCell(shortArguments(call.Arguments)); !strings.Contains(got, `a \| b`) {
		t.Fatalf("cell %q", got)
	}
}

// exclusiveFiles refuses an existing path as the storage port does.
type exclusiveFiles map[string]string

func (e exclusiveFiles) Create(_ context.Context, path string, data []byte) error {
	if _, ok := e[path]; ok {
		return fs.ErrExist
	}
	e[path] = string(data)
	return nil
}

// Two /export within one minute both write: the default name takes a
// suffix, while a path the person named is never replaced.
func TestASecondDefaultExportInTheSameMinuteTakesASuffix(t *testing.T) {
	s := changedSession(t, []changeStep{{"w1", domain.FileChange{Path: "a.go", Before: "one", After: "two"}}})
	state := s.Export()
	files := exclusiveFiles{}
	now := time.Date(2026, 10, 12, 18, 30, 0, 0, time.Local)
	u := ExportSessionUseCase{Files: files, Now: func() time.Time { return now }}
	first, err := u.Execute(context.Background(), state, "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := u.Execute(context.Background(), state, "")
	if err != nil || second != strings.TrimSuffix(first, ".md")+"-2.md" {
		t.Fatalf("second export %q %v after %q", second, err, first)
	}
	if _, err := u.Execute(context.Background(), state, first); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("a named path was replaced: %v", err)
	}
}
