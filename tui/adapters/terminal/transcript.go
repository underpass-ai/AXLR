package terminal

import (
	"strings"

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
		for range strings.Count(wrapped, "\n") + 1 {
			t.visualRows = append(t.visualRows, t.rowKinds[index])
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
	for index, m := range s.Messages {
		switch {
		case m.Role == root.RoleUser:
			t.appendRow(transcriptRow{Label: theme.Icon("user") + " ", LabelTone: toneAccent, Text: string(m.Content), Kind: transcriptRowUser, Indent: true})
		case m.Role == root.RoleAssistant && m.Content != "":
			t.appendRow(transcriptRow{Text: string(m.Content), Kind: transcriptRowAssistant, Markdown: true})
		case m.Role != root.RoleAssistant && m.Role != root.RoleTool:
			t.appendRow(transcriptRow{Text: theme.T("role."+string(m.Role)) + ": " + string(m.Content), Kind: transcriptRowPlain})
		}
		for _, c := range m.ToolCalls {
			t.appendRow(toolRow(s, c, records[string(c.ID)], results[string(c.ID)], theme))
		}
		for archived < len(s.ArchivedDrafts) && s.ArchivedDrafts[archived].AfterMessage == index+1 {
			t.appendRow(transcriptRow{Label: theme.T("transcript.interruptedDraft"), Text: string(s.ArchivedDrafts[archived].Content), Kind: transcriptRowAssistant, Markdown: true})
			archived++
		}
	}
	if s.Draft != "" {
		t.appendRow(transcriptRow{Label: theme.T("transcript.interruptedDraft"), Text: string(s.Draft), Kind: transcriptRowAssistant, Markdown: true})
	}
	if draft != "" {
		t.appendRow(transcriptRow{Text: draft, Kind: transcriptRowAssistant, Markdown: true})
	}
	if theme.editorial() {
		t.rows = editorialRows(t.rows, theme)
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
		indent := 0
		if row.Indent {
			indent = ansi.StringWidth(label)
			clean = strings.ReplaceAll(clean, "\n", "\n"+strings.Repeat(" ", indent))
		}
		clean = t.theme.rowLabel(label, row.LabelTone, row.Kind) + clean
		for i := range strings.Count(clean, "\n") + 1 {
			kinds[line+i] = row.Kind
			indents[line+i] = indent
		}
		line += strings.Count(clean, "\n") + 1
		b.WriteString(clean)
	}
	t.rowKinds = kinds
	t.indents = indents
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
	return strings.Join(lines, "\n")
}
