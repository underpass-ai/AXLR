package terminal

import (
	"strings"

	"charm.land/bubbles/v2/viewport"
	"github.com/charmbracelet/x/ansi"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type Transcript struct {
	Viewport      viewport.Model
	assistantRows map[int]bool
	visualRows    []bool
	theme         Theme
}

func NewTranscript() Transcript {
	v := viewport.New()
	v.SoftWrap = true
	return Transcript{Viewport: v}
}
func (t *Transcript) SetContent(s string) {
	bottom := t.Viewport.AtBottom()
	t.Viewport.SetContent(Sanitize(s))
	t.assistantRows = nil
	t.visualRows = nil
	if bottom {
		t.Viewport.GotoBottom()
	}
}
func (t *Transcript) SetSession(s domain.SessionState, draft string, theme Theme) {
	var b strings.Builder
	rows := make(map[int]bool)
	line := 0
	appendEntry := func(value string, assistant, newline bool) {
		clean := Sanitize(value)
		count := strings.Count(clean, "\n") + 1
		if assistant {
			for i := range count {
				rows[line+i] = true
			}
		}
		b.WriteString(clean)
		if newline {
			b.WriteByte('\n')
		}
		line += count
	}
	for _, m := range s.Messages {
		appendEntry(string(m.Role)+": "+string(m.Content), m.Role == root.RoleAssistant, true)
		for _, c := range m.ToolCalls {
			appendEntry("tool request: "+string(c.Name)+" "+string(c.Arguments.Bytes()), false, true)
			for _, a := range s.Activity {
				if a.Call.ID == c.ID {
					appendEntry("decision: "+string(a.Decision), false, true)
				}
			}
		}
	}
	if s.Draft != "" {
		appendEntry("interrupted draft: "+string(s.Draft), true, true)
	}
	if draft != "" {
		appendEntry("assistant: "+draft, true, false)
	}
	t.SetContent(b.String())
	t.assistantRows = rows
	t.ApplyTheme(theme)
}
func (t *Transcript) AppendUnsent(prompts []string) {
	if len(prompts) == 0 {
		return
	}
	var b strings.Builder
	b.WriteString(t.Viewport.GetContent())
	for _, prompt := range prompts {
		b.WriteString("\nNot sent: ")
		b.WriteString(Sanitize(prompt))
	}
	bottom := t.Viewport.AtBottom()
	t.Viewport.SetContent(b.String())
	if bottom {
		t.Viewport.GotoBottom()
	}
	t.ApplyTheme(t.theme)
}
func (t *Transcript) ApplyTheme(theme Theme) {
	t.theme = theme
	t.Viewport.StyleLineFunc = nil
	t.visualRows = t.visualRows[:0]
	width := max(1, t.Viewport.Width())
	for index, line := range strings.Split(t.Viewport.GetContent(), "\n") {
		count := max(1, (ansi.StringWidth(line)+width-1)/width)
		for range count {
			t.visualRows = append(t.visualRows, t.assistantRows[index])
		}
	}
}
func (t Transcript) View() string {
	view := t.Viewport.View()
	if t.theme.Monochrome || len(t.assistantRows) == 0 || view == "" {
		return view
	}
	lines := strings.Split(view, "\n")
	style := t.theme.AssistantRow()
	for i := range lines {
		index := t.Viewport.YOffset() + i
		if index < len(t.visualRows) && t.visualRows[index] {
			lines[i] = style.Render(lines[i])
		}
	}
	return strings.Join(lines, "\n")
}
