package terminal

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type Transcript struct {
	Viewport   viewport.Model
	Gutter     int
	rows       []transcriptRow
	rowKinds   map[int]transcriptRowKind
	indents    map[int]int
	visualRows []transcriptRowKind
	theme      Theme
	// raw holds unwrapped lines; reflow wraps them at word boundaries to the
	// viewport width. The viewport's own soft wrap cuts mid-word and re-slices
	// every line from its start on each render, which stalls on large results.
	raw       string
	wrapWidth int
	// Highlight marks every case-insensitive occurrence in the visible rows,
	// and CurrentHit the selected one differently.
	Highlight  string
	CurrentHit *searchHit
	// Selection is the mouse-made range the view paints; see [Selection].
	Selection Selection
	// expanded holds the console notes a click opened, for the session
	// expandedFor; rowLines maps each raw line to its row and visualIndex
	// each wrapped line to its row (-1 for none), so a click finds a note.
	expanded    map[string]bool
	expandedFor domain.SessionID
	rowLines    map[int]int
	visualIndex []int
}

func NewTranscript() Transcript {
	return Transcript{Viewport: viewport.New()}
}

// SetWidth resizes the viewport and rewraps the content when the width changes.
func (t *Transcript) SetWidth(width int) {
	t.Viewport.SetWidth(width)
	if width == t.wrapWidth {
		return
	}
	if len(t.rows) > 0 {
		// Tables lay out for the width, so rows render again.
		t.renderRows()
		return
	}
	t.reflow()
}
func (t *Transcript) SetContent(s string) {
	t.rows = nil
	t.rowKinds = nil
	t.indents = nil
	t.rowLines = nil
	t.raw = Sanitize(s)
	t.reflow()
}

// Text is the unwrapped transcript content.
func (t Transcript) Text() string {
	return t.raw
}

// VisualLineCount is the number of wrapped lines the viewport scrolls over.
func (t Transcript) VisualLineCount() int {
	if t.Viewport.GetContent() == "" {
		return 0
	}
	return len(t.visualRows)
}
func (t *Transcript) reflow() {
	width := t.Viewport.Width()
	t.wrapWidth = width
	t.visualRows = t.visualRows[:0]
	t.visualIndex = t.visualIndex[:0]
	var b strings.Builder
	for index, line := range strings.Split(t.raw, "\n") {
		if index > 0 {
			b.WriteByte('\n')
		}
		wrapped := line
		if lineWidth := ansi.StringWidth(line); width > 0 && lineWidth > width {
			wrapped = ansi.Wrap(line, width, "")
			if indent := t.indents[index]; indent > 0 && indent < width {
				pad := strings.Repeat(" ", indent)
				body := ansi.Wrap(ansi.Cut(line, indent, lineWidth), width-indent, "")
				wrapped = ansi.Cut(line, 0, indent) + strings.ReplaceAll(body, "\n", "\n"+pad)
			}
		}
		row, ok := t.rowLines[index]
		if !ok {
			row = -1
		}
		for range strings.Count(wrapped, "\n") + 1 {
			t.visualRows = append(t.visualRows, t.rowKinds[index])
			t.visualIndex = append(t.visualIndex, row)
		}
		b.WriteString(wrapped)
	}
	bottom := t.Viewport.AtBottom()
	t.Viewport.SetContent(b.String())
	if bottom {
		t.Viewport.GotoBottom()
	}
}
func (t *Transcript) SetSession(s domain.SessionState, draft string, theme Theme) {
	t.theme = theme
	t.rows = t.rows[:0]
	if s.ID != t.expandedFor {
		t.expanded, t.expandedFor = nil, s.ID
	}
	archived := 0
	results := make(map[string]*root.Message)
	for i := range s.Messages {
		if s.Messages[i].Role == root.RoleTool {
			results[string(s.Messages[i].ToolCallID)] = &s.Messages[i]
		}
	}
	records := make(map[string]*domain.PendingTool, len(s.Activity))
	for i := range s.Activity {
		records[string(s.Activity[i].Call.ID)] = &s.Activity[i]
	}
	clock := transcriptNow()
	lines := 0
	for lines < len(s.ConsoleLines) && s.ConsoleLines[lines].AfterMessage == 0 {
		t.appendRow(consoleLineRow(s.ConsoleLines[lines], theme))
		lines++
	}
	for index, m := range s.Messages {
		var at time.Time
		if index < len(s.MessageTimes) {
			at = s.MessageTimes[index]
		}
		switch {
		case m.Role == root.RoleUser:
			// What the person typed is a prompt; the notes the console wrote
			// or appended to it are one dim line each.
			typed, notes := splitConsoleNotes(m.Content)
			if typed != "" || len(notes) == 0 {
				t.appendRow(transcriptRow{Label: theme.Icon("user") + " ", LabelTone: toneAccent, Text: typed, Kind: transcriptRowUser, Indent: true, At: at, Aside: formatClock(at, clock)})
			}
			for k, note := range notes {
				key := fmt.Sprintf("%d.%d", index, k)
				row := consoleNoteRow(note, key, t.expanded[key], theme)
				row.At = at
				t.appendRow(row)
			}
		case m.Role == root.RoleAssistant && m.Content != "":
			t.appendRow(transcriptRow{Text: string(m.Content), Kind: transcriptRowAssistant, Markdown: true, At: at})
		case m.Role != root.RoleAssistant && m.Role != root.RoleTool:
			t.appendRow(transcriptRow{Text: theme.T("role."+string(m.Role)) + ": " + string(m.Content), Kind: transcriptRowPlain})
		}
		for _, c := range m.ToolCalls {
			row := toolRow(s, c, records[string(c.ID)], results[string(c.ID)], theme)
			row.At = at
			t.appendRow(row)
		}
		for archived < len(s.ArchivedDrafts) && s.ArchivedDrafts[archived].AfterMessage == index+1 {
			t.appendRow(transcriptRow{Label: theme.T("transcript.interruptedDraft"), Text: string(s.ArchivedDrafts[archived].Content), Kind: transcriptRowAssistant, Markdown: true})
			archived++
		}
		for lines < len(s.ConsoleLines) && s.ConsoleLines[lines].AfterMessage == index+1 {
			t.appendRow(consoleLineRow(s.ConsoleLines[lines], theme))
			lines++
		}
	}
	if s.Draft != "" {
		t.appendRow(transcriptRow{Label: theme.T("transcript.interruptedDraft"), Text: string(s.Draft), Kind: transcriptRowAssistant, Markdown: true})
	}
	if draft != "" {
		t.appendRow(transcriptRow{Text: draft, Kind: transcriptRowAssistant, Markdown: true})
	}
	if theme.editorial() {
		t.rows = editorialRows(t.rows, theme, clock)
	}
	t.renderRows()
	t.ApplyTheme(theme)
}
func (t *Transcript) AppendUnsent(prompts []string) {
	if len(prompts) == 0 {
		return
	}
	for _, prompt := range prompts {
		t.appendRow(transcriptRow{Label: t.theme.Icon("user") + " ", LabelTone: toneWarning, Text: t.theme.T("transcript.notSent") + prompt, Kind: transcriptRowUser, Indent: true})
	}
	t.renderRows()
	t.ApplyTheme(t.theme)
}

