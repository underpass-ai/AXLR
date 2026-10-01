package terminal

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type Header struct{ State domain.SessionState }

const (
	headerPluginLimit = 3
	headerPluginWidth = 14
)

// View is one row: brand, model and workspace on the left; the plugins whose
// tools this session can call on the right.
func (h Header) View(width int, theme Theme) string {
	model := singleLine(string(h.State.Model))
	if h.State.ID == "" {
		model = theme.T("header.chooseModel")
	}
	left := " " + theme.Accent(theme.Icon("brand")+" AXLR") + "  " + model + theme.Muted("  "+shortenHome(singleLine(string(h.State.Workspace))))
	right := ""
	plugins := sessionPlugins(h.State)
	for i, id := range plugins {
		if i == headerPluginLimit {
			right += theme.Muted(fmt.Sprintf("+%d  ", len(plugins)-i))
			break
		}
		mark := theme.Icon("connected")
		if !theme.Monochrome {
			mark = lipgloss.NewStyle().Foreground(lipgloss.Color(theme.palette().Good)).Render(mark)
		}
		right += mark + theme.Muted(" "+ansi.Truncate(id, headerPluginWidth, "…")+"  ")
	}
	gap := width - ansi.StringWidth(left) - ansi.StringWidth(right)
	if gap < 2 {
		return lipgloss.NewStyle().Width(max(1, width)).Render(ansi.Truncate(left, max(1, width), "…"))
	}
	return lipgloss.NewStyle().Width(max(1, width)).Render(left + strings.Repeat(" ", gap) + right)
}

// sessionPlugins lists the plugins that contributed tools to the session's
// snapshot, which is what the model can actually call.
func sessionPlugins(s domain.SessionState) []string {
	seen := map[string]bool{}
	for _, tool := range s.ToolSnapshot {
		if tool.Identity.Kind == domain.ToolKindPlugin {
			seen[singleLine(tool.Identity.Plugin.PluginID.String())] = true
		}
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func shortenHome(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	if rel, err := filepath.Rel(home, path); err == nil && !strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel) {
		return "~" + string(filepath.Separator) + rel
	}
	return path
}
