package terminal

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type slashCommand struct{ name, summary string }

// slashCommands is every command the composer runs locally instead of
// sending to the model, in the order the suggestions list them.
var slashCommands = []slashCommand{
	{"/new", "slash.new"},
	{"/model", "slash.model"},
	{"/theme", "slash.theme"},
	{"/mcp", "slash.mcp"},
	{"/plugin", "slash.plugin"},
	{"/changes", "slash.changes"},
	{"/undo", "slash.undo"},
	{"/approvals", "slash.approvals"},
	{"/autonomy on", "slash.autonomyOn"},
	{"/autonomy off", "slash.autonomyOff"},
	{"/update", "slash.update"},
	{"/normal", "slash.normal"},
	{"/review", "slash.review"},
	{"/writer", "slash.writer"},
	{"/research", "slash.research"},
	{"/debug", "slash.debug"},
	{"/delivery", "slash.delivery"},
	{"/incident", "slash.incident"},
	{"/repair", "slash.repair"},
	{"/improve", "slash.improve"},
	{"/plan", "slash.plan"},
	{"/jobs", "slash.jobs"},
	{"/ceremony", "slash.ceremony"},
	{"/stop-ceremony", "slash.stopCeremony"},
	{"/copy", "slash.copy"},
	{"/clean-screen", "slash.cleanScreen"},
	{"/export", "slash.export"},
	{"/usage", "slash.usage"},
	{"/session", "slash.session"},
	{"/session export", "slash.sessionExport"},
	{"/memory", "slash.memory"},
	{"/memory save", "slash.memorySave"},
	{"/memory compare", "slash.memoryCompare"},
	{"/memory forget", "slash.memoryForget"},
	{"/exit", "slash.exit"},
}

// slashVerbs are the commands whose menu rows after a space ("/memory
// save") appear only once the command and a space are typed.
var slashVerbs = map[string]bool{"/memory": true, "/session": true}

// slashAliases run the same command under another name.
var slashAliases = map[string]string{"/quit": "/exit", "/diff": "/changes", "/plugins": "/plugin", "/revisar": "/review", "/escritor": "/writer", "/investigar": "/research", "/depurar": "/debug", "/entrega": "/delivery", "/incidente": "/incident", "/reparar": "/repair", "/mejorar": "/improve", "/planificar": "/plan", "/parar": "/stop-ceremony", "/copiar": "/copy", "/uso": "/usage", "/sesion": "/session", "/sesión": "/session", "/memoria": "/memory", "/trabajos": "/jobs", "/nueva": "/new", "/clear": "/new", "/exportar": "/export", "/deshacer": "/undo", "/clean_screen": "/clean-screen", "/cls": "/clean-screen", "/limpiar": "/clean-screen"}

// slashSuggestionMax is how many suggestions the menu shows at once on a
// screen tall enough; the selection scrolls the rest into view. On 12
// October 2026 six rows showed a quarter of the commands.
const slashSuggestionMax = 12

// slashLimit is how many suggestions fit above the composer: up to
// slashSuggestionMax, fewer on a short screen, keeping a row for the count
// of the rest.
func (m AppModel) slashLimit() int {
	return max(1, min(slashSuggestionMax, m.Layout.BodyHeight-2))
}

// slashWord is a draft that reads as a command: "/memoría", a typo with an
// accent, is refused like "/memroy" rather than sent to the model, and
// "/clean_scren" like "/clean-scren".
var slashWord = regexp.MustCompile(`^/[\p{L}_-]+$`)

// slashAliasesOf lists each command's other names, sorted, to show dim
// after it in the menu.
var slashAliasesOf = func() map[string][]string {
	out := map[string][]string{}
	for alias, name := range slashAliases {
		out[name] = append(out[name], alias)
	}
	for _, aliases := range out {
		sort.Strings(aliases)
	}
	return out
}()

// slashDraft is the composer's text as the menu reads it: a space after a
// command ("/memory ") asks for its verbs, so only leading space goes.
func slashDraft(value string) string {
	return strings.TrimLeft(value, " \t")
}

