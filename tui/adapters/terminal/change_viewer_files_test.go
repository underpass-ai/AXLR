package terminal

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// Two writes to notas.md and one to config.yaml, in that order, over two turns.
func steppedChanges() domain.SessionState {
	write := func(id, path string, change domain.FileChange) (root.Message, root.Message, domain.PendingTool) {
		call := root.ToolCall{ID: root.ToolCallID(id), Name: "local_write"}
		change.Path = root.RelativePath(path)
		return root.Message{Role: root.RoleAssistant, ToolCalls: []root.ToolCall{call}},
			root.Message{Role: root.RoleTool, ToolCallID: call.ID, Content: "ok"},
			domain.PendingTool{Call: call, Decision: domain.DecisionApprove, Outcome: &domain.ToolOutcome{Change: &change}}
	}
	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	s := domain.SessionState{ID: "0123456789abcdef0123456789abcdef"}
	add := func(prompt string, call, result root.Message, record domain.PendingTool) {
		if prompt != "" {
			s.Messages = append(s.Messages, root.Message{Role: root.RoleUser, Content: root.Text(prompt)})
		}
		s.Messages = append(s.Messages, call, result)
		s.Activity = append(s.Activity, record)
	}
	c1, r1, a1 := write("a", "notas.md", domain.FileChange{Created: true, After: "uno\ndos\n"})
	add("crea", c1, r1, a1)
	c2, r2, a2 := write("b", "notas.md", domain.FileChange{Before: "uno\ndos\n", After: "uno\ndos\ntres\n"})
	add("amplía", c2, r2, a2)
	c3, r3, a3 := write("c", "config.yaml", domain.FileChange{Before: "port: 1\n", After: "port: 2\n"})
	add("", c3, r3, a3)
	for range s.Messages {
		s.MessageTimes = append(s.MessageTimes, at)
	}
	return s
}

func TestChangesGroupByFileWithANetDiffAndExpandableSteps(t *testing.T) {
	v := NewChangeViewer()
	v.Theme = Theme{Monochrome: true}
	v.Open(steppedChanges())
	v.Resize(120, 30)
	if len(v.files) != 2 || v.files[0].Path != "config.yaml" || v.files[1].Path != "notas.md" || len(v.files[1].Steps) != 2 {
		t.Fatalf("files = %+v", v.files)
	}
	net := v.files[1]
	if !net.Net.Created || net.Net.Before != "" || net.Net.After != "uno\ndos\ntres\n" || net.preview.Added != 3 || net.preview.Removed != 0 {
		t.Fatalf("net change = %+v %+v", net.Net, net.preview)
	}
	v.Update(tea.KeyPressMsg{Code: tea.KeyDown}, nil, "")
	if v.position() != "File 2 of 2" {
		t.Fatalf("position = %q", v.position())
	}
	v.Update(tea.KeyPressMsg{Code: tea.KeyRight}, nil, "")
	if len(v.rows) != 4 || v.rows[2] != (changeRow{File: 1, Step: 1}) {
		t.Fatalf("rows after expanding = %+v", v.rows)
	}
	v.Update(tea.KeyPressMsg{Code: tea.KeyDown}, nil, "")
	if v.position() != "File 2 of 2 · step 2 of 2" {
		t.Fatalf("position = %q", v.position())
	}
	header := ansi.Strip(v.detailHeader())
	if !strings.Contains(header, "step 2 of 2 · turn 2") || !strings.Contains(header, "local_write") || !strings.Contains(header, "+1 -0") {
		t.Fatalf("step header = %q", header)
	}
	v.Update(tea.KeyPressMsg{Code: tea.KeyLeft}, nil, "")
	if len(v.rows) != 2 || v.position() != "File 2 of 2" {
		t.Fatalf("left did not collapse back to the file: %+v %q", v.rows, v.position())
	}
}

func TestChangesNameNewFilesAndLineRanges(t *testing.T) {
	created := previewChange(domain.FileChange{Path: "n", Created: true, After: "a\nb\n"})
	if text := created.render(Theme{Monochrome: true}, true); !strings.HasPrefix(text, "new file") {
		t.Fatalf("new file heading: %q", text)
	}
	edited := previewChange(domain.FileChange{Path: "e", Before: "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n", After: "1\n2\n3\n4\n5\nSIX\n7\n8\n9\n10\n"})
	if text := edited.render(Theme{Monochrome: true}, false); !strings.HasPrefix(text, "lines 3–9") {
		t.Fatalf("edit heading: %q", text)
	}
}

func TestChangesAlwaysOpenOnTheList(t *testing.T) {
	m := update(sized(), tea.WindowSizeMsg{Width: 60, Height: 20})
	defer m.zones.Close()
	m.Header.State = steppedChanges()
	m.refreshTranscript()
	m = update(m, ControlIntent("changes"))
	m = update(m, tea.KeyPressMsg{Code: tea.KeyTab})
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEsc})
	m = update(m, ControlIntent("changes"))
	if m.Changes.detailFocus || !strings.Contains(ansi.Strip(m.View().Content), "2 files · 3 changes") {
		t.Fatalf("review reopened on the diff:\n%s", ansi.Strip(m.View().Content))
	}
}
