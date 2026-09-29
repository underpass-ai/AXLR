package terminal

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone/v2"
	"github.com/underpass-ai/AXLR/tui/application"
)

type ToolActivity struct {
	Lines  []string
	Tokens int
}

func (a *ToolActivity) Apply(e application.Event) {
	if e.Usage != nil {
		a.Tokens = e.Usage.TotalTokens
	}
	if e.Kind == application.EventToolActivity {
		line := string(e.Tool.Call.Name) + ": " + string(e.Tool.Decision)
		if e.Tool.Outcome != nil {
			line += " " + string(e.Tool.Outcome.Content)
		}
		a.Lines = append(a.Lines, Sanitize(line))
	}
}
func (a ToolActivity) View(w, h int) string {
	s := fmt.Sprintf("Activity | tokens: %d", a.Tokens)
	for _, line := range a.Lines {
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