// slashSuggestions lists the commands a single-line draft starting with "/"
// could complete to, whatever its case: those whose name starts with it,
// then those with an alias that does, then those whose name or an alias
// contains it ("/trab" finds /jobs through /trabajos, "/ceremony"
// /stop-ceremony). A draft that is already an exact command, or that
// carries text after a space other than a command's own argument, gets
// none: Enter runs a complete command as typed, never a suggestion it is a
// prefix of (bare /autonomy is not /autonomy on).
func slashSuggestions(draft string) []slashCommand {
	if !strings.HasPrefix(draft, "/") || strings.Contains(draft, "\n") {
		return nil
	}
	if _, args, known := parseSlash(draft); known && args == "" && !strings.HasSuffix(draft, " ") {
		return nil
	}
	lower := strings.ToLower(draft)
	if word, rest, spaced := strings.Cut(lower, " "); spaced && slashAliases[word] != "" {
		// "/memoria " offers the verbs of /memory.
		lower = slashAliases[word] + " " + rest
	}
	var out []slashCommand
	seen := map[string]bool{}
	add := func(c slashCommand) {
		if word, _, sub := strings.Cut(c.name, " "); sub && slashVerbs[word] && !strings.HasPrefix(lower, word+" ") {
			// A command's verbs are offered once the command is typed.
			return
		}
		if !seen[c.name] && c.name != lower {
			seen[c.name] = true
			out = append(out, c)
		}
	}
	base := func(name string) string {
		word, _, _ := strings.Cut(name, " ")
		return word
	}
	for _, c := range slashCommands {
		if strings.HasPrefix(c.name, lower) {
			add(c)
		}
	}
	for _, c := range slashCommands {
		for _, alias := range slashAliasesOf[base(c.name)] {
			if strings.HasPrefix(alias, lower) {
				add(c)
			}
		}
	}
	if needle := strings.TrimPrefix(lower, "/"); needle != "" {
		for _, c := range slashCommands {
			if strings.Contains(c.name[1:], needle) {
				add(c)
			}
		}
		for _, c := range slashCommands {
			for _, alias := range slashAliasesOf[base(c.name)] {
				if strings.Contains(alias[1:], needle) {
					add(c)
				}
			}
		}
	}
	return out
}

// slashWindow is the first suggestion the menu shows: the previous one
// while the selection stays inside it, otherwise just enough to reach it.
// On 10 October 2026 the menu listed six of 24 commands and ↓ stopped at
// the sixth; the rest were found only by knowing their prefix.
func slashWindow(top, selected, count, limit int) int {
	top = min(top, selected)
	top = max(top, selected-limit+1)
	return max(0, min(top, count-limit))
}

// parseSlash reads a draft as a local command: its first word, lowercased
// and with aliases resolved, and the text after it. /autonomy takes on or
// off as part of its name. A draft that names no command comes back as
// typed with known false. On 12 October 2026 "/Usage", "/autonomy ON" and
// "/autonomy  on" matched nothing and were sent to the model as paid
// prompts.
func parseSlash(draft string) (command, args string, known bool) {
	draft = strings.TrimSpace(draft)
	if !strings.HasPrefix(draft, "/") {
		return draft, "", false
	}
	word, rest := draft, ""
	if i := strings.IndexFunc(draft, unicode.IsSpace); i >= 0 {
		word, rest = draft[:i], strings.TrimSpace(draft[i:])
	}
	word = strings.ToLower(word)
	if alias, ok := slashAliases[word]; ok {
		word = alias
	}
	if word == "/autonomy" {
		switch strings.ToLower(strings.Join(strings.Fields(rest), " ")) {
		case "on":
			return "/autonomy on", "", true
		case "off":
			return "/autonomy off", "", true
		}
		return word, rest, true
	}
	for _, c := range slashCommands {
		if c.name == word {
			return word, rest, true
		}
	}
	return draft, "", false
}

// isSlashCommand reports whether a draft is exactly a local command, and
// its normalised name.
func isSlashCommand(draft string) (string, bool) {
	command, args, known := parseSlash(draft)
	return command, known && args == ""
}

