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
	// About is the session's memory about and Memories how many events
	// the store holds under the launch's about, when MemoriesKnown
	// (decision J2); the live MADE instance comes from State.
	About         string
	Memories      int
	MemoriesKnown bool
}

// memory is the header's memory segment: the about, the count and the
// MADE instance, muted, after the branch; "" when nothing is known.
func (h Header) memory(theme Theme) string {
	var parts []string
	if h.About != "" {
		parts = append(parts, singleLine(h.About))
	}
	if h.MemoriesKnown {
		if h.Memories == 0 {
			parts = append(parts, theme.T("header.newProject"))
		} else {
			parts = append(parts, theme.Tn("header.memories", h.Memories, h.Memories))
		}
	}
	if instance := h.State.Instance; instance != nil && instance.Instance != "" {
		if instance.Closed {
			parts = append(parts, theme.T("header.instanceClosed"))
		} else {
			parts = append(parts, theme.T("header.instanceOpen"))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "  " + strings.Join(parts, " · ")
}

const (
	headerPluginLimit = 3
	headerPluginWidth = 14
)

// View is one row: brand, "model:" and "workspace:" on the left; the
// plugins whose tools this session can call on the right. The workspace is written from
// the home directory (~/…) with its Git branch (⎇ main, * when tracked
// files changed). On 12 October 2026 a header at 80 columns read
// "/tmp/c…" and plugin names were cut ("● microsoft-doc…") with room to
// spare: when the row is short the path keeps its last segments
// ("…/scratchpad/ws"), then the labels go, and plugin names are cut only
// after that.
func (h Header) View(width int, theme Theme) string {
	model := singleLine(string(h.State.Model))
	labelled := theme.Muted(theme.T("header.model")+" ") + model
	if h.State.ID == "" {
		model = theme.T("header.chooseModel")
		labelled = model
	}
	marker := ""
	if h.SandboxOff {
		// Before the path, which a narrow row cuts first.
		marker = theme.Icon("attention") + " " + theme.T("header.sandboxOff")
		if !theme.Monochrome {
			marker = lipgloss.NewStyle().Foreground(lipgloss.Color(theme.palette().Warning)).Render(marker)
		}
		marker = "  " + marker
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
	// From the most to the least detail: "model:" and "workspace:" with the
	// whole path, then its last two segments, then its last one; the same
	// without the labels; then cut plugin names.
	type layout struct {
		labels bool
		path   string
		limit  int
		memory bool
	}
	var layouts []layout
	memory := h.memory(theme)
	for _, withMemory := range []bool{true, false} {
		if withMemory && memory == "" {
			continue
		}
		for _, labels := range []bool{true, false} {
			for _, p := range []string{path, pathTail(path, 2), pathTail(path, 1)} {
				layouts = append(layouts, layout{labels, p, 0, withMemory})
			}
		}
	}
	layouts = append(layouts, layout{false, pathTail(path, 1), headerPluginWidth, false})
	var left, right string
	for _, l := range layouts {
		brand, workspace := " "+theme.Brand()+"  "+model+marker, ""
		if l.labels {
			brand, workspace = " "+theme.Brand()+"  "+labelled+marker, theme.T("header.workspace")+" "
		}
		segment := ""
		if l.memory {
			segment = memory
		}
		left = brand + theme.Muted("  "+workspace+l.path+branch+segment)
		right = headerPlugins(plugins, l.limit, theme)
		if ansi.StringWidth(left)+ansi.StringWidth(right)+2 <= width {
			break
		}
	}
	gap := width - ansi.StringWidth(left) - ansi.StringWidth(right)
	if plain := " " + ansi.Strip(theme.Brand()) + "  " + model; gap < 2 && width-ansi.StringWidth(right)-2 >= ansi.StringWidth(plain) {
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

// workspacePath names a file the console wrote by its path inside the
// session's workspace (".axlr/exports/x.md"), else as shortenHome does: the
// footer has room for one line.
func (m AppModel) workspacePath(path string) string {
	if workspace := string(m.Header.State.Workspace); workspace != "" {
		if rel, err := filepath.Rel(workspace, path); err == nil && rel != "." && !strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel) {
			return rel
		}
	}
	return shortenHome(path)
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
