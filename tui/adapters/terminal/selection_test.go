package terminal

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	root "github.com/underpass-ai/AXLR/domain"
)

// conversation renders a user question and a two-paragraph reply at 100×30
// and monochrome. The body starts at row 1 under the header and the
// transcript keeps a gutter of two cells, so "alpha beta" sits at row 3 from
// column 2 and "gamma delta" at row 5.
func conversation(t *testing.T) AppModel {
	t.Helper()
	m := sized()
	m.Header.State.Messages = append(m.Header.State.Messages, root.Message{Role: root.RoleUser, Content: "question"}, root.Message{Role: root.RoleAssistant, Content: "alpha beta\n\ngamma delta"})
	m.refreshTranscript()
	lines := strings.Split(m.View().Content, "\n")
	if !strings.HasPrefix(lines[3], "  alpha beta") || !strings.HasPrefix(lines[5], "  gamma delta") {
		t.Fatalf("unexpected layout:\n%s", m.View().Content)
	}
	return m
}

// copied asserts the command puts want on the system clipboard. The batch
// also mirrors to the primary selection and the host tools; only the OSC 52
// message is comparable, and the host fallback is never run here.
func copied(t *testing.T, cmd tea.Cmd, want string) {
	t.Helper()
	if cmd == nil {
		t.Fatal("nothing was copied")
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok || len(batch) < 2 {
		t.Fatalf("copy produced %T", cmd())
	}
	if got := batch[0](); got != tea.SetClipboard(want)() {
		t.Fatalf("clipboard got %v, want %q", got, want)
	}
}

func press(m AppModel, x, y int, mod tea.KeyMod) AppModel {
	return update(m, tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft, Mod: mod})
}

func release(m AppModel, x, y int) (AppModel, tea.Cmd) {
	next, cmd := m.Update(tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft})
	return next.(AppModel), cmd
}

