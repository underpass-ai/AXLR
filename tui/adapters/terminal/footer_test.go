package terminal

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	root "github.com/underpass-ai/AXLR/domain"
)

func TestFooterHintsStateAndTokensFitOneRow(t *testing.T) {
	for _, width := range []int{50, 70, 120} {
		m := update(sized(), tea.WindowSizeMsg{Width: width, Height: 20})
		m.tokens = 12_400
		footer := ansi.Strip(m.footerView())
		if strings.Contains(footer, "\n") || ansi.StringWidth(footer) != width {
			t.Fatalf("width %d: footer is not one full row: %q", width, footer)
		}
		if !strings.HasPrefix(footer, " enter send") || !strings.HasSuffix(strings.TrimRight(footer, " "), "idle · 12.4k tok") {
			t.Fatalf("width %d: footer %q", width, footer)
		}
		m.zones.Close()
	}
}

func TestFooterGivesErrorsTheWholeRow(t *testing.T) {
	m := sized()
	defer m.zones.Close()
	m.Status.Error = "session workspace /home/a/kmp differs from active workspace /home/a/AXLR"
	footer := ansi.Strip(m.footerView())
	if strings.Contains(footer, "enter send") || !strings.Contains(footer, "differs from active workspace /home/a/AXLR") {
		t.Fatalf("error not shown in full: %q", footer)
	}
}

// An error longer than the row wraps over up to three rows, taken from the
// conversation, so its instruction at the end is read; the screen keeps its
// height and the cursor stays on the composer.
func TestFooterWrapsALongErrorInsteadOfCuttingIt(t *testing.T) {
	m := sized()
	defer m.zones.Close()
	m.Composer.Input.SetValue("draft text")
	m.Status.Error = "MADE definition not published: axlr_delivery 2.0. Preparing MADE is a one-time setup: open /mcp, select MADE and press p. " + Translate(English, "error.promptReturned")
	v := m.View()
	lines := strings.Split(ansi.Strip(v.Content), "\n")
	if len(lines) != m.Layout.Height {
		t.Fatalf("the view is %d rows; want %d", len(lines), m.Layout.Height)
	}
	footer := strings.Join(strings.Fields(strings.Join(lines[len(lines)-3:], " ")), " ")
	for _, want := range []string{"press p.", "send it again when ready."} {
		if !strings.Contains(footer, want) {
			t.Fatalf("the error lost %q:\n%s", want, strings.Join(lines[len(lines)-3:], "\n"))
		}
	}
	if v.Cursor == nil || !strings.Contains(lines[v.Cursor.Y], "draft text") {
		t.Fatal("the cursor left the composer row")
	}
	// A click on the conversation still selects the line drawn there.
	var rows []string
	for i := range 60 {
		rows = append(rows, fmt.Sprintf("row %02d", i))
	}
	m.Header.State.Messages = append(m.Header.State.Messages, root.Message{Role: root.RoleAssistant, Content: root.Text(strings.Join(rows, "\n"))})
	m.refreshTranscript()
	lines = strings.Split(ansi.Strip(m.View().Content), "\n")
	content := strings.Split(ansi.Strip(m.Transcript.Viewport.GetContent()), "\n")
	for y := 1; y < len(lines); y++ {
		if !strings.Contains(lines[y], "row ") {
			continue
		}
		point, ok := m.transcriptPoint(4, y)
		if !ok || strings.TrimSpace(content[point.Line]) != strings.TrimSpace(lines[y]) {
			t.Fatalf("screen row %d %q maps to line %q", y, lines[y], content[point.Line])
		}
	}
	m.Status.Error = strings.Repeat("very long error ", 60)
	if footer := strings.Split(m.footerView(), "\n"); len(footer) != 3 || !strings.HasSuffix(strings.TrimRight(ansi.Strip(footer[2]), " "), "…") {
		t.Fatalf("an error past three rows is not cut on the third: %q", footer)
	}
}

func TestFooterCountsOneChangedFileInTheSingular(t *testing.T) {
	for locale, plural := range map[Locale]string{English: "1 files", Spanish: "1 ficheros"} {
		m := sized()
		m.Theme.Locale = locale
		m.Changes.records = make([]changeRecord, 1)
		m.Changes.files = make([]changeFile, 1)
		footer := ansi.Strip(m.footerView())
		if strings.Contains(footer, plural) || !strings.Contains(footer, "ctrl+d "+Translate(locale, "changes.oneFile")) {
			t.Fatalf("%s footer: %q", locale, footer)
		}
		m.zones.Close()
	}
}

// On 10 October 2026 the hint read "ctrl+d 4 changes" for four edits to two
// files, while /changes listed the two files.
func TestFooterCountsChangedFilesNotEdits(t *testing.T) {
	m := sized()
	defer m.zones.Close()
	m.Changes.records = make([]changeRecord, 4)
	m.Changes.files = make([]changeFile, 2)
	if footer := ansi.Strip(m.footerView()); !strings.Contains(footer, "ctrl+d "+m.Theme.Tf("changes.files", 2)) {
		t.Fatalf("footer: %q", footer)
	}
}

func TestFormatTokens(t *testing.T) {
	for n, want := range map[int]string{980: "980", 12_400: "12.4k", 1_250_000: "1.2M"} {
		if got := formatTokens(n); got != want {
			t.Fatalf("formatTokens(%d) = %q; want %q", n, got, want)
		}
	}
}
