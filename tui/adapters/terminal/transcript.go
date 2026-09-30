package terminal

import (
	"strings"

	"charm.land/bubbles/v2/viewport"
	"github.com/charmbracelet/x/ansi"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type Transcript struct {
	Viewport   viewport.Model
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
	t.rows = t.rows[:0]
	archived := 0
	for index, m := range s.Messages {
		kind := transcriptRowPlain
		if m.Role == root.RoleUser {
			kind = transcriptRowUser
		} else if m.Role == root.RoleAssistant {
			kind = transcriptRowAssistant
		}
		t.rows = append(t.rows, transcriptRow{Text: string(m.Role) + ": " + string(m.Content), Kind: kind})
		for _, c := range m.ToolCalls {
			t.rows = append(t.rows, transcriptRow{Text: "tool request: " + string(c.Name) + " " + string(c.Arguments.Bytes())})
			for _, a := range s.Activity {
				if a.Call.ID == c.ID {
					t.rows = append(t.rows, transcriptRow{Text: "decision: " + string(a.Decision)})
				}
			}
		}
		for archived < len(s.ArchivedDrafts) && s.ArchivedDrafts[archived].AfterMessage == index+1 {
			t.rows = append(t.rows, transcriptRow{Text: "interrupted draft: " + string(s.ArchivedDrafts[archived].Content), Kind: transcriptRowAssistant})
			archived++
		}
	}
	if s.Draft != "" {
		t.rows = append(t.rows, transcriptRow{Text: "interrupted draft: " + string(s.Draft), Kind: transcriptRowAssistant})
	}
	if draft != "" {
		t.rows = append(t.rows, transcriptRow{Text: "assistant: " + draft, Kind: transcriptRowAssistant})
	}
	t.renderRows()
	t.ApplyTheme(theme)
}
func (t *Transcript) AppendUnsent(prompts []string) {
	if len(prompts) == 0 {
		return
	}
	for _, prompt := range prompts {
		t.rows = append(t.rows, transcriptRow{Text: "Not sent: " + prompt, Kind: transcriptRowUser})
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
	if t.theme.Monochrome || len(t.rowKinds) == 0 || view == "" {
		return view
	}
	lines := strings.Split(view, "\n")
	for i := range lines {
		index := t.Viewport.YOffset() + i
		if index >= len(t.visualRows) {
			continue
		}
		switch t.visualRows[index] {
		case transcriptRowUser:
			lines[i] = t.theme.UserRow().Render(lines[i])
		case transcriptRowAssistant:
			lines[i] = t.theme.AssistantRow().Render(lines[i])
		}
	}
	return strings.Join(lines, "\n")
}