func TestTranscriptDragCopiesSelectedText(t *testing.T) {
	m := conversation(t)
	m = press(m, 2, 3, 0)
	if !m.Transcript.Selection.Active {
		t.Fatal("press did not start a selection")
	}
	m = update(m, tea.MouseMotionMsg{X: 6, Y: 5, Button: tea.MouseLeft})
	m, cmd := release(m, 6, 5)
	copied(t, cmd, "alpha beta\n\ngamma")
	if m.Transcript.Selection.Active || !m.Transcript.Selection.Set {
		t.Fatal("release did not keep the finished selection")
	}
	if !strings.Contains(m.View().Content, "Copied 3 lines") {
		t.Fatalf("no confirmation:\n%s", m.View().Content)
	}
	m = update(m, tea.KeyPressMsg{Code: 'x', Text: "x"})
	if strings.Contains(m.View().Content, "Copied 3 lines") {
		t.Fatal("confirmation outlived the next key")
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.Transcript.Selection.Set {
		t.Fatal("Esc did not clear the selection")
	}
}

func TestTranscriptClickWithoutDragIsNotASelection(t *testing.T) {
	m := conversation(t)
	m = press(m, 2, 3, 0)
	m, cmd := release(m, 2, 3)
	if cmd != nil || m.Transcript.Selection.Set || m.Status.Notice != "" {
		t.Fatal("a plain click copied something")
	}
	m = press(m, 2, 29, 0)
	if m.Transcript.Selection.Set {
		t.Fatal("a click outside the conversation started a selection")
	}
}

func TestTranscriptShiftClickExtendsSelection(t *testing.T) {
	m := conversation(t)
	m = press(m, 2, 3, 0)
	m = update(m, tea.MouseMotionMsg{X: 6, Y: 3, Button: tea.MouseLeft})
	m, cmd := release(m, 6, 3)
	copied(t, cmd, "alpha")
	m = press(m, 12, 5, tea.ModShift)
	m, cmd = release(m, 12, 5)
	copied(t, cmd, "alpha beta\n\ngamma delta")
}

func TestTranscriptDoubleClickSelectsWordAndTripleClickTheRow(t *testing.T) {
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	selectionNow = func() time.Time { return now }
	defer func() { selectionNow = time.Now }()
	m := conversation(t)
	m = press(m, 9, 3, 0)
	m, cmd := release(m, 9, 3)
	if cmd != nil {
		t.Fatal("the first click copied")
	}
	now = now.Add(100 * time.Millisecond)
	m = press(m, 9, 3, 0)
	m, cmd = release(m, 9, 3)
	copied(t, cmd, "beta")
	now = now.Add(100 * time.Millisecond)
	m = press(m, 9, 3, 0)
	_, cmd = release(m, 9, 3)
	copied(t, cmd, "alpha beta")
	now = now.Add(time.Second)
	m = press(m, 9, 3, 0)
	_, cmd = release(m, 9, 3)
	if cmd != nil {
		t.Fatal("a late click counted as part of the gesture")
	}
}

func TestTranscriptDragReleasedOutsideBodyStillCopies(t *testing.T) {
	m := conversation(t)
	m = press(m, 2, 3, 0)
	m = update(m, tea.MouseMotionMsg{X: 50, Y: 29, Button: tea.MouseLeft})
	_, cmd := release(m, 50, 29)
	copied(t, cmd, "alpha beta\n\ngamma delta")
}

func TestSelectionTextAndCellsHonourWideCharacters(t *testing.T) {
	lines := []string{"héllo wörld", "日本語 text"}
	s := Selection{Anchor: textPoint{Line: 0, Col: 6}, Head: textPoint{Line: 1, Col: 3}, Set: true}
	if got := s.Text(lines); got != "wörld\n日本" {
		t.Fatalf("got %q", got)
	}
	if left, right, ok := s.cellRange(1, 11); !ok || left != 0 || right != 4 {
		t.Fatalf("cells %d-%d %v", left, right, ok)
	}
	if _, _, ok := s.cellRange(2, 11); ok {
		t.Fatal("line outside the selection reported cells")
	}
	if left, right := wordAt("alpha beta-gamma", 8); left != 6 || right != 15 {
		t.Fatalf("word %d-%d", left, right)
	}
	if left, right := wordAt("alpha beta", 5); left != 5 || right != 5 {
		t.Fatalf("blank %d-%d", left, right)
	}
}

func TestCopyCommandCopiesLastReplyAsWritten(t *testing.T) {
	m := conversation(t)
	for _, command := range []string{"/copy", "/copiar"} {
		m.Composer.Input.SetValue(command)
		next, cmd := m.Update(ControlIntent("send"))
		m = next.(AppModel)
		copied(t, cmd, "alpha beta\n\ngamma delta")
		if m.Composer.Input.Value() != "" || !strings.Contains(m.View().Content, "Copied 3 lines") {
			t.Fatalf("%s left %q:\n%s", command, m.Composer.Input.Value(), m.View().Content)
		}
	}
	empty := sized()
	empty.Composer.Input.SetValue("/copy")
	next, cmd := empty.Update(ControlIntent("send"))
	if empty = next.(AppModel); cmd != nil || empty.Status.Error != empty.Theme.T("error.nothingToCopy") {
		t.Fatalf("empty session: cmd=%v error=%q", cmd != nil, empty.Status.Error)
	}
}

func TestCopyCommandPrefersTheSelection(t *testing.T) {
	m := conversation(t)
	m = press(m, 2, 5, 0)
	m = update(m, tea.MouseMotionMsg{X: 6, Y: 5, Button: tea.MouseLeft})
	m, _ = release(m, 6, 5)
	m.Composer.Input.SetValue("/copy")
	_, cmd := m.Update(ControlIntent("send"))
	copied(t, cmd, "gamma")
}

func TestCopyIsAPaletteAction(t *testing.T) {
	m := conversation(t)
	m = update(m, ControlIntent("palette"))
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = next.(AppModel)
	copied(t, cmd, "alpha beta\n\ngamma delta")
	if m.overlay != "" {
		t.Fatalf("palette stayed open: %q", m.overlay)
	}
}

func TestHelpExplainsCopying(t *testing.T) {
	for _, size := range []tea.WindowSizeMsg{{Width: 100, Height: 30}, {Width: 70, Height: 30}, {Width: 55, Height: 30}, {Width: 70, Height: 18}} {
		m := update(sized(), size)
		m = update(m, tea.KeyPressMsg{Code: tea.KeyF1})
		if content := m.View().Content; !strings.Contains(content, "/copy") {
			t.Fatalf("%dx%d help omits copying:\n%s", size.Width, size.Height, content)
		}
	}
}

func TestComposerAcceptsBracketedPaste(t *testing.T) {
	m := sized()
	m = update(m, tea.PasteMsg{Content: "first line\nsecond line"})
	if got := m.Composer.Input.Value(); got != "first line\nsecond line" {
		t.Fatalf("pasted %q", got)
	}
	if !strings.Contains(m.View().Content, "second line") {
		t.Fatal(m.View().Content)
	}
}

func TestSelectionIsPaintedInColour(t *testing.T) {
	tr := NewTranscript()
	tr.Viewport.SetWidth(40)
	tr.Viewport.SetHeight(3)
	tr.Gutter = 2
	tr.SetContent("alpha beta\ngamma delta")
	tr.ApplyTheme(Theme{ID: "ink"})
	plain := tr.View()
	tr.Selection = Selection{Anchor: textPoint{Line: 0, Col: 6}, Head: textPoint{Line: 1, Col: 4}, Set: true}
	painted := tr.View()
	if painted == plain {
		t.Fatal("selection changed nothing")
	}
	if first := strings.Split(painted, "\n")[0]; !strings.Contains(first, "beta") || strings.Index(first, "alpha") > strings.Index(first, "beta") {
		t.Fatalf("first row %q", first)
	}
	if fmt.Sprint(tr.Selection.Text(strings.Split(tr.Viewport.GetContent(), "\n"))) != "beta\ngamma" {
		t.Fatal(tr.Selection.Text(strings.Split(tr.Viewport.GetContent(), "\n")))
	}
}
