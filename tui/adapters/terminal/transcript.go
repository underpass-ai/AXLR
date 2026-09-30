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
	visualRows []transcriptRowKind
	theme      Theme
}

func NewTranscript() Transcript {
	v := viewport.New()
	v.SoftWrap = true
	return Transcript{Viewport: v}
}
func (t *Transcript) SetContent(s string) {
	bottom := t.Viewport.AtBottom()
	t.Viewport.SetContent(Sanitize(s))
	t.rows = nil
	t.rowKinds = nil
	t.visualRows = nil
	if bottom {
		t.Viewport.GotoBottom()
	}
}
func (t *Transcript) SetSession(s domain.SessionState, draft string, theme Theme) {
	t.theme = theme
	t.rows = t.rows[:0]
	archived := 0
	for index, m := range s.Messages {
		kind := transcriptRowPlain
		if m.Role == root.RoleUser {
			kind = transcriptRowUser
		} else if m.Role == root.RoleAssistant {
			kind = transcriptRowAssistant
		}
		text := theme.T("role."+string(m.Role)) + ": " + string(m.Content)
		if m.Role == root.RoleTool {
			call := root.ToolCall{}
			for _, record := range s.Activity {
				if record.Call.ID == m.ToolCallID {
					call = record.Call
					break
				}
			}
			label, memory := toolCallPresentation(s, call)
			text = theme.T("transcript.toolResult") + label + " · " + toolResultSummaryLocale(string(m.Content), theme.Locale)
			if memory {
				kind = transcriptRowMemory
				text = theme.T("transcript.memoryResult") + label + " · " + toolResultSummaryLocale(string(m.Content), theme.Locale)
			}
		}
		if m.Content != "" || m.Role != root.RoleAssistant {
			t.rows = append(t.rows, transcriptRow{Text: text, Kind: kind, GapBefore: len(t.rows) > 0})
		}
		for _, c := range m.ToolCalls {
			label, memory := toolCallPresentation(s, c)
			kind := transcriptRowPlain
			prefix := theme.T("transcript.toolRequest")
			if memory {
				kind = transcriptRowMemory
				prefix = theme.T("transcript.memoryRequest")
			}
			t.rows = append(t.rows, transcriptRow{Text: prefix + label + " " + toolSummaryLocale(string(c.Arguments.Bytes()), theme.Locale), Kind: kind, GapBefore: len(t.rows) > 0})
			for _, a := range s.Activity {
				if a.Call.ID == c.ID {
					t.rows = append(t.rows, transcriptRow{Text: theme.T("transcript.decision") + theme.T("decision."+string(a.Decision)), Kind: kind})
				}
			}
		}
		for archived < len(s.ArchivedDrafts) && s.ArchivedDrafts[archived].AfterMessage == index+1 {
			t.rows = append(t.rows, transcriptRow{Text: theme.T("transcript.interruptedDraft") + string(s.ArchivedDrafts[archived].Content), Kind: transcriptRowAssistant, GapBefore: len(t.rows) > 0})
			archived++
		}
	}
	if s.Draft != "" {
		t.rows = append(t.rows, transcriptRow{Text: theme.T("transcript.interruptedDraft") + string(s.Draft), Kind: transcriptRowAssistant, GapBefore: len(t.rows) > 0})
	}
	if draft != "" {
		t.rows = append(t.rows, transcriptRow{Text: theme.T("transcript.assistant") + draft, Kind: transcriptRowAssistant, GapBefore: len(t.rows) > 0})
	}
	t.renderRows()
	t.ApplyTheme(theme)
}
func (t *Transcript) AppendUnsent(prompts []string) {
	if len(prompts) == 0 {
		return
	}
	for _, prompt := range prompts {
		t.rows = append(t.rows, transcriptRow{Text: t.theme.T("transcript.notSent") + prompt, Kind: transcriptRowUser, GapBefore: len(t.rows) > 0})
	}
	t.renderRows()
	t.ApplyTheme(t.theme)
}
func (t *Transcript) renderRows() {
	var b strings.Builder
	kinds := make(map[int]transcriptRowKind)
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
		for i := range strings.Count(clean, "\n") + 1 {
			kinds[line+i] = row.Kind
		}
		line += strings.Count(clean, "\n") + 1
		b.WriteString(clean)
	}
	bottom := t.Viewport.AtBottom()
	t.Viewport.SetContent(b.String())
	t.rowKinds = kinds
	if bottom {
		t.Viewport.GotoBottom()
	}
}
func (t *Transcript) ApplyTheme(theme Theme) {
	t.theme = theme
	t.Viewport.StyleLineFunc = nil
	t.visualRows = t.visualRows[:0]
	width := max(1, t.Viewport.Width())
	for index, line := range strings.Split(t.Viewport.GetContent(), "\n") {
		count := max(1, (ansi.StringWidth(line)+width-1)/width)
		for range count {
			t.visualRows = append(t.visualRows, t.rowKinds[index])
		}
	}
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
		switch t.visualRows[index] {
		case transcriptRowUser:
			lines[i] = t.theme.UserRow().Width(outerWidth).Render(line)
		case transcriptRowAssistant:
			lines[i] = t.theme.AssistantRow().Width(outerWidth).Render(line)
		case transcriptRowMemory:
			lines[i] = t.theme.MemoryRow().Width(outerWidth).Render(line)
		case transcriptRowGap:
			lines[i] = lipgloss.NewStyle().Width(outerWidth).Render(line)
		default:
			lines[i] = t.theme.ToolRow().Width(outerWidth).Render(line)
		}
	}
	return strings.Join(lines, "\n")
}
