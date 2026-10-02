package terminal

import (
	"fmt"
	"regexp"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type slashCommand struct{ name, summary string }

// slashCommands is every command the composer runs locally instead of
// sending to the model, in the order the suggestions list them.
var slashCommands = []slashCommand{
	{"/model", "slash.model"},
	{"/theme", "slash.theme"},
	{"/mcp", "slash.mcp"},
	{"/plugin", "slash.plugin"},
	{"/changes", "slash.changes"},
	{"/approvals", "slash.approvals"},
	{"/autonomy on", "slash.autonomyOn"},
	{"/autonomy off", "slash.autonomyOff"},
	{"/update", "slash.update"},
	{"/normal", "slash.normal"},
	{"/review", "slash.review"},
	{"/writer", "slash.writer"},
	{"/research", "slash.research"},
	{"/exit", "slash.exit"},
}

// slashAliases run the same command under another name.
var slashAliases = map[string]string{"/quit": "/exit", "/diff": "/changes", "/plugins": "/plugin", "/revisar": "/review", "/escritor": "/writer", "/investigar": "/research"}

const slashSuggestionLimit = 6

var slashWord = regexp.MustCompile(`^/[a-z-]+$`)

// slashSuggestions lists the commands a single-line draft starting with "/"
// could complete to. A draft that is already an exact command, or that
// carries text after a space other than a command's own argument, gets none.
func slashSuggestions(draft string) []slashCommand {
	if !strings.HasPrefix(draft, "/") || strings.Contains(draft, "\n") {
		return nil
	}
	var out []slashCommand
	for _, c := range slashCommands {
		if strings.HasPrefix(c.name, draft) && c.name != draft {
			out = append(out, c)
		}
	}
	return out[:min(len(out), slashSuggestionLimit)]
}

// isSlashCommand reports whether a draft names a local command.
func isSlashCommand(draft string) (string, bool) {
	if alias, ok := slashAliases[draft]; ok {
		draft = alias
	}
	if draft == "/autonomy" {
		return draft, true
	}
	for _, c := range slashCommands {
		if c.name == draft {
			return draft, true
		}
	}
	return draft, false
}

// unknownSlashWord is a lone "/word" that names no command: almost always a
// mistyped command, never worth a model turn.
func unknownSlashWord(draft string) bool {
	_, known := isSlashCommand(draft)
	return !known && slashWord.MatchString(draft)
}

// slashMenu draws the suggestions to sit on the transcript's last rows,
// directly above the composer.
func (m AppModel) slashMenu(suggestions []slashCommand) []string {
	width := max(1, m.Layout.Width)
	nameWidth := 0
	for _, c := range suggestions {
		nameWidth = max(nameWidth, len(c.name))
	}
	lines := make([]string, 0, len(suggestions))
	for i, c := range suggestions {
		row := "  " + c.name + strings.Repeat(" ", nameWidth-len(c.name)+3) + m.Theme.T(c.summary)
		row = ansi.Truncate(row, width, "…")
		style := lipgloss.NewStyle().Width(width)
		if !m.Theme.Monochrome {
			p := m.Theme.palette()
			style = style.Background(lipgloss.Color(p.Raised)).Foreground(lipgloss.Color(p.Muted))
			if i == m.slashSelected {
				style = style.Background(lipgloss.Color(p.Selected)).Foreground(lipgloss.Color(p.Text))
			}
		} else if i == m.slashSelected {
			row = ">" + row[1:]
		}
		lines = append(lines, m.zones.Mark(fmt.Sprintf("%sslash-%d", m.prefix, i), style.Render(row)))
	}
	return lines
}
