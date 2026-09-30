package terminal

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone/v2"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type ToolActivity struct {
	Lines  []string
	Tokens int
	Locale Locale
	labels map[root.ToolName]string
}

func (a *ToolActivity) Apply(e application.Event) {
	if e.Usage != nil {
		a.Tokens = e.Usage.TotalTokens
	}
	if e.Kind == application.EventToolActivity {
		label := string(presentationName(e.Tool.Call))
		if display, ok := a.labels[presentationName(e.Tool.Call)]; ok {
			label = display
		}
		line := label + ": " + Translate(a.Locale, "decision."+string(e.Tool.Decision))
		if e.Tool.Outcome != nil {
			line += " " + toolSummaryLocale(string(e.Tool.Outcome.Content), a.Locale)
		}
		a.Lines = append(a.Lines, Sanitize(line))
	}
}
func (a ToolActivity) View(w, h int) string {
	s := Translatef(a.Locale, "activity.tokens", a.Tokens)
	for _, line := range a.Lines[max(0, len(a.Lines)-max(1, h-1)):] {
		s += "\n" + line
	}
	lines := strings.Split(ansi.Wrap(s, w, ""), "\n")
	if len(lines) > h {
		lines = lines[len(lines)-h:]
	}
	return lipgloss.NewStyle().Width(w).Height(h).Render(strings.Join(lines, "\n"))
}

func (a ToolActivity) Tabs(theme Theme, z *zone.Manager, prefix string, width int, activity bool) string {
	transcript, tools := theme.T("activity.conversation"), theme.T("activity.title")
	if activity {
		tools = theme.Accent(tools)
		transcript = theme.Muted(transcript)
	} else {
		transcript = theme.Accent(transcript)
		tools = theme.Muted(tools)
	}
	content := z.Mark(prefix+"transcript", transcript) + "    " + z.Mark(prefix+"activity", tools) + theme.Muted("    "+theme.T("activity.switchHint"))
	return theme.overlayLine(content, width, false)
}

func (a *ToolActivity) SetSession(s domain.SessionState) {
	a.labels = make(map[root.ToolName]string, len(s.ToolSnapshot))
	for _, tool := range s.ToolSnapshot {
		label, _ := toolPresentation(s, tool.Definition.Name)
		a.labels[tool.Definition.Name] = label
	}
}
