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

type Header struct {
	State domain.SessionState
	// SandboxOff marks a configured exec sandbox that does not confine
	// local commands.
	SandboxOff bool
	// Git is the workspace's branch, when it is a Git repository.
	Git *GitState
}

const (
	headerPluginLimit = 3
	headerPluginWidth = 14
)

// View is one row: brand, model and workspace on the left; the plugins whose
// tools this session can call on the right. The workspace is written from
// the home directory (~/…) with its Git branch (⎇ main, * when tracked
// files changed). On 12 October 2026 a header at 80 columns read
// "/tmp/c…" and plugin names were cut ("● microsoft-doc…") with room to
// spare: when the row is short the path keeps its last segments
// ("…/scratchpad/ws") and plugin names are cut only after that.
func (h Header) View(width int, theme Theme) string {
	model := singleLine(string(h.State.Model))
	if h.State.ID == "" {
		model = theme.T("header.chooseModel")
	}
	brand := " " + theme.Accent(theme.Icon("brand")+" AXLR") + "  " + model
	if h.SandboxOff {
		// Before the path, which a narrow row cuts first.
		marker := theme.Icon("attention") + " " + theme.T("header.sandboxOff")
		if !theme.Monochrome {
			marker = lipgloss.NewStyle().Foreground(lipgloss.Color(theme.palette().Warning)).Render(marker)
		}
		brand += "  " + marker
	}
	path := shortenHome(singleLine(string(h.State.Workspace)))
	branch := ""
	if h.Git != nil && h.Git.Branch != "" {
		branch = "  " + theme.Icon("branch") + " " + singleLine(h.Git.Branch)
		if h.Git.Dirty {
			branch += "*"
		}
	}
	plugins := sessionPlugins(h.State)
	// From the most to the least detail: the whole path, then its last two
	// segments, then its last one, then cut plugin names.
	type layout struct {
		path  string
		limit int
	}
	layouts := []layout{{path, 0}, {pathTail(path, 2), 0}, {pathTail(path, 1), 0}, {pathTail(path, 1), headerPluginWidth}}
	var left, right string
	for _, l := range layouts {
		left = brand + theme.Muted("  "+l.path+branch)
		right = headerPlugins(plugins, l.limit, theme)
		if ansi.StringWidth(left)+ansi.StringWidth(right)+2 <= width {
			break
		}
	}
	gap := width - ansi.StringWidth(left) - ansi.StringWidth(right)
	if plain := " " + theme.Icon("brand") + " AXLR  " + model; gap < 2 && width-ansi.StringWidth(right)-2 >= ansi.StringWidth(plain) {
		// On 10 October 2026, at 80 columns, a long workspace path pushed
		// "● kmp ● made" off the row: the path is cut instead.
		left = ansi.Truncate(left, width-ansi.StringWidth(right)-2, "…")
		gap = width - ansi.StringWidth(left) - ansi.StringWidth(right)
	}
	if gap < 2 {
		return lipgloss.NewStyle().Width(max(1, width)).Render(ansi.Truncate(left, max(1, width), "…"))
	}
	return lipgloss.NewStyle().Width(max(1, width)).Render(left + strings.Repeat(" ", gap) + right)
}

// headerPlugins is the right side: up to headerPluginLimit plugins and how
// many more, each name cut to limit cells when limit is not zero.
func headerPlugins(plugins []string, limit int, theme Theme) string {
	right := ""
	for i, id := range plugins {
		if i == headerPluginLimit {
			right += theme.Muted(fmt.Sprintf("+%d  ", len(plugins)-i))
			break
		}
		mark := theme.Icon("connected")
		if !theme.Monochrome {
			mark = lipgloss.NewStyle().Foreground(lipgloss.Color(theme.palette().Good)).Render(mark)
		}
		name := id
		if limit > 0 {
			name = ansi.Truncate(id, limit, "…")
		}
		right += mark + theme.Muted(" "+name+"  ")
	}
	return right
}

// pathTail keeps a path's last segments after "…/": "…/scratchpad/ws". A
// path with no more segments than that is kept whole.
func pathTail(path string, segments int) string {
	parts := strings.FieldsFunc(path, func(r rune) bool { return r == '/' || r == '\\' })
	if len(parts) <= segments || strings.HasPrefix(path, "~") && len(parts) <= segments+1 {
		return path
	}
	return "…" + string(filepath.Separator) + strings.Join(parts[len(parts)-segments:], string(filepath.Separator))
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