// unknownSlashWord is the "/word" a draft starts with when it names no
// command: almost always a mistyped command, never worth a model turn. A
// lone word is refused; with text after it, it is refused unless the word
// names a path, absolute or in the workspace: on 14 October 2026 "/foo bar"
// cost four requests to hear the model say it did not know "/foo bar".
// "/tmp/foo explain" and "/etc what is this" are prompts.
func unknownSlashWord(draft, workspace string) (string, bool) {
	draft = strings.TrimSpace(draft)
	if _, _, known := parseSlash(draft); known {
		return "", false
	}
	word, rest := draft, ""
	if i := strings.IndexFunc(draft, unicode.IsSpace); i >= 0 {
		word, rest = draft[:i], strings.TrimSpace(draft[i:])
	}
	if !slashWord.MatchString(word) {
		return "", false
	}
	if rest != "" && (pathExists(word) || workspace != "" && pathExists(filepath.Join(workspace, word[1:]))) {
		return "", false
	}
	return word, true
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// slashPromptModes are the work modes that take a prompt after them:
// "/review the auth module" selects review and sends "the auth module".
var slashPromptModes = map[string]bool{"/normal": true, "/review": true, "/writer": true, "/research": true, "/debug": true, "/delivery": true, "/incident": true}

// slashUsages are how the commands that take text are written; any other
// command takes none.
var slashUsages = map[string]string{"/model": "/model [filter]", "/autonomy": "/autonomy [on|off]", "/export": "/export [path]", "/copy": "/copy [code|N]", "/ceremony": "/ceremony [<definition> [version] [brief]] · /ceremony publish <file.yaml>", "/memory": "/memory save · /memory compare <about>… · /memory forget <ref> [why]", "/session": "/session [export [id]]", "/plan": "/plan [goal]"}

func slashUsage(command string) string {
	if usage, ok := slashUsages[command]; ok {
		return usage
	}
	if slashPromptModes[command] {
		return command + " [prompt]"
	}
	return command
}

// slashMenu draws the suggestions to sit on the transcript's last rows,
// directly above the composer: the window around the selection, then how
// many are above and below it when they do not all fit.
func (m AppModel) slashMenu(suggestions []slashCommand) []string {
	width := max(1, m.Layout.Width)
	limit := m.slashLimit()
	top := slashWindow(m.slashTop, m.slashSelected, len(suggestions), limit)
	shown := suggestions[top:min(len(suggestions), top+limit)]
	// The aliases follow the name, dim, so the column of descriptions
	// lines up after the longest of the rows shown.
	aliases := func(c slashCommand) string {
		word, _, _ := strings.Cut(c.name, " ")
		return strings.Join(slashAliasesOf[word], " ")
	}
	nameWidth, aliasWidth := 0, 0
	for _, c := range shown {
		nameWidth = max(nameWidth, len(c.name))
		aliasWidth = max(aliasWidth, ansi.StringWidth(aliases(c)))
	}
	lines := make([]string, 0, len(shown)+1)
	for offset, c := range shown {
		i := top + offset
		alias := aliases(c)
		style := lipgloss.NewStyle().Width(width)
		aliasStyle := lipgloss.NewStyle()
		if !m.Theme.Monochrome {
			p := m.Theme.palette()
			background, foreground := p.Raised, p.Muted
			if i == m.slashSelected {
				background, foreground = p.Selected, p.Text
			}
			style = style.Background(lipgloss.Color(background)).Foreground(lipgloss.Color(foreground))
			aliasStyle = aliasStyle.Background(lipgloss.Color(background)).Foreground(lipgloss.Color(p.Muted)).Faint(true)
		}
		marker := "  "
		if m.Theme.Monochrome && i == m.slashSelected {
			marker = "> "
		}
		row := marker + c.name + strings.Repeat(" ", nameWidth-len(c.name)+2)
		aliasStart := len(row)
		if aliasWidth > 0 {
			row += alias + strings.Repeat(" ", aliasWidth-ansi.StringWidth(alias)+2)
		} else {
			row += " "
		}
		rendered := style.Render(ansi.Truncate(row+m.Theme.T(c.summary), width, "…"))
		if alias != "" && !m.Theme.Monochrome && aliasStart < width {
			rendered = lipgloss.StyleRanges(rendered, lipgloss.NewRange(aliasStart, min(width, aliasStart+ansi.StringWidth(alias)), aliasStyle))
		}
		lines = append(lines, m.zones.Mark(fmt.Sprintf("%sslash-%d", m.prefix, i), rendered))
	}
	// On 10 October 2026 the last row read "↑↓ 18 more" whichever part of
	// the list was in view: it now counts what is above and below.
	above, below := top, len(suggestions)-top-len(shown)
	more := ""
	switch {
	case above > 0 && below > 0:
		more = m.Theme.Tf("slash.aboveBelow", above, below)
	case above > 0:
		more = m.Theme.Tf("slash.above", above)
	case below > 0:
		more = m.Theme.Tf("slash.more", below)
	}
	if more != "" {
		style := lipgloss.NewStyle().Width(width)
		if !m.Theme.Monochrome {
			p := m.Theme.palette()
			style = style.Background(lipgloss.Color(p.Raised)).Foreground(lipgloss.Color(p.Muted))
		}
		lines = append(lines, style.Render(ansi.Truncate("  "+more, width, "…")))
	}
	return lines
}
