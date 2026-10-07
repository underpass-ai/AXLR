package terminal

import (
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/atotto/clipboard"
	"github.com/charmbracelet/x/ansi"
	root "github.com/underpass-ai/AXLR/domain"
)

// textPoint is a cell of the transcript's wrapped content: the visual line
// over the whole scrollable content and the column in cells. Content
// coordinates survive wheel scrolling while the button is held.
type textPoint struct{ Line, Col int }

func (p textPoint) before(q textPoint) bool {
	return p.Line < q.Line || (p.Line == q.Line && p.Col < q.Col)
}

// Selection is a mouse-made range over the transcript. Anchor is the cell
// where the button went down and Head follows the pointer; either may come
// first. Both ends are inclusive, as a terminal's own selection is.
type Selection struct {
	Anchor, Head textPoint
	// Active is set between press and release.
	Active bool
	// Set reports a range exists, possibly a single cell.
	Set bool
}

// Bounds orders the ends.
func (s Selection) Bounds() (start, end textPoint) {
	if s.Head.before(s.Anchor) {
		return s.Head, s.Anchor
	}
	return s.Anchor, s.Head
}

// Text extracts the selected cells from the wrapped content lines as plain
// text, one line per visual row, trailing blanks trimmed. Wrapped rows of a
// long line come back as separate lines, as the person saw them.
func (s Selection) Text(lines []string) string {
	if !s.Set || len(lines) == 0 {
		return ""
	}
	start, end := s.Bounds()
	start.Line = max(0, min(start.Line, len(lines)-1))
	end.Line = max(0, min(end.Line, len(lines)-1))
	out := make([]string, 0, end.Line-start.Line+1)
	for i := start.Line; i <= end.Line; i++ {
		plain := ansi.Strip(lines[i])
		left, right := 0, ansi.StringWidth(plain)
		if i == start.Line {
			left = max(0, start.Col)
		}
		if i == end.Line {
			right = min(right, end.Col+1)
		}
		if left >= right {
			out = append(out, "")
			continue
		}
		out = append(out, strings.TrimRight(ansi.Cut(plain, left, right), " "))
	}
	return strings.Join(out, "\n")
}

// cellRange is the inclusive cell span the selection covers on one visual
// line, or ok=false when the line is outside it. width is the line's own
// width, which bounds an open end.
func (s Selection) cellRange(line, width int) (left, right int, ok bool) {
	if !s.Set {
		return 0, 0, false
	}
	start, end := s.Bounds()
	if line < start.Line || line > end.Line {
		return 0, 0, false
	}
	left, right = 0, width
	if line == start.Line {
		left = start.Col
	}
	if line == end.Line {
		right = min(width, end.Col+1)
	}
	if left >= right {
		return 0, 0, false
	}
	return left, right, true
}

// wordAt is the inclusive cell span of the word under col in a plain line:
// a run of letters, digits and connecting punctuation, or of other
// non-blank characters. A blank cell selects only itself.
func wordAt(plain string, col int) (left, right int) {
	runes := []rune(plain)
	cells := make([]int, len(runes)+1)
	for i, r := range runes {
		cells[i+1] = cells[i] + ansi.StringWidth(string(r))
	}
	index := -1
	for i := range runes {
		if col >= cells[i] && col < cells[i+1] {
			index = i
			break
		}
	}
	if index < 0 {
		return col, col
	}
	class := func(r rune) int {
		switch {
		case unicode.IsSpace(r):
			return 0
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '.' || r == '/':
			return 1
		default:
			return 2
		}
	}
	kind := class(runes[index])
	if kind == 0 {
		return col, col
	}
	first, last := index, index
	for first > 0 && class(runes[first-1]) == kind {
		first--
	}
	for last+1 < len(runes) && class(runes[last+1]) == kind {
		last++
	}
	return cells[first], max(cells[first], cells[last+1]-1)
}

// selectionNow is the clock that tells a double click from two clicks.
var selectionNow = time.Now

// multiClickInterval is the longest pause between clicks of one gesture.
const multiClickInterval = 400 * time.Millisecond

// copyToClipboard hands text to the terminal's clipboard with OSC 52, which
// also works over SSH and inside tmux with set-clipboard on, mirrors it to
// the primary selection so Shift+middle click pastes it on X11, and tries
// the host clipboard tools as a fallback for terminals that ignore OSC 52.
// Nothing reports whether the terminal accepted it.
func copyToClipboard(text string) tea.Cmd {
	if text == "" {
		return nil
	}
	return tea.Batch(tea.SetClipboard(text), tea.SetPrimaryClipboard(text), func() tea.Msg {
		_ = clipboard.WriteAll(text)
		return nil
	})
}

// selectionStyle paints selected cells like the theme's selected rows.
func (t Theme) selectionStyle() lipgloss.Style {
	if t.Monochrome {
		return lipgloss.NewStyle().Reverse(true)
	}
	p := t.palette()
	return lipgloss.NewStyle().Foreground(lipgloss.Color(p.Text)).Background(lipgloss.Color(p.Selected))
}

