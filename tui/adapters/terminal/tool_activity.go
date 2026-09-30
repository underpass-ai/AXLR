package terminal

import (
	"fmt"
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
	labels map[root.ToolName]string
}

func (a *ToolActivity) Apply(e application.Event) {
	if e.Usage != nil {
		a.Tokens = e.Usage.TotalTokens
	}
	if e.Kind == application.EventToolActivity {
		label := string(e.Tool.Call.Name)
		if display, ok := a.labels[e.Tool.Call.Name]; ok {
			label = display
		}
		line := label + ": " + string(e.Tool.Decision)
		if e.Tool.Outcome != nil {
			line += " " + toolSummary(string(e.Tool.Outcome.Content))
		}
		a.Lines = append(a.Lines, Sanitize(line))
	}
}
func (a ToolActivity) View(w, h int) string {
	s := fmt.Sprintf("Activity | tokens: %d", a.Tokens)
	for _, line := range a.Lines[max(0, len(a.Lines)-max(1, h-1)):] {
		s += "\n" + line
	}
	lines := strings.Split(ansi.Wrap(s, w, ""), "\n")
	if len(lines) > h {
		lines = lines[len(lines)-h:]
	}
	return lipgloss.NewStyle().Width(w).Height(h).Render(strings.Join(lines, "\n"))
}

func (a ToolActivity) Tabs(z *zone.Manager, prefix string) string {
	return z.Mark(prefix+"transcript", "[Transcript]") + " " + z.Mark(prefix+"activity", "[Activity]") + " Tab"
}

func (a *ToolActivity) SetSession(s domain.SessionState) {
	a.labels = make(map[root.ToolName]string, len(s.ToolSnapshot))
	for _, tool := range s.ToolSnapshot {
		label, _ := toolPresentation(s, tool.Definition.Name)
		a.labels[tool.Definition.Name] = label
	}
}