// AppendQueued shows a message sent while AXLR is busy; it is delivered
// when the current model step ends.
func (t *Transcript) AppendQueued(prompt string) {
	if prompt == "" {
		return
	}
	t.appendRow(transcriptRow{Label: t.theme.Icon("user") + " ", LabelTone: toneAccent, Text: t.theme.T("transcript.queued") + prompt, Kind: transcriptRowUser, Indent: true})
	t.renderRows()
	t.ApplyTheme(t.theme)
}

// appendRow separates conversation turns with a blank line but keeps
// consecutive tool rows together.
func (t *Transcript) appendRow(row transcriptRow) {
	if len(t.rows) > 0 {
		row.GapBefore = !(row.Kind.isTool() && t.rows[len(t.rows)-1].Kind.isTool())
	}
	t.rows = append(t.rows, row)
}
func (t *Transcript) renderRows() {
	var b strings.Builder
	kinds := make(map[int]transcriptRowKind)
	indents := make(map[int]int)
	rowLines := make(map[int]int)
	line := 0
	for index, row := range t.rows {
		if index > 0 {
			b.WriteByte('\n')
			if row.GapBefore {
				b.WriteByte('\n')
				kinds[line] = transcriptRowGap
				line++
			}
		}
		clean := Sanitize(row.Text)
		if row.Markdown {
			clean = renderMarkdownWidth(clean, t.theme, t.Viewport.Width())
		}
		label := Sanitize(row.Label)
		if width := t.Viewport.Width(); row.Note != "" && !strings.Contains(clean, "\n") && width > 0 {
			// A folded console note stays one line at any width.
			clean = ansi.Truncate(clean, max(1, width-ansi.StringWidth(label)), "…")
		}
		indent := 0
		if row.Indent {
			indent = ansi.StringWidth(label)
			clean = strings.ReplaceAll(clean, "\n", "\n"+strings.Repeat(" ", indent))
		}
		clean = t.theme.rowLabel(label, row.LabelTone, row.Kind) + clean
		if width := t.Viewport.Width(); row.Aside != "" && width > 0 {
			first, rest, _ := strings.Cut(clean, "\n")
			if gap := width - ansi.StringWidth(first) - ansi.StringWidth(row.Aside); gap >= 2 {
				first += strings.Repeat(" ", gap) + t.theme.Muted(row.Aside)
				if rest != "" || strings.Contains(clean, "\n") {
					first += "\n" + rest
				}
				clean = first
			}
		}
		for i := range strings.Count(clean, "\n") + 1 {
			kinds[line+i] = row.Kind
			indents[line+i] = indent
			rowLines[line+i] = index
		}
		line += strings.Count(clean, "\n") + 1
		b.WriteString(clean)
	}
	t.rowKinds = kinds
	t.indents = indents
	t.rowLines = rowLines
	t.raw = b.String()
	t.reflow()
}
func (t *Transcript) ApplyTheme(theme Theme) {
	t.theme = theme
	t.Viewport.StyleLineFunc = nil
	if len(t.rows) > 0 {
		// Markdown colours come from the theme, so rows render again.
		t.renderRows()
		return
	}
	t.reflow()
}
func (t Transcript) View() string {
	view := t.Viewport.View()
	if view == "" {
		return view
	}
	lines := strings.Split(view, "\n")
	outerWidth := max(1, t.Viewport.Width()+2*t.Gutter)
	for i := range lines {
		line := strings.Repeat(" ", t.Gutter) + lines[i]
		line = ansi.Truncate(line, outerWidth, "")
		index := t.Viewport.YOffset() + i
		if t.theme.Monochrome || index >= len(t.visualRows) {
			lines[i] = lipgloss.NewStyle().Width(outerWidth).Render(line)
			continue
		}
		lines[i] = t.theme.rowText(t.visualRows[index]).Width(outerWidth).Render(line)
	}
	if t.Highlight != "" {
		for i := range lines {
			current := -1
			if t.CurrentHit != nil && t.CurrentHit.Line == t.Viewport.YOffset()+i {
				current = t.CurrentHit.Cell + t.Gutter
			}
			lines[i] = highlightMatches(lines[i], t.Highlight, t.theme, current)
		}
	}
	if t.Selection.Set {
		style := t.theme.selectionStyle()
		for i := range lines {
			index := t.Viewport.YOffset() + i
			width := ansi.StringWidth(ansi.Strip(lines[i])) - t.Gutter
			if left, right, ok := t.Selection.cellRange(index, width); ok {
				lines[i] = lipgloss.StyleRanges(lines[i], lipgloss.NewRange(left+t.Gutter, right+t.Gutter, style))
			}
		}
	}
	return strings.Join(lines, "\n")
}

