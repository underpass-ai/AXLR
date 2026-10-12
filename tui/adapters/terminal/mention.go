package terminal

import (
	"fmt"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// mentionState is the @ menu: the workspace's files, listed once per
// session and again after a turn (which may have written files) or on
// Ctrl+R, and the selection. On 12 October 2026 every prompt had to spell
// its paths.
type mentionState struct {
	files     []string
	session   domain.SessionID
	loaded    bool
	loading   bool
	stale     bool
	truncated bool
	err       string
	selected  int
	top       int
	// dismissed is the token Esc closed the menu on, so it stays closed
	// while the cursor is in it.
	dismissed string
}

// workspaceFilesMsg brings the files the walk found.
type workspaceFilesMsg struct {
	session   domain.SessionID
	files     []string
	truncated bool
	err       error
}

// maxMentionMatches bounds the matches ranked for the menu.
const maxMentionMatches = 200

// mentionToken is the "@word" the cursor ends, if any: its text after
// "@", its line and the rune column of the "@".
func (m AppModel) mentionToken() (query string, line, start int, ok bool) {
	lines := strings.Split(m.Composer.Input.Value(), "\n")
	line = m.Composer.Input.Line()
	if line < 0 || line >= len(lines) {
		return "", 0, 0, false
	}
	runes := []rune(lines[line])
	col := min(m.Composer.Input.Column(), len(runes))
	start = col
	for start > 0 && !unicode.IsSpace(runes[start-1]) {
		start--
	}
	if start >= col || runes[start] != '@' {
		return "", 0, 0, false
	}
	return string(runes[start+1 : col]), line, start, true
}

func (m AppModel) mentionKey(line, start int) string { return fmt.Sprintf("%d:%d", line, start) }

// mentionActive reports the @ menu open: a token under the cursor in the
// main composer that Esc did not close, and no slash menu.
func (m AppModel) mentionActive() bool {
	if m.overlay != "" || m.inlineApproval() || len(slashSuggestions(strings.TrimSpace(m.Composer.Input.Value()))) > 0 {
		return false
	}
	_, line, start, ok := m.mentionToken()
	return ok && m.mention.dismissed != m.mentionKey(line, start)
}

func (m AppModel) mentionMatches() []string {
	query, _, _, ok := m.mentionToken()
	if !ok {
		return nil
	}
	return rankFiles(m.mention.files, query, maxMentionMatches)
}

// loadFiles lists the workspace's files in the background when the menu
// needs them and has none for this session, or older than the last turn.
func (m *AppModel) loadFiles(force bool) tea.Cmd {
	fresh := m.mention.loaded && !m.mention.stale && m.mention.session == m.Header.State.ID
	if m.mention.loading || fresh && !force {
		return nil
	}
	m.mention.loading = true
	root, session := string(m.Header.State.Workspace), m.Header.State.ID
	if root == "" {
		root = string(m.deps.Workspace)
	}
	return func() tea.Msg {
		files, truncated, err := listWorkspaceFiles(root)
		return workspaceFilesMsg{session: session, files: files, truncated: truncated, err: err}
	}
}

func (m AppModel) filesLoaded(msg workspaceFilesMsg) AppModel {
	m.mention.loading = false
	if msg.session != m.Header.State.ID {
		// The session changed while the walk ran: list again on demand.
		return m
	}
	m.mention.files, m.mention.truncated, m.mention.loaded, m.mention.stale, m.mention.session = msg.files, msg.truncated, true, false, msg.session
	m.mention.err = ""
	if msg.err != nil {
		m.mention.err = msg.err.Error()
	}
	m.mention.selected, m.mention.top = 0, 0
	return m
}

// mentionKeyPress handles the menu's keys; handled is false for keys the
// composer takes.
func (m AppModel) mentionKeyPress(k tea.KeyPressMsg) (AppModel, tea.Cmd, bool) {
	matches := m.mentionMatches()
	limit := m.slashLimit()
	switch k.String() {
	case "up":
		m.mention.selected = max(0, min(m.mention.selected, len(matches)-1)-1)
		m.mention.top = slashWindow(m.mention.top, m.mention.selected, len(matches), limit)
		return m, nil, true
	case "down":
		m.mention.selected = max(0, min(len(matches)-1, m.mention.selected+1))
		m.mention.top = slashWindow(m.mention.top, m.mention.selected, len(matches), limit)
		return m, nil, true
	case "ctrl+r":
		return m, m.loadFiles(true), true
	case "tab", "enter":
		query, _, _, _ := m.mentionToken()
		if len(matches) == 0 || k.String() == "enter" && query == "" {
			// Enter with nothing to insert, or after a bare "@", sends the
			// prompt as typed; Tab never sends.
			return m, nil, k.String() == "tab"
		}
		m.insertMention(matches[min(m.mention.selected, len(matches)-1)])
		return m, nil, true
	}
	return m, nil, false
}

// insertMention replaces the "@word" under the cursor with path and a
// space; the prompt carries the path and the model reads it.
func (m *AppModel) insertMention(path string) {
	_, line, start, ok := m.mentionToken()
	if !ok {
		return
	}
	lines := strings.Split(m.Composer.Input.Value(), "\n")
	runes := []rune(lines[line])
	col := min(m.Composer.Input.Column(), len(runes))
	inserted := path + " "
	if col < len(runes) && unicode.IsSpace(runes[col]) {
		inserted = path
	}
	lines[line] = string(runes[:start]) + inserted + string(runes[col:])
	m.Composer.Input.SetValue(strings.Join(lines, "\n"))
	m.Composer.Input.MoveToBegin()
	for i := 0; m.Composer.Input.Line() < line && i < 1<<16; i++ {
		m.Composer.Input.CursorDown()
	}
	m.Composer.Input.SetCursorColumn(start + len([]rune(inserted)))
	m.mention.selected, m.mention.top = 0, 0
	m.resizeComposer()
}

// mentionMenu draws the @ menu above the composer as the slash menu is
// drawn: the window around the selection and the count of the rest.
func (m AppModel) mentionMenu() []string {
	width := max(1, m.Layout.Width)
	style := lipgloss.NewStyle().Width(width)
	selectedStyle := style
	if !m.Theme.Monochrome {
		p := m.Theme.palette()
		style = style.Background(lipgloss.Color(p.Raised)).Foreground(lipgloss.Color(p.Muted))
		selectedStyle = selectedStyle.Background(lipgloss.Color(p.Selected)).Foreground(lipgloss.Color(p.Text))
	}
	row := func(s string, st lipgloss.Style) string { return st.Render(ansi.Truncate(s, width, "…")) }
	matches := m.mentionMatches()
	switch {
	case len(matches) == 0 && (m.mention.loading || !m.mention.loaded):
		return []string{row("  "+m.Theme.T("mention.loading"), style)}
	case m.mention.err != "":
		return []string{row("  "+m.Theme.Tf("mention.failed", singleLine(m.mention.err)), style)}
	case len(matches) == 0:
		return []string{row("  "+m.Theme.T("mention.none"), style)}
	}
	limit := m.slashLimit()
	top := slashWindow(m.mention.top, m.mention.selected, len(matches), limit)
	shown := matches[top:min(len(matches), top+limit)]
	lines := make([]string, 0, len(shown)+1)
	for offset, path := range shown {
		i := top + offset
		marker, st := "  ", style
		if i == m.mention.selected {
			st = selectedStyle
			if m.Theme.Monochrome {
				marker = "> "
			}
		}
		lines = append(lines, m.zones.Mark(fmt.Sprintf("%smention-%d", m.prefix, i), row(marker+singleLine(path), st)))
	}
	more := ""
	above, below := top, len(matches)-top-len(shown)
	switch {
	case above > 0 && below > 0:
		more = m.Theme.Tf("slash.aboveBelow", above, below)
	case above > 0:
		more = m.Theme.Tf("slash.above", above)
	case below > 0:
		more = m.Theme.Tf("slash.more", below)
	}
	hint := m.Theme.T("mention.hint")
	if m.mention.truncated {
		hint = m.Theme.Tf("mention.truncated", maxWorkspaceFiles) + " · " + hint
	}
	if more != "" {
		hint = more + " · " + hint
	}
	return append(lines, row("  "+hint, style))
}