// transcriptPoint maps a screen cell to transcript content coordinates. The
// body starts under the one-row header; columns inside the gutter clamp to
// the first cell, and rows past the content clamp to its last line.
func (m AppModel) transcriptPoint(x, y int) (textPoint, bool) {
	top := 1
	height := m.Transcript.Viewport.Height()
	if y < top || y >= top+height {
		return textPoint{}, false
	}
	lines := m.Transcript.VisualLineCount()
	if lines == 0 {
		return textPoint{}, false
	}
	line := min(lines-1, m.Transcript.Viewport.YOffset()+y-top)
	return textPoint{Line: line, Col: max(0, x-m.Transcript.Gutter)}, true
}

// clampedPoint follows a drag that leaves the body: above it selects from
// the first visible row, below it to the last visible row.
func (m AppModel) clampedPoint(x, y int) textPoint {
	top := 1
	bottom := top + max(1, m.Transcript.Viewport.Height()) - 1
	if p, ok := m.transcriptPoint(x, max(top, min(bottom, y))); ok {
		return p
	}
	return textPoint{Col: max(0, x-m.Transcript.Gutter)}
}

// beginSelection starts or extends a selection at a press. Shift extends
// the existing one, as in a terminal; a second click on the same cell takes
// the word and a third the whole row.
func (m *AppModel) beginSelection(click tea.MouseClickMsg) {
	point, ok := m.transcriptPoint(click.X, click.Y)
	if !ok || m.Layout.TooSmall {
		m.clearSelection()
		return
	}
	now := selectionNow()
	repeated := m.lastClick.Set && m.lastClick.Head == point && now.Sub(m.lastClickAt) <= multiClickInterval
	m.lastClickAt = now
	if repeated {
		m.clickCount++
	} else {
		m.clickCount = 1
	}
	m.lastClick = Selection{Head: point, Set: true}
	m.Status.Notice = ""
	if click.Mod.Contains(tea.ModShift) && m.Transcript.Selection.Set {
		m.Transcript.Selection.Head = point
		m.Transcript.Selection.Active = true
		return
	}
	lines := strings.Split(m.Transcript.Viewport.GetContent(), "\n")
	switch {
	case m.clickCount >= 3 && point.Line < len(lines):
		width := ansi.StringWidth(ansi.Strip(lines[point.Line]))
		m.Transcript.Selection = Selection{Anchor: textPoint{Line: point.Line}, Head: textPoint{Line: point.Line, Col: max(0, width-1)}, Active: true, Set: true}
	case m.clickCount == 2 && point.Line < len(lines):
		left, right := wordAt(ansi.Strip(lines[point.Line]), point.Col)
		m.Transcript.Selection = Selection{Anchor: textPoint{Line: point.Line, Col: left}, Head: textPoint{Line: point.Line, Col: right}, Active: true, Set: true}
	default:
		m.Transcript.Selection = Selection{Anchor: point, Head: point, Active: true, Set: true}
	}
}

// extendSelection follows the pointer while the button is held.
func (m *AppModel) extendSelection(motion tea.MouseMotionMsg) {
	if !m.Transcript.Selection.Active {
		return
	}
	m.Transcript.Selection.Head = m.clampedPoint(motion.X, motion.Y)
}

// endSelection copies the range on release. A press and release on one
// cell with nothing else selected is a click, not a selection.
func (m *AppModel) endSelection(release tea.MouseReleaseMsg) tea.Cmd {
	s := m.Transcript.Selection
	if !s.Active {
		return nil
	}
	m.Transcript.Selection.Active = false
	if m.clickCount == 1 && s.Anchor == s.Head {
		m.clearSelection()
		return nil
	}
	return m.copySelection()
}

// copySelection copies the selected transcript text and says so.
func (m *AppModel) copySelection() tea.Cmd {
	text := m.Transcript.Selection.Text(strings.Split(m.Transcript.Viewport.GetContent(), "\n"))
	if strings.TrimSpace(text) == "" {
		return nil
	}
	m.Status.Notice = m.Theme.Tf("status.copied", strings.Count(text, "\n")+1)
	return copyToClipboard(text)
}

// copyLatest serves /copy: the selection when one exists, otherwise the
// last reply as the model wrote it, without wrapping, labels or gutters.
func (m *AppModel) copyLatest() tea.Cmd {
	if m.Transcript.Selection.Set && !m.Transcript.Selection.Active {
		if cmd := m.copySelection(); cmd != nil {
			return cmd
		}
	}
	text := m.draft
	for i := len(m.Header.State.Messages) - 1; i >= 0 && text == ""; i-- {
		if msg := m.Header.State.Messages[i]; msg.Role == root.RoleAssistant && msg.Content != "" {
			text = string(msg.Content)
		}
	}
	if text == "" {
		text = string(m.Header.State.Draft)
	}
	if text == "" {
		m.Status.Error = m.Theme.T("error.nothingToCopy")
		return nil
	}
	m.Status.Error = ""
	m.Status.Notice = m.Theme.Tf("status.copied", strings.Count(text, "\n")+1)
	return copyToClipboard(text)
}

func (m *AppModel) clearSelection() {
	m.Transcript.Selection = Selection{}
}