// transcriptNow is the clock that turns message times into labels.
var transcriptNow = time.Now

// formatClock is "15:04" for today and "2006-01-02 15:04" otherwise, in
// local time; unknown times give no label.
func formatClock(at, now time.Time) string {
	if at.IsZero() {
		return ""
	}
	local, today := at.Local(), now.Local()
	if y, m, d := local.Date(); y == today.Year() && m == today.Month() && d == today.Day() {
		return local.Format("15:04")
	}
	return local.Format("2006-01-02 15:04")
}

// highlightMatches styles every case-insensitive occurrence of query in a
// rendered line, measuring positions in cells as lipgloss ranges expect;
// the one starting at cell current (-1 for none) is the selected hit.
func highlightMatches(line, query string, theme Theme, current int) string {
	needle := []rune(query)
	if len(needle) == 0 {
		return line
	}
	style := lipgloss.NewStyle().Reverse(true)
	selected := lipgloss.NewStyle().Reverse(true).Underline(true).Bold(true)
	if !theme.Monochrome {
		p := theme.palette()
		style = lipgloss.NewStyle().Background(lipgloss.Color(p.Warning)).Foreground(lipgloss.Color(p.Background)).Bold(true)
		selected = lipgloss.NewStyle().Background(lipgloss.Color(p.Accent)).Foreground(lipgloss.Color(p.Background)).Bold(true).Underline(true)
	}
	runes := []rune(ansi.Strip(line))
	var ranges []lipgloss.Range
	for i := 0; i+len(needle) <= len(runes); {
		if !strings.EqualFold(string(runes[i:i+len(needle)]), query) {
			i++
			continue
		}
		start := ansi.StringWidth(string(runes[:i]))
		hit := style
		if start == current {
			hit = selected
		}
		ranges = append(ranges, lipgloss.NewRange(start, start+ansi.StringWidth(string(runes[i:i+len(needle)])), hit))
		i += len(needle)
	}
	return lipgloss.StyleRanges(line, ranges...)
}

// NoteAt is the console note drawn on a wrapped line, if any.
func (t Transcript) NoteAt(line int) (string, bool) {
	if line < 0 || line >= len(t.visualIndex) || t.visualIndex[line] < 0 || t.visualIndex[line] >= len(t.rows) {
		return "", false
	}
	key := t.rows[t.visualIndex[line]].Note
	return key, key != ""
}

// ToggleNote expands a folded console note or folds an expanded one; the
// next SetSession draws it so.
func (t *Transcript) ToggleNote(key string) {
	if t.expanded == nil {
		t.expanded = map[string]bool{}
	}
	t.expanded[key] = !t.expanded[key]
}
